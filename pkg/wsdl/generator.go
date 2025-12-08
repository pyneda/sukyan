package wsdl

import (
	"fmt"
	"strings"
)

// Generator creates SOAP requests from parsed WSDL
type Generator struct {
	document *WSDLDocument
	config   GenerationConfig
	strategy *DefaultValueStrategy
}

// NewGenerator creates a new SOAP request generator
func NewGenerator(doc *WSDLDocument, config GenerationConfig) *Generator {
	return &Generator{
		document: doc,
		config:   config,
		strategy: NewDefaultValueStrategy(),
	}
}

// GenerateRequests generates all service endpoints with SOAP requests
func (g *Generator) GenerateRequests() ([]ServiceEndpoint, error) {
	var endpoints []ServiceEndpoint

	for _, service := range g.document.Services {
		for _, port := range service.Ports {
			endpoint, err := g.generateServiceEndpoint(service, port)
			if err != nil {
				// Log but continue with other ports
				continue
			}
			endpoints = append(endpoints, endpoint)
		}
	}

	return endpoints, nil
}

// generateServiceEndpoint creates an endpoint for a service port
func (g *Generator) generateServiceEndpoint(service Service, port Port) (ServiceEndpoint, error) {
	// Find the binding
	binding := g.findBinding(port.Binding)
	if binding == nil {
		return ServiceEndpoint{}, fmt.Errorf("binding not found: %s", port.Binding)
	}

	// Find the portType
	portType := g.findPortType(binding.Type)
	if portType == nil {
		return ServiceEndpoint{}, fmt.Errorf("portType not found: %s", binding.Type)
	}

	// Determine SOAP version
	soapVersion := port.SOAPVersion
	if soapVersion == "" {
		soapVersion = binding.SOAPVersion
	}
	if soapVersion == "" {
		soapVersion = "1.1"
	}

	endpoint := ServiceEndpoint{
		ServiceName:  service.Name,
		PortName:     port.Name,
		Address:      g.resolveAddress(port.Address),
		SOAPVersion:  soapVersion,
		BindingStyle: binding.Style,
		Operations:   make([]OperationEndpoint, 0),
	}

	// Generate operations
	for _, bindingOp := range binding.Operations {
		// Find corresponding abstract operation
		abstractOp := g.findOperation(portType, bindingOp.Name)
		if abstractOp == nil {
			continue
		}

		opEndpoint := g.generateOperationEndpoint(bindingOp, abstractOp, endpoint)
		endpoint.Operations = append(endpoint.Operations, opEndpoint)
	}

	return endpoint, nil
}

// generateOperationEndpoint creates an operation endpoint with request
func (g *Generator) generateOperationEndpoint(bindingOp BindingOperation, abstractOp *Operation, service ServiceEndpoint) OperationEndpoint {
	style := g.getOperationStyle(bindingOp, service.BindingStyle)

	opEndpoint := OperationEndpoint{
		Name:       bindingOp.Name,
		SOAPAction: bindingOp.SOAPAction,
		Style:      style,
		InputParts: g.extractPartMetadata(abstractOp.Input),
		Requests:   make([]RequestVariation, 0),
	}

	// Extract output parts if present
	if abstractOp.Output != nil {
		opEndpoint.OutputParts = g.extractPartMetadata(abstractOp.Output)
	}

	// Generate happy path request
	request := g.generateSOAPRequest(service, bindingOp, abstractOp, style)
	opEndpoint.Requests = append(opEndpoint.Requests, request)

	return opEndpoint
}

// generateSOAPRequest creates a SOAP envelope request
func (g *Generator) generateSOAPRequest(service ServiceEndpoint, bindingOp BindingOperation, abstractOp *Operation, style string) RequestVariation {
	// Build headers
	headers := map[string]string{
		"Content-Type": GetSOAPContentType(service.SOAPVersion),
	}

	// Add SOAPAction header (required for SOAP 1.1, optional for 1.2)
	if bindingOp.SOAPAction != "" {
		if service.SOAPVersion == "1.2" {
			// For SOAP 1.2, SOAPAction goes in Content-Type header
			headers["Content-Type"] = fmt.Sprintf("%s; action=\"%s\"", headers["Content-Type"], bindingOp.SOAPAction)
		} else {
			headers["SOAPAction"] = fmt.Sprintf("\"%s\"", bindingOp.SOAPAction)
		}
	} else if service.SOAPVersion == "1.1" {
		// SOAP 1.1 requires SOAPAction header even if empty
		headers["SOAPAction"] = "\"\""
	}

	// Copy custom headers
	for k, v := range g.config.Headers {
		headers[k] = v
	}

	// Generate SOAP envelope body
	body := g.buildSOAPEnvelope(service, bindingOp, abstractOp, style)

	return RequestVariation{
		Label:       "Happy Path",
		URL:         service.Address,
		Headers:     headers,
		Body:        body,
		Description: fmt.Sprintf("SOAP %s %s request for %s operation", service.SOAPVersion, style, bindingOp.Name),
	}
}

// buildSOAPEnvelope constructs the XML SOAP envelope
func (g *Generator) buildSOAPEnvelope(service ServiceEndpoint, bindingOp BindingOperation, abstractOp *Operation, style string) string {
	var builder strings.Builder

	soapNS := GetSOAPEnvelopeNamespace(service.SOAPVersion)
	targetNS := g.document.TargetNamespace

	// XML declaration
	builder.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")

	// SOAP envelope
	builder.WriteString("<soap:Envelope xmlns:soap=\"")
	builder.WriteString(soapNS)
	builder.WriteString("\"")
	if targetNS != "" {
		builder.WriteString(" xmlns:tns=\"")
		builder.WriteString(targetNS)
		builder.WriteString("\"")
	}
	builder.WriteString(">\n")

	// SOAP header (empty for now)
	builder.WriteString("  <soap:Header/>\n")

	// SOAP body
	builder.WriteString("  <soap:Body>\n")
	builder.WriteString(g.buildBodyContent(bindingOp, abstractOp, style, targetNS))
	builder.WriteString("  </soap:Body>\n")

	builder.WriteString("</soap:Envelope>")

	return builder.String()
}

// buildBodyContent generates the SOAP body based on operation style
func (g *Generator) buildBodyContent(bindingOp BindingOperation, abstractOp *Operation, style string, targetNS string) string {
	if abstractOp.Input == nil {
		return ""
	}

	// Find the input message
	inputMsg := g.findMessage(abstractOp.Input.Message)
	if inputMsg == nil {
		return ""
	}

	var builder strings.Builder
	indent := "    "

	if style == "rpc" {
		// RPC style: wrap parts in operation element
		builder.WriteString(indent)
		builder.WriteString("<tns:")
		builder.WriteString(bindingOp.Name)
		builder.WriteString(">\n")

		for _, part := range inputMsg.Parts {
			partXML := g.strategy.GenerateXMLForMessagePart(&part, g.document.TypeRegistry, indent+"  ", targetNS)
			builder.WriteString(partXML)
		}

		builder.WriteString(indent)
		builder.WriteString("</tns:")
		builder.WriteString(bindingOp.Name)
		builder.WriteString(">\n")
	} else {
		// Document style: parts directly in body
		for _, part := range inputMsg.Parts {
			partXML := g.strategy.GenerateXMLForMessagePart(&part, g.document.TypeRegistry, indent, targetNS)
			builder.WriteString(partXML)
		}
	}

	return builder.String()
}

// extractPartMetadata extracts metadata for message parts
func (g *Generator) extractPartMetadata(ioRef *IORef) []PartMetadata {
	if ioRef == nil {
		return nil
	}

	msg := g.findMessage(ioRef.Message)
	if msg == nil {
		return nil
	}

	var parts []PartMetadata
	for _, part := range msg.Parts {
		metadata := PartMetadata{
			Name:     part.Name,
			Required: true, // Message parts are typically required
		}

		if part.Element != "" {
			metadata.ElementName = part.Element
			elemName := ExtractLocalName(part.Element)

			// Look up element in registry
			if elem, ok := g.document.TypeRegistry.Elements[elemName]; ok {
				metadata.NestedFields = g.extractElementMetadata(elem, 0)
				metadata.IsComplex = elem.ComplexType != nil || g.isComplexType(elem.Type)
			}
		} else if part.Type != "" {
			metadata.TypeName = part.Type
			typeName := ExtractLocalName(part.Type)
			metadata.IsComplex = g.isComplexType(typeName)

			// Look up complex type in registry
			if ct, ok := g.document.TypeRegistry.ComplexTypes[typeName]; ok {
				metadata.NestedFields = g.extractComplexTypeMetadata(ct, 0)
			}
		}

		parts = append(parts, metadata)
	}

	return parts
}

// extractElementMetadata extracts metadata for an element
func (g *Generator) extractElementMetadata(elem *XSDElement, depth int) []PartMetadata {
	if elem == nil || depth > 5 {
		return nil
	}

	var fields []PartMetadata

	if elem.ComplexType != nil {
		fields = g.extractComplexTypeMetadata(elem.ComplexType, depth)
	} else if elem.Type != "" {
		typeName := ExtractLocalName(elem.Type)
		if ct, ok := g.document.TypeRegistry.ComplexTypes[typeName]; ok {
			fields = g.extractComplexTypeMetadata(ct, depth)
		}
	}

	return fields
}

// extractComplexTypeMetadata extracts metadata for a complex type
func (g *Generator) extractComplexTypeMetadata(ct *XSDComplexType, depth int) []PartMetadata {
	if ct == nil || depth > 5 {
		return nil
	}

	var fields []PartMetadata

	// Handle sequence
	if ct.Sequence != nil {
		for _, elem := range ct.Sequence.Elements {
			field := g.createFieldMetadata(&elem, depth)
			fields = append(fields, field)
		}
	}

	// Handle all
	if ct.All != nil {
		for _, elem := range ct.All.Elements {
			field := g.createFieldMetadata(&elem, depth)
			fields = append(fields, field)
		}
	}

	// Handle choice
	if ct.Choice != nil {
		for _, elem := range ct.Choice.Elements {
			field := g.createFieldMetadata(&elem, depth)
			field.Required = false // Choice elements are optional
			fields = append(fields, field)
		}
	}

	// Handle complexContent extension
	if ct.ComplexContent != nil && ct.ComplexContent.Extension != nil {
		// Get base type fields
		baseName := ExtractLocalName(ct.ComplexContent.Extension.Base)
		if baseCT, ok := g.document.TypeRegistry.ComplexTypes[baseName]; ok {
			baseFields := g.extractComplexTypeMetadata(baseCT, depth+1)
			fields = append(baseFields, fields...)
		}

		// Add extension fields
		if ct.ComplexContent.Extension.Sequence != nil {
			for _, elem := range ct.ComplexContent.Extension.Sequence.Elements {
				field := g.createFieldMetadata(&elem, depth)
				fields = append(fields, field)
			}
		}
	}

	return fields
}

// createFieldMetadata creates metadata for an element field
func (g *Generator) createFieldMetadata(elem *XSDElement, depth int) PartMetadata {
	field := PartMetadata{
		Name:     elem.Name,
		TypeName: elem.Type,
		Required: elem.MinOccurs != "0",
	}

	// Handle element reference
	if elem.Ref != "" {
		refName := ExtractLocalName(elem.Ref)
		field.Name = refName
		if refElem, ok := g.document.TypeRegistry.Elements[refName]; ok {
			field.TypeName = refElem.Type
			field.IsComplex = refElem.ComplexType != nil || g.isComplexType(refElem.Type)
			if field.IsComplex && depth < 5 {
				field.NestedFields = g.extractElementMetadata(refElem, depth+1)
			}
		}
		return field
	}

	// Check if complex type
	if elem.ComplexType != nil {
		field.IsComplex = true
		if depth < 5 {
			field.NestedFields = g.extractComplexTypeMetadata(elem.ComplexType, depth+1)
		}
	} else if elem.Type != "" {
		typeName := ExtractLocalName(elem.Type)
		field.IsComplex = g.isComplexType(typeName)
		if field.IsComplex && depth < 5 {
			if ct, ok := g.document.TypeRegistry.ComplexTypes[typeName]; ok {
				field.NestedFields = g.extractComplexTypeMetadata(ct, depth+1)
			}
		}
	}

	return field
}

// Helper methods

func (g *Generator) findBinding(bindingRef string) *Binding {
	bindingName := ExtractLocalName(bindingRef)
	for i := range g.document.Bindings {
		if g.document.Bindings[i].Name == bindingName {
			return &g.document.Bindings[i]
		}
	}
	return nil
}

func (g *Generator) findPortType(portTypeRef string) *PortType {
	portTypeName := ExtractLocalName(portTypeRef)
	for i := range g.document.PortTypes {
		if g.document.PortTypes[i].Name == portTypeName {
			return &g.document.PortTypes[i]
		}
	}
	return nil
}

func (g *Generator) findOperation(portType *PortType, opName string) *Operation {
	for i := range portType.Operations {
		if portType.Operations[i].Name == opName {
			return &portType.Operations[i]
		}
	}
	return nil
}

func (g *Generator) findMessage(messageRef string) *Message {
	messageName := ExtractLocalName(messageRef)
	if msg, ok := g.document.TypeRegistry.Messages[messageName]; ok {
		return msg
	}
	if msg, ok := g.document.TypeRegistry.Messages[messageRef]; ok {
		return msg
	}
	// Fallback to linear search
	for i := range g.document.Messages {
		if g.document.Messages[i].Name == messageName {
			return &g.document.Messages[i]
		}
	}
	return nil
}

func (g *Generator) getOperationStyle(bindingOp BindingOperation, defaultStyle string) string {
	if bindingOp.Style != "" {
		return bindingOp.Style
	}
	if defaultStyle != "" {
		return defaultStyle
	}
	return "document"
}

func (g *Generator) resolveAddress(address string) string {
	if g.config.BaseURL != "" {
		return g.config.BaseURL
	}
	return address
}

func (g *Generator) isComplexType(typeName string) bool {
	if typeName == "" {
		return false
	}
	localName := ExtractLocalName(typeName)
	if IsXSDBuiltinType(localName) {
		return false
	}
	_, found := g.document.TypeRegistry.ComplexTypes[localName]
	if !found {
		_, found = g.document.TypeRegistry.ComplexTypes[typeName]
	}
	return found
}
