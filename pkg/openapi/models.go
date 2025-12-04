package openapi

// Endpoint represents a single API endpoint (Method + Path) and its generated requests
type Endpoint struct {
	Method      string              `json:"method"`
	Path        string              `json:"path"`
	OperationID string              `json:"operation_id,omitempty"`
	Summary     string              `json:"summary,omitempty"`
	Description string              `json:"description,omitempty"`
	Parameters  []ParameterMetadata `json:"parameters,omitempty"`
	Requests    []RequestVariation  `json:"requests"`
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
