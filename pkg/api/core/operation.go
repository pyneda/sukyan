package core

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type Operation struct {
	ID           uuid.UUID   `json:"id"`
	DefinitionID uuid.UUID   `json:"definition_id"`
	APIType      APIType     `json:"api_type"`
	Name         string      `json:"name"`
	Method       string      `json:"method"`
	Path         string      `json:"path"`
	BaseURL      string      `json:"base_url"`
	Parameters   []Parameter `json:"parameters"`
	Summary      string      `json:"summary,omitempty"`
	Description  string      `json:"description,omitempty"`
	OperationID  string      `json:"operation_id,omitempty"`
	Tags         []string    `json:"tags,omitempty"`
	Deprecated   bool        `json:"deprecated,omitempty"`
	Security     []SecurityRequirement `json:"security,omitempty"`
	ContentTypes RequestContentTypes   `json:"content_types,omitempty"`

	OpenAPI  *OpenAPIMetadata  `json:"openapi,omitempty"`
	GraphQL  *GraphQLMetadata  `json:"graphql,omitempty"`
	SOAP     *SOAPMetadata     `json:"soap,omitempty"`
}

type SecurityRequirement struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Scopes []string `json:"scopes,omitempty"`
}

type RequestContentTypes struct {
	Request  []string `json:"request,omitempty"`
	Response []string `json:"response,omitempty"`
}

type OpenAPIMetadata struct {
	Version       string   `json:"version,omitempty"`
	RequestBody   *RequestBodyInfo `json:"request_body,omitempty"`
	Servers       []string `json:"servers,omitempty"`
	ExternalDocs  string   `json:"external_docs,omitempty"`
}

type RequestBodyInfo struct {
	Required    bool     `json:"required,omitempty"`
	Description string   `json:"description,omitempty"`
	ContentType string   `json:"content_type,omitempty"`
	Schema      any      `json:"schema,omitempty"`
}

type GraphQLMetadata struct {
	OperationType  string `json:"operation_type"`
	ReturnType     string `json:"return_type,omitempty"`
	IsDeprecated   bool   `json:"is_deprecated,omitempty"`
	TypeName       string `json:"type_name,omitempty"`
}

type SOAPMetadata struct {
	ServiceName    string `json:"service_name,omitempty"`
	PortName       string `json:"port_name,omitempty"`
	SOAPAction     string `json:"soap_action,omitempty"`
	BindingStyle   string `json:"binding_style,omitempty"`
	SOAPVersion    string `json:"soap_version,omitempty"`
	TargetNS       string `json:"target_ns,omitempty"`
	InputMessage   string `json:"input_message,omitempty"`
	OutputMessage  string `json:"output_message,omitempty"`
}

func (o Operation) String() string {
	return fmt.Sprintf("%s %s %s", o.APIType, o.Method, o.Path)
}

func (o Operation) FullURL() string {
	baseURL := strings.TrimSuffix(o.BaseURL, "/")
	path := o.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return baseURL + path
}

func (o Operation) GetParameterSet() *ParameterSet {
	return NewParameterSet(o.Parameters...)
}

func (o Operation) HasPathParameters() bool {
	for _, p := range o.Parameters {
		if p.Location == ParameterLocationPath {
			return true
		}
	}
	return false
}

func (o Operation) HasQueryParameters() bool {
	for _, p := range o.Parameters {
		if p.Location == ParameterLocationQuery {
			return true
		}
	}
	return false
}

func (o Operation) HasBodyParameters() bool {
	for _, p := range o.Parameters {
		if p.Location == ParameterLocationBody {
			return true
		}
	}
	return false
}

func (o Operation) HasRequiredParameters() bool {
	for _, p := range o.Parameters {
		if p.Required {
			return true
		}
	}
	return false
}

func (o Operation) GetRequiredParameters() []Parameter {
	var result []Parameter
	for _, p := range o.Parameters {
		if p.Required {
			result = append(result, p)
		}
	}
	return result
}

func (o Operation) GetParametersWithConstraints() []Parameter {
	var result []Parameter
	for _, p := range o.Parameters {
		if p.HasConstraints() {
			result = append(result, p)
		}
	}
	return result
}

func (o Operation) IsRESTful() bool {
	return o.APIType == APITypeOpenAPI
}

func (o Operation) IsGraphQL() bool {
	return o.APIType == APITypeGraphQL
}

func (o Operation) IsSOAP() bool {
	return o.APIType == APITypeSOAP
}

func (o Operation) HasConstraints() bool {
	for _, p := range o.Parameters {
		if p.HasConstraints() {
			return true
		}
	}
	return false
}

func (o Operation) SupportsMethod(method string) bool {
	return strings.EqualFold(o.Method, method)
}

type OperationSet struct {
	Operations []Operation
	APIType    APIType
	BaseURL    string
}

func NewOperationSet(apiType APIType, baseURL string) *OperationSet {
	return &OperationSet{
		APIType: apiType,
		BaseURL: baseURL,
	}
}

func (os *OperationSet) Add(op Operation) {
	os.Operations = append(os.Operations, op)
}

func (os *OperationSet) GetByPath(path string) []Operation {
	var result []Operation
	for _, op := range os.Operations {
		if op.Path == path {
			result = append(result, op)
		}
	}
	return result
}

func (os *OperationSet) GetByMethod(method string) []Operation {
	var result []Operation
	for _, op := range os.Operations {
		if strings.EqualFold(op.Method, method) {
			result = append(result, op)
		}
	}
	return result
}

func (os *OperationSet) GetByPathAndMethod(path, method string) *Operation {
	for i := range os.Operations {
		if os.Operations[i].Path == path && strings.EqualFold(os.Operations[i].Method, method) {
			return &os.Operations[i]
		}
	}
	return nil
}

func (os *OperationSet) GetByID(id uuid.UUID) *Operation {
	for i := range os.Operations {
		if os.Operations[i].ID == id {
			return &os.Operations[i]
		}
	}
	return nil
}

func (os *OperationSet) Count() int {
	return len(os.Operations)
}
