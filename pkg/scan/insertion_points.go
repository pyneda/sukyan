package scan

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/url"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/scan/reflection"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type InsertionPointType string

// HTTP insertion point types
const (
	InsertionPointTypeParameter InsertionPointType = "parameter"
	InsertionPointTypeHeader    InsertionPointType = "header"
	InsertionPointTypeBody      InsertionPointType = "body"
	InsertionPointTypeCookie    InsertionPointType = "cookie"
	InsertionPointTypeURLPath   InsertionPointType = "urlpath"
	InsertionPointTypeFullBody  InsertionPointType = "fullbody"
)

// HTTP XML insertion point types (per-element addressing inside an XML body)
const (
	InsertionPointTypeXMLElement   InsertionPointType = "xml_element"
	InsertionPointTypeXMLAttribute InsertionPointType = "xml_attribute"
)

// GraphQL insertion point types
const (
	InsertionPointTypeGraphQLVariable  InsertionPointType = "graphql_variable"
	InsertionPointTypeGraphQLInlineArg InsertionPointType = "graphql_inline_arg"
)

// WebSocket general insertion point types
const (
	InsertionPointTypeWSRawMessage InsertionPointType = "ws_raw_message"
)

// WebSocket GraphQL insertion point types (arguments/variables inside a graphql-ws payload.query).
const (
	InsertionPointTypeWSGraphQLInlineArg InsertionPointType = "ws_graphql_inline_arg"
	InsertionPointTypeWSGraphQLVariable  InsertionPointType = "ws_graphql_variable"
)

// WebSocket JSON insertion point types
const (
	InsertionPointTypeWSJSONField      InsertionPointType = "ws_json_field"      // Any JSON field
	InsertionPointTypeWSJSONValue      InsertionPointType = "ws_json_value"      // Value in a key-value pair
	InsertionPointTypeWSJSONKey        InsertionPointType = "ws_json_key"        // Key in a key-value pair
	InsertionPointTypeWSJSONArrayItem  InsertionPointType = "ws_json_array_item" // Array item
	InsertionPointTypeWSJSONArrayIndex InsertionPointType = "ws_json_array_idx"  // Array index (position)
	InsertionPointTypeWSJSONObject     InsertionPointType = "ws_json_object"     // Entire object
	InsertionPointTypeWSJSONArray      InsertionPointType = "ws_json_array"      // Entire array
)

// WebSocket XML insertion point types. Tag names, namespaces and processing
// instructions were dropped: they addressed no message parameter and their global
// regex rewrites corrupted unrelated parts of the document.
const (
	InsertionPointTypeWSXMLElement   InsertionPointType = "ws_xml_element"   // Element value/content
	InsertionPointTypeWSXMLAttribute InsertionPointType = "ws_xml_attribute" // XML attribute value
)

// String provides a string representation of the insertion point type
func (ipt InsertionPointType) String() string {
	return string(ipt)
}

// HumanReadableName returns a user-friendly name for the insertion point type
func (ipt InsertionPointType) HumanReadableName() string {
	switch ipt {
	case InsertionPointTypeParameter:
		return "URL Parameter"
	case InsertionPointTypeHeader:
		return "HTTP Header"
	case InsertionPointTypeBody:
		return "Request Body Field"
	case InsertionPointTypeCookie:
		return "Cookie"
	case InsertionPointTypeURLPath:
		return "URL Path Component"
	case InsertionPointTypeFullBody:
		return "Full Request Body"
	case InsertionPointTypeXMLElement:
		return "XML Element Value"
	case InsertionPointTypeXMLAttribute:
		return "XML Attribute Value"

	// GraphQL types
	case InsertionPointTypeGraphQLVariable:
		return "GraphQL Variable"
	case InsertionPointTypeGraphQLInlineArg:
		return "GraphQL Inline Argument"

	// WebSocket JSON types
	case InsertionPointTypeWSJSONField:
		return "WebSocket JSON Field"
	case InsertionPointTypeWSJSONValue:
		return "WebSocket JSON Value"
	case InsertionPointTypeWSJSONKey:
		return "WebSocket JSON Key"
	case InsertionPointTypeWSJSONArrayItem:
		return "WebSocket JSON Array Item"
	case InsertionPointTypeWSJSONArrayIndex:
		return "WebSocket JSON Array Index"
	case InsertionPointTypeWSJSONObject:
		return "WebSocket JSON Object"
	case InsertionPointTypeWSJSONArray:
		return "WebSocket JSON Array"

	// WebSocket XML types
	case InsertionPointTypeWSXMLElement:
		return "WebSocket XML Element Value"
	case InsertionPointTypeWSXMLAttribute:
		return "WebSocket XML Attribute"
	case InsertionPointTypeWSRawMessage:
		return "WebSocket Raw Message"

	// WebSocket GraphQL types
	case InsertionPointTypeWSGraphQLInlineArg:
		return "WebSocket GraphQL Inline Argument"
	case InsertionPointTypeWSGraphQLVariable:
		return "WebSocket GraphQL Variable"
	default:
		return fmt.Sprintf("Unknown (%s)", string(ipt))
	}
}

// AllInsertionPointTypes returns all supported insertion point types
func AllInsertionPointTypes() []InsertionPointType {
	return []InsertionPointType{
		// HTTP types
		InsertionPointTypeParameter,
		InsertionPointTypeHeader,
		InsertionPointTypeBody,
		InsertionPointTypeCookie,
		InsertionPointTypeURLPath,
		InsertionPointTypeFullBody,
		InsertionPointTypeXMLElement,
		InsertionPointTypeXMLAttribute,

		// GraphQL types
		InsertionPointTypeGraphQLVariable,
		InsertionPointTypeGraphQLInlineArg,

		// WebSocket JSON types
		InsertionPointTypeWSJSONField,
		InsertionPointTypeWSJSONValue,
		InsertionPointTypeWSJSONKey,
		InsertionPointTypeWSJSONArrayItem,
		InsertionPointTypeWSJSONArrayIndex,
		InsertionPointTypeWSJSONObject,
		InsertionPointTypeWSJSONArray,

		// WebSocket XML types
		InsertionPointTypeWSXMLElement,
		InsertionPointTypeWSXMLAttribute,

		// WebSocket general types
		InsertionPointTypeWSRawMessage,

		// WebSocket GraphQL types
		InsertionPointTypeWSGraphQLInlineArg,
		InsertionPointTypeWSGraphQLVariable,
	}
}

// HTTPInsertionPointTypes returns all HTTP-specific insertion point types
func HTTPInsertionPointTypes() []InsertionPointType {
	return []InsertionPointType{
		InsertionPointTypeParameter,
		InsertionPointTypeHeader,
		InsertionPointTypeBody,
		InsertionPointTypeCookie,
		InsertionPointTypeURLPath,
		InsertionPointTypeFullBody,
		InsertionPointTypeXMLElement,
		InsertionPointTypeXMLAttribute,
		InsertionPointTypeGraphQLVariable,
		InsertionPointTypeGraphQLInlineArg,
	}
}

// WebSocketInsertionPointTypes returns all WebSocket-specific insertion point types
func WebSocketInsertionPointTypes() []InsertionPointType {
	return []InsertionPointType{
		InsertionPointTypeWSJSONField,
		InsertionPointTypeWSJSONValue,
		InsertionPointTypeWSJSONKey,
		InsertionPointTypeWSJSONArrayItem,
		InsertionPointTypeWSJSONArrayIndex,
		InsertionPointTypeWSJSONObject,
		InsertionPointTypeWSJSONArray,

		InsertionPointTypeWSXMLElement,
		InsertionPointTypeWSXMLAttribute,

		InsertionPointTypeWSRawMessage,

		InsertionPointTypeWSGraphQLInlineArg,
		InsertionPointTypeWSGraphQLVariable,
	}
}

// WebSocketXMLInsertionPointTypes returns XML-specific WebSocket insertion point types
func WebSocketXMLInsertionPointTypes() []InsertionPointType {
	return []InsertionPointType{
		InsertionPointTypeWSXMLElement,
		InsertionPointTypeWSXMLAttribute,
	}
}

// IsWebSocketType returns true if the insertion point type is WebSocket-specific
func (ipt InsertionPointType) IsWebSocketType() bool {
	switch ipt {
	case InsertionPointTypeWSJSONField,
		InsertionPointTypeWSJSONValue,
		InsertionPointTypeWSJSONKey,
		InsertionPointTypeWSJSONArrayItem,
		InsertionPointTypeWSJSONArrayIndex,
		InsertionPointTypeWSJSONObject,
		InsertionPointTypeWSJSONArray:
		return true

	case InsertionPointTypeWSXMLElement,
		InsertionPointTypeWSXMLAttribute:
		return true

	case InsertionPointTypeWSRawMessage:
		return true

	case InsertionPointTypeWSGraphQLInlineArg,
		InsertionPointTypeWSGraphQLVariable:
		return true
	default:
		return false
	}
}

// IsHTTPType returns true if the insertion point type is HTTP-specific
func (ipt InsertionPointType) IsHTTPType() bool {
	switch ipt {
	case InsertionPointTypeParameter, InsertionPointTypeHeader, InsertionPointTypeBody,
		InsertionPointTypeCookie, InsertionPointTypeURLPath, InsertionPointTypeFullBody,
		InsertionPointTypeXMLElement, InsertionPointTypeXMLAttribute,
		InsertionPointTypeGraphQLVariable, InsertionPointTypeGraphQLInlineArg:
		return true
	default:
		return false
	}
}

// IsXMLType returns true if the insertion point type is XML-specific
func (ipt InsertionPointType) IsXMLType() bool {
	switch ipt {
	case InsertionPointTypeXMLElement,
		InsertionPointTypeXMLAttribute:
		return true

	case InsertionPointTypeWSXMLElement,
		InsertionPointTypeWSXMLAttribute:
		return true
	default:
		return false
	}
}

// IsJSONType returns true if the insertion point type is JSON-specific
func (ipt InsertionPointType) IsJSONType() bool {
	switch ipt {
	case InsertionPointTypeWSJSONField,
		InsertionPointTypeWSJSONValue,
		InsertionPointTypeWSJSONKey,
		InsertionPointTypeWSJSONArrayItem,
		InsertionPointTypeWSJSONArrayIndex,
		InsertionPointTypeWSJSONObject,
		InsertionPointTypeWSJSONArray:
		return true
	default:
		return false
	}
}

// InsertionPointSpan is the byte range of an insertion point's value inside
// OriginalData. Points addressed by span can be rewritten without re-parsing and
// without the name ambiguity that makes repeated siblings indistinguishable.
type InsertionPointSpan struct {
	Start int
	End   int
	Valid bool
}

type InsertionPoint struct {
	Type         InsertionPointType
	Name         string       // the name of the parameter/header/cookie
	Value        string       // the current value
	ValueType    lib.DataType // the type of the value (string, int, float, etc.)
	OriginalData string       // the original data (URL, header string, body, cookie string) in which this insertion point was found
	Span         InsertionPointSpan
	Behaviour    InsertionPointBehaviour
}

func (i *InsertionPoint) String() string {
	return fmt.Sprintf("%s: %s", i.Type, i.Name)
}

// LogSummary returns a concise map suitable for structured logging
func (i *InsertionPoint) LogSummary() map[string]interface{} {
	summary := map[string]interface{}{
		"type":      string(i.Type),
		"name":      i.Name,
		"value":     i.Value,
		"valueType": string(i.ValueType),
	}

	// Add behaviour flags only if they're true (non-default)
	if i.Behaviour.IsReflected {
		summary["isReflected"] = true
	}
	if i.Behaviour.IsDynamic {
		summary["isDynamic"] = true
	}

	// Add reflection context summary if available
	if i.Behaviour.ReflectionAnalysis != nil {
		ra := i.Behaviour.ReflectionAnalysis
		contexts := []string{}
		if ra.HasHTMLContext {
			contexts = append(contexts, "html")
		}
		if ra.HasScriptContext {
			contexts = append(contexts, "script")
		}
		if ra.HasAttributeContext {
			contexts = append(contexts, "attribute")
		}
		if ra.HasCSSContext {
			contexts = append(contexts, "css")
		}
		if ra.HasCommentContext {
			contexts = append(contexts, "comment")
		}
		if len(contexts) > 0 {
			summary["contexts"] = contexts
		}
	}

	return summary
}

// LogSummarySlice returns a concise slice of maps suitable for structured logging
// of multiple insertion points
func LogSummarySlice(points []InsertionPoint) []map[string]interface{} {
	summaries := make([]map[string]interface{}, len(points))
	for i := range points {
		summaries[i] = points[i].LogSummary()
	}
	return summaries
}

type InsertionPointBehaviour struct {
	// AcceptedDataTypes []lib.DataType
	IsReflected        bool
	ReflectionContexts []string
	IsDynamic          bool
	// Transformations   []Transformation

	// ReflectionAnalysis contains comprehensive reflection analysis results
	// including context detection, character efficiencies, and exploitability flags
	ReflectionAnalysis *reflection.ReflectionAnalysis
}

type Transformation struct {
	From         string
	FromDatatype lib.DataType
	To           string
	ToDatatype   lib.DataType
}

// WebSocketJSONInsertionPointTypes returns JSON-specific WebSocket insertion point types
func WebSocketJSONInsertionPointTypes() []InsertionPointType {
	return []InsertionPointType{
		InsertionPointTypeWSJSONField,
		InsertionPointTypeWSJSONValue,
		InsertionPointTypeWSJSONKey,
		InsertionPointTypeWSJSONArrayItem,
		InsertionPointTypeWSJSONArrayIndex,
		InsertionPointTypeWSJSONObject,
		InsertionPointTypeWSJSONArray,
	}
}

// Handle URL parameters
func handleURLParameters(urlData *url.URL) ([]InsertionPoint, error) {
	var points []InsertionPoint

	// URL parameters
	for name, values := range urlData.Query() {
		for _, value := range values {
			points = append(points, InsertionPoint{
				Type:         "parameter",
				Name:         name,
				Value:        value,
				ValueType:    lib.GuessDataType(value),
				OriginalData: urlData.String(),
			})
		}
	}

	return points, nil
}

// Handle URL paths
func handleURLPaths(urlData *url.URL) ([]InsertionPoint, error) {
	var points []InsertionPoint

	segments := strings.Split(urlData.Path, "/")
	lastIndex := -1
	for i, segment := range segments {
		if segment != "" {
			lastIndex = i
		}
	}

	for i, pathPart := range segments {
		if pathPart == "" {
			continue
		}
		// Fixed route names ("api", "engine", "render") are routing selectors,
		// not user input: fuzzing them only produces 404s. Variable segments
		// (ids, uuids, hashes) and the trailing segment — the realistic
		// traversal/file target — are still tested.
		if i != lastIndex && !lib.IsLikelyDynamicPathSegment(pathPart) {
			continue
		}
		points = append(points, InsertionPoint{
			Type:         InsertionPointTypeURLPath,
			Name:         pathPart,
			Value:        pathPart,
			ValueType:    lib.GuessDataType(pathPart),
			OriginalData: urlData.String(),
		})
	}

	return points, nil
}

// Handle Headers
func handleHeaders(header map[string][]string) ([]InsertionPoint, error) {
	var points []InsertionPoint
	for name, values := range header {
		if name == "cookie" {
			continue
		}
		for _, value := range values {
			points = append(points, InsertionPoint{
				Type:      InsertionPointTypeHeader,
				Name:      name,
				Value:     value,
				ValueType: lib.GuessDataType(value),

				OriginalData: header[name][0],
			})
		}
	}

	return points, nil
}

// Handle Cookies
func handleCookies(header map[string][]string) ([]InsertionPoint, error) {
	var points []InsertionPoint

	// Cookies
	if cookies, ok := header["Cookie"]; ok {
		for _, cookieString := range cookies {
			cookieValues := strings.Split(cookieString, ";")
			for _, cookieValue := range cookieValues {
				cookieParts := strings.SplitN(strings.TrimSpace(cookieValue), "=", 2)
				if len(cookieParts) == 2 {
					points = append(points, InsertionPoint{
						Type:      InsertionPointTypeCookie,
						Name:      cookieParts[0],
						Value:     cookieParts[1],
						ValueType: lib.GuessDataType(cookieParts[1]),

						OriginalData: cookieString,
					})
				}
			}
		}
	}

	return points, nil
}

func headerValueCaseInsensitive(headers map[string][]string, name string) string {
	for key, values := range headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

// defaultMaxXMLInsertionPoints bounds how many per-element points a single XML body
// contributes. Every point costs a full payload sweep, so an unbounded document would
// otherwise multiply that item's traffic without bound.
const defaultMaxXMLInsertionPoints = 25

// xmlBodyInsertionPointName is the name of the whole-document XML point. xxe.yaml's
// insertion_point_name launch condition matches on it.
const xmlBodyInsertionPointName = "xml"

func maxXMLInsertionPoints() int {
	if configured := viper.GetInt("scan.insertion_points.max_xml_points"); configured > 0 {
		return configured
	}
	return defaultMaxXMLInsertionPoints
}

// isXMLContentType also matches application/soap+xml, which SOAP 1.2 endpoints use and
// which contains neither of the two generic XML media types.
func isXMLContentType(contentType string) bool {
	return strings.Contains(contentType, "application/xml") ||
		strings.Contains(contentType, "text/xml") ||
		strings.Contains(contentType, "application/soap+xml")
}

func hasFullBodyPoint(points []InsertionPoint) bool {
	for _, p := range points {
		if p.Type == InsertionPointTypeFullBody {
			return true
		}
	}
	return false
}

// Handle Body parameters
func handleBodyParameters(contentType string, body []byte) ([]InsertionPoint, error) {
	var points []InsertionPoint

	// URL-encoded body
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		formData, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}

		for name, values := range formData {
			for _, value := range values {
				points = append(points, InsertionPoint{
					Type:      InsertionPointTypeBody,
					Name:      name,
					Value:     value,
					ValueType: lib.GuessDataType(value),

					OriginalData: string(body),
				})
			}
		}
	}

	// JSON body
	if strings.Contains(contentType, "application/json") {
		var jsonData map[string]interface{}
		err := json.Unmarshal(body, &jsonData)
		if err != nil {
			return nil, err
		}

		for name, value := range jsonData {
			valueStr := fmt.Sprintf("%v", value)
			points = append(points, InsertionPoint{
				Type:      InsertionPointTypeBody,
				Name:      name,
				Value:     valueStr,
				ValueType: lib.GuessDataType(valueStr),

				OriginalData: string(body),
			})
		}
	}

	// XML body: encoding/xml cannot unmarshal into a map, and XXE payloads are full
	// documents, so expose a single whole-body-replaceable point. It is named "xml" to
	// satisfy xxe.yaml's insertion_point_name launch condition and typed TypeXML so the
	// smart-mode filter keeps it (see pkg/active/history.go).
	if isXMLContentType(contentType) {
		points = append(points, InsertionPoint{
			Type:         InsertionPointTypeFullBody,
			Name:         xmlBodyInsertionPointName,
			Value:        string(body),
			ValueType:    lib.TypeXML,
			OriginalData: string(body),
		})
	}

	// Multipart form body
	// Multipart form body
	if strings.Contains(contentType, "multipart/form-data") {
		_, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			return nil, err
		}
		boundary, ok := params["boundary"]
		if !ok {
			return nil, errors.New("Content-Type does not contain boundary parameter")
		}

		mr := multipart.NewReader(strings.NewReader(string(body)), boundary)
		form, err := mr.ReadForm(10 << 20) // Max memory 10 MB
		if err != nil {
			return nil, err
		}

		for name, values := range form.Value {
			for _, value := range values {
				points = append(points, InsertionPoint{
					Type:      InsertionPointTypeBody,
					Name:      name,
					Value:     value,
					ValueType: lib.GuessDataType(value),

					OriginalData: string(body),
				})
			}
		}
	}

	return points, nil
}

// isScoped reports whether an insertion point kind should be extracted. An empty
// scope means the caller configured no restriction, which is the same meaning
// HistoryItemScanOptions.IsScopedInsertionPoint gives it — treating it as "none"
// here would silently reduce callers that omit the option (the API scan executor
// among them) to body-only fuzzing.
func isScoped(scoped []string, kind string) bool {
	return len(scoped) == 0 || lib.SliceContains(scoped, kind)
}

func GetInsertionPoints(history *db.History, scoped []string) ([]InsertionPoint, error) {
	var points []InsertionPoint

	// Analyze URL
	urlData, err := url.Parse(history.URL)
	if err != nil {
		return nil, err
	}
	if isScoped(scoped, "parameters") {
		urlPoints, err := handleURLParameters(urlData)
		if err != nil {
			return nil, err
		}
		points = append(points, urlPoints...)
	}

	if isScoped(scoped, "urlpath") {
		urlPathPoints, err := handleURLPaths(urlData)
		if err != nil {
			return nil, err
		}
		points = append(points, urlPathPoints...)
	}

	headers, err := history.RequestHeaders()
	if err != nil {
		log.Error().Err(err).Str("headers", "failed to parse").Msg("Error getting request headers as map")
	} else {
		if isScoped(scoped, "headers") {
			// Headers
			headerPoints, err := handleHeaders(headers)
			if err != nil {
				return nil, err
			}
			points = append(points, headerPoints...)
		}

		if isScoped(scoped, "cookies") {
			// Cookies
			cookiePoints, err := handleCookies(headers)
			if err != nil {
				return nil, err
			}
			points = append(points, cookiePoints...)
		}
	}

	// Body parameters
	body, _ := history.RequestBody()
	bodyStr := string(body)

	// The RequestContentType column is empty for crawler-discovered browser POSTs; the
	// content type only survives in RawRequest, so fall back to the parsed headers when
	// the column is empty (keeping populated cases byte-identical).
	effectiveContentType := history.RequestContentType
	if effectiveContentType == "" {
		effectiveContentType = headerValueCaseInsensitive(headers, "Content-Type")
	}

	// Use the header-fallback content type: crawler/proxy-captured GraphQL POSTs leave
	// RequestContentType empty and would otherwise be seen as a plain JSON body.
	graphQLEnvelope := parseGraphQLEnvelope(effectiveContentType, body)

	// query, variables and operationName are transport structure, not application
	// input: replacing any of them (or the whole body) makes the server reject the
	// request with a 400 before a resolver runs, so those points can never yield a
	// finding. Only the GraphQL points below reach application code.
	if graphQLEnvelope == nil {
		bodyPoints, err := handleBodyParameters(effectiveContentType, body)
		if err != nil {
			return nil, err
		}
		points = append(points, bodyPoints...)

		// Per-element XML points make individual SOAP/XML parameters addressable. The
		// whole-body point above stays as the XXE surface; these carry the value-injection
		// surface it cannot reach.
		if isScoped(scoped, "xml") && isXMLContentType(effectiveContentType) {
			points = append(points, ExtractXMLPoints(bodyStr, XMLPointOptions{
				ElementType: InsertionPointTypeXMLElement,
				MaxPoints:   maxXMLInsertionPoints(),
			})...)
		}

		if len(bodyPoints) > 0 && !hasFullBodyPoint(bodyPoints) {
			points = append(points, InsertionPoint{
				Type:         InsertionPointTypeFullBody,
				Name:         "fullbody",
				Value:        bodyStr,
				ValueType:    lib.GuessDataType(bodyStr),
				OriginalData: bodyStr,
			})
		}
	}

	if isScoped(scoped, "graphql") && graphQLEnvelope != nil {
		if vars, ok := graphQLEnvelope["variables"].(map[string]any); ok && len(vars) > 0 {
			points = append(points, extractGraphQLVariablePoints("", vars, bodyStr)...)
		}
		if queryStr, ok := graphQLEnvelope["query"].(string); ok {
			points = append(points, extractGraphQLInlineArgPoints(queryStr, bodyStr)...)
		}
	}

	return points, nil
}

func parseGraphQLEnvelope(contentType string, body []byte) map[string]any {
	if len(body) == 0 || !strings.Contains(contentType, "application/json") {
		return nil
	}
	var jsonData map[string]any
	if err := json.Unmarshal(body, &jsonData); err != nil || !isGraphQLBody(jsonData) {
		return nil
	}
	return jsonData
}
