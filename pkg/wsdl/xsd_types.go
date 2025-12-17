package wsdl

// XSDSchema represents an XSD schema embedded in WSDL or imported externally
type XSDSchema struct {
	TargetNamespace    string           `json:"target_namespace,omitempty"`
	ElementFormDefault string           `json:"element_form_default,omitempty"` // "qualified" or "unqualified"
	Imports            []XSDImport      `json:"-"`                              // For internal resolution
	Includes           []XSDInclude     `json:"-"`                              // For internal resolution
	Elements           []XSDElement     `json:"elements,omitempty"`
	ComplexTypes       []XSDComplexType `json:"complex_types,omitempty"`
	SimpleTypes        []XSDSimpleType  `json:"simple_types,omitempty"`
}

// XSDImport represents xsd:import element
type XSDImport struct {
	Namespace      string `json:"namespace,omitempty"`
	SchemaLocation string `json:"schema_location,omitempty"`
}

// XSDInclude represents xsd:include element (same namespace)
type XSDInclude struct {
	SchemaLocation string `json:"schema_location,omitempty"`
}

// XSDElement represents an element declaration
type XSDElement struct {
	Name            string          `json:"name"`
	Type            string          `json:"type,omitempty"`       // QName reference to type
	Ref             string          `json:"ref,omitempty"`        // QName reference to another element
	MinOccurs       string          `json:"min_occurs,omitempty"` // Default: "1"
	MaxOccurs       string          `json:"max_occurs,omitempty"` // Default: "1", can be "unbounded"
	Nillable        bool            `json:"nillable,omitempty"`
	Default         string          `json:"default,omitempty"`
	Fixed           string          `json:"fixed,omitempty"`
	ComplexType     *XSDComplexType `json:"complex_type,omitempty"` // Inline complex type
	SimpleType      *XSDSimpleType  `json:"simple_type,omitempty"`  // Inline simple type
	Substitution    string          `json:"substitution_group,omitempty"`
	TargetNamespace string          `json:"target_namespace,omitempty"` // Inherited from schema
}

// XSDComplexType represents a complex type definition
type XSDComplexType struct {
	Name           string              `json:"name,omitempty"` // Empty for anonymous types
	Abstract       bool                `json:"abstract,omitempty"`
	Mixed          bool                `json:"mixed,omitempty"`
	Sequence       *XSDSequence        `json:"sequence,omitempty"`
	All            *XSDAll             `json:"all,omitempty"`
	Choice         *XSDChoice          `json:"choice,omitempty"`
	ComplexContent *XSDComplexContent  `json:"complex_content,omitempty"`
	SimpleContent  *XSDSimpleContent   `json:"simple_content,omitempty"`
	Attributes     []XSDAttribute      `json:"attributes,omitempty"`
	AttributeGroup []XSDAttributeGroup `json:"attribute_groups,omitempty"`
	AnyAttribute   *XSDAnyAttribute    `json:"any_attribute,omitempty"`
}

// XSDSequence represents xsd:sequence compositor (ordered elements)
type XSDSequence struct {
	MinOccurs string        `json:"min_occurs,omitempty"`
	MaxOccurs string        `json:"max_occurs,omitempty"`
	Elements  []XSDElement  `json:"elements,omitempty"`
	Choices   []XSDChoice   `json:"choices,omitempty"`
	Sequences []XSDSequence `json:"sequences,omitempty"`
	Any       []XSDAny      `json:"any,omitempty"`
}

// XSDAll represents xsd:all compositor (unordered elements, each at most once)
type XSDAll struct {
	MinOccurs string       `json:"min_occurs,omitempty"`
	MaxOccurs string       `json:"max_occurs,omitempty"`
	Elements  []XSDElement `json:"elements,omitempty"`
}

// XSDChoice represents xsd:choice compositor (one of the elements)
type XSDChoice struct {
	MinOccurs string        `json:"min_occurs,omitempty"`
	MaxOccurs string        `json:"max_occurs,omitempty"`
	Elements  []XSDElement  `json:"elements,omitempty"`
	Sequences []XSDSequence `json:"sequences,omitempty"`
	Choices   []XSDChoice   `json:"choices,omitempty"`
	Any       []XSDAny      `json:"any,omitempty"`
}

// XSDAny represents xsd:any wildcard element
type XSDAny struct {
	Namespace       string `json:"namespace,omitempty"`        // ##any, ##other, ##local, ##targetNamespace, or URI list
	ProcessContents string `json:"process_contents,omitempty"` // strict, lax, skip
	MinOccurs       string `json:"min_occurs,omitempty"`
	MaxOccurs       string `json:"max_occurs,omitempty"`
}

// XSDComplexContent for complex type with complex content model
type XSDComplexContent struct {
	Mixed       bool            `json:"mixed,omitempty"`
	Extension   *XSDExtension   `json:"extension,omitempty"`
	Restriction *XSDRestriction `json:"restriction,omitempty"`
}

// XSDSimpleContent for complex type with simple content model
type XSDSimpleContent struct {
	Extension   *XSDExtension   `json:"extension,omitempty"`
	Restriction *XSDRestriction `json:"restriction,omitempty"`
}

// XSDExtension represents xsd:extension
type XSDExtension struct {
	Base           string              `json:"base"` // QName of base type
	Sequence       *XSDSequence        `json:"sequence,omitempty"`
	All            *XSDAll             `json:"all,omitempty"`
	Choice         *XSDChoice          `json:"choice,omitempty"`
	Attributes     []XSDAttribute      `json:"attributes,omitempty"`
	AttributeGroup []XSDAttributeGroup `json:"attribute_groups,omitempty"`
	AnyAttribute   *XSDAnyAttribute    `json:"any_attribute,omitempty"`
}

// XSDRestriction represents xsd:restriction for both simple and complex types
type XSDRestriction struct {
	Base           string              `json:"base"` // QName of base type
	Enumeration    []string            `json:"enumeration,omitempty"`
	MinLength      *int                `json:"min_length,omitempty"`
	MaxLength      *int                `json:"max_length,omitempty"`
	Length         *int                `json:"length,omitempty"`
	Pattern        string              `json:"pattern,omitempty"`
	WhiteSpace     string              `json:"white_space,omitempty"` // preserve, replace, collapse
	MinInclusive   string              `json:"min_inclusive,omitempty"`
	MaxInclusive   string              `json:"max_inclusive,omitempty"`
	MinExclusive   string              `json:"min_exclusive,omitempty"`
	MaxExclusive   string              `json:"max_exclusive,omitempty"`
	TotalDigits    *int                `json:"total_digits,omitempty"`
	FractionDigits *int                `json:"fraction_digits,omitempty"`
	Sequence       *XSDSequence        `json:"sequence,omitempty"` // For complex content restriction
	All            *XSDAll             `json:"all,omitempty"`
	Choice         *XSDChoice          `json:"choice,omitempty"`
	Attributes     []XSDAttribute      `json:"attributes,omitempty"`
	AttributeGroup []XSDAttributeGroup `json:"attribute_groups,omitempty"`
	AnyAttribute   *XSDAnyAttribute    `json:"any_attribute,omitempty"`
}

// XSDSimpleType represents a simple type definition
type XSDSimpleType struct {
	Name        string          `json:"name,omitempty"` // Empty for anonymous types
	Restriction *XSDRestriction `json:"restriction,omitempty"`
	List        *XSDList        `json:"list,omitempty"`
	Union       *XSDUnion       `json:"union,omitempty"`
}

// XSDList represents xsd:list (space-separated list of values)
type XSDList struct {
	ItemType   string         `json:"item_type,omitempty"`   // QName reference
	SimpleType *XSDSimpleType `json:"simple_type,omitempty"` // Inline item type
}

// XSDUnion represents xsd:union (value from one of multiple types)
type XSDUnion struct {
	MemberTypes string          `json:"member_types,omitempty"` // Space-separated QNames
	SimpleTypes []XSDSimpleType `json:"simple_types,omitempty"` // Inline member types
}

// XSDAttribute represents an attribute declaration
type XSDAttribute struct {
	Name       string         `json:"name,omitempty"`
	Ref        string         `json:"ref,omitempty"`  // QName reference to global attribute
	Type       string         `json:"type,omitempty"` // QName reference to type
	Use        string         `json:"use,omitempty"`  // "required", "optional", "prohibited"
	Default    string         `json:"default,omitempty"`
	Fixed      string         `json:"fixed,omitempty"`
	Form       string         `json:"form,omitempty"`        // "qualified" or "unqualified"
	SimpleType *XSDSimpleType `json:"simple_type,omitempty"` // Inline simple type
}

// XSDAttributeGroup represents a group of attributes
type XSDAttributeGroup struct {
	Name           string              `json:"name,omitempty"`
	Ref            string              `json:"ref,omitempty"` // QName reference
	Attributes     []XSDAttribute      `json:"attributes,omitempty"`
	AttributeGroup []XSDAttributeGroup `json:"attribute_groups,omitempty"`
	AnyAttribute   *XSDAnyAttribute    `json:"any_attribute,omitempty"`
}

// XSDAnyAttribute represents xsd:anyAttribute wildcard
type XSDAnyAttribute struct {
	Namespace       string `json:"namespace,omitempty"`
	ProcessContents string `json:"process_contents,omitempty"`
}

// Common XSD namespace constants
const (
	XSDNamespace    = "http://www.w3.org/2001/XMLSchema"
	SOAP11Namespace = "http://schemas.xmlsoap.org/wsdl/soap/"
	SOAP12Namespace = "http://schemas.xmlsoap.org/wsdl/soap12/"
	WSDLNamespace   = "http://schemas.xmlsoap.org/wsdl/"

	// SOAP envelope namespaces
	SOAP11EnvelopeNS = "http://schemas.xmlsoap.org/soap/envelope/"
	SOAP12EnvelopeNS = "http://www.w3.org/2003/05/soap-envelope"

	// Transport
	SOAPHTTPTransport = "http://schemas.xmlsoap.org/soap/http"
)

// XSD built-in type names
const (
	XSDString             = "string"
	XSDBoolean            = "boolean"
	XSDDecimal            = "decimal"
	XSDFloat              = "float"
	XSDDouble             = "double"
	XSDDuration           = "duration"
	XSDDateTime           = "dateTime"
	XSDTime               = "time"
	XSDDate               = "date"
	XSDGYearMonth         = "gYearMonth"
	XSDGYear              = "gYear"
	XSDGMonthDay          = "gMonthDay"
	XSDGDay               = "gDay"
	XSDGMonth             = "gMonth"
	XSDHexBinary          = "hexBinary"
	XSDBase64Binary       = "base64Binary"
	XSDAnyURI             = "anyURI"
	XSDQName              = "QName"
	XSDNOTATION           = "NOTATION"
	XSDNormalizedString   = "normalizedString"
	XSDToken              = "token"
	XSDLanguage           = "language"
	XSDNMTOKEN            = "NMTOKEN"
	XSDNMTOKENS           = "NMTOKENS"
	XSDName               = "Name"
	XSDNCName             = "NCName"
	XSDID                 = "ID"
	XSDIDREF              = "IDREF"
	XSDIDREFS             = "IDREFS"
	XSDENTITY             = "ENTITY"
	XSDENTITIES           = "ENTITIES"
	XSDInteger            = "integer"
	XSDNonPositiveInteger = "nonPositiveInteger"
	XSDNegativeInteger    = "negativeInteger"
	XSDLong               = "long"
	XSDInt                = "int"
	XSDShort              = "short"
	XSDByte               = "byte"
	XSDNonNegativeInteger = "nonNegativeInteger"
	XSDUnsignedLong       = "unsignedLong"
	XSDUnsignedInt        = "unsignedInt"
	XSDUnsignedShort      = "unsignedShort"
	XSDUnsignedByte       = "unsignedByte"
	XSDPositiveInteger    = "positiveInteger"
	XSDAnyType            = "anyType"
	XSDAnySimpleType      = "anySimpleType"
)

// IsXSDBuiltinType checks if a type name is a built-in XSD type
func IsXSDBuiltinType(typeName string) bool {
	builtins := map[string]bool{
		XSDString: true, XSDBoolean: true, XSDDecimal: true, XSDFloat: true,
		XSDDouble: true, XSDDuration: true, XSDDateTime: true, XSDTime: true,
		XSDDate: true, XSDGYearMonth: true, XSDGYear: true, XSDGMonthDay: true,
		XSDGDay: true, XSDGMonth: true, XSDHexBinary: true, XSDBase64Binary: true,
		XSDAnyURI: true, XSDQName: true, XSDNOTATION: true, XSDNormalizedString: true,
		XSDToken: true, XSDLanguage: true, XSDNMTOKEN: true, XSDNMTOKENS: true,
		XSDName: true, XSDNCName: true, XSDID: true, XSDIDREF: true, XSDIDREFS: true,
		XSDENTITY: true, XSDENTITIES: true, XSDInteger: true, XSDNonPositiveInteger: true,
		XSDNegativeInteger: true, XSDLong: true, XSDInt: true, XSDShort: true,
		XSDByte: true, XSDNonNegativeInteger: true, XSDUnsignedLong: true,
		XSDUnsignedInt: true, XSDUnsignedShort: true, XSDUnsignedByte: true,
		XSDPositiveInteger: true, XSDAnyType: true, XSDAnySimpleType: true,
	}
	return builtins[typeName]
}
