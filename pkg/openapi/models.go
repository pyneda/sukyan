package openapi

// Endpoint represents a single API endpoint (Method + Path) and its generated requests
type Endpoint struct {
	Method      string                `json:"method"`
	Path        string                `json:"path"`
	OperationID string                `json:"operation_id,omitempty"`
	Summary     string                `json:"summary,omitempty"`
	Description string                `json:"description,omitempty"`
	Parameters  []ParameterMetadata   `json:"parameters,omitempty"`
	Security    []SecurityRequirement `json:"security,omitempty"` // Security requirements for this endpoint
	Requests    []RequestVariation    `json:"requests"`
}

// SecurityRequirement represents one valid authentication option.
// All schemes in the Schemes slice must be used together (AND relationship).
// Multiple SecurityRequirements in an array represent alternatives (OR relationship).
type SecurityRequirement struct {
	Schemes []SecuritySchemeRef `json:"schemes"`
}

// SecuritySchemeRef references a security scheme with its scopes
type SecuritySchemeRef struct {
	Name   string   `json:"name"`             // Reference to SecurityScheme.Name
	Scopes []string `json:"scopes,omitempty"` // OAuth2 scopes if applicable
}

// SecurityScheme defines an authentication method available in the API
type SecurityScheme struct {
	Name             string `json:"name"`                         // Scheme identifier (e.g., "bearerAuth")
	Type             string `json:"type"`                         // http, apiKey, oauth2, openIdConnect, mutualTLS
	Scheme           string `json:"scheme,omitempty"`             // For http type: bearer, basic, digest, etc.
	In               string `json:"in,omitempty"`                 // For apiKey: header, query, cookie
	ParameterName    string `json:"parameter_name,omitempty"`     // Header/query/cookie name for apiKey
	BearerFormat     string `json:"bearer_format,omitempty"`      // Hint about token format (e.g., "JWT")
	Description      string `json:"description,omitempty"`        // Human-readable description
	OpenIDConnectURL string `json:"openid_connect_url,omitempty"` // For openIdConnect type
}

// ParameterMetadata describes a parameter for the endpoint
type ParameterMetadata struct {
	Name     string                 `json:"name"`
	In       string                 `json:"in"` // query, header, path, cookie
	Required bool                   `json:"required"`
	Schema   map[string]interface{} `json:"schema,omitempty"` // Simplified JSON schema details
}

// RequestVariation represents a specific generated request for an endpoint
type RequestVariation struct {
	Label       string            `json:"label"` // e.g., "Happy Path", "SQLi in 'id'", "Boundary 'limit'"
	URL         string            `json:"url"`   // Full URL including query params
	Headers     map[string]string `json:"headers,omitempty"`
	Body        []byte            `json:"body,omitempty"`
	Description string            `json:"description,omitempty"`
}

// GenerationConfig controls how requests are generated
type GenerationConfig struct {
	BaseURL               string
	IncludeOptionalParams bool
	FuzzingEnabled        bool
	// Strategies can be passed here if we want to allow custom ones,
	// but for serialization purposes we might want to keep this simple or use a functional option pattern elsewhere.
}

// ValueStrategy defines how to generate values for parameters
type ValueStrategy interface {
	// Generate returns a list of values and a description for each value (e.g. "boundary max")
	Generate(schema map[string]interface{}) []GeneratedValue
}

type GeneratedValue struct {
	Value       interface{}
	Description string
}
