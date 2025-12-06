package openapi

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// GenerateRequests generates endpoints and their request variations
func GenerateRequests(doc *Document, config GenerationConfig) ([]Endpoint, error) {
	var endpoints []Endpoint

	// Extract security schemes for auth header generation
	securitySchemes := doc.GetSecuritySchemes()
	globalSecurity := doc.GetGlobalSecurity()

	ops := doc.GetOperations()
	for path, methods := range ops {
		for method, op := range methods {
			endpoint := Endpoint{
				Method:      method,
				Path:        path,
				OperationID: op.OperationID,
				Summary:     op.Summary,
				Description: op.Description,
				Requests:    []RequestVariation{},
			}

			// Extract parameters metadata
			for _, paramRef := range op.Parameters {
				if paramRef.Value == nil {
					continue
				}
				param := paramRef.Value
				endpoint.Parameters = append(endpoint.Parameters, ParameterMetadata{
					Name:     param.Name,
					In:       param.In,
					Required: param.Required,
					Schema:   schemaToMap(param.Schema),
				})
			}

			// Determine which security schemes apply to this operation
			opSecurity := globalSecurity
			if op.Security != nil && len(*op.Security) > 0 {
				opSecurity = nil
				for _, req := range *op.Security {
					for name := range req {
						opSecurity = append(opSecurity, name)
					}
				}
			}

			// Generate variations
			seenRequests := make(map[string]bool)
			var uniqueRequests []RequestVariation

			addRequest := func(req RequestVariation) {
				sig := getRequestSignature(req)
				if !seenRequests[sig] {
					seenRequests[sig] = true
					uniqueRequests = append(uniqueRequests, req)
				}
			}

			// 1. Happy Path (Default values for everything)
			happyRequest := generateRequest(path, method, op, config, nil, securitySchemes, opSecurity)
			happyRequest.Label = "Happy Path"
			addRequest(happyRequest)

			// 2. Fuzzing (if enabled)
			if config.FuzzingEnabled {
				fuzzRequests := generateFuzzRequests(path, method, op, config, securitySchemes, opSecurity)
				for _, req := range fuzzRequests {
					addRequest(req)
				}
			}

			endpoint.Requests = uniqueRequests
			endpoints = append(endpoints, endpoint)
		}
	}

	return endpoints, nil
}

func getRequestSignature(req RequestVariation) string {
	// Create a unique signature based on URL, Headers, and Body
	// Headers need to be sorted to ensure consistency
	var headerKeys []string
	for k := range req.Headers {
		headerKeys = append(headerKeys, k)
	}
	// Simple bubble sort for small number of headers is fine
	for i := 0; i < len(headerKeys)-1; i++ {
		for j := 0; j < len(headerKeys)-i-1; j++ {
			if headerKeys[j] > headerKeys[j+1] {
				headerKeys[j], headerKeys[j+1] = headerKeys[j+1], headerKeys[j]
			}
		}
	}

	var headerSig strings.Builder
	for _, k := range headerKeys {
		headerSig.WriteString(k)
		headerSig.WriteString(":")
		headerSig.WriteString(req.Headers[k])
		headerSig.WriteString(";")
	}

	return fmt.Sprintf("%s|%s|%s", req.URL, headerSig.String(), string(req.Body))
}

func generateRequest(path, method string, op *openapi3.Operation, config GenerationConfig, fuzzParam *FuzzTarget, securitySchemes []SecuritySchemeInfo, opSecurity []string) RequestVariation {
	req := RequestVariation{
		Headers: make(map[string]string),
	}

	// Base URL
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost"
	}

	// Parse URL to handle query params
	u, _ := url.Parse(baseURL)
	u.Path = joinPath(u.Path, path)

	queryParams := u.Query()

	// Create a map for security query params (will be merged later)
	securityQueryParams := make(map[string]string)

	// Apply authentication based on security schemes
	applySecurityHeaders(req.Headers, securityQueryParams, securitySchemes, opSecurity)

	// Merge security query params into the main query params
	for key, value := range securityQueryParams {
		queryParams.Set(key, value)
	}

	// Handle Parameters
	for _, paramRef := range op.Parameters {
		if paramRef.Value == nil {
			continue
		}
		param := paramRef.Value

		// Determine value
		var value interface{}
		if fuzzParam != nil && fuzzParam.Name == param.Name && fuzzParam.In == param.In {
			value = fuzzParam.Value
		} else {
			// Use default strategy
			strat := &DefaultValueStrategy{}
			vals := strat.Generate(schemaToMap(param.Schema))
			if len(vals) > 0 {
				value = vals[0].Value
			}
		}

		strVal := fmt.Sprintf("%v", value)

		switch param.In {
		case "path":
			u.Path = strings.ReplaceAll(u.Path, "{"+param.Name+"}", strVal)
		case "query":
			queryParams.Set(param.Name, strVal)
		case "header":
			req.Headers[param.Name] = strVal
		case "cookie":
			existingCookies := req.Headers["Cookie"]
			newCookie := fmt.Sprintf("%s=%s", param.Name, strVal)
			if existingCookies != "" {
				req.Headers["Cookie"] = existingCookies + "; " + newCookie
			} else {
				req.Headers["Cookie"] = newCookie
			}
		case "body":
			// Handle Swagger 2.0 style body parameter
			// If it's an object, we might need to construct it
			if param.Schema != nil && param.Schema.Value != nil {
				s := param.Schema.Value
				// If we have a fuzz target for a property of this body
				if fuzzParam != nil && fuzzParam.In == "body" && fuzzParam.Name != param.Name {
					// This implies we are fuzzing a property inside this body object
					// We need to reconstruct the object with defaults + fuzz value
					bodyMap := make(map[string]interface{})
					for propName, propSchemaRef := range s.Properties {
						if propSchemaRef.Value == nil {
							continue
						}
						var val interface{}
						if fuzzParam.Name == propName {
							val = fuzzParam.Value
						} else {
							strat := &DefaultValueStrategy{}
							vals := strat.Generate(schemaToMap(propSchemaRef))
							if len(vals) > 0 {
								val = vals[0].Value
							}
						}
						bodyMap[propName] = val
					}
					jsonBody, _ := json.Marshal(bodyMap)
					req.Body = jsonBody
				} else {
					// Just use the value (which might be the object itself if we generated it that way,
					// but generateDefaultValue for object returns empty map currently)
					// If value is a map, marshal it.
					if _, ok := value.(map[string]interface{}); ok {
						jsonBody, _ := json.Marshal(value)
						req.Body = jsonBody
					} else {
						req.Body = []byte(strVal)
					}
				}
				req.Headers["Content-Type"] = "application/json"
			}
		}
	}

	u.RawQuery = queryParams.Encode()
	req.URL = u.String()

	// Handle Body
	if op.RequestBody != nil && op.RequestBody.Value != nil {
		content := op.RequestBody.Value.Content
		// Prioritize JSON
		if mediaType, ok := content["application/json"]; ok {
			req.Headers["Content-Type"] = "application/json"
			bodyMap := make(map[string]interface{})

			if mediaType.Schema != nil && mediaType.Schema.Value != nil {
				schema := mediaType.Schema.Value
				for propName, propSchemaRef := range schema.Properties {
					if propSchemaRef.Value == nil {
						continue
					}

					var val interface{}
					// Check if we are fuzzing this body property
					if fuzzParam != nil && fuzzParam.In == "body" && fuzzParam.Name == propName {
						val = fuzzParam.Value
					} else {
						strat := &DefaultValueStrategy{}
						vals := strat.Generate(schemaToMap(propSchemaRef))
						if len(vals) > 0 {
							val = vals[0].Value
						}
					}
					bodyMap[propName] = val
				}
			}
			jsonBody, _ := json.Marshal(bodyMap)
			req.Body = jsonBody
		}
	}

	return req
}

type FuzzTarget struct {
	Name  string
	In    string
	Value interface{}
}

func generateFuzzRequests(path, method string, op *openapi3.Operation, config GenerationConfig, securitySchemes []SecuritySchemeInfo, opSecurity []string) []RequestVariation {
	var requests []RequestVariation

	// Use default strategies
	strategies := []ValueStrategy{&InterestingValuesStrategy{}}

	// Fuzz Parameters
	for _, paramRef := range op.Parameters {
		if paramRef.Value == nil {
			continue
		}
		param := paramRef.Value
		schema := schemaToMap(param.Schema)

		for _, strategy := range strategies {
			// Special handling for body objects to fuzz their properties
			if param.In == "body" {
				if props, ok := schema["properties"].(map[string]interface{}); ok {
					for propName, propSchema := range props {
						if propMap, ok := propSchema.(map[string]interface{}); ok {
							propValues := strategy.Generate(propMap)
							for _, val := range propValues {
								target := &FuzzTarget{
									Name:  propName, // Target the property name
									In:    "body",
									Value: val.Value,
								}
								req := generateRequest(path, method, op, config, target, securitySchemes, opSecurity)
								req.Label = fmt.Sprintf("Fuzz body '%s': %s", propName, val.Description)
								requests = append(requests, req)
							}
						}
					}
					continue // Skip fuzzing the object itself as a whole for now
				}
			}

			values := strategy.Generate(schema)
			for _, val := range values {
				// Skip default values in fuzzing to avoid duplicates with Happy Path if desired,
				// but InterestingStrategy includes default as baseline, so maybe keep it or filter.
				// For now, we include everything.

				target := &FuzzTarget{
					Name:  param.Name,
					In:    param.In,
					Value: val.Value,
				}

				req := generateRequest(path, method, op, config, target, securitySchemes, opSecurity)
				req.Label = fmt.Sprintf("Fuzz %s '%s': %s", param.In, param.Name, val.Description)
				requests = append(requests, req)
			}
		}
	}

	// Fuzz Body Properties (JSON only for now)
	if op.RequestBody != nil && op.RequestBody.Value != nil {
		content := op.RequestBody.Value.Content
		if mediaType, ok := content["application/json"]; ok {
			if mediaType.Schema != nil && mediaType.Schema.Value != nil {
				schema := mediaType.Schema.Value
				for propName, propSchemaRef := range schema.Properties {
					if propSchemaRef.Value == nil {
						continue
					}
					propSchema := schemaToMap(propSchemaRef)

					for _, strategy := range strategies {
						values := strategy.Generate(propSchema)
						for _, val := range values {
							target := &FuzzTarget{
								Name:  propName,
								In:    "body",
								Value: val.Value,
							}
							req := generateRequest(path, method, op, config, target, securitySchemes, opSecurity)
							req.Label = fmt.Sprintf("Fuzz body '%s': %s", propName, val.Description)
							requests = append(requests, req)
						}
					}
				}
			}
		}
	}

	return requests
}

func schemaToMap(schemaRef *openapi3.SchemaRef) map[string]interface{} {
	if schemaRef == nil || schemaRef.Value == nil {
		return nil
	}
	s := schemaRef.Value
	m := make(map[string]interface{})
	if s.Type != nil {
		types := s.Type.Slice()
		if len(types) > 0 {
			m["type"] = types[0]
		}
	}
	m["format"] = s.Format
	m["example"] = s.Example
	m["default"] = s.Default

	if len(s.Properties) > 0 {
		props := make(map[string]interface{})
		for k, v := range s.Properties {
			props[k] = schemaToMap(v)
		}
		m["properties"] = props
	}

	if s.Items != nil {
		m["items"] = schemaToMap(s.Items)
	}

	return m
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

// applySecurityHeaders adds authentication headers based on security schemes
// Returns headers, query params, and cookies that should be applied
func applySecurityHeaders(headers map[string]string, queryParams map[string]string, schemes []SecuritySchemeInfo, opSecurity []string) {
	for _, schemeName := range opSecurity {
		for _, scheme := range schemes {
			if scheme.Name != schemeName {
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
					// For any other HTTP auth scheme, use a generic format
					if scheme.Scheme != "" {
						headers["Authorization"] = scheme.Scheme + " <CREDENTIALS>"
					}
				}
			case "apiKey":
				keyName := scheme.Header
				if keyName == "" {
					keyName = "X-API-Key"
				}
				switch scheme.In {
				case "header":
					headers[keyName] = "<API_KEY>"
				case "query":
					if queryParams != nil {
						queryParams[keyName] = "<API_KEY>"
					}
				case "cookie":
					// For cookies, we add a Cookie header
					// Note: Multiple cookies would need more sophisticated handling
					existingCookies := headers["Cookie"]
					if existingCookies != "" {
						headers["Cookie"] = existingCookies + "; " + keyName + "=<API_KEY>"
					} else {
						headers["Cookie"] = keyName + "=<API_KEY>"
					}
				}
			case "oauth2", "openIdConnect":
				headers["Authorization"] = "Bearer <ACCESS_TOKEN>"
			case "mutualTLS":
				// mutualTLS requires client certificate, not a header
				// We can add a comment header to indicate this
				headers["X-Auth-Note"] = "Requires mutual TLS client certificate"
			}
		}
	}
}
