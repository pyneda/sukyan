package openapi

import (
	"net/url"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// Document wraps the openapi3.T struct
type Document struct {
	spec      *openapi3.T
	sourceURL string
}

// OperationEntry is a single path/method pair together with the parameters that
// actually apply to it.
type OperationEntry struct {
	Path      string
	Method    string
	Operation *openapi3.Operation
	PathItem  *openapi3.PathItem
	// Parameters merges the path item's shared parameters with the operation's own.
	// The spec says an operation-level parameter overrides a path-level one with the
	// same name and location.
	Parameters []*openapi3.Parameter
}

// Operations returns every operation in the document, ordered by path then method
// so that repeated runs over the same spec produce identical output.
func (d *Document) Operations() []OperationEntry {
	if d == nil || d.spec == nil || d.spec.Paths == nil {
		return nil
	}

	paths := d.spec.Paths.Map()
	sortedPaths := make([]string, 0, len(paths))
	for path := range paths {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)

	var entries []OperationEntry
	for _, path := range sortedPaths {
		pathItem := paths[path]
		if pathItem == nil {
			continue
		}
		operations := pathItem.Operations()
		methods := make([]string, 0, len(operations))
		for method := range operations {
			methods = append(methods, method)
		}
		sort.Strings(methods)

		for _, method := range methods {
			op := operations[method]
			if op == nil {
				continue
			}
			entries = append(entries, OperationEntry{
				Path:       path,
				Method:     method,
				Operation:  op,
				PathItem:   pathItem,
				Parameters: effectiveParameters(pathItem, op),
			})
		}
	}
	return entries
}

// GetOperations returns all operations in the document keyed by path and method.
func (d *Document) GetOperations() map[string]map[string]*openapi3.Operation {
	ops := make(map[string]map[string]*openapi3.Operation)
	for _, entry := range d.Operations() {
		if ops[entry.Path] == nil {
			ops[entry.Path] = make(map[string]*openapi3.Operation)
		}
		ops[entry.Path][entry.Method] = entry.Operation
	}
	return ops
}

func effectiveParameters(pathItem *openapi3.PathItem, op *openapi3.Operation) []*openapi3.Parameter {
	type key struct{ name, in string }

	index := make(map[key]int)
	var params []*openapi3.Parameter

	add := func(refs openapi3.Parameters) {
		for _, ref := range refs {
			if ref == nil || ref.Value == nil || ref.Value.Name == "" {
				continue
			}
			k := key{ref.Value.Name, ref.Value.In}
			if position, ok := index[k]; ok {
				params[position] = ref.Value
				continue
			}
			index[k] = len(params)
			params = append(params, ref.Value)
		}
	}

	if pathItem != nil {
		add(pathItem.Parameters)
	}
	if op != nil {
		add(op.Parameters)
	}
	return params
}

// Servers returns every declared server URL with its variables substituted and
// relative URLs resolved against the document source when it is known.
func (d *Document) Servers() []string {
	if d == nil || d.spec == nil {
		return nil
	}
	servers := make([]string, 0, len(d.spec.Servers))
	for _, server := range d.spec.Servers {
		expanded := expandServerURL(server)
		if expanded == "" {
			continue
		}
		servers = append(servers, d.resolveURL(expanded))
	}
	return servers
}

// BaseURL returns the first absolute server URL, or an empty string when the
// document declares none. Server variables are substituted with their defaults and
// relative URLs (such as "/api/v3") are resolved against the source URL, so callers
// never receive a value that cannot be requested.
func (d *Document) BaseURL() string {
	for _, server := range d.Servers() {
		if parsed, err := url.Parse(server); err == nil && parsed.IsAbs() && parsed.Host != "" {
			return server
		}
	}
	return ""
}

// SourceURL returns the location the document was retrieved from, if provided.
func (d *Document) SourceURL() string {
	if d == nil {
		return ""
	}
	return d.sourceURL
}

// Spec exposes the loaded document for packages that build their own model from it,
// so that Swagger 2.0 conversion, OpenAPI 3.1 normalisation and the external
// reference policy are applied in one place rather than reimplemented per consumer.
func (d *Document) Spec() *openapi3.T {
	if d == nil {
		return nil
	}
	return d.spec
}

func expandServerURL(server *openapi3.Server) string {
	if server == nil {
		return ""
	}
	expanded := server.URL
	for name, variable := range server.Variables {
		if variable == nil {
			continue
		}
		value := variable.Default
		if value == "" && len(variable.Enum) > 0 {
			value = variable.Enum[0]
		}
		if value == "" {
			continue
		}
		expanded = strings.ReplaceAll(expanded, "{"+name+"}", value)
	}
	return expanded
}

func (d *Document) resolveURL(raw string) string {
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if ref.IsAbs() {
		return raw
	}
	base, err := url.Parse(d.sourceURL)
	if err != nil || !base.IsAbs() {
		return raw
	}
	return base.ResolveReference(ref).String()
}

// SecuritySchemeInfo contains extracted security scheme information (internal use)
type SecuritySchemeInfo struct {
	Name   string // Scheme name (e.g., "bearerAuth")
	Type   string // http, apiKey, oauth2, openIdConnect
	Scheme string // bearer, basic (for http type)
	In     string // header, query, cookie (for apiKey type)
	Header string // Header name (for apiKey type)
}

// GetSecuritySchemes returns the security schemes defined in the spec, ordered by
// name. Swagger 2.0 documents are converted on load, so securityDefinitions
// normally arrive as components.securitySchemes; the extension fallback covers
// documents whose conversion failed.
func (d *Document) GetSecuritySchemes() []SecurityScheme {
	if d == nil || d.spec == nil {
		return nil
	}

	var schemes []SecurityScheme

	if d.spec.Components != nil && d.spec.Components.SecuritySchemes != nil {
		for name, schemeRef := range d.spec.Components.SecuritySchemes {
			if schemeRef == nil || schemeRef.Value == nil {
				continue
			}
			scheme := schemeRef.Value
			schemes = append(schemes, SecurityScheme{
				Name:             name,
				Type:             scheme.Type,
				Scheme:           scheme.Scheme,
				In:               scheme.In,
				ParameterName:    scheme.Name,
				BearerFormat:     scheme.BearerFormat,
				Description:      scheme.Description,
				OpenIDConnectURL: scheme.OpenIdConnectUrl,
			})
		}
	}

	if len(schemes) == 0 && d.spec.Extensions != nil {
		schemes = append(schemes, securitySchemesFromExtensions(d.spec.Extensions)...)
	}

	sort.Slice(schemes, func(i, j int) bool { return schemes[i].Name < schemes[j].Name })
	return schemes
}

func securitySchemesFromExtensions(extensions map[string]interface{}) []SecurityScheme {
	definitions, ok := extensions["securityDefinitions"].(map[string]interface{})
	if !ok {
		return nil
	}

	var schemes []SecurityScheme
	for name, definition := range definitions {
		fields, ok := definition.(map[string]interface{})
		if !ok {
			continue
		}
		scheme := SecurityScheme{Name: name}
		if value, ok := fields["type"].(string); ok {
			scheme.Type = value
		}
		if value, ok := fields["scheme"].(string); ok {
			scheme.Scheme = value
		}
		if value, ok := fields["in"].(string); ok {
			scheme.In = value
		}
		if value, ok := fields["name"].(string); ok {
			scheme.ParameterName = value
		}
		if value, ok := fields["description"].(string); ok {
			scheme.Description = value
		}
		schemes = append(schemes, scheme)
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
	if d == nil || d.spec == nil {
		return nil
	}
	return convertSecurityRequirements(d.spec.Security)
}

// GetOperationSecurityRequirements returns the security requirements for an
// operation. The second return value reports whether the operation overrides the
// document-level requirements; an operation declaring "security: []" overrides them
// with no requirement at all, which is how a spec marks an endpoint as public.
func (d *Document) GetOperationSecurityRequirements(op *openapi3.Operation) ([]SecurityRequirement, bool) {
	if op == nil || op.Security == nil {
		return nil, false
	}
	return convertSecurityRequirements(*op.Security), true
}

// convertSecurityRequirements converts OpenAPI security requirements to our model.
// An empty requirement object ({}) is preserved as an alternative with no schemes,
// which is how a spec says authentication is optional.
func convertSecurityRequirements(reqs openapi3.SecurityRequirements) []SecurityRequirement {
	result := make([]SecurityRequirement, 0, len(reqs))

	for _, req := range reqs {
		names := make([]string, 0, len(req))
		for schemeName := range req {
			names = append(names, schemeName)
		}
		sort.Strings(names)

		secReq := SecurityRequirement{Schemes: make([]SecuritySchemeRef, 0, len(names))}
		for _, schemeName := range names {
			secReq.Schemes = append(secReq.Schemes, SecuritySchemeRef{
				Name:   schemeName,
				Scopes: req[schemeName],
			})
		}
		result = append(result, secReq)
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// GetGlobalSecurity returns the global security scheme names as a flat, sorted list.
func (d *Document) GetGlobalSecurity() []string {
	if d == nil || d.spec == nil {
		return nil
	}
	return flattenSecurityNames(d.spec.Security)
}

func flattenSecurityNames(reqs openapi3.SecurityRequirements) []string {
	seen := make(map[string]bool)
	var names []string
	for _, req := range reqs {
		for name := range req {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}
