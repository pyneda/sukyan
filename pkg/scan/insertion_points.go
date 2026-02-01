package scan

import (
	"encoding/json"
	"encoding/xml"
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

// GraphQL insertion point types
const (
	InsertionPointTypeGraphQLVariable  InsertionPointType = "graphql_variable"
	InsertionPointTypeGraphQLInlineArg InsertionPointType = "graphql_inline_arg"
)

// WebSocket general insertion point types
const (
	InsertionPointTypeWSRawMessage InsertionPointType = "ws_raw_message"
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

// WebSocket XML insertion point types
const (
	InsertionPointTypeWSXMLElement    InsertionPointType = "ws_xml_element"    // Element value/content
	InsertionPointTypeWSXMLTag        InsertionPointType = "ws_xml_tag"        // Tag name itself
	InsertionPointTypeWSXMLAttribute  InsertionPointType = "ws_xml_attribute"  // XML attribute value
	InsertionPointTypeWSXMLNamespace  InsertionPointType = "ws_xml_namespace"  // Namespace value
	InsertionPointTypeWSXMLNSPrefix   InsertionPointType = "ws_xml_ns_prefix"  // Namespace prefix
	InsertionPointTypeWSXMLProcessing InsertionPointType = "ws_xml_processing" // Processing instruction
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
	case InsertionPointTypeWSXMLTag:
		return "WebSocket XML Tag Name"
	case InsertionPointTypeWSXMLAttribute:
		return "WebSocket XML Attribute"
	case InsertionPointTypeWSXMLNamespace:
		return "WebSocket XML Namespace"
	case InsertionPointTypeWSXMLNSPrefix:
		return "WebSocket XML Namespace Prefix"
	case InsertionPointTypeWSXMLProcessing:
		return "WebSocket XML Processing Instruction"
	case InsertionPointTypeWSRawMessage:
		return "WebSocket Raw Message"
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
		InsertionPointTypeWSXMLTag,
		InsertionPointTypeWSXMLAttribute,
		InsertionPointTypeWSXMLNamespace,
		InsertionPointTypeWSXMLNSPrefix,
		InsertionPointTypeWSXMLProcessing,

		// WebSocket general types
		InsertionPointTypeWSRawMessage,
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
		InsertionPointTypeWSXMLTag,
		InsertionPointTypeWSXMLAttribute,
		InsertionPointTypeWSXMLNamespace,
		InsertionPointTypeWSXMLNSPrefix,
		InsertionPointTypeWSXMLProcessing,

		InsertionPointTypeWSRawMessage,
	}
}

// WebSocketXMLInsertionPointTypes returns XML-specific WebSocket insertion point types
func WebSocketXMLInsertionPointTypes() []InsertionPointType {
	return []InsertionPointType{
		InsertionPointTypeWSXMLElement,
		InsertionPointTypeWSXMLTag,
		InsertionPointTypeWSXMLAttribute,
		InsertionPointTypeWSXMLNamespace,
		InsertionPointTypeWSXMLNSPrefix,
		InsertionPointTypeWSXMLProcessing,
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
		InsertionPointTypeWSXMLTag,
		InsertionPointTypeWSXMLAttribute,
		InsertionPointTypeWSXMLNamespace,
		InsertionPointTypeWSXMLNSPrefix,
		InsertionPointTypeWSXMLProcessing:
		return true

	case InsertionPointTypeWSRawMessage:
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
		InsertionPointTypeGraphQLVariable, InsertionPointTypeGraphQLInlineArg:
		return true
	default:
		return false
	}
}

// IsXMLType returns true if the insertion point type is XML-specific
func (ipt InsertionPointType) IsXMLType() bool {
	switch ipt {
	case InsertionPointTypeWSXMLElement,
		InsertionPointTypeWSXMLTag,
		InsertionPointTypeWSXMLAttribute,
		InsertionPointTypeWSXMLNamespace,
		InsertionPointTypeWSXMLNSPrefix,
		InsertionPointTypeWSXMLProcessing:
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

type InsertionPoint struct {
	Type         InsertionPointType
	Name         string       // the name of the parameter/header/cookie
	Value        string       // the current value
	ValueType    lib.DataType // the type of the value (string, int, float, etc.)
	OriginalData string       // the original data (URL, header string, body, cookie string) in which this insertion point was found
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

	// URL parameters
	for _, pathPart := range strings.Split(urlData.Path, "/") {
		if pathPart == "" {
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

	// XML body
	if strings.Contains(contentType, "application/xml") {
		var xmlData map[string]interface{}
		err := xml.Unmarshal(body, &xmlData)
		if err != nil {
			return nil, err
		}

		for name, value := range xmlData {
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

func GetInsertionPoints(history *db.History, scoped []string) ([]InsertionPoint, error) {
	var points []InsertionPoint

	// Analyze URL
	urlData, err := url.Parse(history.URL)
	if err != nil {
		return nil, err
	}
	if lib.SliceContains(scoped, "parameters") {
		urlPoints, err := handleURLParameters(urlData)
		if err != nil {
			return nil, err
		}
		points = append(points, urlPoints...)
	}

	if lib.SliceContains(scoped, "urlpath") {
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
		if lib.SliceContains(scoped, "headers") {
			// Headers
			headerPoints, err := handleHeaders(headers)
			if err != nil {
				return nil, err
			}
			points = append(points, headerPoints...)
		}

		if lib.SliceContains(scoped, "cookies") {
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

	bodyPoints, err := handleBodyParameters(history.RequestContentType, body)
	if err != nil {
		return nil, err
	}
	points = append(points, bodyPoints...)
	if len(bodyPoints) > 0 {
		points = append(points, InsertionPoint{
			Type:         InsertionPointTypeFullBody,
			Name:         "fullbody",
			Value:        bodyStr,
			ValueType:    lib.GuessDataType(bodyStr),
			OriginalData: bodyStr,
		})
	}

	// GraphQL-specific insertion points (variables + inline args)
	if (len(scoped) == 0 || lib.SliceContains(scoped, "graphql")) &&
		strings.Contains(history.RequestContentType, "application/json") && len(body) > 0 {
		var jsonData map[string]any
		if err := json.Unmarshal(body, &jsonData); err == nil && isGraphQLBody(jsonData) {
			if vars, ok := jsonData["variables"].(map[string]any); ok && len(vars) > 0 {
				points = append(points, extractGraphQLVariablePoints("", vars, bodyStr)...)
			}
			if queryStr, ok := jsonData["query"].(string); ok {
				points = append(points, extractGraphQLInlineArgPoints(queryStr, bodyStr)...)
			}
		}
	}

	return points, nil
}
