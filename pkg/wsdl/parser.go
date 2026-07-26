package wsdl

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	DefaultMaxDocumentSize = 16 << 20
	DefaultMaxDocuments    = 50
	DefaultMaxDepth        = 10
	// DefaultMaxTotalDuration bounds a whole parse, imports included. Callers
	// that have no context to pass still get a worker that cannot be pinned
	// open by a target that stalls every import.
	DefaultMaxTotalDuration = 2 * time.Minute
)

// Parser handles WSDL document parsing with import resolution.
//
// A WSDL is fetched from a target under test, so its import and schemaLocation
// values are attacker-controlled. Traversal is therefore bounded in depth,
// document count and body size, restricted to http(s), and confined to the
// origin the document came from unless the caller opts out. Credentials
// supplied via WithHeaders are only ever sent to that same origin.
type Parser struct {
	client           *http.Client
	headers          map[string]string
	maxDepth         int
	maxDocuments     int
	maxDocumentSize  int64
	allowCrossOrigin bool
	maxTotalDuration time.Duration
	ctx              context.Context
	parseCtx         context.Context
	origin           string
	imported         map[string]bool
	documentsFetched int
}

// NewParser creates a new WSDL parser
func NewParser() *Parser {
	return &Parser{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		headers:          make(map[string]string),
		maxDepth:         DefaultMaxDepth,
		maxDocuments:     DefaultMaxDocuments,
		maxDocumentSize:  DefaultMaxDocumentSize,
		maxTotalDuration: DefaultMaxTotalDuration,
		imported:         make(map[string]bool),
	}
}

// WithContext ties the parse, including every import fetch, to a caller's
// context so a cancelled scan job stops the work immediately. A parse is
// bounded by DefaultMaxTotalDuration even without one.
func (p *Parser) WithContext(ctx context.Context) *Parser {
	p.ctx = ctx
	return p
}

// WithMaxTotalDuration bounds the wall-clock time of an entire parse.
func (p *Parser) WithMaxTotalDuration(d time.Duration) *Parser {
	p.maxTotalDuration = d
	return p
}

// WithMaxDocuments caps how many documents a single parse may fetch.
func (p *Parser) WithMaxDocuments(n int) *Parser {
	p.maxDocuments = n
	return p
}

// WithMaxDocumentSize caps the accepted size of any single fetched document.
func (p *Parser) WithMaxDocumentSize(n int64) *Parser {
	p.maxDocumentSize = n
	return p
}

// WithCrossOriginImports allows imports pointing at hosts other than the one
// the WSDL came from. Off by default because those locations are chosen by the
// document, not by us.
func (p *Parser) WithCrossOriginImports(allow bool) *Parser {
	p.allowCrossOrigin = allow
	return p
}

func sameOrigin(a, b string) bool {
	ua, err := url.Parse(a)
	if err != nil {
		return false
	}
	ub, err := url.Parse(b)
	if err != nil {
		return false
	}
	return strings.EqualFold(ua.Scheme, ub.Scheme) && strings.EqualFold(ua.Host, ub.Host)
}

// allowImport reports whether an import location may be fetched.
func (p *Parser) allowImport(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid import URL %q: %w", rawURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("refusing non-http import scheme %q", parsed.Scheme)
	}
	if !p.allowCrossOrigin {
		// Without a source URL there is no origin to compare against, so the
		// only safe reading of a document-chosen location is to refuse it.
		if p.origin == "" {
			return fmt.Errorf("refusing import %q: source document has no origin", rawURL)
		}
		if !sameOrigin(p.origin, rawURL) {
			return fmt.Errorf("refusing cross-origin import %q", rawURL)
		}
	}
	return nil
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
func (p *Parser) ParseFromURL(target string) (*WSDLDocument, error) {
	cancel := p.resetTraversal(target)
	defer cancel()
	p.imported[target] = true

	data, err := p.fetchDocument(target)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch WSDL: %w", err)
	}

	return p.parseDocument(data, target)
}

// ParseFromBytes parses WSDL from byte array
func (p *Parser) ParseFromBytes(data []byte, sourceURL string) (*WSDLDocument, error) {
	cancel := p.resetTraversal(sourceURL)
	defer cancel()
	return p.parseDocument(data, sourceURL)
}

// resetTraversal clears per-parse state and derives the deadline that bounds
// every fetch this parse makes. The returned func must be called when the
// parse finishes.
func (p *Parser) resetTraversal(sourceURL string) context.CancelFunc {
	p.imported = make(map[string]bool)
	p.documentsFetched = 0
	p.origin = sourceURL

	parent := p.ctx
	if parent == nil {
		parent = context.Background()
	}
	if p.maxTotalDuration <= 0 {
		p.parseCtx = parent
		return func() {}
	}

	ctx, cancel := context.WithTimeout(parent, p.maxTotalDuration)
	p.parseCtx = ctx
	return cancel
}

func (p *Parser) parseDocument(data []byte, sourceURL string) (*WSDLDocument, error) {
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

// fetchDocument retrieves a document from URL, bounded by the parser's
// document budget, size cap and context.
func (p *Parser) fetchDocument(target string) ([]byte, error) {
	if p.maxDocuments > 0 && p.documentsFetched >= p.maxDocuments {
		return nil, fmt.Errorf("document budget of %d exhausted", p.maxDocuments)
	}
	p.documentsFetched++

	ctx := p.parseCtx
	if ctx == nil {
		ctx = context.Background()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "text/xml, application/xml, application/wsdl+xml")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3")

	// Caller-supplied headers may carry scan credentials; never replay them to
	// a host the document merely pointed us at.
	if p.origin != "" && sameOrigin(p.origin, target) {
		for key, value := range p.headers {
			req.Header.Set(key, value)
		}
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	limit := p.maxDocumentSize
	if limit <= 0 {
		limit = DefaultMaxDocumentSize
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return nil, fmt.Errorf("unexpected status %d (response unreadable: %v)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("reading document: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("document exceeds maximum size of %d bytes", limit)
	}

	return body, nil
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
		schema.ComplexTypes = append(schema.ComplexTypes, p.convertRawComplexType(&ct, schema.TargetNamespace))
	}

	for _, st := range raw.SimpleTypes {
		schema.SimpleTypes = append(schema.SimpleTypes, p.convertRawSimpleType(&st, schema.TargetNamespace))
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
		ct := p.convertRawComplexType(raw.ComplexType, targetNS)
		elem.ComplexType = &ct
	}

	if raw.SimpleType != nil {
		st := p.convertRawSimpleType(raw.SimpleType, targetNS)
		elem.SimpleType = &st
	}

	return elem
}

// convertRawComplexType converts raw XSD complex type to domain model
func (p *Parser) convertRawComplexType(raw *rawComplexType, targetNS string) XSDComplexType {
	ct := XSDComplexType{
		Name:     raw.Name,
		Abstract: raw.Abstract,
		Mixed:    raw.Mixed,
	}

	if raw.Sequence != nil {
		seq := p.convertRawSequence(raw.Sequence, targetNS)
		ct.Sequence = &seq
	}

	if raw.All != nil {
		all := p.convertRawAll(raw.All, targetNS)
		ct.All = &all
	}

	if raw.Choice != nil {
		choice := p.convertRawChoice(raw.Choice, targetNS)
		ct.Choice = &choice
	}

	if raw.ComplexContent != nil {
		cc := p.convertRawComplexContent(raw.ComplexContent, targetNS)
		ct.ComplexContent = &cc
	}

	if raw.SimpleContent != nil {
		sc := p.convertRawSimpleContent(raw.SimpleContent, targetNS)
		ct.SimpleContent = &sc
	}

	for _, attr := range raw.Attributes {
		ct.Attributes = append(ct.Attributes, p.convertRawAttribute(&attr))
	}

	return ct
}

// convertRawSequence converts raw XSD sequence to domain model
func (p *Parser) convertRawSequence(raw *rawSequence, targetNS string) XSDSequence {
	seq := XSDSequence{
		MinOccurs: raw.MinOccurs,
		MaxOccurs: raw.MaxOccurs,
		Elements:  make([]XSDElement, 0, len(raw.Elements)),
	}

	for _, elem := range raw.Elements {
		seq.Elements = append(seq.Elements, p.convertRawElement(&elem, targetNS))
	}

	for _, choice := range raw.Choices {
		seq.Choices = append(seq.Choices, p.convertRawChoice(&choice, targetNS))
	}

	for _, nested := range raw.Sequences {
		seq.Sequences = append(seq.Sequences, p.convertRawSequence(&nested, targetNS))
	}

	for _, any := range raw.Any {
		seq.Any = append(seq.Any, XSDAny{
			Namespace:       any.Namespace,
			ProcessContents: any.ProcessContents,
			MinOccurs:       any.MinOccurs,
			MaxOccurs:       any.MaxOccurs,
		})
	}

	return seq
}

// convertRawAll converts raw XSD all to domain model
func (p *Parser) convertRawAll(raw *rawAll, targetNS string) XSDAll {
	all := XSDAll{
		MinOccurs: raw.MinOccurs,
		MaxOccurs: raw.MaxOccurs,
		Elements:  make([]XSDElement, 0, len(raw.Elements)),
	}

	for _, elem := range raw.Elements {
		all.Elements = append(all.Elements, p.convertRawElement(&elem, targetNS))
	}

	return all
}

// convertRawChoice converts raw XSD choice to domain model
func (p *Parser) convertRawChoice(raw *rawChoice, targetNS string) XSDChoice {
	choice := XSDChoice{
		MinOccurs: raw.MinOccurs,
		MaxOccurs: raw.MaxOccurs,
		Elements:  make([]XSDElement, 0, len(raw.Elements)),
	}

	for _, elem := range raw.Elements {
		choice.Elements = append(choice.Elements, p.convertRawElement(&elem, targetNS))
	}

	for _, nested := range raw.Sequences {
		choice.Sequences = append(choice.Sequences, p.convertRawSequence(&nested, targetNS))
	}

	for _, nested := range raw.Choices {
		choice.Choices = append(choice.Choices, p.convertRawChoice(&nested, targetNS))
	}

	for _, any := range raw.Any {
		choice.Any = append(choice.Any, XSDAny{
			Namespace:       any.Namespace,
			ProcessContents: any.ProcessContents,
			MinOccurs:       any.MinOccurs,
			MaxOccurs:       any.MaxOccurs,
		})
	}

	return choice
}

// convertRawComplexContent converts raw complex content to domain model
func (p *Parser) convertRawComplexContent(raw *rawComplexContent, targetNS string) XSDComplexContent {
	cc := XSDComplexContent{
		Mixed: raw.Mixed,
	}

	if raw.Extension != nil {
		ext := p.convertRawExtension(raw.Extension, targetNS)
		cc.Extension = &ext
	}

	if raw.Restriction != nil {
		rest := p.convertRawRestriction(raw.Restriction, targetNS)
		cc.Restriction = &rest
	}

	return cc
}

// convertRawSimpleContent converts raw simple content to domain model
func (p *Parser) convertRawSimpleContent(raw *rawSimpleContent, targetNS string) XSDSimpleContent {
	sc := XSDSimpleContent{}

	if raw.Extension != nil {
		ext := p.convertRawExtension(raw.Extension, targetNS)
		sc.Extension = &ext
	}

	if raw.Restriction != nil {
		rest := p.convertRawRestriction(raw.Restriction, targetNS)
		sc.Restriction = &rest
	}

	return sc
}

// convertRawExtension converts raw extension to domain model
func (p *Parser) convertRawExtension(raw *rawExtension, targetNS string) XSDExtension {
	ext := XSDExtension{
		Base: raw.Base,
	}

	if raw.Sequence != nil {
		seq := p.convertRawSequence(raw.Sequence, targetNS)
		ext.Sequence = &seq
	}

	if raw.All != nil {
		all := p.convertRawAll(raw.All, targetNS)
		ext.All = &all
	}

	if raw.Choice != nil {
		choice := p.convertRawChoice(raw.Choice, targetNS)
		ext.Choice = &choice
	}

	for _, attr := range raw.Attributes {
		ext.Attributes = append(ext.Attributes, p.convertRawAttribute(&attr))
	}

	return ext
}

// convertRawRestriction converts raw restriction to domain model
func (p *Parser) convertRawRestriction(raw *rawRestriction, targetNS string) XSDRestriction {
	rest := XSDRestriction{
		Base:         raw.Base,
		WhiteSpace:   facetString(raw.WhiteSpace),
		MinInclusive: facetString(raw.MinInclusive),
		MaxInclusive: facetString(raw.MaxInclusive),
		MinExclusive: facetString(raw.MinExclusive),
		MaxExclusive: facetString(raw.MaxExclusive),
	}

	if len(raw.Pattern) > 0 {
		rest.Pattern = raw.Pattern[0].Value
	}

	for _, enum := range raw.Enumeration {
		rest.Enumeration = append(rest.Enumeration, enum.Value)
	}

	rest.MinLength = facetInt(raw.MinLength)
	rest.MaxLength = facetInt(raw.MaxLength)
	rest.Length = facetInt(raw.Length)
	rest.TotalDigits = facetInt(raw.TotalDigits)
	rest.FractionDigits = facetInt(raw.FractionDigits)

	if raw.Sequence != nil {
		seq := p.convertRawSequence(raw.Sequence, targetNS)
		rest.Sequence = &seq
	}

	if raw.All != nil {
		all := p.convertRawAll(raw.All, targetNS)
		rest.All = &all
	}

	if raw.Choice != nil {
		choice := p.convertRawChoice(raw.Choice, targetNS)
		rest.Choice = &choice
	}

	for _, attr := range raw.Attributes {
		rest.Attributes = append(rest.Attributes, p.convertRawAttribute(&attr))
	}

	return rest
}

// convertRawSimpleType converts raw simple type to domain model
func (p *Parser) convertRawSimpleType(raw *rawSimpleType, targetNS string) XSDSimpleType {
	st := XSDSimpleType{
		Name: raw.Name,
	}

	if raw.Restriction != nil {
		rest := p.convertRawRestriction(raw.Restriction, targetNS)
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

	op.Input = convertRawBindingIO(raw.Input)
	op.Output = convertRawBindingIO(raw.Output)

	for _, fault := range raw.Faults {
		converted := BindingFault{Name: fault.Name}
		if soapFault := fault.SOAPFault; soapFault != nil {
			converted.Use = soapFault.Use
			converted.Namespace = soapFault.Namespace
			converted.EncodingStyle = soapFault.EncodingStyle
		}
		if soapFault := fault.SOAP12Fault; soapFault != nil {
			converted.Use = soapFault.Use
			converted.Namespace = soapFault.Namespace
			converted.EncodingStyle = soapFault.EncodingStyle
		}
		op.Faults = append(op.Faults, converted)
	}

	return op
}

func convertRawBindingIO(raw *rawBindingIO) *BindingIO {
	if raw == nil {
		return nil
	}

	io := &BindingIO{}
	for _, body := range []*rawSOAPBody{raw.SOAPBody, raw.SOAP12Body} {
		if body == nil {
			continue
		}
		io.Use = body.Use
		io.Namespace = body.Namespace
		io.EncodingStyle = body.EncodingStyle
		io.Parts = body.Parts
	}

	headers := append(append([]rawSOAPHeader{}, raw.SOAPHeaders...), raw.SOAP12Headers...)
	for _, header := range headers {
		io.Headers = append(io.Headers, BindingHeader{
			Message:       header.Message,
			Part:          header.Part,
			Use:           header.Use,
			Namespace:     header.Namespace,
			EncodingStyle: header.EncodingStyle,
		})
	}

	return io
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
		if err := p.allowImport(resolvedURL); err != nil {
			log.Debug().Err(err).Str("location", location).Msg("Skipping WSDL import")
			continue
		}
		p.imported[resolvedURL] = true

		importedData, err := p.fetchDocument(resolvedURL)
		if err != nil {
			log.Debug().Err(err).Str("url", resolvedURL).Msg("Failed to fetch WSDL import")
			continue
		}

		importedDoc, err := p.parseImportedWSDL(importedData, resolvedURL, depth+1)
		if err != nil {
			log.Debug().Err(err).Str("url", resolvedURL).Msg("Failed to parse WSDL import")
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

// parseImportedWSDL parses a document reached through wsdl:import, preserving
// the traversal budget and depth of the parse in progress.
func (p *Parser) parseImportedWSDL(data []byte, sourceURL string, depth int) (*WSDLDocument, error) {
	if depth > p.maxDepth {
		return nil, fmt.Errorf("max import depth exceeded (%d)", p.maxDepth)
	}

	var rawWSDL rawDefinitions
	if err := xml.Unmarshal(data, &rawWSDL); err != nil {
		return nil, fmt.Errorf("failed to parse imported WSDL: %w", err)
	}

	namespaces := p.extractNamespaces(data)

	doc, err := p.convertRawWSDL(&rawWSDL, namespaces)
	if err != nil {
		return nil, fmt.Errorf("failed to convert imported WSDL %s: %w", sourceURL, err)
	}

	if err := p.resolveImports(doc, sourceURL, depth, namespaces); err != nil {
		return nil, fmt.Errorf("failed to resolve imports of %s: %w", sourceURL, err)
	}

	return doc, nil
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
		if err := p.allowImport(resolvedURL); err != nil {
			log.Debug().Err(err).Str("location", imp.SchemaLocation).Msg("Skipping XSD import")
			continue
		}
		p.imported[resolvedURL] = true

		xsdData, err := p.fetchDocument(resolvedURL)
		if err != nil {
			log.Debug().Err(err).Str("url", resolvedURL).Msg("Failed to fetch XSD import")
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
		if err := p.allowImport(resolvedURL); err != nil {
			log.Debug().Err(err).Str("location", inc.SchemaLocation).Msg("Skipping XSD include")
			continue
		}
		p.imported[resolvedURL] = true

		xsdData, err := p.fetchDocument(resolvedURL)
		if err != nil {
			log.Debug().Err(err).Str("url", resolvedURL).Msg("Failed to fetch XSD include")
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

// buildTypeRegistry builds a registry for quick type lookup.
// Entries are keyed both by "{namespace}local" and by bare local name. The bare
// key is a convenience for lookups that have lost namespace context; when two
// schemas define the same local name the first one wins, so that a later import
// cannot silently replace a type the document already resolved.
func (p *Parser) buildTypeRegistry(doc *WSDLDocument) *TypeRegistry {
	registry := NewTypeRegistry()

	for i := range doc.Messages {
		msg := &doc.Messages[i]
		registry.Messages[MakeTypeKey(doc.TargetNamespace, msg.Name)] = msg
		if _, exists := registry.Messages[msg.Name]; !exists {
			registry.Messages[msg.Name] = msg
		}
	}

	if doc.Types == nil {
		return registry
	}

	for i := range doc.Types.Schemas {
		schema := &doc.Types.Schemas[i]
		ns := schema.TargetNamespace

		for j := range schema.Elements {
			elem := &schema.Elements[j]
			if elem.Name == "" {
				continue
			}
			registry.Elements[MakeTypeKey(ns, elem.Name)] = elem
			if _, exists := registry.Elements[elem.Name]; !exists {
				registry.Elements[elem.Name] = elem
			}
		}

		for j := range schema.ComplexTypes {
			ct := &schema.ComplexTypes[j]
			if ct.Name == "" {
				continue
			}
			registry.ComplexTypes[MakeTypeKey(ns, ct.Name)] = ct
			if _, exists := registry.ComplexTypes[ct.Name]; !exists {
				registry.ComplexTypes[ct.Name] = ct
			}
		}

		for j := range schema.SimpleTypes {
			st := &schema.SimpleTypes[j]
			if st.Name == "" {
				continue
			}
			registry.SimpleTypes[MakeTypeKey(ns, st.Name)] = st
			if _, exists := registry.SimpleTypes[st.Name]; !exists {
				registry.SimpleTypes[st.Name] = st
			}
		}
	}

	return registry
}

func facetString(f *rawFacet) string {
	if f == nil {
		return ""
	}
	return f.Value
}

func facetInt(f *rawFacet) *int {
	if f == nil {
		return nil
	}
	v, err := strconv.Atoi(strings.TrimSpace(f.Value))
	if err != nil {
		return nil
	}
	return &v
}

// extractDocumentation extracts text from documentation element
func extractDocumentation(doc *rawDocumentation) string {
	if doc == nil {
		return ""
	}
	return strings.TrimSpace(doc.Content)
}
