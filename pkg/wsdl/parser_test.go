package wsdl

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return data
}

func parseFixture(t *testing.T, name string) *WSDLDocument {
	t.Helper()
	doc, err := NewParser().ParseFromBytes(loadFixture(t, name), "http://example.com/svc?wsdl")
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	return doc
}

// A WSDL containing xsd:restriction must parse. Enumerations and length facets
// are ubiquitous in .NET and JAX-WS schemas, and a failure here drops the whole
// service from the scan rather than degrading gracefully.
func TestParseRestrictionFacets(t *testing.T) {
	doc := parseFixture(t, "dotnet_asmx.wsdl")

	status, ok := doc.TypeRegistry.SimpleTypes["AccountStatus"]
	if !ok {
		t.Fatal("AccountStatus simple type not registered")
	}
	if status.Restriction == nil {
		t.Fatal("AccountStatus has no restriction")
	}
	want := []string{"Active", "Suspended", "Closed"}
	if len(status.Restriction.Enumeration) != len(want) {
		t.Fatalf("enumeration = %v, want %v", status.Restriction.Enumeration, want)
	}
	for i, v := range want {
		if status.Restriction.Enumeration[i] != v {
			t.Errorf("enumeration[%d] = %q, want %q", i, status.Restriction.Enumeration[i], v)
		}
	}

	postal, ok := doc.TypeRegistry.SimpleTypes["PostalCode"]
	if !ok {
		t.Fatal("PostalCode simple type not registered")
	}
	if postal.Restriction.Pattern != "[0-9]{5}" {
		t.Errorf("pattern = %q, want [0-9]{5}", postal.Restriction.Pattern)
	}
	if postal.Restriction.MaxLength == nil || *postal.Restriction.MaxLength != 5 {
		t.Errorf("maxLength = %v, want 5", postal.Restriction.MaxLength)
	}
	if postal.Restriction.MinLength == nil || *postal.Restriction.MinLength != 5 {
		t.Errorf("minLength = %v, want 5", postal.Restriction.MinLength)
	}
}

func TestParseNumericFacets(t *testing.T) {
	const doc = `<?xml version="1.0"?>
<definitions xmlns="http://schemas.xmlsoap.org/wsdl/" xmlns:xsd="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t">
  <types><xsd:schema targetNamespace="urn:t">
    <xsd:simpleType name="Amount">
      <xsd:restriction base="xsd:decimal">
        <xsd:minInclusive value="0"/>
        <xsd:maxInclusive value="1000"/>
        <xsd:totalDigits value="7"/>
        <xsd:fractionDigits value="2"/>
      </xsd:restriction>
    </xsd:simpleType>
    <xsd:simpleType name="Ratio">
      <xsd:restriction base="xsd:decimal">
        <xsd:minExclusive value="0"/>
        <xsd:maxExclusive value="1"/>
      </xsd:restriction>
    </xsd:simpleType>
  </xsd:schema></types>
</definitions>`

	parsed, err := NewParser().ParseFromBytes([]byte(doc), "http://h/s?wsdl")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	amount := parsed.TypeRegistry.SimpleTypes["Amount"]
	if amount == nil || amount.Restriction == nil {
		t.Fatal("Amount not parsed")
	}
	if amount.Restriction.MinInclusive != "0" || amount.Restriction.MaxInclusive != "1000" {
		t.Errorf("inclusive bounds = %q..%q, want 0..1000",
			amount.Restriction.MinInclusive, amount.Restriction.MaxInclusive)
	}
	if amount.Restriction.TotalDigits == nil || *amount.Restriction.TotalDigits != 7 {
		t.Errorf("totalDigits = %v, want 7", amount.Restriction.TotalDigits)
	}
	if amount.Restriction.FractionDigits == nil || *amount.Restriction.FractionDigits != 2 {
		t.Errorf("fractionDigits = %v, want 2", amount.Restriction.FractionDigits)
	}

	ratio := parsed.TypeRegistry.SimpleTypes["Ratio"]
	if ratio == nil || ratio.Restriction == nil {
		t.Fatal("Ratio not parsed")
	}
	if ratio.Restriction.MinExclusive != "0" || ratio.Restriction.MaxExclusive != "1" {
		t.Errorf("exclusive bounds = %q..%q, want 0..1",
			ratio.Restriction.MinExclusive, ratio.Restriction.MaxExclusive)
	}
}

func TestParseDotNetServiceStructure(t *testing.T) {
	doc := parseFixture(t, "dotnet_asmx.wsdl")

	if doc.TargetNamespace != "http://tempuri.org/" {
		t.Errorf("targetNamespace = %q", doc.TargetNamespace)
	}
	if len(doc.Services) != 1 {
		t.Fatalf("services = %d, want 1", len(doc.Services))
	}
	if len(doc.Services[0].Ports) != 2 {
		t.Fatalf("ports = %d, want 2 (soap11 + soap12)", len(doc.Services[0].Ports))
	}

	byName := map[string]Port{}
	for _, p := range doc.Services[0].Ports {
		byName[p.Name] = p
	}
	if got := byName["UserServiceSoap"].SOAPVersion; got != "1.1" {
		t.Errorf("soap11 port version = %q, want 1.1", got)
	}
	if got := byName["UserServiceSoap12"].SOAPVersion; got != "1.2" {
		t.Errorf("soap12 port version = %q, want 1.2", got)
	}
	for name, p := range byName {
		if p.Address != "http://example.com/UserService.asmx" {
			t.Errorf("port %s address = %q", name, p.Address)
		}
	}

	if len(doc.Bindings) != 2 {
		t.Fatalf("bindings = %d, want 2", len(doc.Bindings))
	}
	for _, b := range doc.Bindings {
		if b.Transport != SOAPHTTPTransport {
			t.Errorf("binding %s transport = %q", b.Name, b.Transport)
		}
		if len(b.Operations) != 1 {
			t.Errorf("binding %s operations = %d, want 1", b.Name, len(b.Operations))
		}
		if b.Operations[0].SOAPAction != "http://tempuri.org/GetUser" {
			t.Errorf("binding %s soapAction = %q", b.Name, b.Operations[0].SOAPAction)
		}
	}
}

func TestParseOperationDocumentation(t *testing.T) {
	doc := parseFixture(t, "dotnet_asmx.wsdl")
	if len(doc.PortTypes) != 1 {
		t.Fatalf("portTypes = %d", len(doc.PortTypes))
	}
	op := doc.PortTypes[0].Operations[0]
	if op.Documentation != "Returns a user by identifier." {
		t.Errorf("documentation = %q", op.Documentation)
	}
	if op.Input == nil || op.Input.Message != "tns:GetUserSoapIn" {
		t.Errorf("input message = %+v", op.Input)
	}
	if op.Output == nil || op.Output.Message != "tns:GetUserSoapOut" {
		t.Errorf("output message = %+v", op.Output)
	}
}

func TestParseNestedComplexTypes(t *testing.T) {
	doc := parseFixture(t, "dotnet_asmx.wsdl")

	user := doc.TypeRegistry.ComplexTypes["User"]
	if user == nil {
		t.Fatal("User complex type not registered")
	}
	if user.Sequence == nil || len(user.Sequence.Elements) != 3 {
		t.Fatalf("User sequence = %+v, want 3 elements", user.Sequence)
	}

	addr := doc.TypeRegistry.ComplexTypes["Address"]
	if addr == nil {
		t.Fatal("Address complex type not registered")
	}
	names := []string{}
	for _, e := range addr.Sequence.Elements {
		names = append(names, e.Name)
	}
	want := "Street,City,PostalCode"
	if got := strings.Join(names, ","); got != want {
		t.Errorf("Address elements = %q, want %q", got, want)
	}
}

// Elements nested inside <choice><sequence> are real, fuzzable operation
// arguments. Dropping them silently removes attack surface.
func TestParseSequenceInsideChoice(t *testing.T) {
	doc := parseFixture(t, "axis_rpc.wsdl")

	tr := doc.TypeRegistry.ComplexTypes["TransferRequest"]
	if tr == nil {
		t.Fatal("TransferRequest not registered")
	}
	if tr.Sequence == nil {
		t.Fatal("TransferRequest has no sequence")
	}
	if len(tr.Sequence.Choices) == 0 {
		t.Fatal("choice inside sequence was dropped")
	}

	choice := tr.Sequence.Choices[0]
	if len(choice.Elements) != 1 || choice.Elements[0].Name != "amount" {
		t.Errorf("choice direct elements = %+v, want [amount]", choice.Elements)
	}
	if len(choice.Sequences) == 0 {
		t.Fatal("sequence nested inside choice was dropped")
	}
	nested := []string{}
	for _, e := range choice.Sequences[0].Elements {
		nested = append(nested, e.Name)
	}
	if got := strings.Join(nested, ","); got != "currency,minorUnits" {
		t.Errorf("nested sequence elements = %q, want currency,minorUnits", got)
	}
}

func TestParseRPCMessageParts(t *testing.T) {
	doc := parseFixture(t, "axis_rpc.wsdl")

	var msg *Message
	for i := range doc.Messages {
		if doc.Messages[i].Name == "transferRequest" {
			msg = &doc.Messages[i]
		}
	}
	if msg == nil {
		t.Fatal("transferRequest message not found")
	}
	if len(msg.Parts) != 3 {
		t.Fatalf("parts = %d, want 3", len(msg.Parts))
	}
	wantTypes := map[string]string{
		"creds":          "impl:Credentials",
		"request":        "impl:TransferRequest",
		"idempotencyKey": "xsd:string",
	}
	for _, p := range msg.Parts {
		if wantTypes[p.Name] != p.Type {
			t.Errorf("part %s type = %q, want %q", p.Name, p.Type, wantTypes[p.Name])
		}
	}

	binding := doc.Bindings[0]
	if binding.Style != "rpc" {
		t.Errorf("binding style = %q, want rpc", binding.Style)
	}
	if binding.Operations[0].Input == nil {
		t.Fatal("binding operation has no input")
	}
	in := binding.Operations[0].Input
	if in.Use != "encoded" {
		t.Errorf("soap:body use = %q, want encoded", in.Use)
	}
	if in.Namespace != "urn:AccountService" {
		t.Errorf("soap:body namespace = %q, want urn:AccountService", in.Namespace)
	}
	if in.EncodingStyle != "http://schemas.xmlsoap.org/soap/encoding/" {
		t.Errorf("soap:body encodingStyle = %q", in.EncodingStyle)
	}
}

// Faults are declared in both portType and binding. They carry error-surface
// information the scanner reports on, so they must survive conversion.
func TestParseOperationFaults(t *testing.T) {
	doc := parseFixture(t, "axis_rpc.wsdl")

	op := doc.PortTypes[0].Operations[0]
	if len(op.Faults) != 1 {
		t.Fatalf("portType operation faults = %d, want 1", len(op.Faults))
	}
	if op.Faults[0].Name != "AccountFault" || op.Faults[0].Message != "impl:AccountFault" {
		t.Errorf("fault = %+v", op.Faults[0])
	}

	bindOp := doc.Bindings[0].Operations[0]
	if len(bindOp.Faults) != 1 {
		t.Fatalf("binding operation faults = %d, want 1", len(bindOp.Faults))
	}
	if bindOp.Faults[0].Name != "AccountFault" {
		t.Errorf("binding fault name = %q", bindOp.Faults[0].Name)
	}
	if bindOp.Faults[0].Use != "encoded" {
		t.Errorf("binding fault use = %q, want encoded", bindOp.Faults[0].Use)
	}
}

// SOAP headers are a genuine attack surface (auth tokens, routing, WS-Security).
// They must be exposed, not silently discarded.
func TestParseSOAPHeaders(t *testing.T) {
	doc := parseFixture(t, "dotnet_asmx.wsdl")

	var soap11 *Binding
	for i := range doc.Bindings {
		if doc.Bindings[i].SOAPVersion == "1.1" {
			soap11 = &doc.Bindings[i]
		}
	}
	if soap11 == nil {
		t.Fatal("soap 1.1 binding not found")
	}
	in := soap11.Operations[0].Input
	if in == nil {
		t.Fatal("binding operation input missing")
	}
	if len(in.Headers) != 1 {
		t.Fatalf("soap:header entries = %d, want 1", len(in.Headers))
	}
	h := in.Headers[0]
	if h.Message != "tns:GetUserAuthHeader" || h.Part != "AuthHeader" || h.Use != "literal" {
		t.Errorf("soap:header = %+v", h)
	}
}

// Two schemas in different namespaces may each define a type with the same
// local name. Namespace-blind registry keys make one silently shadow the other,
// so the generated request carries the wrong fields.
func TestTypeRegistryNamespaceCollision(t *testing.T) {
	doc := parseFixture(t, "ns_collision.wsdl")

	orderItem := doc.TypeRegistry.ComplexTypes[MakeTypeKey("urn:orders", "Item")]
	shipItem := doc.TypeRegistry.ComplexTypes[MakeTypeKey("urn:shipping", "Item")]
	if orderItem == nil {
		t.Fatal("urn:orders Item not registered under its namespace")
	}
	if shipItem == nil {
		t.Fatal("urn:shipping Item not registered under its namespace")
	}

	orderFields := []string{}
	for _, e := range orderItem.Sequence.Elements {
		orderFields = append(orderFields, e.Name)
	}
	if got := strings.Join(orderFields, ","); got != "sku,quantity" {
		t.Errorf("urn:orders Item fields = %q, want sku,quantity", got)
	}

	shipFields := []string{}
	for _, e := range shipItem.Sequence.Elements {
		shipFields = append(shipFields, e.Name)
	}
	if got := strings.Join(shipFields, ","); got != "trackingNumber,carrier,weightKg" {
		t.Errorf("urn:shipping Item fields = %q, want trackingNumber,carrier,weightKg", got)
	}
}

func TestParseRealWorldSpyneDocLiteral(t *testing.T) {
	doc := parseFixture(t, "spyne_doclit.wsdl")

	if doc.TargetNamespace != "http://testbed.local/soap/sqli" {
		t.Errorf("targetNamespace = %q", doc.TargetNamespace)
	}
	if len(doc.Services) != 1 || len(doc.Services[0].Ports) != 1 {
		t.Fatalf("services/ports = %d/%d", len(doc.Services), len(doc.Services[0].Ports))
	}
	if got := doc.Services[0].Ports[0].Address; got != "http://127.0.0.1:9094/soap/sqli" {
		t.Errorf("address = %q", got)
	}
	if len(doc.PortTypes[0].Operations) != 5 {
		t.Errorf("operations = %d, want 5", len(doc.PortTypes[0].Operations))
	}

	wrapper := doc.TypeRegistry.Elements["ListProducts"]
	if wrapper == nil {
		t.Fatal("ListProducts wrapper element not registered")
	}
	ct := doc.TypeRegistry.ComplexTypes["ListProducts"]
	if ct == nil || ct.Sequence == nil || len(ct.Sequence.Elements) != 1 {
		t.Fatalf("ListProducts wrapper type = %+v", ct)
	}
	if ct.Sequence.Elements[0].Name != "sort_column" {
		t.Errorf("wrapper child = %q, want sort_column", ct.Sequence.Elements[0].Name)
	}
}

func TestParseMalformedXMLReturnsError(t *testing.T) {
	cases := map[string]string{
		"truncated":  `<definitions xmlns="http://schemas.xmlsoap.org/wsdl/"><types>`,
		"not xml":    `this is not xml at all`,
		"empty body": ``,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewParser().ParseFromBytes([]byte(body), "http://h/s"); err == nil {
				t.Error("expected an error for malformed input")
			}
		})
	}
}

func TestParseEmptyDefinitionsIsNotAnError(t *testing.T) {
	const body = `<?xml version="1.0"?><definitions xmlns="http://schemas.xmlsoap.org/wsdl/" targetNamespace="urn:empty"/>`
	doc, err := NewParser().ParseFromBytes([]byte(body), "http://h/s")
	if err != nil {
		t.Fatalf("empty but well-formed WSDL should parse: %v", err)
	}
	if doc.TargetNamespace != "urn:empty" {
		t.Errorf("targetNamespace = %q", doc.TargetNamespace)
	}
	if doc.TypeRegistry == nil {
		t.Error("type registry should always be initialised")
	}
}

func TestParseFromURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.Write(loadFixture(t, "dotnet_asmx.wsdl"))
	}))
	defer srv.Close()

	doc, err := NewParser().ParseFromURL(srv.URL + "/svc?wsdl")
	if err != nil {
		t.Fatalf("ParseFromURL: %v", err)
	}
	if len(doc.Services) != 1 {
		t.Errorf("services = %d, want 1", len(doc.Services))
	}
}

func TestParseFromURLNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := NewParser().ParseFromURL(srv.URL); err == nil {
		t.Error("expected an error on 404")
	}
}

// Relative schemaLocation values must resolve against the WSDL's directory.
// Getting this wrong 404s the import and silently drops every type it defines.
func TestResolveRelativeXSDImport(t *testing.T) {
	var requested []string
	mux := http.NewServeMux()
	mux.HandleFunc("/soap/types.xsd", func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:imported">
  <xsd:complexType name="ImportedType">
    <xsd:sequence><xsd:element name="importedField" type="xsd:string"/></xsd:sequence>
  </xsd:complexType>
</xsd:schema>`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	wsdlDoc := fmt.Sprintf(`<?xml version="1.0"?>
<definitions xmlns="http://schemas.xmlsoap.org/wsdl/" xmlns:xsd="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t">
  <types><xsd:schema targetNamespace="urn:t">
    <xsd:import namespace="urn:imported" schemaLocation="types.xsd"/>
  </xsd:schema></types>
</definitions>`)

	doc, err := NewParser().ParseFromBytes([]byte(wsdlDoc), srv.URL+"/soap/service?wsdl")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if doc.TypeRegistry.ComplexTypes["ImportedType"] == nil {
		t.Errorf("imported type missing; server saw requests for %v (expected /soap/types.xsd)", requested)
	}
}

func TestImportCycleTerminates(t *testing.T) {
	var mu = make(chan struct{}, 1)
	mu <- struct{}{}
	hits := 0

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-mu
		hits++
		mu <- struct{}{}
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprintf(w, `<?xml version="1.0"?>
<definitions xmlns="http://schemas.xmlsoap.org/wsdl/" targetNamespace="urn:t">
  <import namespace="urn:a" location="%s/a.wsdl"/>
  <import namespace="urn:b" location="%s/b.wsdl"/>
</definitions>`, srv.URL, srv.URL)
	}))
	defer srv.Close()

	done := make(chan error, 1)
	go func() {
		_, err := NewParser().ParseFromURL(srv.URL + "/root.wsdl")
		done <- err
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("import cycle did not terminate")
	}
	if hits > 20 {
		t.Errorf("cyclic imports fetched %d documents, expected bounded traversal", hits)
	}
}
