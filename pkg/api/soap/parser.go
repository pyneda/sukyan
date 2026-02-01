package soap

import (
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
	pkgWsdl "github.com/pyneda/sukyan/pkg/wsdl"
	"github.com/rs/zerolog/log"
)

type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(definition *db.APIDefinition) ([]core.Operation, error) {
	if definition.Type != db.APIDefinitionTypeWSDL {
		return nil, fmt.Errorf("expected WSDL definition, got %s", definition.Type)
	}

	if len(definition.RawDefinition) == 0 {
		return nil, fmt.Errorf("empty raw definition")
	}

	parser := pkgWsdl.NewParser()
	wsdlDoc, err := parser.ParseFromBytes(definition.RawDefinition, definition.SourceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse WSDL: %w", err)
	}

	baseURL := definition.BaseURL
	if baseURL == "" {
		baseURL = definition.SourceURL
	}

	var operations []core.Operation

	for _, service := range wsdlDoc.Services {
		for _, port := range service.Ports {
			binding := p.findBinding(wsdlDoc, port.Binding)
			if binding == nil {
				continue
			}

			portType := p.findPortType(wsdlDoc, binding.Type)
			if portType == nil {
				continue
			}

			address := port.Address
			if address == "" {
				address = baseURL
			}

			for _, bindingOp := range binding.Operations {
				portTypeOp := p.findOperation(portType, bindingOp.Name)
				if portTypeOp == nil {
					continue
				}

				op := p.convertOperation(definition.ID, address, service, port, *binding, bindingOp, portTypeOp, wsdlDoc)
				operations = append(operations, op)
			}
		}
	}

	log.Debug().
		Int("operations", len(operations)).
		Int("services", len(wsdlDoc.Services)).
		Msg("Parsed WSDL definition")

	return operations, nil
}

func (p *Parser) convertOperation(
	definitionID uuid.UUID,
	baseURL string,
	service pkgWsdl.Service,
	port pkgWsdl.Port,
	binding pkgWsdl.Binding,
	bindingOp pkgWsdl.BindingOperation,
	portTypeOp *pkgWsdl.Operation,
	doc *pkgWsdl.WSDLDocument,
) core.Operation {
	operation := core.Operation{
		ID:           uuid.New(),
		DefinitionID: definitionID,
		APIType:      core.APITypeSOAP,
		Name:         bindingOp.Name,
		Method:       "POST",
		Path:         "",
		BaseURL:      baseURL,
		Summary:      portTypeOp.Documentation,
		Description:  portTypeOp.Documentation,
		SOAP: &core.SOAPMetadata{
			ServiceName:  service.Name,
			PortName:     port.Name,
			SOAPAction:   bindingOp.SOAPAction,
			BindingStyle: p.getOperationStyle(binding, bindingOp),
			SOAPVersion:  port.SOAPVersion,
			TargetNS:     doc.TargetNamespace,
		},
	}

	if portTypeOp.Input != nil {
		operation.SOAP.InputMessage = portTypeOp.Input.Message
		params := p.extractMessageParams(portTypeOp.Input.Message, doc)
		operation.Parameters = append(operation.Parameters, params...)
	}

	if portTypeOp.Output != nil {
		operation.SOAP.OutputMessage = portTypeOp.Output.Message
	}

	return operation
}

func (p *Parser) extractMessageParams(messageName string, doc *pkgWsdl.WSDLDocument) []core.Parameter {
	var params []core.Parameter

	message := p.findMessage(doc, messageName)
	if message == nil {
		return params
	}

	for _, part := range message.Parts {
		param := core.Parameter{
			Name:     part.Name,
			Location: core.ParameterLocationBody,
			Required: true,
		}

		if part.Element != "" {
			elem := p.findElement(doc, part.Element)
			if elem != nil {
				p.extractElementInfo(elem, doc, &param)
			}
		} else if part.Type != "" {
			param.DataType = p.mapXSDType(part.Type)
			p.extractTypeConstraints(part.Type, doc, &param)
		}

		params = append(params, param)
	}

	return params
}

func (p *Parser) extractElementInfo(elem *pkgWsdl.XSDElement, doc *pkgWsdl.WSDLDocument, param *core.Parameter) {
	p.extractElementInfoWithDepth(elem, doc, param, make(map[string]bool), 0)
}

func (p *Parser) extractElementInfoWithDepth(elem *pkgWsdl.XSDElement, doc *pkgWsdl.WSDLDocument, param *core.Parameter, visited map[string]bool, depth int) {
	if depth > maxSOAPDepth {
		return
	}

	if elem.Type != "" {
		param.DataType = p.mapXSDType(elem.Type)
		p.extractTypeConstraintsWithDepth(elem.Type, doc, param, visited, depth)
	}

	if elem.ComplexType != nil {
		param.DataType = core.DataTypeObject
		param.NestedParams = p.extractComplexTypeParamsWithDepth(elem.ComplexType, doc, visited, depth+1)
	}

	if elem.SimpleType != nil {
		param.DataType = core.DataTypeString
		p.extractSimpleTypeConstraints(elem.SimpleType, param)
	}

	if elem.MinOccurs != "" {
		if minOccurs, err := strconv.Atoi(elem.MinOccurs); err == nil {
			param.Required = minOccurs > 0
		}
	}

	if elem.Default != "" {
		param.DefaultValue = elem.Default
	}

	param.Nullable = elem.Nillable
}

const maxSOAPDepth = 10

func (p *Parser) extractComplexTypeParams(ct *pkgWsdl.XSDComplexType, doc *pkgWsdl.WSDLDocument) []core.Parameter {
	return p.extractComplexTypeParamsWithDepth(ct, doc, make(map[string]bool), 0)
}

func (p *Parser) extractComplexTypeParamsWithDepth(ct *pkgWsdl.XSDComplexType, doc *pkgWsdl.WSDLDocument, visited map[string]bool, depth int) []core.Parameter {
	var params []core.Parameter

	if depth > maxSOAPDepth {
		return params
	}

	extractElems := func(elems []pkgWsdl.XSDElement, required bool) {
		for _, elem := range elems {
			param := core.Parameter{
				Name:     elem.Name,
				Location: core.ParameterLocationBody,
			}
			if required {
				param.Required = p.isElementRequired(&elem)
			}
			p.extractElementInfoWithDepth(&elem, doc, &param, visited, depth+1)
			params = append(params, param)
		}
	}

	if ct.Sequence != nil {
		extractElems(ct.Sequence.Elements, true)
	}

	if ct.All != nil {
		extractElems(ct.All.Elements, true)
	}

	if ct.Choice != nil {
		extractElems(ct.Choice.Elements, false)
	}

	if ct.ComplexContent != nil && ct.ComplexContent.Extension != nil {
		if ct.ComplexContent.Extension.Sequence != nil {
			extractElems(ct.ComplexContent.Extension.Sequence.Elements, true)
		}
	}

	return params
}

func (p *Parser) extractSimpleTypeConstraints(st *pkgWsdl.XSDSimpleType, param *core.Parameter) {
	if st.Restriction == nil {
		return
	}

	rest := st.Restriction

	if rest.Pattern != "" {
		param.Constraints.Pattern = rest.Pattern
	}

	if rest.MinLength != nil {
		minLen := int(*rest.MinLength)
		param.Constraints.MinLength = &minLen
	}

	if rest.MaxLength != nil {
		maxLen := int(*rest.MaxLength)
		param.Constraints.MaxLength = &maxLen
	}

	if rest.MinInclusive != "" {
		if val, err := strconv.ParseFloat(rest.MinInclusive, 64); err == nil {
			param.Constraints.Minimum = &val
		}
	}

	if rest.MaxInclusive != "" {
		if val, err := strconv.ParseFloat(rest.MaxInclusive, 64); err == nil {
			param.Constraints.Maximum = &val
		}
	}

	if rest.MinExclusive != "" {
		if val, err := strconv.ParseFloat(rest.MinExclusive, 64); err == nil {
			param.Constraints.Minimum = &val
			param.Constraints.ExclusiveMin = true
		}
	}

	if rest.MaxExclusive != "" {
		if val, err := strconv.ParseFloat(rest.MaxExclusive, 64); err == nil {
			param.Constraints.Maximum = &val
			param.Constraints.ExclusiveMax = true
		}
	}

	if len(rest.Enumeration) > 0 {
		for _, e := range rest.Enumeration {
			param.Constraints.Enum = append(param.Constraints.Enum, e)
		}
	}
}

func (p *Parser) extractTypeConstraints(typeName string, doc *pkgWsdl.WSDLDocument, param *core.Parameter) {
	p.extractTypeConstraintsWithDepth(typeName, doc, param, make(map[string]bool), 0)
}

func (p *Parser) extractTypeConstraintsWithDepth(typeName string, doc *pkgWsdl.WSDLDocument, param *core.Parameter, visited map[string]bool, depth int) {
	if doc.TypeRegistry == nil || depth > maxSOAPDepth {
		return
	}

	localName := p.extractLocalName(typeName)

	if visited[localName] {
		return
	}
	visited[localName] = true

	if st, ok := doc.TypeRegistry.SimpleTypes[localName]; ok && st.Restriction != nil {
		p.extractSimpleTypeConstraints(st, param)
	}

	if ct, ok := doc.TypeRegistry.ComplexTypes[localName]; ok {
		param.DataType = core.DataTypeObject
		param.NestedParams = p.extractComplexTypeParamsWithDepth(ct, doc, visited, depth+1)
	}
}

func (p *Parser) mapXSDType(xsdType string) core.DataType {
	localName := p.extractLocalName(xsdType)

	switch localName {
	case "string", "normalizedString", "token", "language", "Name", "NCName", "ID", "IDREF", "IDREFS", "ENTITY", "ENTITIES", "NMTOKEN", "NMTOKENS":
		return core.DataTypeString
	case "int", "integer", "long", "short", "byte", "unsignedInt", "unsignedLong", "unsignedShort", "unsignedByte", "nonNegativeInteger", "positiveInteger", "nonPositiveInteger", "negativeInteger":
		return core.DataTypeInteger
	case "decimal", "float", "double":
		return core.DataTypeNumber
	case "boolean":
		return core.DataTypeBoolean
	case "date", "dateTime", "time", "gYearMonth", "gYear", "gMonthDay", "gDay", "gMonth", "duration":
		return core.DataTypeString
	case "base64Binary", "hexBinary":
		return core.DataTypeString
	case "anyURI":
		return core.DataTypeString
	default:
		return core.DataTypeString
	}
}

func (p *Parser) extractLocalName(qname string) string {
	parts := splitQName(qname)
	return parts[1]
}

func splitQName(qname string) [2]string {
	for i := len(qname) - 1; i >= 0; i-- {
		if qname[i] == ':' {
			return [2]string{qname[:i], qname[i+1:]}
		}
	}
	return [2]string{"", qname}
}

func (p *Parser) isElementRequired(elem *pkgWsdl.XSDElement) bool {
	if elem.MinOccurs == "" {
		return true
	}
	minOccurs, err := strconv.Atoi(elem.MinOccurs)
	if err != nil {
		return true
	}
	return minOccurs > 0
}

func (p *Parser) getOperationStyle(binding pkgWsdl.Binding, op pkgWsdl.BindingOperation) string {
	if op.Style != "" {
		return op.Style
	}
	if binding.Style != "" {
		return binding.Style
	}
	return "document"
}

func (p *Parser) findBinding(doc *pkgWsdl.WSDLDocument, bindingName string) *pkgWsdl.Binding {
	localName := p.extractLocalName(bindingName)
	for i := range doc.Bindings {
		if doc.Bindings[i].Name == localName || doc.Bindings[i].Name == bindingName {
			return &doc.Bindings[i]
		}
	}
	return nil
}

func (p *Parser) findPortType(doc *pkgWsdl.WSDLDocument, portTypeName string) *pkgWsdl.PortType {
	localName := p.extractLocalName(portTypeName)
	for i := range doc.PortTypes {
		if doc.PortTypes[i].Name == localName || doc.PortTypes[i].Name == portTypeName {
			return &doc.PortTypes[i]
		}
	}
	return nil
}

func (p *Parser) findOperation(portType *pkgWsdl.PortType, opName string) *pkgWsdl.Operation {
	for i := range portType.Operations {
		if portType.Operations[i].Name == opName {
			return &portType.Operations[i]
		}
	}
	return nil
}

func (p *Parser) findMessage(doc *pkgWsdl.WSDLDocument, messageName string) *pkgWsdl.Message {
	localName := p.extractLocalName(messageName)
	for i := range doc.Messages {
		if doc.Messages[i].Name == localName || doc.Messages[i].Name == messageName {
			return &doc.Messages[i]
		}
	}
	return nil
}

func (p *Parser) findElement(doc *pkgWsdl.WSDLDocument, elementName string) *pkgWsdl.XSDElement {
	if doc.TypeRegistry == nil {
		return nil
	}

	localName := p.extractLocalName(elementName)
	if elem, ok := doc.TypeRegistry.Elements[localName]; ok {
		return elem
	}
	if elem, ok := doc.TypeRegistry.Elements[elementName]; ok {
		return elem
	}
	return nil
}

func ParseFromRawDefinition(rawDefinition []byte, sourceURL string) ([]core.Operation, error) {
	parser := NewParser()

	tempDef := &db.APIDefinition{
		Type:          db.APIDefinitionTypeWSDL,
		RawDefinition: rawDefinition,
		SourceURL:     sourceURL,
	}

	return parser.Parse(tempDef)
}
