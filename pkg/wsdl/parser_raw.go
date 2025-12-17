package wsdl

import "encoding/xml"

// Raw XML parsing structures for WSDL 1.1
// These map directly to XML and are converted to domain models

// rawDefinitions is the root WSDL element
type rawDefinitions struct {
	XMLName         xml.Name          `xml:"definitions"`
	TargetNamespace string            `xml:"targetNamespace,attr"`
	Name            string            `xml:"name,attr"`
	Types           *rawTypes         `xml:"types"`
	Messages        []rawMessage      `xml:"message"`
	PortTypes       []rawPortType     `xml:"portType"`
	Bindings        []rawBinding      `xml:"binding"`
	Services        []rawService      `xml:"service"`
	Imports         []rawWSDLImport   `xml:"import"`
	Documentation   *rawDocumentation `xml:"documentation"`
}

// rawWSDLImport represents wsdl:import
type rawWSDLImport struct {
	Namespace string `xml:"namespace,attr"`
	Location  string `xml:"location,attr"`
}

// rawTypes contains type definitions (XSD schemas)
type rawTypes struct {
	Schemas []rawSchema `xml:"schema"`
}

// rawDocumentation represents wsdl:documentation
type rawDocumentation struct {
	Content string `xml:",chardata"`
}

// rawMessage represents wsdl:message
type rawMessage struct {
	Name          string            `xml:"name,attr"`
	Parts         []rawMessagePart  `xml:"part"`
	Documentation *rawDocumentation `xml:"documentation"`
}

// rawMessagePart represents wsdl:part
type rawMessagePart struct {
	Name    string `xml:"name,attr"`
	Element string `xml:"element,attr"`
	Type    string `xml:"type,attr"`
}

// rawPortType represents wsdl:portType
type rawPortType struct {
	Name          string            `xml:"name,attr"`
	Operations    []rawOperation    `xml:"operation"`
	Documentation *rawDocumentation `xml:"documentation"`
}

// rawOperation represents wsdl:operation in portType
type rawOperation struct {
	Name          string            `xml:"name,attr"`
	Input         *rawIORef         `xml:"input"`
	Output        *rawIORef         `xml:"output"`
	Faults        []rawIORef        `xml:"fault"`
	Documentation *rawDocumentation `xml:"documentation"`
}

// rawIORef represents input/output/fault reference
type rawIORef struct {
	Name    string `xml:"name,attr"`
	Message string `xml:"message,attr"`
}

// rawBinding represents wsdl:binding
type rawBinding struct {
	Name          string                `xml:"name,attr"`
	Type          string                `xml:"type,attr"`
	SOAPBinding   *rawSOAPBinding       `xml:"http://schemas.xmlsoap.org/wsdl/soap/ binding"`
	SOAP12Binding *rawSOAPBinding       `xml:"http://schemas.xmlsoap.org/wsdl/soap12/ binding"`
	Operations    []rawBindingOperation `xml:"operation"`
}

// rawSOAPBinding represents soap:binding
type rawSOAPBinding struct {
	Style     string `xml:"style,attr"`
	Transport string `xml:"transport,attr"`
}

// rawBindingOperation represents wsdl:operation in binding
type rawBindingOperation struct {
	Name            string            `xml:"name,attr"`
	SOAPOperation   *rawSOAPOperation `xml:"http://schemas.xmlsoap.org/wsdl/soap/ operation"`
	SOAP12Operation *rawSOAPOperation `xml:"http://schemas.xmlsoap.org/wsdl/soap12/ operation"`
	Input           *rawBindingIO     `xml:"input"`
	Output          *rawBindingIO     `xml:"output"`
	Faults          []rawBindingFault `xml:"fault"`
}

// rawSOAPOperation represents soap:operation
type rawSOAPOperation struct {
	SOAPAction string `xml:"soapAction,attr"`
	Style      string `xml:"style,attr"`
}

// rawBindingIO represents input/output in binding operation
type rawBindingIO struct {
	SOAPBody   *rawSOAPBody   `xml:"http://schemas.xmlsoap.org/wsdl/soap/ body"`
	SOAP12Body *rawSOAPBody   `xml:"http://schemas.xmlsoap.org/wsdl/soap12/ body"`
	SOAPHeader *rawSOAPHeader `xml:"http://schemas.xmlsoap.org/wsdl/soap/ header"`
}

// rawSOAPBody represents soap:body
type rawSOAPBody struct {
	Use           string `xml:"use,attr"`
	Namespace     string `xml:"namespace,attr"`
	EncodingStyle string `xml:"encodingStyle,attr"`
	Parts         string `xml:"parts,attr"`
}

// rawSOAPHeader represents soap:header
type rawSOAPHeader struct {
	Message       string `xml:"message,attr"`
	Part          string `xml:"part,attr"`
	Use           string `xml:"use,attr"`
	Namespace     string `xml:"namespace,attr"`
	EncodingStyle string `xml:"encodingStyle,attr"`
}

// rawBindingFault represents fault in binding operation
type rawBindingFault struct {
	Name      string        `xml:"name,attr"`
	SOAPFault *rawSOAPFault `xml:"http://schemas.xmlsoap.org/wsdl/soap/ fault"`
}

// rawSOAPFault represents soap:fault
type rawSOAPFault struct {
	Name          string `xml:"name,attr"`
	Use           string `xml:"use,attr"`
	Namespace     string `xml:"namespace,attr"`
	EncodingStyle string `xml:"encodingStyle,attr"`
}

// rawService represents wsdl:service
type rawService struct {
	Name          string            `xml:"name,attr"`
	Ports         []rawPort         `xml:"port"`
	Documentation *rawDocumentation `xml:"documentation"`
}

// rawPort represents wsdl:port
type rawPort struct {
	Name          string          `xml:"name,attr"`
	Binding       string          `xml:"binding,attr"`
	SOAPAddress   *rawSOAPAddress `xml:"http://schemas.xmlsoap.org/wsdl/soap/ address"`
	SOAP12Address *rawSOAPAddress `xml:"http://schemas.xmlsoap.org/wsdl/soap12/ address"`
}

// rawSOAPAddress represents soap:address
type rawSOAPAddress struct {
	Location string `xml:"location,attr"`
}

// XSD Raw Structures

// rawSchema represents xsd:schema
type rawSchema struct {
	XMLName            xml.Name         `xml:"schema"`
	TargetNamespace    string           `xml:"targetNamespace,attr"`
	ElementFormDefault string           `xml:"elementFormDefault,attr"`
	Imports            []rawXSDImport   `xml:"import"`
	Includes           []rawXSDInclude  `xml:"include"`
	Elements           []rawElement     `xml:"element"`
	ComplexTypes       []rawComplexType `xml:"complexType"`
	SimpleTypes        []rawSimpleType  `xml:"simpleType"`
}

// rawXSDImport represents xsd:import
type rawXSDImport struct {
	Namespace      string `xml:"namespace,attr"`
	SchemaLocation string `xml:"schemaLocation,attr"`
}

// rawXSDInclude represents xsd:include
type rawXSDInclude struct {
	SchemaLocation string `xml:"schemaLocation,attr"`
}

// rawElement represents xsd:element
type rawElement struct {
	Name        string          `xml:"name,attr"`
	Type        string          `xml:"type,attr"`
	Ref         string          `xml:"ref,attr"`
	MinOccurs   string          `xml:"minOccurs,attr"`
	MaxOccurs   string          `xml:"maxOccurs,attr"`
	Nillable    bool            `xml:"nillable,attr"`
	Default     string          `xml:"default,attr"`
	Fixed       string          `xml:"fixed,attr"`
	ComplexType *rawComplexType `xml:"complexType"`
	SimpleType  *rawSimpleType  `xml:"simpleType"`
}

// rawComplexType represents xsd:complexType
type rawComplexType struct {
	Name           string             `xml:"name,attr"`
	Abstract       bool               `xml:"abstract,attr"`
	Mixed          bool               `xml:"mixed,attr"`
	Sequence       *rawSequence       `xml:"sequence"`
	All            *rawAll            `xml:"all"`
	Choice         *rawChoice         `xml:"choice"`
	ComplexContent *rawComplexContent `xml:"complexContent"`
	SimpleContent  *rawSimpleContent  `xml:"simpleContent"`
	Attributes     []rawAttribute     `xml:"attribute"`
}

// rawSequence represents xsd:sequence
type rawSequence struct {
	MinOccurs string       `xml:"minOccurs,attr"`
	MaxOccurs string       `xml:"maxOccurs,attr"`
	Elements  []rawElement `xml:"element"`
	Choices   []rawChoice  `xml:"choice"`
	Any       []rawAny     `xml:"any"`
}

// rawAll represents xsd:all
type rawAll struct {
	MinOccurs string       `xml:"minOccurs,attr"`
	MaxOccurs string       `xml:"maxOccurs,attr"`
	Elements  []rawElement `xml:"element"`
}

// rawChoice represents xsd:choice
type rawChoice struct {
	MinOccurs string        `xml:"minOccurs,attr"`
	MaxOccurs string        `xml:"maxOccurs,attr"`
	Elements  []rawElement  `xml:"element"`
	Sequences []rawSequence `xml:"sequence"`
	Any       []rawAny      `xml:"any"`
}

// rawAny represents xsd:any
type rawAny struct {
	Namespace       string `xml:"namespace,attr"`
	ProcessContents string `xml:"processContents,attr"`
	MinOccurs       string `xml:"minOccurs,attr"`
	MaxOccurs       string `xml:"maxOccurs,attr"`
}

// rawComplexContent represents xsd:complexContent
type rawComplexContent struct {
	Mixed       bool            `xml:"mixed,attr"`
	Extension   *rawExtension   `xml:"extension"`
	Restriction *rawRestriction `xml:"restriction"`
}

// rawSimpleContent represents xsd:simpleContent
type rawSimpleContent struct {
	Extension   *rawExtension   `xml:"extension"`
	Restriction *rawRestriction `xml:"restriction"`
}

// rawExtension represents xsd:extension
type rawExtension struct {
	Base       string         `xml:"base,attr"`
	Sequence   *rawSequence   `xml:"sequence"`
	All        *rawAll        `xml:"all"`
	Choice     *rawChoice     `xml:"choice"`
	Attributes []rawAttribute `xml:"attribute"`
}

// rawRestriction represents xsd:restriction
type rawRestriction struct {
	Base           string           `xml:"base,attr"`
	Enumeration    []rawEnumeration `xml:"enumeration"`
	MinLength      *int             `xml:"minLength>value,attr"`
	MaxLength      *int             `xml:"maxLength>value,attr"`
	Length         *int             `xml:"length>value,attr"`
	Pattern        string           `xml:"pattern>value,attr"`
	WhiteSpace     string           `xml:"whiteSpace>value,attr"`
	MinInclusive   string           `xml:"minInclusive>value,attr"`
	MaxInclusive   string           `xml:"maxInclusive>value,attr"`
	MinExclusive   string           `xml:"minExclusive>value,attr"`
	MaxExclusive   string           `xml:"maxExclusive>value,attr"`
	TotalDigits    *int             `xml:"totalDigits>value,attr"`
	FractionDigits *int             `xml:"fractionDigits>value,attr"`
	Sequence       *rawSequence     `xml:"sequence"`
	All            *rawAll          `xml:"all"`
	Choice         *rawChoice       `xml:"choice"`
	Attributes     []rawAttribute   `xml:"attribute"`
}

// rawEnumeration represents xsd:enumeration
type rawEnumeration struct {
	Value string `xml:"value,attr"`
}

// rawSimpleType represents xsd:simpleType
type rawSimpleType struct {
	Name        string          `xml:"name,attr"`
	Restriction *rawRestriction `xml:"restriction"`
	List        *rawList        `xml:"list"`
	Union       *rawUnion       `xml:"union"`
}

// rawList represents xsd:list
type rawList struct {
	ItemType string `xml:"itemType,attr"`
}

// rawUnion represents xsd:union
type rawUnion struct {
	MemberTypes string `xml:"memberTypes,attr"`
}

// rawAttribute represents xsd:attribute
type rawAttribute struct {
	Name    string `xml:"name,attr"`
	Ref     string `xml:"ref,attr"`
	Type    string `xml:"type,attr"`
	Use     string `xml:"use,attr"`
	Default string `xml:"default,attr"`
	Fixed   string `xml:"fixed,attr"`
	Form    string `xml:"form,attr"`
}
