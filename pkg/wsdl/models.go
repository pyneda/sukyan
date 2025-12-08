package wsdl

// WSDLDocument represents a parsed WSDL 1.1 document
type WSDLDocument struct {
	TargetNamespace string        `json:"target_namespace"`
	Name            string        `json:"name,omitempty"`
	Documentation   string        `json:"documentation,omitempty"`
	Types           *Types        `json:"types,omitempty"`
	Messages        []Message     `json:"messages"`
	PortTypes       []PortType    `json:"port_types"`
	Bindings        []Binding     `json:"bindings"`
	Services        []Service     `json:"services"`
	Imports         []WSDLImport  `json:"-"` // For internal resolution only
	TypeRegistry    *TypeRegistry `json:"type_registry,omitempty"`
}

// WSDLImport represents a wsdl:import element
type WSDLImport struct {
	Namespace string `json:"namespace,omitempty"`
	Location  string `json:"location,omitempty"`
}

// Types contains the type definitions (XSD schemas)
type Types struct {
	Schemas []XSDSchema `json:"schemas"`
}

// Service represents a WSDL service (collection of ports/endpoints)
type Service struct {
	Name          string `json:"name"`
	Documentation string `json:"documentation,omitempty"`
	Ports         []Port `json:"ports"`
}

// Port represents a single endpoint (binding + address)
type Port struct {
	Name        string `json:"name"`
	Binding     string `json:"binding"`      // QName reference to binding
	Address     string `json:"address"`      // Endpoint URL
	SOAPVersion string `json:"soap_version"` // "1.1" or "1.2"
}

// Binding represents the concrete protocol binding for a port type
type Binding struct {
	Name        string             `json:"name"`
	Type        string             `json:"type"`                  // QName reference to portType
	Style       string             `json:"style,omitempty"`       // "document" or "rpc"
	Transport   string             `json:"transport,omitempty"`   // e.g., "http://schemas.xmlsoap.org/soap/http"
	SOAPVersion string             `json:"soap_version,omitempty"` // "1.1" or "1.2"
	Operations  []BindingOperation `json:"operations"`
}

// BindingOperation defines operation-level binding details
type BindingOperation struct {
	Name       string        `json:"name"`
	SOAPAction string        `json:"soap_action,omitempty"`
	Style      string        `json:"style,omitempty"` // Overrides binding-level style
	Input      *BindingIO    `json:"input,omitempty"`
	Output     *BindingIO    `json:"output,omitempty"`
	Faults     []BindingFault `json:"faults,omitempty"`
}

// BindingIO describes the encoding for input/output messages
type BindingIO struct {
	Use           string `json:"use,omitempty"`            // "literal" or "encoded"
	Namespace     string `json:"namespace,omitempty"`      // Namespace for encoded messages
	EncodingStyle string `json:"encoding_style,omitempty"` // SOAP encoding style URI
}

// BindingFault describes the encoding for fault messages
type BindingFault struct {
	Name          string `json:"name"`
	Use           string `json:"use,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
	EncodingStyle string `json:"encoding_style,omitempty"`
}

// PortType defines abstract operation signatures (interface)
type PortType struct {
	Name          string      `json:"name"`
	Documentation string      `json:"documentation,omitempty"`
	Operations    []Operation `json:"operations"`
}

// Operation defines a single RPC operation
type Operation struct {
	Name          string  `json:"name"`
	Documentation string  `json:"documentation,omitempty"`
	Input         *IORef  `json:"input,omitempty"`
	Output        *IORef  `json:"output,omitempty"`
	Faults        []IORef `json:"faults,omitempty"`
}

// IORef references a message (input, output, or fault)
type IORef struct {
	Name    string `json:"name,omitempty"`    // Name attribute (optional for input/output)
	Message string `json:"message,omitempty"` // QName reference to message
}

// Message defines an abstract data definition
type Message struct {
	Name          string        `json:"name"`
	Documentation string        `json:"documentation,omitempty"`
	Parts         []MessagePart `json:"parts"`
}

// MessagePart references a type or element within a message
type MessagePart struct {
	Name    string `json:"name"`
	Element string `json:"element,omitempty"` // QName reference to element (doc/literal style)
	Type    string `json:"type,omitempty"`    // QName reference to type (rpc style)
}

// TypeRegistry provides quick lookup of resolved types
type TypeRegistry struct {
	Elements     map[string]*XSDElement     `json:"elements,omitempty"`
	ComplexTypes map[string]*XSDComplexType `json:"complex_types,omitempty"`
	SimpleTypes  map[string]*XSDSimpleType  `json:"simple_types,omitempty"`
	Messages     map[string]*Message        `json:"messages,omitempty"`
}

// NewTypeRegistry creates an empty type registry
func NewTypeRegistry() *TypeRegistry {
	return &TypeRegistry{
		Elements:     make(map[string]*XSDElement),
		ComplexTypes: make(map[string]*XSDComplexType),
		SimpleTypes:  make(map[string]*XSDSimpleType),
		Messages:     make(map[string]*Message),
	}
}

// ServiceEndpoint represents a generated service endpoint for API response
type ServiceEndpoint struct {
	ServiceName  string              `json:"service_name"`
	PortName     string              `json:"port_name"`
	Address      string              `json:"address"`
	SOAPVersion  string              `json:"soap_version"`
	BindingStyle string              `json:"binding_style"`
	Operations   []OperationEndpoint `json:"operations"`
}

// OperationEndpoint represents a single operation with generated requests
type OperationEndpoint struct {
	Name        string             `json:"name"`
	SOAPAction  string             `json:"soap_action,omitempty"`
	Style       string             `json:"style"` // "document" or "rpc"
	InputParts  []PartMetadata     `json:"input_parts,omitempty"`
	OutputParts []PartMetadata     `json:"output_parts,omitempty"`
	Requests    []RequestVariation `json:"requests"`
}

// PartMetadata describes a message part for the API
type PartMetadata struct {
	Name         string         `json:"name"`
	TypeName     string         `json:"type_name,omitempty"`
	ElementName  string         `json:"element_name,omitempty"`
	Required     bool           `json:"required"`
	IsComplex    bool           `json:"is_complex"`
	NestedFields []PartMetadata `json:"nested_fields,omitempty"`
}

// RequestVariation represents a generated SOAP request
type RequestVariation struct {
	Label       string            `json:"label"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers"`
	Body        string            `json:"body"` // XML string
	Description string            `json:"description,omitempty"`
}

// GenerationConfig controls how SOAP requests are generated
type GenerationConfig struct {
	BaseURL               string            `json:"base_url,omitempty"`
	IncludeOptionalParams bool              `json:"include_optional_params"`
	Headers               map[string]string `json:"headers,omitempty"`
	PreferSOAP12          bool              `json:"prefer_soap_12"`
}

// DefaultGenerationConfig returns sensible defaults
func DefaultGenerationConfig() GenerationConfig {
	return GenerationConfig{
		IncludeOptionalParams: true,
		PreferSOAP12:          false,
		Headers:               make(map[string]string),
	}
}

// ValueStrategy defines how to generate values for XSD types
type ValueStrategy interface {
	GenerateForType(xsdType string) string
	GenerateForElement(elem *XSDElement, registry *TypeRegistry) interface{}
}

// GeneratedValue represents a generated value with metadata
type GeneratedValue struct {
	Value       interface{} `json:"value"`
	Description string      `json:"description"`
}
