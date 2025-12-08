package wsdl

import (
	"strings"
	"time"
)

// DefaultValueStrategy generates sensible default values for XSD types
type DefaultValueStrategy struct{}

// NewDefaultValueStrategy creates a new default value strategy
func NewDefaultValueStrategy() *DefaultValueStrategy {
	return &DefaultValueStrategy{}
}

// GenerateForType generates a default value for an XSD type name
func (s *DefaultValueStrategy) GenerateForType(xsdType string) string {
	// Handle prefixed types (e.g., "xsd:string", "xs:int", "tns:CustomType")
	typeName := strings.ToLower(ExtractLocalName(xsdType))

	switch typeName {
	// String types
	case "string", "normalizedstring", "token", "language", "nmtoken", "nmtokens",
		"name", "ncname", "id", "idref", "idrefs", "entity", "entities":
		return "string_value"

	// Integer types
	case "int", "integer", "long", "short", "byte":
		return "1"
	case "positiveinteger", "unsignedint", "unsignedlong", "unsignedshort", "unsignedbyte":
		return "1"
	case "negativeinteger":
		return "-1"
	case "nonpositiveinteger":
		return "0"
	case "nonnegativeinteger":
		return "0"

	// Decimal/Float types
	case "decimal", "float", "double":
		return "1.0"

	// Boolean
	case "boolean":
		return "true"

	// Date/Time types
	case "date":
		return time.Now().Format("2006-01-02")
	case "datetime":
		return time.Now().Format(time.RFC3339)
	case "time":
		return time.Now().Format("15:04:05")
	case "duration":
		return "P1D"
	case "gyear":
		return time.Now().Format("2006")
	case "gmonth":
		return "--" + time.Now().Format("01")
	case "gday":
		return "---" + time.Now().Format("02")
	case "gyearmonth":
		return time.Now().Format("2006-01")
	case "gmonthday":
		return "--" + time.Now().Format("01-02")

	// Binary types
	case "base64binary":
		return "dGVzdA==" // "test" in base64
	case "hexbinary":
		return "74657374" // "test" in hex

	// URI/QName types
	case "anyuri":
		return "https://example.com"
	case "qname":
		return "prefix:localpart"
	case "notation":
		return "notation"

	// Any type
	case "anytype", "anysimpletype":
		return "value"

	default:
		return "value"
	}
}

// GenerateForElement generates a value for an XSD element
func (s *DefaultValueStrategy) GenerateForElement(elem *XSDElement, registry *TypeRegistry) interface{} {
	if elem == nil {
		return "value"
	}

	// If element has an inline complex type, build an object
	if elem.ComplexType != nil {
		return s.generateComplexTypeValue(elem.ComplexType, registry, 0)
	}

	// If element has an inline simple type, use its restriction
	if elem.SimpleType != nil {
		return s.generateSimpleTypeValue(elem.SimpleType)
	}

	// If element references a type
	if elem.Type != "" {
		localType := ExtractLocalName(elem.Type)

		// Check if it's a built-in XSD type
		if IsXSDBuiltinType(localType) {
			return s.GenerateForType(localType)
		}

		// Check complex types in registry
		if ct, ok := registry.ComplexTypes[localType]; ok {
			return s.generateComplexTypeValue(ct, registry, 0)
		}
		if ct, ok := registry.ComplexTypes[elem.Type]; ok {
			return s.generateComplexTypeValue(ct, registry, 0)
		}

		// Check simple types in registry
		if st, ok := registry.SimpleTypes[localType]; ok {
			return s.generateSimpleTypeValue(st)
		}
		if st, ok := registry.SimpleTypes[elem.Type]; ok {
			return s.generateSimpleTypeValue(st)
		}

		// Fall back to type name as primitive
		return s.GenerateForType(localType)
	}

	return "value"
}

// generateComplexTypeValue generates a value for a complex type
func (s *DefaultValueStrategy) generateComplexTypeValue(ct *XSDComplexType, registry *TypeRegistry, depth int) map[string]interface{} {
	if depth > 5 {
		return make(map[string]interface{})
	}

	result := make(map[string]interface{})

	// Handle sequence
	if ct.Sequence != nil {
		for _, elem := range ct.Sequence.Elements {
			result[elem.Name] = s.GenerateForElement(&elem, registry)
		}
	}

	// Handle all
	if ct.All != nil {
		for _, elem := range ct.All.Elements {
			result[elem.Name] = s.GenerateForElement(&elem, registry)
		}
	}

	// Handle choice (pick first element)
	if ct.Choice != nil && len(ct.Choice.Elements) > 0 {
		elem := ct.Choice.Elements[0]
		result[elem.Name] = s.GenerateForElement(&elem, registry)
	}

	// Handle complexContent (extension/restriction)
	if ct.ComplexContent != nil {
		if ct.ComplexContent.Extension != nil {
			// Get base type fields first
			if base := ct.ComplexContent.Extension.Base; base != "" {
				baseName := ExtractLocalName(base)
				if baseCT, ok := registry.ComplexTypes[baseName]; ok {
					baseResult := s.generateComplexTypeValue(baseCT, registry, depth+1)
					for k, v := range baseResult {
						result[k] = v
					}
				}
			}
			// Add extension fields
			if ct.ComplexContent.Extension.Sequence != nil {
				for _, elem := range ct.ComplexContent.Extension.Sequence.Elements {
					result[elem.Name] = s.GenerateForElement(&elem, registry)
				}
			}
		}
		if ct.ComplexContent.Restriction != nil {
			if ct.ComplexContent.Restriction.Sequence != nil {
				for _, elem := range ct.ComplexContent.Restriction.Sequence.Elements {
					result[elem.Name] = s.GenerateForElement(&elem, registry)
				}
			}
		}
	}

	// Handle simpleContent (value with attributes)
	if ct.SimpleContent != nil {
		if ct.SimpleContent.Extension != nil {
			baseType := ct.SimpleContent.Extension.Base
			result["_value"] = s.GenerateForType(baseType)
		}
	}

	return result
}

// generateSimpleTypeValue generates a value for a simple type
func (s *DefaultValueStrategy) generateSimpleTypeValue(st *XSDSimpleType) string {
	if st.Restriction != nil {
		// If enumeration exists, use first value
		if len(st.Restriction.Enumeration) > 0 {
			return st.Restriction.Enumeration[0]
		}
		// Otherwise generate based on base type
		return s.GenerateForType(st.Restriction.Base)
	}

	if st.List != nil {
		// Generate a single item for list type
		return s.GenerateForType(st.List.ItemType)
	}

	if st.Union != nil {
		// Generate for first member type
		members := strings.Fields(st.Union.MemberTypes)
		if len(members) > 0 {
			return s.GenerateForType(members[0])
		}
	}

	return "value"
}

// GenerateXMLForElement generates XML string for an element
func (s *DefaultValueStrategy) GenerateXMLForElement(elem *XSDElement, registry *TypeRegistry, indent string, targetNS string, depth int) string {
	if elem == nil || depth > 10 {
		return ""
	}

	var builder strings.Builder
	elemName := elem.Name

	// Handle element reference
	if elem.Ref != "" {
		refName := ExtractLocalName(elem.Ref)
		if refElem, ok := registry.Elements[refName]; ok {
			return s.GenerateXMLForElement(refElem, registry, indent, targetNS, depth)
		}
		if refElem, ok := registry.Elements[elem.Ref]; ok {
			return s.GenerateXMLForElement(refElem, registry, indent, targetNS, depth)
		}
		elemName = refName
	}

	// Handle inline complex type
	if elem.ComplexType != nil {
		builder.WriteString(indent)
		builder.WriteString("<")
		builder.WriteString(elemName)
		builder.WriteString(">\n")
		builder.WriteString(s.generateComplexTypeXML(elem.ComplexType, registry, indent+"  ", targetNS, depth+1))
		builder.WriteString(indent)
		builder.WriteString("</")
		builder.WriteString(elemName)
		builder.WriteString(">\n")
		return builder.String()
	}

	// Handle type reference
	if elem.Type != "" {
		localType := ExtractLocalName(elem.Type)

		// Check if complex type
		if ct, ok := registry.ComplexTypes[localType]; ok {
			builder.WriteString(indent)
			builder.WriteString("<")
			builder.WriteString(elemName)
			builder.WriteString(">\n")
			builder.WriteString(s.generateComplexTypeXML(ct, registry, indent+"  ", targetNS, depth+1))
			builder.WriteString(indent)
			builder.WriteString("</")
			builder.WriteString(elemName)
			builder.WriteString(">\n")
			return builder.String()
		}
		if ct, ok := registry.ComplexTypes[elem.Type]; ok {
			builder.WriteString(indent)
			builder.WriteString("<")
			builder.WriteString(elemName)
			builder.WriteString(">\n")
			builder.WriteString(s.generateComplexTypeXML(ct, registry, indent+"  ", targetNS, depth+1))
			builder.WriteString(indent)
			builder.WriteString("</")
			builder.WriteString(elemName)
			builder.WriteString(">\n")
			return builder.String()
		}

		// Simple type or built-in
		value := s.GenerateForType(localType)
		builder.WriteString(indent)
		builder.WriteString("<")
		builder.WriteString(elemName)
		builder.WriteString(">")
		builder.WriteString(XMLEscape(value))
		builder.WriteString("</")
		builder.WriteString(elemName)
		builder.WriteString(">\n")
		return builder.String()
	}

	// Default simple element
	builder.WriteString(indent)
	builder.WriteString("<")
	builder.WriteString(elemName)
	builder.WriteString(">")
	builder.WriteString("value")
	builder.WriteString("</")
	builder.WriteString(elemName)
	builder.WriteString(">\n")

	return builder.String()
}

// generateComplexTypeXML generates XML for a complex type
func (s *DefaultValueStrategy) generateComplexTypeXML(ct *XSDComplexType, registry *TypeRegistry, indent string, targetNS string, depth int) string {
	if ct == nil || depth > 10 {
		return ""
	}

	var builder strings.Builder

	// Handle sequence
	if ct.Sequence != nil {
		for _, elem := range ct.Sequence.Elements {
			builder.WriteString(s.GenerateXMLForElement(&elem, registry, indent, targetNS, depth))
		}
	}

	// Handle all
	if ct.All != nil {
		for _, elem := range ct.All.Elements {
			builder.WriteString(s.GenerateXMLForElement(&elem, registry, indent, targetNS, depth))
		}
	}

	// Handle choice (use first element)
	if ct.Choice != nil && len(ct.Choice.Elements) > 0 {
		elem := ct.Choice.Elements[0]
		builder.WriteString(s.GenerateXMLForElement(&elem, registry, indent, targetNS, depth))
	}

	// Handle complexContent extension
	if ct.ComplexContent != nil && ct.ComplexContent.Extension != nil {
		// Add base type content first
		if base := ct.ComplexContent.Extension.Base; base != "" {
			baseName := ExtractLocalName(base)
			if baseCT, ok := registry.ComplexTypes[baseName]; ok {
				builder.WriteString(s.generateComplexTypeXML(baseCT, registry, indent, targetNS, depth+1))
			}
		}
		// Add extension content
		if ct.ComplexContent.Extension.Sequence != nil {
			for _, elem := range ct.ComplexContent.Extension.Sequence.Elements {
				builder.WriteString(s.GenerateXMLForElement(&elem, registry, indent, targetNS, depth))
			}
		}
	}

	// Handle simpleContent
	if ct.SimpleContent != nil {
		if ct.SimpleContent.Extension != nil {
			value := s.GenerateForType(ct.SimpleContent.Extension.Base)
			builder.WriteString(indent)
			builder.WriteString(XMLEscape(value))
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

// GenerateXMLForMessagePart generates XML for a message part
func (s *DefaultValueStrategy) GenerateXMLForMessagePart(part *MessagePart, registry *TypeRegistry, indent string, targetNS string) string {
	if part == nil {
		return ""
	}

	// Element-based part (document/literal style)
	if part.Element != "" {
		elemName := ExtractLocalName(part.Element)
		if elem, ok := registry.Elements[elemName]; ok {
			return s.GenerateXMLForElement(elem, registry, indent, targetNS, 0)
		}
		if elem, ok := registry.Elements[part.Element]; ok {
			return s.GenerateXMLForElement(elem, registry, indent, targetNS, 0)
		}
		// Element not found in registry, generate simple element
		return indent + "<" + elemName + ">value</" + elemName + ">\n"
	}

	// Type-based part (RPC style)
	if part.Type != "" {
		localType := ExtractLocalName(part.Type)

		// Check complex types
		if ct, ok := registry.ComplexTypes[localType]; ok {
			var builder strings.Builder
			builder.WriteString(indent)
			builder.WriteString("<")
			builder.WriteString(part.Name)
			builder.WriteString(">\n")
			builder.WriteString(s.generateComplexTypeXML(ct, registry, indent+"  ", targetNS, 0))
			builder.WriteString(indent)
			builder.WriteString("</")
			builder.WriteString(part.Name)
			builder.WriteString(">\n")
			return builder.String()
		}

		// Simple type
		value := s.GenerateForType(localType)
		return indent + "<" + part.Name + ">" + XMLEscape(value) + "</" + part.Name + ">\n"
	}

	return ""
}
