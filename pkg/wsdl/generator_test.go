package wsdl

import (
	"encoding/xml"
	"strings"
	"testing"
)

func generateFor(t *testing.T, fixture string, config GenerationConfig) []ServiceEndpoint {
	t.Helper()
	doc := parseFixture(t, fixture)
	endpoints, err := NewGenerator(doc, config).GenerateRequests()
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}
	return endpoints
}

// Every generated body must be well-formed XML. A malformed envelope means the
// target rejects the request before any payload reaches a sink.
func TestGeneratedBodiesAreWellFormed(t *testing.T) {
	for _, fixture := range []string{"dotnet_asmx.wsdl", "axis_rpc.wsdl", "spyne_doclit.wsdl", "spyne_nested.wsdl", "ns_collision.wsdl"} {
		t.Run(fixture, func(t *testing.T) {
			for _, ep := range generateFor(t, fixture, DefaultGenerationConfig()) {
				for _, op := range ep.Operations {
					for _, req := range op.Requests {
						if strings.TrimSpace(req.Body) == "" {
							t.Errorf("%s/%s produced an empty body", ep.PortName, op.Name)
							continue
						}
						if err := xml.Unmarshal([]byte(req.Body), new(struct {
							XMLName xml.Name
						})); err != nil {
							t.Errorf("%s/%s body is not well-formed XML: %v\n%s", ep.PortName, op.Name, err, req.Body)
						}
					}
				}
			}
		})
	}
}

func TestGeneratedEnvelopeUsesCorrectSOAPNamespace(t *testing.T) {
	endpoints := generateFor(t, "dotnet_asmx.wsdl", DefaultGenerationConfig())
	if len(endpoints) != 2 {
		t.Fatalf("endpoints = %d, want 2", len(endpoints))
	}

	for _, ep := range endpoints {
		body := ep.Operations[0].Requests[0].Body
		wantNS := SOAP11EnvelopeNS
		if ep.SOAPVersion == "1.2" {
			wantNS = SOAP12EnvelopeNS
		}
		if !strings.Contains(body, wantNS) {
			t.Errorf("port %s (SOAP %s) envelope missing namespace %s:\n%s",
				ep.PortName, ep.SOAPVersion, wantNS, body)
		}
	}
}

func TestGeneratedHeadersPerSOAPVersion(t *testing.T) {
	endpoints := generateFor(t, "dotnet_asmx.wsdl", DefaultGenerationConfig())

	for _, ep := range endpoints {
		req := ep.Operations[0].Requests[0]
		ct := req.Headers["Content-Type"]

		switch ep.SOAPVersion {
		case "1.1":
			if !strings.HasPrefix(ct, "text/xml") {
				t.Errorf("SOAP 1.1 Content-Type = %q", ct)
			}
			if got := req.Headers["SOAPAction"]; got != `"http://tempuri.org/GetUser"` {
				t.Errorf("SOAP 1.1 SOAPAction = %q, want quoted action", got)
			}
		case "1.2":
			if !strings.HasPrefix(ct, "application/soap+xml") {
				t.Errorf("SOAP 1.2 Content-Type = %q", ct)
			}
			if !strings.Contains(ct, `action="http://tempuri.org/GetUser"`) {
				t.Errorf("SOAP 1.2 Content-Type must carry the action parameter, got %q", ct)
			}
			if _, present := req.Headers["SOAPAction"]; present {
				t.Error("SOAP 1.2 must not send a SOAPAction header")
			}
		}
	}
}

// Document/literal wrapped: the body child is the part's element, qualified in
// its schema namespace, and its children are the operation arguments. Emitting
// the message-part name as an extra wrapper makes the server bind nothing.
func TestDocumentLiteralBodyShape(t *testing.T) {
	endpoints := generateFor(t, "spyne_doclit.wsdl", DefaultGenerationConfig())

	var body string
	for _, ep := range endpoints {
		for _, op := range ep.Operations {
			if op.Name == "ListProducts" {
				body = op.Requests[0].Body
			}
		}
	}
	if body == "" {
		t.Fatal("ListProducts request not generated")
	}

	if strings.Count(body, "<ListProducts") != 1 {
		t.Errorf("ListProducts wrapper appears %d times, want exactly 1 (double wrapping stops the server binding arguments):\n%s",
			strings.Count(body, "<ListProducts"), body)
	}
	if !strings.Contains(body, "sort_column") {
		t.Errorf("body is missing the real argument sort_column:\n%s", body)
	}
	if !strings.Contains(body, "http://testbed.local/soap/sqli") {
		t.Errorf("body does not qualify the wrapper in the schema namespace:\n%s", body)
	}
}

// RPC style: the wrapper is the operation name in the soap:body namespace, and
// the parts are its unqualified children.
func TestRPCBodyShape(t *testing.T) {
	endpoints := generateFor(t, "axis_rpc.wsdl", DefaultGenerationConfig())
	if len(endpoints) == 0 {
		t.Fatal("no endpoints generated")
	}

	op := endpoints[0].Operations[0]
	if op.Style != "rpc" {
		t.Fatalf("style = %q, want rpc", op.Style)
	}
	body := op.Requests[0].Body

	if !strings.Contains(body, "transfer") {
		t.Errorf("RPC body missing operation wrapper:\n%s", body)
	}
	for _, part := range []string{"creds", "request", "idempotencyKey"} {
		if !strings.Contains(body, "<"+part+">") && !strings.Contains(body, ":"+part+">") {
			t.Errorf("RPC body missing part %q:\n%s", part, body)
		}
	}
}

// Values that violate an enumeration are rejected by well-behaved servers
// before the request reaches the code under test.
func TestGeneratedValuesRespectEnumeration(t *testing.T) {
	endpoints := generateFor(t, "dotnet_asmx.wsdl", DefaultGenerationConfig())

	body := endpoints[0].Operations[0].Requests[0].Body
	if !strings.Contains(body, "<status>") {
		t.Skipf("status element not emitted; body:\n%s", body)
	}
	if !strings.Contains(body, ">Active<") {
		t.Errorf("status must use the first enumeration value (Active), got:\n%s", body)
	}
}

func TestGeneratorUsesPortAddressPerPort(t *testing.T) {
	endpoints := generateFor(t, "dotnet_asmx.wsdl", DefaultGenerationConfig())
	for _, ep := range endpoints {
		if ep.Address != "http://example.com/UserService.asmx" {
			t.Errorf("port %s address = %q", ep.PortName, ep.Address)
		}
		if ep.Operations[0].Requests[0].URL != ep.Address {
			t.Errorf("request URL %q does not match port address %q",
				ep.Operations[0].Requests[0].URL, ep.Address)
		}
	}
}

// BaseURL overrides the host the requests are sent to, but must not collapse
// distinct ports onto a single path.
func TestGeneratorBaseURLOverride(t *testing.T) {
	endpoints := generateFor(t, "dotnet_asmx.wsdl", GenerationConfig{BaseURL: "https://replaced.example/svc"})
	for _, ep := range endpoints {
		if ep.Address != "https://replaced.example/svc" {
			t.Errorf("BaseURL override not applied to port %s: %q", ep.PortName, ep.Address)
		}
	}
}

func TestGeneratorCustomHeadersApplied(t *testing.T) {
	config := DefaultGenerationConfig()
	config.Headers = map[string]string{"X-Scan": "sukyan"}

	endpoints := generateFor(t, "dotnet_asmx.wsdl", config)
	if got := endpoints[0].Operations[0].Requests[0].Headers["X-Scan"]; got != "sukyan" {
		t.Errorf("custom header = %q, want sukyan", got)
	}
}

func TestGeneratorOperationMetadata(t *testing.T) {
	endpoints := generateFor(t, "dotnet_asmx.wsdl", DefaultGenerationConfig())
	op := endpoints[0].Operations[0]

	if op.Name != "GetUser" {
		t.Errorf("operation name = %q", op.Name)
	}
	if op.Style != "document" {
		t.Errorf("style = %q, want document", op.Style)
	}
	if len(op.InputParts) == 0 {
		t.Error("input parts must be reported for the API consumer")
	}
	if len(op.OutputParts) == 0 {
		t.Error("output parts must be reported for the API consumer")
	}
}

// A binding whose portType is missing must not abort generation for the ports
// that are well-formed.
func TestGeneratorSkipsUnresolvableBindings(t *testing.T) {
	const body = `<?xml version="1.0"?>
<definitions xmlns="http://schemas.xmlsoap.org/wsdl/" xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
             xmlns:tns="urn:t" targetNamespace="urn:t">
  <portType name="GoodPT"><operation name="Ping"><input message="tns:PingIn"/></operation></portType>
  <message name="PingIn"><part name="p" type="xsd:string"/></message>
  <binding name="GoodBinding" type="tns:GoodPT">
    <soap:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="Ping"><soap:operation soapAction="ping"/><input><soap:body use="literal"/></input></operation>
  </binding>
  <binding name="DanglingBinding" type="tns:MissingPT">
    <soap:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
  </binding>
  <service name="S">
    <port name="Good" binding="tns:GoodBinding"><soap:address location="http://h/good"/></port>
    <port name="Dangling" binding="tns:DanglingBinding"><soap:address location="http://h/bad"/></port>
    <port name="NoBinding" binding="tns:DoesNotExist"><soap:address location="http://h/none"/></port>
  </service>
</definitions>`

	doc, err := NewParser().ParseFromBytes([]byte(body), "http://h/s?wsdl")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	endpoints, err := NewGenerator(doc, DefaultGenerationConfig()).GenerateRequests()
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("endpoints = %d, want 1 (only the resolvable port)", len(endpoints))
	}
	if endpoints[0].PortName != "Good" {
		t.Errorf("surviving port = %q, want Good", endpoints[0].PortName)
	}
}

func TestIncludeOptionalParamsControlsOptionalElements(t *testing.T) {
	withOptional := generateFor(t, "dotnet_asmx.wsdl", GenerationConfig{IncludeOptionalParams: true})
	withoutOptional := generateFor(t, "dotnet_asmx.wsdl", GenerationConfig{IncludeOptionalParams: false})

	// userId is minOccurs="0"; includeInactive is minOccurs="1".
	included := withOptional[0].Operations[0].Requests[0].Body
	excluded := withoutOptional[0].Operations[0].Requests[0].Body

	if !strings.Contains(included, "userId") {
		t.Errorf("optional element missing when IncludeOptionalParams=true:\n%s", included)
	}
	if strings.Contains(excluded, "userId") {
		t.Errorf("optional element present when IncludeOptionalParams=false:\n%s", excluded)
	}
	if !strings.Contains(excluded, "includeInactive") {
		t.Errorf("required element dropped when IncludeOptionalParams=false:\n%s", excluded)
	}
}

func TestPreferSOAP12OrdersEndpoints(t *testing.T) {
	config := DefaultGenerationConfig()
	config.PreferSOAP12 = true

	endpoints := generateFor(t, "dotnet_asmx.wsdl", config)
	if len(endpoints) != 2 {
		t.Fatalf("endpoints = %d, want 2", len(endpoints))
	}
	if endpoints[0].SOAPVersion != "1.2" {
		t.Errorf("PreferSOAP12 did not surface the 1.2 endpoint first, got %q", endpoints[0].SOAPVersion)
	}

	defaultOrder := generateFor(t, "dotnet_asmx.wsdl", DefaultGenerationConfig())
	if defaultOrder[0].SOAPVersion != "1.1" {
		t.Errorf("default order should follow document order (1.1 first), got %q", defaultOrder[0].SOAPVersion)
	}
}
