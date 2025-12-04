package openapi

import "github.com/getkin/kin-openapi/openapi3"

// Document wraps the openapi3.T struct
type Document struct {
	spec *openapi3.T
}

// GetOperations returns all operations in the document
func (d *Document) GetOperations() map[string]map[string]*openapi3.Operation {
	ops := make(map[string]map[string]*openapi3.Operation)

	for path, pathItem := range d.spec.Paths.Map() {
		ops[path] = make(map[string]*openapi3.Operation)
		for method, op := range pathItem.Operations() {
			ops[path][method] = op
		}
	}
	return ops
}

// BaseURL attempts to determine the base URL from the servers list
func (d *Document) BaseURL() string {
	if len(d.spec.Servers) > 0 {
		return d.spec.Servers[0].URL
	}
	return ""
}

// SecuritySchemeInfo contains extracted security scheme information
type SecuritySchemeInfo struct {
	Name   string // Scheme name (e.g., "bearerAuth")
	Type   string // http, apiKey, oauth2, openIdConnect
	Scheme string // bearer, basic (for http type)
	In     string // header, query, cookie (for apiKey type)
	Header string // Header name (for apiKey type)
}

// GetSecuritySchemes returns the security schemes defined in the spec
// Supports both OpenAPI 3.0 (components.securitySchemes) and Swagger 2.0 (securityDefinitions)
func (d *Document) GetSecuritySchemes() []SecuritySchemeInfo {
	var schemes []SecuritySchemeInfo

	// Try OpenAPI 3.0 format first (components.securitySchemes)
	if d.spec.Components != nil && d.spec.Components.SecuritySchemes != nil {
		for name, schemeRef := range d.spec.Components.SecuritySchemes {
			if schemeRef.Value == nil {
				continue
			}
			scheme := schemeRef.Value
			info := SecuritySchemeInfo{
				Name:   name,
				Type:   scheme.Type,
				Scheme: scheme.Scheme,
				In:     scheme.In,
				Header: scheme.Name,
			}
			schemes = append(schemes, info)
		}
	}

	// If no schemes found, try Swagger 2.0 format (securityDefinitions in Extensions)
	if len(schemes) == 0 && d.spec.Extensions != nil {
		if secDefs, ok := d.spec.Extensions["securityDefinitions"]; ok {
			// secDefs is typically a map[string]interface{}
			if secDefsMap, ok := secDefs.(map[string]interface{}); ok {
				for name, def := range secDefsMap {
					if defMap, ok := def.(map[string]interface{}); ok {
						info := SecuritySchemeInfo{
							Name: name,
						}

						if t, ok := defMap["type"].(string); ok {
							info.Type = t
						}
						if s, ok := defMap["scheme"].(string); ok {
							info.Scheme = s
						}
						if in, ok := defMap["in"].(string); ok {
							info.In = in
						}
						if n, ok := defMap["name"].(string); ok {
							info.Header = n
						}

						schemes = append(schemes, info)
					}
				}
			}
		}
	}

	return schemes
}

// GetGlobalSecurity returns the global security requirements
func (d *Document) GetGlobalSecurity() []string {
	var securityNames []string
	for _, req := range d.spec.Security {
		for name := range req {
			securityNames = append(securityNames, name)
		}
	}
	return securityNames
}
