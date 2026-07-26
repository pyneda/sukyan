package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pyneda/sukyan/pkg/api/core"
)

type RequestBuilder struct {
	DefaultHeaders map[string]string
	AuthConfig     *AuthConfig
}

type AuthConfig struct {
	BearerToken   string
	BasicUsername string
	BasicPassword string
	APIKey        string
	APIKeyHeader  string
	APIKeyIn      string
	CustomHeaders map[string]string
}

func NewRequestBuilder() *RequestBuilder {
	return &RequestBuilder{
		DefaultHeaders: map[string]string{
			"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36",
			"Accept":     "application/json, */*",
		},
	}
}

func (b *RequestBuilder) WithAuth(config *AuthConfig) *RequestBuilder {
	b.AuthConfig = config
	return b
}

func (b *RequestBuilder) Build(ctx context.Context, op core.Operation, paramValues map[string]any) (*http.Request, error) {
	fullURL, err := b.buildURL(op, paramValues)
	if err != nil {
		return nil, fmt.Errorf("building URL: %w", err)
	}

	var body []byte
	contentType := "application/json"

	if op.HasBodyParameters() {
		body, contentType, err = b.buildBody(op, paramValues)
		if err != nil {
			return nil, fmt.Errorf("building body: %w", err)
		}
	}

	method := op.Method
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if len(body) > 0 && contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	b.addHeaderParams(req, op, paramValues)
	b.addCookieParams(req, op, paramValues)
	b.addDefaultHeaders(req)
	b.applyAuth(req)

	return req, nil
}

func (b *RequestBuilder) BuildWithModifiedParam(ctx context.Context, op core.Operation, paramName string, modifiedValue any, paramValues map[string]any) (*http.Request, error) {
	modifiedValues := make(map[string]any)
	for k, v := range paramValues {
		modifiedValues[k] = v
	}
	modifiedValues[paramName] = modifiedValue
	return b.Build(ctx, op, modifiedValues)
}

func (b *RequestBuilder) GetDefaultParamValues(op core.Operation) map[string]any {
	values := make(map[string]any)
	for _, param := range op.Parameters {
		values[param.Name] = param.GetEffectiveValue()
	}
	return values
}

// pathPlaceholder matches a path template segment such as "{petId}".
var pathPlaceholder = regexp.MustCompile(`\{[^{}/]*\}`)

func (b *RequestBuilder) buildURL(op core.Operation, paramValues map[string]any) (string, error) {
	base, err := url.Parse(strings.TrimSuffix(op.BaseURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		// A base URL without a scheme and host produces a request that cannot be sent
		// at all; failing here names the endpoint instead of emitting "/pet/1".
		return "", fmt.Errorf("operation %s %s has no usable base URL (%q)", op.Method, op.Path, op.BaseURL)
	}

	path := op.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	for _, param := range op.Parameters {
		if param.Location == core.ParameterLocationPath {
			value := paramValues[param.Name]
			if value == nil {
				value = param.GetEffectiveValue()
			}
			// A path segment must never render as the literal "<nil>": that breaks
			// routing and produces bogus URLs like /pet/%3Cnil%3E for value-less params.
			if value == nil {
				value = "1"
			}
			placeholder := "{" + param.Name + "}"
			encoded := url.PathEscape(serializeValue(value))
			path = strings.ReplaceAll(path, placeholder, encoded)
		}
	}

	// A template the spec never declares a parameter for would otherwise be requested
	// literally as "/users/%7Bid%7D", which only ever reaches a 404.
	path = pathPlaceholder.ReplaceAllString(path, "1")

	fullURL := strings.TrimSuffix(base.String(), "/") + path

	queryParams := url.Values{}
	for _, param := range op.Parameters {
		if param.Location == core.ParameterLocationQuery {
			value := paramValues[param.Name]
			if value == nil && !param.Required {
				continue
			}
			if value == nil {
				value = param.GetEffectiveValue()
			}

			addQueryParam(queryParams, param, value)
		}
	}

	if len(queryParams) > 0 {
		fullURL += "?" + queryParams.Encode()
	}

	return fullURL, nil
}

// addQueryParam serialises a value according to the parameter's style and explode
// settings. Emitting repeated keys for a non-exploded array, or a JSON blob for a
// deepObject, sends values the API parses differently from what the spec declares.
func addQueryParam(queryParams url.Values, param core.Parameter, value any) {
	items, isList := listValues(value)
	if isList {
		if explodeParam(param) {
			for _, item := range items {
				queryParams.Add(param.Name, item)
			}
			return
		}
		queryParams.Set(param.Name, strings.Join(items, listSeparator(param.Style)))
		return
	}

	if fields, ok := value.(map[string]any); ok && param.Style == "deepObject" {
		for _, name := range sortedKeys(fields) {
			queryParams.Set(fmt.Sprintf("%s[%s]", param.Name, name), serializeValue(fields[name]))
		}
		return
	}

	queryParams.Set(param.Name, serializeValue(value))
}

func listValues(value any) ([]string, bool) {
	switch typed := value.(type) {
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			items = append(items, serializeValue(item))
		}
		return items, true
	case []string:
		return typed, true
	default:
		return nil, false
	}
}

func listSeparator(style string) string {
	switch style {
	case "spaceDelimited":
		return " "
	case "pipeDelimited":
		return "|"
	default:
		return ","
	}
}

// explodeParam reports the effective explode value; the OpenAPI default is true for
// the form style used by query parameters and false for every other style.
func explodeParam(param core.Parameter) bool {
	if param.Explode != nil {
		return *param.Explode
	}
	return param.Style == "" || param.Style == "form"
}

// serializeValue renders a parameter value for a URL, header or form field. Go's
// "%v" turns floats into scientific notation and maps into "map[k:v]", neither of
// which any API accepts.
func serializeValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case int:
		return strconv.Itoa(typed)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case float32:
		return formatFloat(float64(typed), 32)
	case float64:
		return formatFloat(typed, 64)
	case json.Number:
		return typed.String()
	case []any, []string, map[string]any:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return string(encoded)
	default:
		return fmt.Sprintf("%v", typed)
	}
}

// formatFloat keeps ordinary magnitudes in plain decimal, since scientific notation
// is rejected by many numeric parsers, and only uses exponent form for values that
// would otherwise expand into hundreds of digits.
func formatFloat(value float64, bits int) string {
	abs := math.Abs(value)
	if math.IsInf(value, 0) || math.IsNaN(value) || (abs != 0 && (abs < 1e-6 || abs >= 1e21)) {
		return strconv.FormatFloat(value, 'g', -1, bits)
	}
	return strconv.FormatFloat(value, 'f', -1, bits)
}

// sanitizeHeaderToken strips the control characters a spec-supplied header or cookie
// name could otherwise smuggle into a request; Go's Header.Set does not filter them.
func sanitizeHeaderToken(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
}

func (b *RequestBuilder) buildBody(op core.Operation, paramValues map[string]any) ([]byte, string, error) {
	bodyParams := make(map[string]any)

	for _, param := range op.Parameters {
		if param.Location == core.ParameterLocationBody {
			value := paramValues[param.Name]
			if value == nil {
				value = param.GetEffectiveValue()
			}
			bodyParams[param.Name] = value
		}
	}

	if len(bodyParams) == 0 {
		return nil, "", nil
	}

	contentType := "application/json"
	if op.OpenAPI != nil && op.OpenAPI.RequestBody != nil && op.OpenAPI.RequestBody.ContentType != "" {
		contentType = op.OpenAPI.RequestBody.ContentType
	}

	// A scalar or array body is carried whole by a single parameter. Wrapping it in
	// an object would send {"body": [...]} where the API expects [...].
	structured := true
	if op.OpenAPI != nil && op.OpenAPI.RequestBody != nil {
		structured = op.OpenAPI.RequestBody.Structured
	}

	// The whole payload is carried by one parameter only when the parser said the body
	// is unstructured AND that single "body" parameter is all there is. Operations
	// built by hand, or stored before Structured existed, leave the flag false while
	// still holding real named fields, so the shape check has to agree.
	var payload any = bodyParams
	isWholeBody := false
	if !structured && len(bodyParams) == 1 {
		if value, ok := bodyParams[wholeBodyParamName]; ok {
			payload = value
			isWholeBody = true
		}
	}

	// A non-object body has no fields to name, so the field-based encodings send the
	// raw value rather than inventing a field called "body". XML and multipart are
	// excluded: they carry an envelope a bare scalar cannot satisfy, and a server
	// rejects the request before the operation is ever exercised.
	if isWholeBody && !strings.Contains(contentType, "json") &&
		!strings.Contains(contentType, "xml") && !strings.HasPrefix(contentType, "multipart/") {
		return []byte(serializeValue(payload)), contentType, nil
	}

	var body []byte
	var err error

	switch {
	case contentType == "application/x-www-form-urlencoded":
		formValues := url.Values{}
		for _, name := range sortedKeys(bodyParams) {
			formValues.Set(name, serializeValue(bodyParams[name]))
		}
		body = []byte(formValues.Encode())
	case strings.HasPrefix(contentType, "multipart/"):
		buf := new(bytes.Buffer)
		writer := multipart.NewWriter(buf)
		if err := writer.SetBoundary(multipartBoundary); err != nil {
			return nil, "", fmt.Errorf("setting multipart boundary: %w", err)
		}
		for _, name := range sortedKeys(bodyParams) {
			if err := writer.WriteField(name, serializeValue(bodyParams[name])); err != nil {
				return nil, "", fmt.Errorf("writing multipart field %s: %w", name, err)
			}
		}
		if err := writer.Close(); err != nil {
			return nil, "", fmt.Errorf("closing multipart writer: %w", err)
		}
		body = buf.Bytes()
		contentType = writer.FormDataContentType()
	case strings.Contains(contentType, "xml"):
		if isWholeBody {
			body, err = encodeXMLScalarBody(serializeValue(payload))
		} else {
			body, err = encodeXMLBody(bodyParams)
		}
		if err != nil {
			return nil, "", fmt.Errorf("encoding xml body: %w", err)
		}
	case strings.HasPrefix(contentType, "text/"):
		body = []byte(serializeValue(payload))
	default:
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, "", fmt.Errorf("marshaling body: %w", err)
		}
	}

	return body, contentType, nil
}

// multipartBoundary is fixed so that two runs over the same operation produce
// byte-identical requests; the default writer boundary is random.
const multipartBoundary = "sukyanAPIBoundary"

// wholeBodyParamName is the name the parser gives the single parameter that carries
// a non-object request body.
const wholeBodyParamName = "body"

// encodeXMLScalarBody wraps a whole-body scalar in the same <root> envelope a
// structured body gets. The schema names no element for it and a bare scalar is not
// a document, so without the envelope the payload is unparseable XML.
func encodeXMLScalarBody(value string) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteString("<root>")
	if err := xml.EscapeText(&buffer, []byte(value)); err != nil {
		return nil, err
	}
	buffer.WriteString("</root>")
	return buffer.Bytes(), nil
}

func encodeXMLBody(fields map[string]any) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteString("<root>")
	for _, name := range sortedKeys(fields) {
		element := xml.StartElement{Name: xml.Name{Local: name}}
		if err := xml.NewEncoder(&buffer).EncodeElement(serializeValue(fields[name]), element); err != nil {
			return nil, err
		}
	}
	buffer.WriteString("</root>")
	return buffer.Bytes(), nil
}

func sortedKeys(fields map[string]any) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (b *RequestBuilder) addHeaderParams(req *http.Request, op core.Operation, paramValues map[string]any) {
	for _, param := range op.Parameters {
		if param.Location == core.ParameterLocationHeader {
			value := paramValues[param.Name]
			if value == nil && !param.Required {
				continue
			}
			if value == nil {
				value = param.GetEffectiveValue()
			}
			name := sanitizeHeaderToken(param.Name)
			if name == "" {
				continue
			}
			req.Header.Set(name, sanitizeHeaderToken(serializeValue(value)))
		}
	}
}

func (b *RequestBuilder) addCookieParams(req *http.Request, op core.Operation, paramValues map[string]any) {
	for _, param := range op.Parameters {
		if param.Location == core.ParameterLocationCookie {
			value := paramValues[param.Name]
			if value == nil && !param.Required {
				continue
			}
			if value == nil {
				value = param.GetEffectiveValue()
			}
			name := sanitizeHeaderToken(param.Name)
			if name == "" {
				continue
			}
			req.AddCookie(&http.Cookie{
				Name:  name,
				Value: sanitizeHeaderToken(serializeValue(value)),
			})
		}
	}
}

func (b *RequestBuilder) addDefaultHeaders(req *http.Request) {
	for k, v := range b.DefaultHeaders {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}
}

func (b *RequestBuilder) applyAuth(req *http.Request) {
	if b.AuthConfig == nil {
		return
	}

	if b.AuthConfig.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.AuthConfig.BearerToken)
	}

	if b.AuthConfig.BasicUsername != "" || b.AuthConfig.BasicPassword != "" {
		req.SetBasicAuth(b.AuthConfig.BasicUsername, b.AuthConfig.BasicPassword)
	}

	if b.AuthConfig.APIKey != "" && b.AuthConfig.APIKeyHeader != "" {
		switch b.AuthConfig.APIKeyIn {
		case "query":
			q := req.URL.Query()
			q.Set(b.AuthConfig.APIKeyHeader, b.AuthConfig.APIKey)
			req.URL.RawQuery = q.Encode()
		case "cookie":
			req.AddCookie(&http.Cookie{
				Name:  b.AuthConfig.APIKeyHeader,
				Value: b.AuthConfig.APIKey,
			})
		default:
			req.Header.Set(b.AuthConfig.APIKeyHeader, b.AuthConfig.APIKey)
		}
	}

	for k, v := range b.AuthConfig.CustomHeaders {
		req.Header.Set(k, v)
	}
}

func BuildRequest(ctx context.Context, op core.Operation, paramValues map[string]any) (*http.Request, error) {
	builder := NewRequestBuilder()
	return builder.Build(ctx, op, paramValues)
}
