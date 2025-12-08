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

// SecuritySchemeInfo contains extracted security scheme information (internal use)
type SecuritySchemeInfo struct {
	Name   string // Scheme name (e.g., "bearerAuth")
	Type   string // http, apiKey, oauth2, openIdConnect
	Scheme string // bearer, basic (for http type)
	In     string // header, query, cookie (for apiKey type)
	Header string // Header name (for apiKey type)
}

// GetSecuritySchemes returns the security schemes defined in the spec as SecurityScheme structs
// Supports both OpenAPI 3.0 (components.securitySchemes) and Swagger 2.0 (securityDefinitions)
func (d *Document) GetSecuritySchemes() []SecurityScheme {
	var schemes []SecurityScheme

	// Try OpenAPI 3.0 format first (components.securitySchemes)
	if d.spec.Components != nil && d.spec.Components.SecuritySchemes != nil {
		for name, schemeRef := range d.spec.Components.SecuritySchemes {
			if schemeRef.Value == nil {
				continue
			}
			scheme := schemeRef.Value
			info := SecurityScheme{
				Name:          name,
				Type:          scheme.Type,
				Scheme:        scheme.Scheme,
				In:            scheme.In,
				ParameterName: scheme.Name,
				BearerFormat:  scheme.BearerFormat,
				Description:   scheme.Description,
			}
			if scheme.OpenIdConnectUrl != "" {
				info.OpenIDConnectURL = scheme.OpenIdConnectUrl
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
						info := SecurityScheme{
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
							info.ParameterName = n
						}
						if desc, ok := defMap["description"].(string); ok {
							info.Description = desc
						}

						schemes = append(schemes, info)
					}
				}
			}
		}
	}

	return schemes
}

// GetSecuritySchemesLegacy returns security schemes in the legacy format for internal use
func (d *Document) GetSecuritySchemesLegacy() []SecuritySchemeInfo {
	var schemes []SecuritySchemeInfo
	for _, s := range d.GetSecuritySchemes() {
		schemes = append(schemes, SecuritySchemeInfo{
			Name:   s.Name,
			Type:   s.Type,
			Scheme: s.Scheme,
			In:     s.In,
			Header: s.ParameterName,
		})
	}
	return schemes
}

// GetGlobalSecurityRequirements returns the global security requirements with proper OR/AND structure
// Each SecurityRequirement in the returned slice is an alternative (OR relationship)
// Each SecuritySchemeRef within a SecurityRequirement must all be satisfied (AND relationship)
func (d *Document) GetGlobalSecurityRequirements() []SecurityRequirement {
	return convertSecurityRequirements(d.spec.Security)
}

// GetOperationSecurityRequirements returns security requirements for a specific operation
// If the operation has its own security defined, it overrides global security
// If operation security is empty array, it means no auth required (overrides global)
// If operation security is nil, global security applies
func (d *Document) GetOperationSecurityRequirements(op *openapi3.Operation) ([]SecurityRequirement, bool) {
	if op.Security == nil {
		// No override, use global
		return nil, false
	}

	// Operation has its own security (even if empty - which means no auth required)
	return convertSecurityRequirements(*op.Security), true
}

// convertSecurityRequirements converts OpenAPI security requirements to our model
func convertSecurityRequirements(reqs openapi3.SecurityRequirements) []SecurityRequirement {
	var result []SecurityRequirement

	for _, req := range reqs {
		// Each req is a map[string][]string where:
		// - key is the security scheme name
		// - value is the list of scopes (for OAuth2)
		// All entries in one req must be satisfied together (AND)
		secReq := SecurityRequirement{
			Schemes: make([]SecuritySchemeRef, 0, len(req)),
		}

		for schemeName, scopes := range req {
			secReq.Schemes = append(secReq.Schemes, SecuritySchemeRef{
				Name:   schemeName,
				Scopes: scopes,
			})
		}

		if len(secReq.Schemes) > 0 {
			result = append(result, secReq)
		}
	}

	return result
}

// GetGlobalSecurity returns the global security requirements as flat list (legacy, for backward compat)
func (d *Document) GetGlobalSecurity() []string {
	var securityNames []string
	for _, req := range d.spec.Security {
		for name := range req {
			securityNames = append(securityNames, name)
		}
	}
	return securityNames
}
