package wsdl

import (
	"bytes"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Parser handles WSDL document parsing with import resolution
type Parser struct {
	client   *http.Client
	headers  map[string]string
	maxDepth int             // Max import recursion depth
	imported map[string]bool // Track imported URLs to prevent cycles
}

// NewParser creates a new WSDL parser
func NewParser() *Parser {
	return &Parser{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		headers:  make(map[string]string),
		maxDepth: 10,
		imported: make(map[string]bool),
	}
}

// WithHeaders sets custom headers for the parser
func (p *Parser) WithHeaders(headers map[string]string) *Parser {
	p.headers = headers
	return p
}

// WithClient sets a custom HTTP client
func (p *Parser) WithClient(client *http.Client) *Parser {
	p.client = client
	return p
}

// WithMaxDepth sets the maximum import recursion depth
func (p *Parser) WithMaxDepth(depth int) *Parser {
	p.maxDepth = depth
	return p
}

// ParseFromURL fetches and parses a WSDL from a URL
func (p *Parser) ParseFromURL(url string) (*WSDLDocument, error) {
	// Reset imported tracker for new parse
	p.imported = make(map[string]bool)
	p.imported[url] = true

	data, err := p.fetchDocument(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch WSDL: %w", err)
	}

	return p.ParseFromBytes(data, url)
}

// ParseFromBytes parses WSDL from byte array
func (p *Parser) ParseFromBytes(data []byte, sourceURL string) (*WSDLDocument, error) {
	// Parse raw XML
	var rawWSDL rawDefinitions
	if err := xml.Unmarshal(data, &rawWSDL); err != nil {
		return nil, fmt.Errorf("failed to parse WSDL XML: %w", err)
	}

	// Build namespace map from the document
	namespaces := p.extractNamespaces(data)

	// Convert to domain model
	doc, err := p.convertRawWSDL(&rawWSDL, namespaces)
	if err != nil {
		return nil, fmt.Errorf("failed to convert WSDL: %w", err)
	}

	// Resolve imports
	if err := p.resolveImports(doc, sourceURL, 0, namespaces); err != nil {
		return nil, fmt.Errorf("failed to resolve imports: %w", err)
	}

	// Build type registry
	doc.TypeRegistry = p.buildTypeRegistry(doc)

	return doc, nil
}

// fetchDocument retrieves a document from URL
func (p *Parser) fetchDocument(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "text/xml, application/xml, application/wsdl+xml")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3")

	for key, value := range p.headers {
		req.Header.Set(key, value)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return io.ReadAll(resp.Body)
}

// extractNamespaces parses XML to extract namespace declarations
func (p *Parser) extractNamespaces(data []byte) *NamespaceMap {
	nsMap := NewNamespaceMap()
	decoder := xml.NewDecoder(bytes.NewReader(data))

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			for _, attr := range t.Attr {
				if attr.Name.Space == "xmlns" || attr.Name.Local == "xmlns" {
					prefix := ""
					if attr.Name.Space == "xmlns" {
						prefix = attr.Name.Local
					}
					nsMap.Add(prefix, attr.Value)
				}
			}
			// Only need namespaces from root element for most cases
			return nsMap
		}
	}

	return nsMap
}

// convertRawWSDL converts raw XML structures to domain model
func (p *Parser) convertRawWSDL(raw *rawDefinitions, namespaces *NamespaceMap) (*WSDLDocument, error) {
	doc := &WSDLDocument{
		TargetNamespace: raw.TargetNamespace,
		Name:            raw.Name,
		Messages:        make([]Message, 0, len(raw.Messages)),
		PortTypes:       make([]PortType, 0, len(raw.PortTypes)),
		Bindings:        make([]Binding, 0, len(raw.Bindings)),
		Services:        make([]Service, 0, len(raw.Services)),
		Imports:         make([]WSDLImport, 0, len(raw.Imports)),
	}

	// Convert imports
	for _, imp := range raw.Imports {
		doc.Imports = append(doc.Imports, WSDLImport{
			Namespace: imp.Namespace,
			Location:  imp.Location,
		})
	}

	// Convert types
	if raw.Types != nil {
		doc.Types = &Types{
			Schemas: make([]XSDSchema, 0),
		}
		for _, schema := range raw.Types.Schemas {
			converted := p.convertRawSchema(&schema, namespaces)
			doc.Types.Schemas = append(doc.Types.Schemas, converted)
		}
	}

	// Convert messages
	for _, msg := range raw.Messages {
		converted := p.convertRawMessage(&msg)
		doc.Messages = append(doc.Messages, converted)
	}

	// Convert port types
	for _, pt := range raw.PortTypes {
		converted := p.convertRawPortType(&pt)
		doc.PortTypes = append(doc.PortTypes, converted)
	}

	// Convert bindings
	for _, binding := range raw.Bindings {
		converted := p.convertRawBinding(&binding, namespaces)
		doc.Bindings = append(doc.Bindings, converted)
	}

	// Convert services
	for _, svc := range raw.Services {
		converted := p.convertRawService(&svc, namespaces)
		doc.Services = append(doc.Services, converted)
	}

	return doc, nil
}

// convertRawSchema converts raw XSD schema to domain model
func (p *Parser) convertRawSchema(raw *rawSchema, namespaces *NamespaceMap) XSDSchema {
	schema := XSDSchema{
		TargetNamespace:    raw.TargetNamespace,
		ElementFormDefault: raw.ElementFormDefault,
		Imports:            make([]XSDImport, 0, len(raw.Imports)),
		Includes:           make([]XSDInclude, 0, len(raw.Includes)),
		Elements:           make([]XSDElement, 0, len(raw.Elements)),
		ComplexTypes:       make([]XSDComplexType, 0, len(raw.ComplexTypes)),
		SimpleTypes:        make([]XSDSimpleType, 0, len(raw.SimpleTypes)),
	}

	for _, imp := range raw.Imports {
		schema.Imports = append(schema.Imports, XSDImport{
			Namespace:      imp.Namespace,
			SchemaLocation: imp.SchemaLocation,
		})
	}

	for _, inc := range raw.Includes {
		schema.Includes = append(schema.Includes, XSDInclude{
			SchemaLocation: inc.SchemaLocation,
		})
	}

	for _, elem := range raw.Elements {
		schema.Elements = append(schema.Elements, p.convertRawElement(&elem, schema.TargetNamespace))
	}

	for _, ct := range raw.ComplexTypes {
		schema.ComplexTypes = append(schema.ComplexTypes, p.convertRawComplexType(&ct))
	}

	for _, st := range raw.SimpleTypes {
		schema.SimpleTypes = append(schema.SimpleTypes, p.convertRawSimpleType(&st))
	}

	return schema
}

// convertRawElement converts raw XSD element to domain model
func (p *Parser) convertRawElement(raw *rawElement, targetNS string) XSDElement {
	elem := XSDElement{
		Name:            raw.Name,
		Type:            raw.Type,
		Ref:             raw.Ref,
		MinOccurs:       raw.MinOccurs,
		MaxOccurs:       raw.MaxOccurs,
		Nillable:        raw.Nillable,
		Default:         raw.Default,
		Fixed:           raw.Fixed,
		TargetNamespace: targetNS,
	}

	if raw.ComplexType != nil {
		ct := p.convertRawComplexType(raw.ComplexType)
		elem.ComplexType = &ct
	}

	if raw.SimpleType != nil {
		st := p.convertRawSimpleType(raw.SimpleType)
		elem.SimpleType = &st
	}

	return elem
}

// convertRawComplexType converts raw XSD complex type to domain model
func (p *Parser) convertRawComplexType(raw *rawComplexType) XSDComplexType {
	ct := XSDComplexType{
		Name:     raw.Name,
		Abstract: raw.Abstract,
		Mixed:    raw.Mixed,
	}

	if raw.Sequence != nil {
		seq := p.convertRawSequence(raw.Sequence)
		ct.Sequence = &seq
	}

	if raw.All != nil {
		all := p.convertRawAll(raw.All)
		ct.All = &all
	}

	if raw.Choice != nil {
		choice := p.convertRawChoice(raw.Choice)
		ct.Choice = &choice
	}

	if raw.ComplexContent != nil {
		cc := p.convertRawComplexContent(raw.ComplexContent)
		ct.ComplexContent = &cc
	}

	if raw.SimpleContent != nil {
		sc := p.convertRawSimpleContent(raw.SimpleContent)
		ct.SimpleContent = &sc
	}

	for _, attr := range raw.Attributes {
		ct.Attributes = append(ct.Attributes, p.convertRawAttribute(&attr))
	}

	return ct
}

// convertRawSequence converts raw XSD sequence to domain model
func (p *Parser) convertRawSequence(raw *rawSequence) XSDSequence {
	seq := XSDSequence{
		MinOccurs: raw.MinOccurs,
		MaxOccurs: raw.MaxOccurs,
		Elements:  make([]XSDElement, 0, len(raw.Elements)),
	}

	for _, elem := range raw.Elements {
		seq.Elements = append(seq.Elements, p.convertRawElement(&elem, ""))
	}

	for _, choice := range raw.Choices {
		c := p.convertRawChoice(&choice)
		seq.Choices = append(seq.Choices, c)
	}

	return seq
}

// convertRawAll converts raw XSD all to domain model
func (p *Parser) convertRawAll(raw *rawAll) XSDAll {
	all := XSDAll{
		MinOccurs: raw.MinOccurs,
		MaxOccurs: raw.MaxOccurs,
		Elements:  make([]XSDElement, 0, len(raw.Elements)),
	}

	for _, elem := range raw.Elements {
		all.Elements = append(all.Elements, p.convertRawElement(&elem, ""))
	}

	return all
}

// convertRawChoice converts raw XSD choice to domain model
func (p *Parser) convertRawChoice(raw *rawChoice) XSDChoice {
	choice := XSDChoice{
		MinOccurs: raw.MinOccurs,
		MaxOccurs: raw.MaxOccurs,
		Elements:  make([]XSDElement, 0, len(raw.Elements)),
	}

	for _, elem := range raw.Elements {
		choice.Elements = append(choice.Elements, p.convertRawElement(&elem, ""))
	}

	return choice
}

// convertRawComplexContent converts raw complex content to domain model
func (p *Parser) convertRawComplexContent(raw *rawComplexContent) XSDComplexContent {
	cc := XSDComplexContent{
		Mixed: raw.Mixed,
	}

	if raw.Extension != nil {
		ext := p.convertRawExtension(raw.Extension)
		cc.Extension = &ext
	}

	if raw.Restriction != nil {
		rest := p.convertRawRestriction(raw.Restriction)
		cc.Restriction = &rest
	}

	return cc
}

// convertRawSimpleContent converts raw simple content to domain model
func (p *Parser) convertRawSimpleContent(raw *rawSimpleContent) XSDSimpleContent {
	sc := XSDSimpleContent{}

	if raw.Extension != nil {
		ext := p.convertRawExtension(raw.Extension)
		sc.Extension = &ext
	}

	if raw.Restriction != nil {
		rest := p.convertRawRestriction(raw.Restriction)
		sc.Restriction = &rest
	}

	return sc
}

// convertRawExtension converts raw extension to domain model
func (p *Parser) convertRawExtension(raw *rawExtension) XSDExtension {
	ext := XSDExtension{
		Base: raw.Base,
	}

	if raw.Sequence != nil {
		seq := p.convertRawSequence(raw.Sequence)
		ext.Sequence = &seq
	}

	if raw.All != nil {
		all := p.convertRawAll(raw.All)
		ext.All = &all
	}

	if raw.Choice != nil {
		choice := p.convertRawChoice(raw.Choice)
		ext.Choice = &choice
	}

	for _, attr := range raw.Attributes {
		ext.Attributes = append(ext.Attributes, p.convertRawAttribute(&attr))
	}

	return ext
}

// convertRawRestriction converts raw restriction to domain model
func (p *Parser) convertRawRestriction(raw *rawRestriction) XSDRestriction {
	rest := XSDRestriction{
		Base:         raw.Base,
		Pattern:      raw.Pattern,
		WhiteSpace:   raw.WhiteSpace,
		MinInclusive: raw.MinInclusive,
		MaxInclusive: raw.MaxInclusive,
		MinExclusive: raw.MinExclusive,
		MaxExclusive: raw.MaxExclusive,
	}

	for _, enum := range raw.Enumeration {
		rest.Enumeration = append(rest.Enumeration, enum.Value)
	}

	if raw.MinLength != nil {
		rest.MinLength = raw.MinLength
	}
	if raw.MaxLength != nil {
		rest.MaxLength = raw.MaxLength
	}
	if raw.Length != nil {
		rest.Length = raw.Length
	}
	if raw.TotalDigits != nil {
		rest.TotalDigits = raw.TotalDigits
	}
	if raw.FractionDigits != nil {
		rest.FractionDigits = raw.FractionDigits
	}

	if raw.Sequence != nil {
		seq := p.convertRawSequence(raw.Sequence)
		rest.Sequence = &seq
	}

	for _, attr := range raw.Attributes {
		rest.Attributes = append(rest.Attributes, p.convertRawAttribute(&attr))
	}

	return rest
}

// convertRawSimpleType converts raw simple type to domain model
func (p *Parser) convertRawSimpleType(raw *rawSimpleType) XSDSimpleType {
	st := XSDSimpleType{
		Name: raw.Name,
	}

	if raw.Restriction != nil {
		rest := p.convertRawRestriction(raw.Restriction)
		st.Restriction = &rest
	}

	if raw.List != nil {
		st.List = &XSDList{
			ItemType: raw.List.ItemType,
		}
	}

	if raw.Union != nil {
		st.Union = &XSDUnion{
			MemberTypes: raw.Union.MemberTypes,
		}
	}

	return st
}

// convertRawAttribute converts raw attribute to domain model
func (p *Parser) convertRawAttribute(raw *rawAttribute) XSDAttribute {
	return XSDAttribute{
		Name:    raw.Name,
		Ref:     raw.Ref,
		Type:    raw.Type,
		Use:     raw.Use,
		Default: raw.Default,
		Fixed:   raw.Fixed,
		Form:    raw.Form,
	}
}

// convertRawMessage converts raw message to domain model
func (p *Parser) convertRawMessage(raw *rawMessage) Message {
	msg := Message{
		Name:          raw.Name,
		Documentation: extractDocumentation(raw.Documentation),
		Parts:         make([]MessagePart, 0, len(raw.Parts)),
	}

	for _, part := range raw.Parts {
		msg.Parts = append(msg.Parts, MessagePart{
			Name:    part.Name,
			Element: part.Element,
			Type:    part.Type,
		})
	}

	return msg
}

// convertRawPortType converts raw port type to domain model
func (p *Parser) convertRawPortType(raw *rawPortType) PortType {
	pt := PortType{
		Name:          raw.Name,
		Documentation: extractDocumentation(raw.Documentation),
		Operations:    make([]Operation, 0, len(raw.Operations)),
	}

	for _, op := range raw.Operations {
		pt.Operations = append(pt.Operations, p.convertRawOperation(&op))
	}

	return pt
}

// convertRawOperation converts raw operation to domain model
func (p *Parser) convertRawOperation(raw *rawOperation) Operation {
	op := Operation{
		Name:          raw.Name,
		Documentation: extractDocumentation(raw.Documentation),
	}

	if raw.Input != nil {
		op.Input = &IORef{
			Name:    raw.Input.Name,
			Message: raw.Input.Message,
		}
	}

	if raw.Output != nil {
		op.Output = &IORef{
			Name:    raw.Output.Name,
			Message: raw.Output.Message,
		}
	}

	for _, fault := range raw.Faults {
		op.Faults = append(op.Faults, IORef{
			Name:    fault.Name,
			Message: fault.Message,
		})
	}

	return op
}

// convertRawBinding converts raw binding to domain model
func (p *Parser) convertRawBinding(raw *rawBinding, namespaces *NamespaceMap) Binding {
	binding := Binding{
		Name:       raw.Name,
		Type:       raw.Type,
		Operations: make([]BindingOperation, 0, len(raw.Operations)),
	}

	// Extract SOAP binding info
	if raw.SOAPBinding != nil {
		binding.Style = raw.SOAPBinding.Style
		binding.Transport = raw.SOAPBinding.Transport
		binding.SOAPVersion = "1.1"
	}
	if raw.SOAP12Binding != nil {
		binding.Style = raw.SOAP12Binding.Style
		binding.Transport = raw.SOAP12Binding.Transport
		binding.SOAPVersion = "1.2"
	}

	for _, op := range raw.Operations {
		binding.Operations = append(binding.Operations, p.convertRawBindingOperation(&op))
	}

	return binding
}

// convertRawBindingOperation converts raw binding operation to domain model
func (p *Parser) convertRawBindingOperation(raw *rawBindingOperation) BindingOperation {
	op := BindingOperation{
		Name: raw.Name,
	}

	// Extract SOAP operation info
	if raw.SOAPOperation != nil {
		op.SOAPAction = raw.SOAPOperation.SOAPAction
		op.Style = raw.SOAPOperation.Style
	}
	if raw.SOAP12Operation != nil {
		op.SOAPAction = raw.SOAP12Operation.SOAPAction
		op.Style = raw.SOAP12Operation.Style
	}

	if raw.Input != nil {
		op.Input = &BindingIO{}
		if raw.Input.SOAPBody != nil {
			op.Input.Use = raw.Input.SOAPBody.Use
			op.Input.Namespace = raw.Input.SOAPBody.Namespace
			op.Input.EncodingStyle = raw.Input.SOAPBody.EncodingStyle
		}
		if raw.Input.SOAP12Body != nil {
			op.Input.Use = raw.Input.SOAP12Body.Use
			op.Input.Namespace = raw.Input.SOAP12Body.Namespace
			op.Input.EncodingStyle = raw.Input.SOAP12Body.EncodingStyle
		}
	}

	if raw.Output != nil {
		op.Output = &BindingIO{}
		if raw.Output.SOAPBody != nil {
			op.Output.Use = raw.Output.SOAPBody.Use
			op.Output.Namespace = raw.Output.SOAPBody.Namespace
			op.Output.EncodingStyle = raw.Output.SOAPBody.EncodingStyle
		}
		if raw.Output.SOAP12Body != nil {
			op.Output.Use = raw.Output.SOAP12Body.Use
			op.Output.Namespace = raw.Output.SOAP12Body.Namespace
			op.Output.EncodingStyle = raw.Output.SOAP12Body.EncodingStyle
		}
	}

	return op
}

// convertRawService converts raw service to domain model
func (p *Parser) convertRawService(raw *rawService, namespaces *NamespaceMap) Service {
	svc := Service{
		Name:          raw.Name,
		Documentation: extractDocumentation(raw.Documentation),
		Ports:         make([]Port, 0, len(raw.Ports)),
	}

	for _, port := range raw.Ports {
		p := Port{
			Name:    port.Name,
			Binding: port.Binding,
		}

		// Extract SOAP address
		if port.SOAPAddress != nil {
			p.Address = port.SOAPAddress.Location
			p.SOAPVersion = "1.1"
		}
		if port.SOAP12Address != nil {
			p.Address = port.SOAP12Address.Location
			p.SOAPVersion = "1.2"
		}

		svc.Ports = append(svc.Ports, p)
	}

	return svc
}

// resolveImports recursively fetches and merges imported WSDLs and XSDs
func (p *Parser) resolveImports(doc *WSDLDocument, sourceURL string, depth int, namespaces *NamespaceMap) error {
	if depth > p.maxDepth {
		return fmt.Errorf("max import depth exceeded (%d)", p.maxDepth)
	}

	baseURL := ExtractDirectoryURL(sourceURL)

	// Resolve WSDL imports
	for _, imp := range doc.Imports {
		location := imp.Location
		if location == "" {
			continue
		}

		resolvedURL := ResolveURL(baseURL, location)
		if p.imported[resolvedURL] {
			continue // Already imported
		}
		p.imported[resolvedURL] = true

		importedData, err := p.fetchDocument(resolvedURL)
		if err != nil {
			// Log warning but continue - some imports may be optional
			continue
		}

		importedDoc, err := p.ParseFromBytes(importedData, resolvedURL)
		if err != nil {
			continue
		}

		// Merge imported document
		p.mergeWSDL(doc, importedDoc)
	}

	// Resolve XSD imports within Types
	if doc.Types != nil {
		for i := range doc.Types.Schemas {
			if err := p.resolveXSDImports(&doc.Types.Schemas[i], baseURL, depth+1); err != nil {
				// Log but continue
				continue
			}
		}
	}

	return nil
}

// resolveXSDImports handles xsd:import and xsd:include
func (p *Parser) resolveXSDImports(schema *XSDSchema, baseURL string, depth int) error {
	if depth > p.maxDepth {
		return nil
	}

	// Process xsd:import
	for _, imp := range schema.Imports {
		if imp.SchemaLocation == "" {
			continue
		}

		resolvedURL := ResolveURL(baseURL, imp.SchemaLocation)
		if p.imported[resolvedURL] {
			continue
		}
		p.imported[resolvedURL] = true

		xsdData, err := p.fetchDocument(resolvedURL)
		if err != nil {
			continue
		}

		importedSchema, err := p.parseXSDSchema(xsdData)
		if err != nil {
			continue
		}

		// Recursively resolve nested imports
		schemaBaseURL := ExtractDirectoryURL(resolvedURL)
		if err := p.resolveXSDImports(importedSchema, schemaBaseURL, depth+1); err != nil {
			continue
		}

		// Merge into current schema
		p.mergeXSDSchema(schema, importedSchema)
	}

	// Process xsd:include (same namespace)
	for _, inc := range schema.Includes {
		if inc.SchemaLocation == "" {
			continue
		}

		resolvedURL := ResolveURL(baseURL, inc.SchemaLocation)
		if p.imported[resolvedURL] {
			continue
		}
		p.imported[resolvedURL] = true

		xsdData, err := p.fetchDocument(resolvedURL)
		if err != nil {
			continue
		}

		includedSchema, err := p.parseXSDSchema(xsdData)
		if err != nil {
			continue
		}

		// Recursively resolve nested includes
		schemaBaseURL := ExtractDirectoryURL(resolvedURL)
		if err := p.resolveXSDImports(includedSchema, schemaBaseURL, depth+1); err != nil {
			continue
		}

		// Merge into current schema
		p.mergeXSDSchema(schema, includedSchema)
	}

	return nil
}

// parseXSDSchema parses a standalone XSD schema
func (p *Parser) parseXSDSchema(data []byte) (*XSDSchema, error) {
	var raw rawSchema
	if err := xml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse XSD: %w", err)
	}

	schema := p.convertRawSchema(&raw, NewNamespaceMap())
	return &schema, nil
}

// mergeWSDL merges imported WSDL into target
func (p *Parser) mergeWSDL(target, source *WSDLDocument) {
	target.Messages = append(target.Messages, source.Messages...)
	target.PortTypes = append(target.PortTypes, source.PortTypes...)
	target.Bindings = append(target.Bindings, source.Bindings...)
	target.Services = append(target.Services, source.Services...)

	if source.Types != nil {
		if target.Types == nil {
			target.Types = &Types{Schemas: make([]XSDSchema, 0)}
		}
		target.Types.Schemas = append(target.Types.Schemas, source.Types.Schemas...)
	}
}

// mergeXSDSchema merges imported XSD schema into target
func (p *Parser) mergeXSDSchema(target, source *XSDSchema) {
	target.Elements = append(target.Elements, source.Elements...)
	target.ComplexTypes = append(target.ComplexTypes, source.ComplexTypes...)
	target.SimpleTypes = append(target.SimpleTypes, source.SimpleTypes...)
}

// buildTypeRegistry builds a registry for quick type lookup
func (p *Parser) buildTypeRegistry(doc *WSDLDocument) *TypeRegistry {
	registry := NewTypeRegistry()

	// Register messages
	for i := range doc.Messages {
		msg := &doc.Messages[i]
		key := MakeTypeKey(doc.TargetNamespace, msg.Name)
		registry.Messages[key] = msg
		// Also register without namespace for simple lookup
		registry.Messages[msg.Name] = msg
	}

	// Register types from schemas
	if doc.Types != nil {
		for i := range doc.Types.Schemas {
			schema := &doc.Types.Schemas[i]
			ns := schema.TargetNamespace

			for j := range schema.Elements {
				elem := &schema.Elements[j]
				key := MakeTypeKey(ns, elem.Name)
				registry.Elements[key] = elem
				registry.Elements[elem.Name] = elem
			}

			for j := range schema.ComplexTypes {
				ct := &schema.ComplexTypes[j]
				if ct.Name != "" {
					key := MakeTypeKey(ns, ct.Name)
					registry.ComplexTypes[key] = ct
					registry.ComplexTypes[ct.Name] = ct
				}
			}

			for j := range schema.SimpleTypes {
				st := &schema.SimpleTypes[j]
				if st.Name != "" {
					key := MakeTypeKey(ns, st.Name)
					registry.SimpleTypes[key] = st
					registry.SimpleTypes[st.Name] = st
				}
			}
		}
	}

	return registry
}

// extractDocumentation extracts text from documentation element
func extractDocumentation(doc *rawDocumentation) string {
	if doc == nil {
		return ""
	}
	return strings.TrimSpace(doc.Content)
}
