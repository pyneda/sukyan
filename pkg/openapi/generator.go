package openapi

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"mime/multipart"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

const (
	defaultBaseURL = "http://localhost"
	// defaultPathSegment fills a path placeholder the spec never declares a
	// parameter for. Sending a literal "{id}" reaches a 404 instead of the handler.
	defaultPathSegment = "1"
	// defaultMaxRequestsPerEndpoint bounds fuzz expansion so a large spec cannot
	// produce an unbounded number of variations.
	defaultMaxRequestsPerEndpoint = 250
	multipartBoundary             = "sukyanOpenAPIBoundary"
)

var pathPlaceholder = regexp.MustCompile(`\{[^{}/]*\}`)

// GenerateRequests generates endpoints and their request variations
func GenerateRequests(doc *Document, config GenerationConfig) ([]Endpoint, error) {
	if doc == nil {
		return nil, errors.New("nil openapi document")
	}

	base, err := parseBaseURL(config.BaseURL)
	if err != nil {
		return nil, err
	}

	securitySchemes := doc.GetSecuritySchemesLegacy()
	globalSecurityRequirements := doc.GetGlobalSecurityRequirements()

	maxRequests := config.MaxRequestsPerEndpoint
	if maxRequests <= 0 {
		maxRequests = defaultMaxRequestsPerEndpoint
	}

	entries := doc.Operations()
	endpoints := make([]Endpoint, 0, len(entries))

	for _, entry := range entries {
		endpoint := Endpoint{
			Method:      entry.Method,
			Path:        entry.Path,
			OperationID: entry.Operation.OperationID,
			Summary:     entry.Operation.Summary,
			Description: entry.Operation.Description,
			Requests:    []RequestVariation{},
		}
		for _, param := range entry.Parameters {
			endpoint.Parameters = append(endpoint.Parameters, ParameterMetadata{
				Name:     param.Name,
				In:       param.In,
				Required: param.Required,
				Schema:   schemaToMap(param.Schema),
			})
		}

		requirements := globalSecurityRequirements
		if operationRequirements, overridden := doc.GetOperationSecurityRequirements(entry.Operation); overridden {
			requirements = operationRequirements
		}
		endpoint.Security = requirements
		endpoint.RequiresAuth = requiresAuth(requirements)

		ctx := &generationContext{
			entry:      entry,
			params:     entry.Parameters,
			baseline:   baselineParameters(entry.Parameters, config),
			config:     config,
			base:       base,
			schemes:    securitySchemes,
			opSecurity: firstAlternativeSchemes(requirements),
		}

		seen := make(map[string]bool)
		var requests []RequestVariation
		add := func(req RequestVariation) bool {
			if len(requests) >= maxRequests {
				return false
			}
			signature := getRequestSignature(req)
			if seen[signature] {
				return true
			}
			seen[signature] = true
			requests = append(requests, req)
			return true
		}

		happy := ctx.build(nil)
		happy.Label = "Happy Path"
		add(happy)

		// A minimal baseline is what IncludeOptionalParams asks for, but optional
		// parameters are still attack surface, so a full-parameter variation is always
		// emitted. It deduplicates away when there is nothing optional to add.
		if full := ctx.buildWithAllParameters(); full != nil {
			full.Label = "Happy Path (all parameters)"
			add(*full)
		}

		if config.FuzzingEnabled {
			for _, req := range ctx.fuzz() {
				if !add(req) {
					break
				}
			}
		}

		endpoint.Requests = requests
		endpoints = append(endpoints, endpoint)
	}

	return endpoints, nil
}

func parseBaseURL(raw string) (*url.URL, error) {
	if raw == "" {
		raw = defaultBaseURL
	}
	base, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid base url %q: %w", raw, err)
	}
	if base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("invalid base url %q: expected an absolute url", raw)
	}
	return base, nil
}

// baselineParameters selects the parameters carried by every request. Without
// IncludeOptionalParams only required ones are sent, keeping the baseline request
// minimal; path parameters are always kept because the URL does not resolve without
// them. Fuzzing still reaches optional parameters — see generationContext.includes —
// so narrowing the baseline never narrows the tested attack surface.
func baselineParameters(params []*openapi3.Parameter, config GenerationConfig) map[*openapi3.Parameter]bool {
	baseline := make(map[*openapi3.Parameter]bool, len(params))
	for _, param := range params {
		if config.IncludeOptionalParams || param.Required || param.In == openapi3.ParameterInPath {
			baseline[param] = true
		}
	}
	return baseline
}

// firstAlternativeSchemes returns the schemes of the first authentication
// alternative. Alternatives are an OR: attaching credentials from all of them at
// once would send a request no real client would make.
func firstAlternativeSchemes(requirements []SecurityRequirement) []string {
	for _, requirement := range requirements {
		if len(requirement.Schemes) == 0 {
			continue
		}
		names := make([]string, 0, len(requirement.Schemes))
		for _, scheme := range requirement.Schemes {
			names = append(names, scheme.Name)
		}
		return names
	}
	return nil
}

func requiresAuth(requirements []SecurityRequirement) bool {
	if len(requirements) == 0 {
		return false
	}
	for _, requirement := range requirements {
		if len(requirement.Schemes) == 0 {
			return false
		}
	}
	return true
}

func getRequestSignature(req RequestVariation) string {
	headerKeys := make([]string, 0, len(req.Headers))
	for key := range req.Headers {
		headerKeys = append(headerKeys, key)
	}
	sort.Strings(headerKeys)

	var headerSig strings.Builder
	for _, key := range headerKeys {
		headerSig.WriteString(key)
		headerSig.WriteString(":")
		headerSig.WriteString(req.Headers[key])
		headerSig.WriteString(";")
	}

	return fmt.Sprintf("%s|%s|%s", req.URL, headerSig.String(), string(req.Body))
}

type generationContext struct {
	entry      OperationEntry
	params     []*openapi3.Parameter
	baseline   map[*openapi3.Parameter]bool
	config     GenerationConfig
	base       *url.URL
	schemes    []SecuritySchemeInfo
	opSecurity []string
}

// includes reports whether a parameter belongs in this request: everything in the
// baseline, plus whichever parameter is currently being fuzzed.
func (c *generationContext) includes(param *openapi3.Parameter, fuzz *FuzzTarget) bool {
	if c.baseline[param] {
		return true
	}
	return fuzz != nil && fuzz.Name == param.Name && fuzz.In == param.In
}

// buildWithAllParameters returns a request carrying every declared parameter, or nil
// when the baseline already covers them all.
func (c *generationContext) buildWithAllParameters() *RequestVariation {
	if len(c.baseline) == len(c.params) {
		return nil
	}
	full := &generationContext{
		entry:      c.entry,
		params:     c.params,
		baseline:   baselineParameters(c.params, GenerationConfig{IncludeOptionalParams: true}),
		config:     c.config,
		base:       c.base,
		schemes:    c.schemes,
		opSecurity: c.opSecurity,
	}
	request := full.build(nil)
	return &request
}

type FuzzTarget struct {
	Name  string
	In    string
	Value interface{}
}

func (c *generationContext) build(fuzz *FuzzTarget) RequestVariation {
	req := RequestVariation{Headers: make(map[string]string)}

	target := *c.base
	queryParams := target.Query()
	securityQueryParams := make(map[string]string)
	applySecurityHeaders(req.Headers, securityQueryParams, c.schemes, c.opSecurity)

	securityKeys := make([]string, 0, len(securityQueryParams))
	for key := range securityQueryParams {
		securityKeys = append(securityKeys, key)
	}
	sort.Strings(securityKeys)
	for _, key := range securityKeys {
		queryParams.Set(key, securityQueryParams[key])
	}

	pathValues := make(map[string]string)
	var cookies []string

	for _, param := range c.params {
		if !c.includes(param, fuzz) {
			continue
		}
		value := c.valueFor(param, fuzz)

		switch param.In {
		case openapi3.ParameterInPath:
			pathValues[param.Name] = serializeScalar(value)
		case openapi3.ParameterInQuery:
			applyQueryParam(queryParams, param, value)
		case openapi3.ParameterInHeader:
			if name := sanitizeHeaderToken(param.Name); name != "" {
				req.Headers[name] = sanitizeHeaderToken(serializeScalar(value))
			}
		case openapi3.ParameterInCookie:
			cookies = append(cookies, fmt.Sprintf("%s=%s", sanitizeHeaderToken(param.Name), sanitizeHeaderToken(serializeScalar(value))))
		}
	}

	if len(cookies) > 0 {
		existing := req.Headers["Cookie"]
		joined := strings.Join(cookies, "; ")
		if existing != "" {
			req.Headers["Cookie"] = existing + "; " + joined
		} else {
			req.Headers["Cookie"] = joined
		}
	}

	setPath(&target, c.base, c.entry.Path, pathValues)
	target.RawQuery = queryParams.Encode()
	req.URL = target.String()

	if contentType, body, ok := c.buildBody(fuzz); ok {
		req.Headers["Content-Type"] = contentType
		req.Body = body
	}

	return req
}

func (c *generationContext) valueFor(param *openapi3.Parameter, fuzz *FuzzTarget) interface{} {
	if fuzz != nil && fuzz.Name == param.Name && fuzz.In == param.In {
		return fuzz.Value
	}
	// A spec-provided example is a value the API is known to accept, so it beats a
	// synthesised placeholder at reaching real handler code.
	if example := parameterExample(param); example != nil {
		return example
	}
	return defaultValueFor(param.Schema)
}

func parameterExample(param *openapi3.Parameter) interface{} {
	if param.Example != nil {
		return param.Example
	}
	for _, name := range sortedExampleNames(param.Examples) {
		if ref := param.Examples[name]; ref != nil && ref.Value != nil && ref.Value.Value != nil {
			return ref.Value.Value
		}
	}
	return nil
}

func sortedExampleNames(examples openapi3.Examples) []string {
	names := make([]string, 0, len(examples))
	for name := range examples {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func defaultValueFor(schema *openapi3.SchemaRef) interface{} {
	return defaultValueFromMap(schemaToMap(schema))
}

func defaultValueFromMap(schema map[string]interface{}) interface{} {
	values := (&DefaultValueStrategy{}).Generate(schema)
	if len(values) == 0 {
		return nil
	}
	return values[0].Value
}

// setPath substitutes the declared path parameters and escapes their values so a
// value containing "/" or "?" cannot change which route is addressed.
func setPath(target *url.URL, base *url.URL, template string, values map[string]string) {
	resolved := template
	for name, value := range values {
		resolved = strings.ReplaceAll(resolved, "{"+name+"}", url.PathEscape(value))
	}
	resolved = pathPlaceholder.ReplaceAllString(resolved, defaultPathSegment)

	escaped := joinPath(base.EscapedPath(), resolved)
	decoded, err := url.PathUnescape(escaped)
	if err != nil {
		target.Path = escaped
		target.RawPath = ""
		return
	}
	target.Path = decoded
	target.RawPath = escaped
}

func applyQueryParam(queryParams url.Values, param *openapi3.Parameter, value interface{}) {
	switch typed := value.(type) {
	case []interface{}:
		if explodeParam(param) {
			queryParams.Del(param.Name)
			for _, item := range typed {
				queryParams.Add(param.Name, serializeScalar(item))
			}
			return
		}
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, serializeScalar(item))
		}
		queryParams.Set(param.Name, strings.Join(parts, ","))
	case map[string]interface{}:
		if param.Style == "deepObject" {
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				queryParams.Set(fmt.Sprintf("%s[%s]", param.Name, key), serializeScalar(typed[key]))
			}
			return
		}
		queryParams.Set(param.Name, serializeScalar(typed))
	default:
		queryParams.Set(param.Name, serializeScalar(value))
	}
}

// explodeParam reports the effective explode value; the OpenAPI default is true for
// the form style used by query parameters and false everywhere else.
func explodeParam(param *openapi3.Parameter) bool {
	if param.Explode != nil {
		return *param.Explode
	}
	return param.Style == "" || param.Style == "form"
}

// serializeScalar renders a value for a URL or header. Floats use plain decimal
// notation because scientific notation is rejected by most numeric parsers.
func serializeScalar(value interface{}) string {
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
	case []interface{}, map[string]interface{}:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return string(encoded)
	default:
		return fmt.Sprintf("%v", typed)
	}
}

// sanitizeHeaderToken strips the control characters a spec-supplied header name or
// value could otherwise smuggle into a request.
func sanitizeHeaderToken(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
}

// formatFloat keeps ordinary magnitudes in plain decimal, since scientific notation
// is rejected by many numeric parsers, and only falls back to exponent form for
// values that would otherwise expand into hundreds of digits.
func formatFloat(value float64, bits int) string {
	abs := math.Abs(value)
	if math.IsInf(value, 0) || math.IsNaN(value) || (abs != 0 && (abs < 1e-6 || abs >= 1e21)) {
		return strconv.FormatFloat(value, 'g', -1, bits)
	}
	return strconv.FormatFloat(value, 'f', -1, bits)
}

func (c *generationContext) buildBody(fuzz *FuzzTarget) (string, []byte, bool) {
	body := c.entry.Operation.RequestBody
	if body == nil || body.Value == nil || len(body.Value.Content) == 0 {
		return c.buildLegacyBody(fuzz)
	}

	contentType := SelectBodyContentType(body.Value.Content)
	if contentType == "" {
		return "", nil, false
	}

	schema := bodySchema(body.Value, contentType)
	value := c.bodyValue(schema, fuzz)
	encoded, effectiveType, err := encodeBody(contentType, value)
	if err != nil {
		return "", nil, false
	}
	return effectiveType, encoded, true
}

// buildLegacyBody handles a Swagger 2.0 "in: body" parameter that survived when
// conversion to OpenAPI 3 was not possible.
func (c *generationContext) buildLegacyBody(fuzz *FuzzTarget) (string, []byte, bool) {
	for _, param := range c.entry.Parameters {
		if param.In != "body" || param.Schema == nil {
			continue
		}
		value := c.bodyValue(param.Schema, fuzz)
		encoded, effectiveType, err := encodeBody("application/json", value)
		if err != nil {
			return "", nil, false
		}
		return effectiveType, encoded, true
	}
	return "", nil, false
}

// bodyValue builds the body payload, substituting a fuzz value for one property
// when the fuzz target names a body property.
func (c *generationContext) bodyValue(schema *openapi3.SchemaRef, fuzz *FuzzTarget) interface{} {
	if schema == nil || schema.Value == nil {
		return map[string]interface{}{}
	}

	view := ResolveSchema(schema.Value, NewNodeBudget())
	if !view.IsObject() && len(view.Properties) == 0 {
		if fuzz != nil && fuzz.In == "body" && fuzz.Name == "" {
			return fuzz.Value
		}
		return defaultValueFor(schema)
	}

	value := make(map[string]interface{}, len(view.Properties))
	for name, propRef := range view.Properties {
		// A readOnly property is response-only; sending it makes many APIs reject the
		// whole request.
		if propRef != nil && propRef.Value != nil && propRef.Value.ReadOnly {
			continue
		}
		if fuzz != nil && fuzz.In == "body" && fuzz.Name == name {
			value[name] = fuzz.Value
			continue
		}
		value[name] = defaultValueFromMap(schemaToMapBelow(propRef, view.Sources))
	}
	return value
}

// encodeBody returns the encoded body and the content type to send with it. The two
// can differ: a multipart body is only parseable if the header carries the boundary
// the writer used.
func encodeBody(contentType string, value interface{}) ([]byte, string, error) {
	switch {
	case strings.Contains(contentType, "json"):
		body, err := json.Marshal(value)
		return body, contentType, err
	case contentType == "application/x-www-form-urlencoded":
		return []byte(encodeForm(value).Encode()), contentType, nil
	case strings.HasPrefix(contentType, "multipart/"):
		return encodeMultipart(value)
	case strings.Contains(contentType, "xml"):
		body, err := encodeXML(value)
		return body, contentType, err
	case strings.HasPrefix(contentType, "text/"):
		return []byte(serializeScalar(value)), contentType, nil
	default:
		body, err := json.Marshal(value)
		return body, contentType, err
	}
}

func encodeForm(value interface{}) url.Values {
	form := url.Values{}
	fields, ok := value.(map[string]interface{})
	if !ok {
		return form
	}
	for _, name := range sortedKeys(fields) {
		form.Set(name, serializeScalar(fields[name]))
	}
	return form
}

func encodeMultipart(value interface{}) ([]byte, string, error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	if err := writer.SetBoundary(multipartBoundary); err != nil {
		return nil, "", err
	}

	if fields, ok := value.(map[string]interface{}); ok {
		for _, name := range sortedKeys(fields) {
			if err := writer.WriteField(name, serializeScalar(fields[name])); err != nil {
				return nil, "", err
			}
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func encodeXML(value interface{}) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteString("<root>")

	if fields, ok := value.(map[string]interface{}); ok {
		for _, name := range sortedKeys(fields) {
			element := xml.Name{Local: name}
			if err := xml.NewEncoder(&buffer).EncodeElement(serializeScalar(fields[name]), xml.StartElement{Name: element}); err != nil {
				return nil, err
			}
		}
	} else {
		if err := xml.EscapeText(&buffer, []byte(serializeScalar(value))); err != nil {
			return nil, err
		}
	}

	buffer.WriteString("</root>")
	return buffer.Bytes(), nil
}

func sortedKeys(fields map[string]interface{}) []string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (c *generationContext) fuzz() []RequestVariation {
	strategies := []ValueStrategy{&InterestingValuesStrategy{}}
	var requests []RequestVariation

	for _, param := range c.params {
		schema := schemaToMap(param.Schema)
		for _, strategy := range strategies {
			for _, value := range strategy.Generate(schema) {
				req := c.build(&FuzzTarget{Name: param.Name, In: param.In, Value: value.Value})
				req.Label = fmt.Sprintf("Fuzz %s '%s': %s", param.In, param.Name, value.Description)
				requests = append(requests, req)
			}
		}
	}

	for _, target := range c.bodyFuzzTargets() {
		for _, strategy := range strategies {
			for _, value := range strategy.Generate(target.schema) {
				req := c.build(&FuzzTarget{Name: target.name, In: "body", Value: value.Value})
				if target.name == "" {
					req.Label = fmt.Sprintf("Fuzz body: %s", value.Description)
				} else {
					req.Label = fmt.Sprintf("Fuzz body '%s': %s", target.name, value.Description)
				}
				requests = append(requests, req)
			}
		}
	}

	return requests
}

type bodyFuzzTarget struct {
	name   string
	schema map[string]interface{}
}

func (c *generationContext) bodyFuzzTargets() []bodyFuzzTarget {
	schema := c.bodyFuzzSchema()
	if schema == nil || schema.Value == nil {
		return nil
	}

	view := ResolveSchema(schema.Value, NewNodeBudget())
	if !view.IsObject() && len(view.Properties) == 0 {
		return []bodyFuzzTarget{{name: "", schema: schemaToMap(schema)}}
	}

	names := sortedSchemaNames(view.Properties)
	targets := make([]bodyFuzzTarget, 0, len(names))
	for _, name := range names {
		targets = append(targets, bodyFuzzTarget{name: name, schema: schemaToMapBelow(view.Properties[name], view.Sources)})
	}
	return targets
}

func (c *generationContext) bodyFuzzSchema() *openapi3.SchemaRef {
	body := c.entry.Operation.RequestBody
	if body != nil && body.Value != nil && len(body.Value.Content) > 0 {
		return bodySchema(body.Value, SelectBodyContentType(body.Value.Content))
	}
	for _, param := range c.entry.Parameters {
		if param.In == "body" && param.Schema != nil {
			return param.Schema
		}
	}
	return nil
}

func joinPath(base, path string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}

// SecurityApplication holds auth details to be applied to a request
type SecurityApplication struct {
	Headers     map[string]string
	QueryParams map[string]string
	Cookies     map[string]string
}

// applySecurityHeaders adds placeholder authentication material for the schemes an
// operation declares. Scheme names are processed in sorted order so that two
// schemes writing the same header produce a stable result.
func applySecurityHeaders(headers map[string]string, queryParams map[string]string, schemes []SecuritySchemeInfo, opSecurity []string) {
	byName := make(map[string]SecuritySchemeInfo, len(schemes))
	for _, scheme := range schemes {
		byName[scheme.Name] = scheme
	}

	names := append([]string(nil), opSecurity...)
	sort.Strings(names)

	for _, name := range names {
		scheme, ok := byName[name]
		if !ok {
			continue
		}
		switch scheme.Type {
		case "http":
			switch scheme.Scheme {
			case "bearer":
				headers["Authorization"] = "Bearer <TOKEN>"
			case "basic":
				headers["Authorization"] = "Basic <BASE64_CREDENTIALS>"
			case "digest":
				headers["Authorization"] = "Digest <DIGEST_CREDENTIALS>"
			case "hoba":
				headers["Authorization"] = "HOBA <HOBA_CREDENTIALS>"
			case "mutual":
				headers["Authorization"] = "Mutual <MUTUAL_CREDENTIALS>"
			case "negotiate":
				headers["Authorization"] = "Negotiate <NEGOTIATE_CREDENTIALS>"
			case "oauth":
				headers["Authorization"] = "OAuth <OAUTH_CREDENTIALS>"
			case "scram-sha-1":
				headers["Authorization"] = "SCRAM-SHA-1 <SCRAM_CREDENTIALS>"
			case "scram-sha-256":
				headers["Authorization"] = "SCRAM-SHA-256 <SCRAM_CREDENTIALS>"
			case "vapid":
				headers["Authorization"] = "vapid <VAPID_CREDENTIALS>"
			default:
				if scheme.Scheme != "" {
					headers["Authorization"] = scheme.Scheme + " <CREDENTIALS>"
				}
			}
		case "apiKey":
			// A scheme without a parameter name is malformed; inventing one would send a
			// credential the API never reads and mislabel where auth lives.
			keyName := sanitizeHeaderToken(scheme.Header)
			if keyName == "" {
				continue
			}
			switch strings.ToLower(scheme.In) {
			case "header":
				headers[keyName] = "<API_KEY>"
			case "query":
				if queryParams != nil {
					queryParams[keyName] = "<API_KEY>"
				}
			case "cookie":
				existingCookies := headers["Cookie"]
				if existingCookies != "" {
					headers["Cookie"] = existingCookies + "; " + keyName + "=<API_KEY>"
				} else {
					headers["Cookie"] = keyName + "=<API_KEY>"
				}
			}
		case "oauth2", "openIdConnect":
			headers["Authorization"] = "Bearer <ACCESS_TOKEN>"
		}
	}
}
