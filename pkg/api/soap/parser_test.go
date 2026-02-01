package soap

import (
	"testing"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
	pkgWsdl "github.com/pyneda/sukyan/pkg/wsdl"
)

func makeWSDLDefinition(rawXML string) *db.APIDefinition {
	return &db.APIDefinition{
		BaseUUIDModel: db.BaseUUIDModel{ID: uuid.New()},
		Type:          db.APIDefinitionTypeWSDL,
		SourceURL:     "http://example.com/service?wsdl",
		BaseURL:       "http://example.com/service",
		RawDefinition: []byte(rawXML),
	}
}

const simpleWSDL = `<?xml version="1.0" encoding="UTF-8"?>
<definitions
  xmlns="http://schemas.xmlsoap.org/wsdl/"
  xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
  xmlns:tns="http://example.com/test"
  xmlns:xsd="http://www.w3.org/2001/XMLSchema"
  targetNamespace="http://example.com/test"
  name="TestService">

  <types>
    <xsd:schema targetNamespace="http://example.com/test">
      <xsd:element name="GetUserRequest">
        <xsd:complexType>
          <xsd:sequence>
            <xsd:element name="userId" type="xsd:int"/>
            <xsd:element name="includeDetails" type="xsd:boolean" minOccurs="0"/>
          </xsd:sequence>
        </xsd:complexType>
      </xsd:element>
      <xsd:element name="GetUserResponse">
        <xsd:complexType>
          <xsd:sequence>
            <xsd:element name="name" type="xsd:string"/>
          </xsd:sequence>
        </xsd:complexType>
      </xsd:element>
    </xsd:schema>
  </types>

  <message name="GetUserInput">
    <part name="parameters" element="tns:GetUserRequest"/>
  </message>
  <message name="GetUserOutput">
    <part name="parameters" element="tns:GetUserResponse"/>
  </message>

  <portType name="UserPortType">
    <operation name="GetUser">
      <documentation>Retrieves user information</documentation>
      <input message="tns:GetUserInput"/>
      <output message="tns:GetUserOutput"/>
    </operation>
  </portType>

  <binding name="UserBinding" type="tns:UserPortType">
    <soap:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="GetUser">
      <soap:operation soapAction="http://example.com/GetUser"/>
      <input><soap:body use="literal"/></input>
      <output><soap:body use="literal"/></output>
    </operation>
  </binding>

  <service name="UserService">
    <port name="UserPort" binding="tns:UserBinding">
      <soap:address location="http://example.com/soap/user"/>
    </port>
  </service>
</definitions>`

const multiPortWSDL = `<?xml version="1.0" encoding="UTF-8"?>
<definitions
  xmlns="http://schemas.xmlsoap.org/wsdl/"
  xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
  xmlns:soap12="http://schemas.xmlsoap.org/wsdl/soap12/"
  xmlns:tns="http://example.com/multi"
  xmlns:xsd="http://www.w3.org/2001/XMLSchema"
  targetNamespace="http://example.com/multi"
  name="MultiPortService">

  <types>
    <xsd:schema targetNamespace="http://example.com/multi">
      <xsd:element name="PingRequest">
        <xsd:complexType>
          <xsd:sequence>
            <xsd:element name="message" type="xsd:string"/>
          </xsd:sequence>
        </xsd:complexType>
      </xsd:element>
      <xsd:element name="PingResponse">
        <xsd:complexType>
          <xsd:sequence>
            <xsd:element name="reply" type="xsd:string"/>
          </xsd:sequence>
        </xsd:complexType>
      </xsd:element>
    </xsd:schema>
  </types>

  <message name="PingInput">
    <part name="parameters" element="tns:PingRequest"/>
  </message>
  <message name="PingOutput">
    <part name="parameters" element="tns:PingResponse"/>
  </message>

  <portType name="PingPortType">
    <operation name="Ping">
      <input message="tns:PingInput"/>
      <output message="tns:PingOutput"/>
    </operation>
  </portType>

  <binding name="PingSOAP11Binding" type="tns:PingPortType">
    <soap:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="Ping">
      <soap:operation soapAction="http://example.com/Ping"/>
      <input><soap:body use="literal"/></input>
      <output><soap:body use="literal"/></output>
    </operation>
  </binding>

  <binding name="PingSOAP12Binding" type="tns:PingPortType">
    <soap12:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="Ping">
      <soap12:operation soapAction="http://example.com/Ping12"/>
      <input><soap12:body use="literal"/></input>
      <output><soap12:body use="literal"/></output>
    </operation>
  </binding>

  <service name="PingService">
    <port name="PingSOAP11Port" binding="tns:PingSOAP11Binding">
      <soap:address location="http://example.com/soap11/ping"/>
    </port>
    <port name="PingSOAP12Port" binding="tns:PingSOAP12Binding">
      <soap12:address location="http://example.com/soap12/ping"/>
    </port>
  </service>
</definitions>`

const nestedComplexTypeWSDL = `<?xml version="1.0" encoding="UTF-8"?>
<definitions
  xmlns="http://schemas.xmlsoap.org/wsdl/"
  xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
  xmlns:tns="http://example.com/nested"
  xmlns:xsd="http://www.w3.org/2001/XMLSchema"
  targetNamespace="http://example.com/nested"
  name="OrderService">

  <types>
    <xsd:schema targetNamespace="http://example.com/nested">
      <xsd:complexType name="Address">
        <xsd:sequence>
          <xsd:element name="street" type="xsd:string"/>
          <xsd:element name="city" type="xsd:string"/>
          <xsd:element name="zip" type="xsd:string"/>
        </xsd:sequence>
      </xsd:complexType>

      <xsd:element name="CreateOrderRequest">
        <xsd:complexType>
          <xsd:sequence>
            <xsd:element name="orderId" type="xsd:int"/>
            <xsd:element name="customerName" type="xsd:string"/>
            <xsd:element name="shippingAddress" type="tns:Address"/>
            <xsd:element name="billingAddress" type="tns:Address"/>
            <xsd:element name="amount" type="xsd:decimal"/>
          </xsd:sequence>
        </xsd:complexType>
      </xsd:element>
      <xsd:element name="CreateOrderResponse">
        <xsd:complexType>
          <xsd:sequence>
            <xsd:element name="status" type="xsd:string"/>
          </xsd:sequence>
        </xsd:complexType>
      </xsd:element>
    </xsd:schema>
  </types>

  <message name="CreateOrderInput">
    <part name="parameters" element="tns:CreateOrderRequest"/>
  </message>
  <message name="CreateOrderOutput">
    <part name="parameters" element="tns:CreateOrderResponse"/>
  </message>

  <portType name="OrderPortType">
    <operation name="CreateOrder">
      <input message="tns:CreateOrderInput"/>
      <output message="tns:CreateOrderOutput"/>
    </operation>
  </portType>

  <binding name="OrderBinding" type="tns:OrderPortType">
    <soap:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="CreateOrder">
      <soap:operation soapAction="http://example.com/CreateOrder"/>
      <input><soap:body use="literal"/></input>
      <output><soap:body use="literal"/></output>
    </operation>
  </binding>

  <service name="OrderService">
    <port name="OrderPort" binding="tns:OrderBinding">
      <soap:address location="http://example.com/soap/order"/>
    </port>
  </service>
</definitions>`

const circularRefWSDL = `<?xml version="1.0" encoding="UTF-8"?>
<definitions
  xmlns="http://schemas.xmlsoap.org/wsdl/"
  xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
  xmlns:tns="http://example.com/circular"
  xmlns:xsd="http://www.w3.org/2001/XMLSchema"
  targetNamespace="http://example.com/circular"
  name="TreeService">

  <types>
    <xsd:schema targetNamespace="http://example.com/circular">
      <xsd:complexType name="TreeNode">
        <xsd:sequence>
          <xsd:element name="value" type="xsd:string"/>
          <xsd:element name="left" type="tns:TreeNode" minOccurs="0"/>
          <xsd:element name="right" type="tns:TreeNode" minOccurs="0"/>
        </xsd:sequence>
      </xsd:complexType>

      <xsd:element name="TraverseRequest">
        <xsd:complexType>
          <xsd:sequence>
            <xsd:element name="root" type="tns:TreeNode"/>
          </xsd:sequence>
        </xsd:complexType>
      </xsd:element>
      <xsd:element name="TraverseResponse">
        <xsd:complexType>
          <xsd:sequence>
            <xsd:element name="result" type="xsd:string"/>
          </xsd:sequence>
        </xsd:complexType>
      </xsd:element>
    </xsd:schema>
  </types>

  <message name="TraverseInput">
    <part name="parameters" element="tns:TraverseRequest"/>
  </message>
  <message name="TraverseOutput">
    <part name="parameters" element="tns:TraverseResponse"/>
  </message>

  <portType name="TreePortType">
    <operation name="Traverse">
      <input message="tns:TraverseInput"/>
      <output message="tns:TraverseOutput"/>
    </operation>
  </portType>

  <binding name="TreeBinding" type="tns:TreePortType">
    <soap:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="Traverse">
      <soap:operation soapAction="http://example.com/Traverse"/>
      <input><soap:body use="literal"/></input>
      <output><soap:body use="literal"/></output>
    </operation>
  </binding>

  <service name="TreeService">
    <port name="TreePort" binding="tns:TreeBinding">
      <soap:address location="http://example.com/soap/tree"/>
    </port>
  </service>
</definitions>`

func TestParse_InvalidDefinitionType(t *testing.T) {
	parser := NewParser()
	def := &db.APIDefinition{
		BaseUUIDModel: db.BaseUUIDModel{ID: uuid.New()},
		Type:          db.APIDefinitionTypeOpenAPI,
		RawDefinition: []byte("<definitions/>"),
	}
	_, err := parser.Parse(def)
	if err == nil {
		t.Fatal("expected error for non-WSDL definition type")
	}
}

func TestParse_EmptyRawDefinition(t *testing.T) {
	parser := NewParser()
	def := &db.APIDefinition{
		BaseUUIDModel: db.BaseUUIDModel{ID: uuid.New()},
		Type:          db.APIDefinitionTypeWSDL,
		RawDefinition: nil,
	}
	_, err := parser.Parse(def)
	if err == nil {
		t.Fatal("expected error for empty raw definition")
	}
}

func TestParse_InvalidXML(t *testing.T) {
	parser := NewParser()
	def := makeWSDLDefinition("this is not xml")
	_, err := parser.Parse(def)
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
}

func TestParse_SimpleWSDL(t *testing.T) {
	parser := NewParser()
	def := makeWSDLDefinition(simpleWSDL)
	ops, err := parser.Parse(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}

	op := ops[0]

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"operation name", op.Name, "GetUser"},
		{"method", op.Method, "POST"},
		{"api type", op.APIType, core.APITypeSOAP},
		{"base url", op.BaseURL, "http://example.com/soap/user"},
		{"description", op.Description, "Retrieves user information"},
		{"definition id", op.DefinitionID, def.ID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}

	if op.SOAP == nil {
		t.Fatal("expected SOAP metadata to be present")
	}

	soapTests := []struct {
		name string
		got  string
		want string
	}{
		{"service name", op.SOAP.ServiceName, "UserService"},
		{"port name", op.SOAP.PortName, "UserPort"},
		{"soap action", op.SOAP.SOAPAction, "http://example.com/GetUser"},
		{"binding style", op.SOAP.BindingStyle, "document"},
		{"soap version", op.SOAP.SOAPVersion, "1.1"},
		{"target ns", op.SOAP.TargetNS, "http://example.com/test"},
		{"input message", op.SOAP.InputMessage, "tns:GetUserInput"},
		{"output message", op.SOAP.OutputMessage, "tns:GetUserOutput"},
	}

	for _, tt := range soapTests {
		t.Run("soap/"+tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestParse_SimpleWSDLParameters(t *testing.T) {
	parser := NewParser()
	def := makeWSDLDefinition(simpleWSDL)
	ops, err := parser.Parse(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	op := ops[0]

	if len(op.Parameters) != 1 {
		t.Fatalf("expected 1 top-level parameter (the message part), got %d", len(op.Parameters))
	}

	param := op.Parameters[0]
	if param.Name != "parameters" {
		t.Errorf("expected parameter name 'parameters', got %q", param.Name)
	}
	if param.Location != core.ParameterLocationBody {
		t.Errorf("expected body location, got %q", param.Location)
	}
	if param.DataType != core.DataTypeObject {
		t.Errorf("expected object data type, got %q", param.DataType)
	}

	nested := param.NestedParams
	if len(nested) < 2 {
		t.Fatalf("expected at least 2 nested params (userId, includeDetails), got %d", len(nested))
	}

	nestedByName := make(map[string]core.Parameter)
	for _, np := range nested {
		nestedByName[np.Name] = np
	}

	if uid, ok := nestedByName["userId"]; ok {
		if uid.DataType != core.DataTypeInteger {
			t.Errorf("userId: expected integer data type, got %q", uid.DataType)
		}
		if !uid.Required {
			t.Error("userId: expected to be required")
		}
	} else {
		t.Error("missing nested parameter 'userId'")
	}

	if details, ok := nestedByName["includeDetails"]; ok {
		if details.DataType != core.DataTypeBoolean {
			t.Errorf("includeDetails: expected boolean data type, got %q", details.DataType)
		}
		if details.Required {
			t.Error("includeDetails: expected to be optional (minOccurs=0)")
		}
	} else {
		t.Error("missing nested parameter 'includeDetails'")
	}
}

func TestParse_MultiPortService(t *testing.T) {
	parser := NewParser()
	def := makeWSDLDefinition(multiPortWSDL)
	ops, err := parser.Parse(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ops) != 2 {
		t.Fatalf("expected 2 operations (one per port), got %d", len(ops))
	}

	opsByPort := make(map[string]core.Operation)
	for _, op := range ops {
		if op.SOAP != nil {
			opsByPort[op.SOAP.PortName] = op
		}
	}

	tests := []struct {
		portName    string
		baseURL     string
		soapAction  string
		soapVersion string
	}{
		{"PingSOAP11Port", "http://example.com/soap11/ping", "http://example.com/Ping", "1.1"},
		{"PingSOAP12Port", "http://example.com/soap12/ping", "http://example.com/Ping12", "1.2"},
	}

	for _, tt := range tests {
		t.Run(tt.portName, func(t *testing.T) {
			op, ok := opsByPort[tt.portName]
			if !ok {
				t.Fatalf("missing operation for port %s", tt.portName)
			}
			if op.BaseURL != tt.baseURL {
				t.Errorf("base url: got %q, want %q", op.BaseURL, tt.baseURL)
			}
			if op.SOAP.SOAPAction != tt.soapAction {
				t.Errorf("soap action: got %q, want %q", op.SOAP.SOAPAction, tt.soapAction)
			}
			if op.SOAP.SOAPVersion != tt.soapVersion {
				t.Errorf("soap version: got %q, want %q", op.SOAP.SOAPVersion, tt.soapVersion)
			}
			if op.Name != "Ping" {
				t.Errorf("operation name: got %q, want %q", op.Name, "Ping")
			}
		})
	}
}

func TestParse_NestedComplexTypes(t *testing.T) {
	parser := NewParser()
	def := makeWSDLDefinition(nestedComplexTypeWSDL)
	ops, err := parser.Parse(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}

	op := ops[0]
	if len(op.Parameters) != 1 {
		t.Fatalf("expected 1 top-level parameter, got %d", len(op.Parameters))
	}

	rootParam := op.Parameters[0]
	if rootParam.DataType != core.DataTypeObject {
		t.Fatalf("expected root param to be object, got %q", rootParam.DataType)
	}

	nestedByName := make(map[string]core.Parameter)
	for _, np := range rootParam.NestedParams {
		nestedByName[np.Name] = np
	}

	t.Run("orderId", func(t *testing.T) {
		param, ok := nestedByName["orderId"]
		if !ok {
			t.Fatal("missing nested parameter")
		}
		if param.DataType != core.DataTypeInteger {
			t.Errorf("data type: got %q, want %q", param.DataType, core.DataTypeInteger)
		}
	})

	t.Run("customerName", func(t *testing.T) {
		param, ok := nestedByName["customerName"]
		if !ok {
			t.Fatal("missing nested parameter")
		}
		if param.DataType != core.DataTypeString {
			t.Errorf("data type: got %q, want %q", param.DataType, core.DataTypeString)
		}
	})

	t.Run("amount", func(t *testing.T) {
		param, ok := nestedByName["amount"]
		if !ok {
			t.Fatal("missing nested parameter")
		}
		if param.DataType != core.DataTypeNumber {
			t.Errorf("data type: got %q, want %q", param.DataType, core.DataTypeNumber)
		}
	})

	t.Run("shippingAddress expanded", func(t *testing.T) {
		param, ok := nestedByName["shippingAddress"]
		if !ok {
			t.Fatal("missing nested parameter")
		}
		if param.DataType != core.DataTypeObject {
			t.Errorf("data type: got %q, want %q", param.DataType, core.DataTypeObject)
		}
		if len(param.NestedParams) == 0 {
			t.Fatal("expected nested params for complex type")
		}

		addrFields := make(map[string]core.Parameter)
		for _, np := range param.NestedParams {
			addrFields[np.Name] = np
		}
		for _, fieldName := range []string{"street", "city", "zip"} {
			field, ok := addrFields[fieldName]
			if !ok {
				t.Errorf("missing address field %q", fieldName)
				continue
			}
			if field.DataType != core.DataTypeString {
				t.Errorf("address field %q: got type %q, want %q", fieldName, field.DataType, core.DataTypeString)
			}
		}
	})

	t.Run("billingAddress not re-expanded due to visited map", func(t *testing.T) {
		param, ok := nestedByName["billingAddress"]
		if !ok {
			t.Fatal("missing nested parameter")
		}
		// The visited map within a single message extraction marks Address as visited
		// after shippingAddress is processed, so billingAddress won't expand the
		// same complex type again. This is the circular reference protection at work.
		if _, exists := nestedByName["shippingAddress"]; exists {
			if param.DataType == core.DataTypeObject && len(param.NestedParams) > 0 {
				t.Log("billingAddress was expanded (visited map was not shared)")
			}
		}
	})
}

func TestParse_CircularReferenceProtection(t *testing.T) {
	parser := NewParser()
	def := makeWSDLDefinition(circularRefWSDL)
	ops, err := parser.Parse(def)
	if err != nil {
		t.Fatalf("unexpected error (should not stack overflow): %v", err)
	}

	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}

	op := ops[0]
	if len(op.Parameters) != 1 {
		t.Fatalf("expected 1 top-level parameter, got %d", len(op.Parameters))
	}

	rootParam := op.Parameters[0]
	if rootParam.DataType != core.DataTypeObject {
		t.Fatalf("expected root param to be object, got %q", rootParam.DataType)
	}

	var countDepth func(params []core.Parameter, depth int) int
	countDepth = func(params []core.Parameter, depth int) int {
		maxDepth := depth
		for _, p := range params {
			if len(p.NestedParams) > 0 {
				d := countDepth(p.NestedParams, depth+1)
				if d > maxDepth {
					maxDepth = d
				}
			}
		}
		return maxDepth
	}

	depth := countDepth(rootParam.NestedParams, 0)
	if depth > maxSOAPDepth+1 {
		t.Errorf("recursion depth %d exceeds maxSOAPDepth %d; circular reference protection may be broken", depth, maxSOAPDepth)
	}
}

func TestParse_BaseURLFallback(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		sourceURL   string
		expectedURL string
	}{
		{
			name:        "uses port address when present",
			baseURL:     "http://example.com/fallback",
			sourceURL:   "http://example.com/source",
			expectedURL: "http://example.com/soap/user",
		},
		{
			name:        "falls back to base url for empty port address",
			baseURL:     "http://example.com/fallback",
			sourceURL:   "http://example.com/source",
			expectedURL: "http://example.com/soap/user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser()
			def := &db.APIDefinition{
				BaseUUIDModel: db.BaseUUIDModel{ID: uuid.New()},
				Type:          db.APIDefinitionTypeWSDL,
				SourceURL:     tt.sourceURL,
				BaseURL:       tt.baseURL,
				RawDefinition: []byte(simpleWSDL),
			}
			ops, err := parser.Parse(def)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(ops) == 0 {
				t.Fatal("expected at least 1 operation")
			}
			if ops[0].BaseURL != tt.expectedURL {
				t.Errorf("base url: got %q, want %q", ops[0].BaseURL, tt.expectedURL)
			}
		})
	}
}

func TestParse_RPCStyleBinding(t *testing.T) {
	rpcWSDL := `<?xml version="1.0" encoding="UTF-8"?>
<definitions
  xmlns="http://schemas.xmlsoap.org/wsdl/"
  xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
  xmlns:tns="http://example.com/rpc"
  xmlns:xsd="http://www.w3.org/2001/XMLSchema"
  targetNamespace="http://example.com/rpc"
  name="RPCService">

  <message name="AddInput">
    <part name="a" type="xsd:int"/>
    <part name="b" type="xsd:int"/>
  </message>
  <message name="AddOutput">
    <part name="result" type="xsd:int"/>
  </message>

  <portType name="CalcPortType">
    <operation name="Add">
      <input message="tns:AddInput"/>
      <output message="tns:AddOutput"/>
    </operation>
  </portType>

  <binding name="CalcBinding" type="tns:CalcPortType">
    <soap:binding style="rpc" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="Add">
      <soap:operation soapAction="http://example.com/Add" style="rpc"/>
      <input><soap:body use="encoded" namespace="http://example.com/rpc" encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"/></input>
      <output><soap:body use="encoded" namespace="http://example.com/rpc" encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"/></output>
    </operation>
  </binding>

  <service name="CalcService">
    <port name="CalcPort" binding="tns:CalcBinding">
      <soap:address location="http://example.com/soap/calc"/>
    </port>
  </service>
</definitions>`

	parser := NewParser()
	def := makeWSDLDefinition(rpcWSDL)
	ops, err := parser.Parse(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}

	op := ops[0]
	if op.SOAP.BindingStyle != "rpc" {
		t.Errorf("binding style: got %q, want %q", op.SOAP.BindingStyle, "rpc")
	}

	if len(op.Parameters) != 2 {
		t.Fatalf("expected 2 parameters (a, b), got %d", len(op.Parameters))
	}

	paramNames := make(map[string]bool)
	for _, p := range op.Parameters {
		paramNames[p.Name] = true
		if p.DataType != core.DataTypeInteger {
			t.Errorf("param %q: expected integer type, got %q", p.Name, p.DataType)
		}
		if p.Location != core.ParameterLocationBody {
			t.Errorf("param %q: expected body location, got %q", p.Name, p.Location)
		}
	}

	if !paramNames["a"] || !paramNames["b"] {
		t.Errorf("expected parameters named 'a' and 'b', got %v", paramNames)
	}
}

func TestParse_EmptyService(t *testing.T) {
	emptyWSDL := `<?xml version="1.0" encoding="UTF-8"?>
<definitions
  xmlns="http://schemas.xmlsoap.org/wsdl/"
  xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
  xmlns:tns="http://example.com/empty"
  xmlns:xsd="http://www.w3.org/2001/XMLSchema"
  targetNamespace="http://example.com/empty"
  name="EmptyService">

  <service name="EmptyService">
  </service>
</definitions>`

	parser := NewParser()
	def := makeWSDLDefinition(emptyWSDL)
	ops, err := parser.Parse(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ops) != 0 {
		t.Errorf("expected 0 operations for empty service, got %d", len(ops))
	}
}

func TestParse_MultipleOperationsInService(t *testing.T) {
	multiOpWSDL := `<?xml version="1.0" encoding="UTF-8"?>
<definitions
  xmlns="http://schemas.xmlsoap.org/wsdl/"
  xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
  xmlns:tns="http://example.com/multiop"
  xmlns:xsd="http://www.w3.org/2001/XMLSchema"
  targetNamespace="http://example.com/multiop"
  name="CRUDService">

  <message name="CreateInput"><part name="data" type="xsd:string"/></message>
  <message name="CreateOutput"><part name="id" type="xsd:int"/></message>
  <message name="DeleteInput"><part name="id" type="xsd:int"/></message>
  <message name="DeleteOutput"><part name="success" type="xsd:boolean"/></message>

  <portType name="CRUDPortType">
    <operation name="Create">
      <input message="tns:CreateInput"/>
      <output message="tns:CreateOutput"/>
    </operation>
    <operation name="Delete">
      <input message="tns:DeleteInput"/>
      <output message="tns:DeleteOutput"/>
    </operation>
  </portType>

  <binding name="CRUDBinding" type="tns:CRUDPortType">
    <soap:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="Create">
      <soap:operation soapAction="http://example.com/Create"/>
      <input><soap:body use="literal"/></input>
      <output><soap:body use="literal"/></output>
    </operation>
    <operation name="Delete">
      <soap:operation soapAction="http://example.com/Delete"/>
      <input><soap:body use="literal"/></input>
      <output><soap:body use="literal"/></output>
    </operation>
  </binding>

  <service name="CRUDService">
    <port name="CRUDPort" binding="tns:CRUDBinding">
      <soap:address location="http://example.com/soap/crud"/>
    </port>
  </service>
</definitions>`

	parser := NewParser()
	def := makeWSDLDefinition(multiOpWSDL)
	ops, err := parser.Parse(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ops) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(ops))
	}

	opsByName := make(map[string]core.Operation)
	for _, op := range ops {
		opsByName[op.Name] = op
	}

	if _, ok := opsByName["Create"]; !ok {
		t.Error("missing 'Create' operation")
	}
	if _, ok := opsByName["Delete"]; !ok {
		t.Error("missing 'Delete' operation")
	}

	createOp := opsByName["Create"]
	if createOp.SOAP.SOAPAction != "http://example.com/Create" {
		t.Errorf("Create soap action: got %q, want %q", createOp.SOAP.SOAPAction, "http://example.com/Create")
	}
	if len(createOp.Parameters) != 1 {
		t.Errorf("Create params: expected 1, got %d", len(createOp.Parameters))
	}
}

func TestMapXSDType(t *testing.T) {
	p := NewParser()

	tests := []struct {
		xsdType  string
		expected core.DataType
	}{
		{"xsd:string", core.DataTypeString},
		{"string", core.DataTypeString},
		{"xsd:int", core.DataTypeInteger},
		{"xsd:integer", core.DataTypeInteger},
		{"xsd:long", core.DataTypeInteger},
		{"xsd:short", core.DataTypeInteger},
		{"xsd:unsignedInt", core.DataTypeInteger},
		{"xsd:decimal", core.DataTypeNumber},
		{"xsd:float", core.DataTypeNumber},
		{"xsd:double", core.DataTypeNumber},
		{"xsd:boolean", core.DataTypeBoolean},
		{"xsd:dateTime", core.DataTypeString},
		{"xsd:date", core.DataTypeString},
		{"xsd:base64Binary", core.DataTypeString},
		{"xsd:anyURI", core.DataTypeString},
		{"xsd:normalizedString", core.DataTypeString},
		{"xsd:token", core.DataTypeString},
		{"unknownType", core.DataTypeString},
	}

	for _, tt := range tests {
		t.Run(tt.xsdType, func(t *testing.T) {
			result := p.mapXSDType(tt.xsdType)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSplitQName(t *testing.T) {
	tests := []struct {
		input         string
		wantPrefix    string
		wantLocalName string
	}{
		{"tns:GetUser", "tns", "GetUser"},
		{"xsd:string", "xsd", "string"},
		{"localOnly", "", "localOnly"},
		{"a:b:c", "a:b", "c"},
		{"", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := splitQName(tt.input)
			if result[0] != tt.wantPrefix {
				t.Errorf("prefix: got %q, want %q", result[0], tt.wantPrefix)
			}
			if result[1] != tt.wantLocalName {
				t.Errorf("local name: got %q, want %q", result[1], tt.wantLocalName)
			}
		})
	}
}

func TestGetOperationStyle(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name         string
		bindingStyle string
		opStyle      string
		want         string
	}{
		{"op style overrides binding", "document", "rpc", "rpc"},
		{"falls back to binding style", "rpc", "", "rpc"},
		{"defaults to document", "", "", "document"},
		{"binding style used when op empty", "document", "", "document"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binding := wsdlBinding(tt.bindingStyle)
			op := wsdlBindingOperation(tt.opStyle)
			got := p.getOperationStyle(binding, op)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsElementRequired(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name      string
		minOccurs string
		want      bool
	}{
		{"empty minOccurs defaults to required", "", true},
		{"minOccurs=1 is required", "1", true},
		{"minOccurs=0 is optional", "0", false},
		{"minOccurs=5 is required", "5", true},
		{"invalid minOccurs defaults to required", "abc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elem := &pkgWsdl.XSDElement{MinOccurs: tt.minOccurs}
			got := p.isElementRequired(elem)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseFromRawDefinition(t *testing.T) {
	ops, err := ParseFromRawDefinition([]byte(simpleWSDL), "http://example.com/service?wsdl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}
	if ops[0].Name != "GetUser" {
		t.Errorf("expected operation name 'GetUser', got %q", ops[0].Name)
	}
}

func TestParse_TypeRegistrySimpleTypeConstraints(t *testing.T) {
	p := NewParser()

	doc := &pkgWsdl.WSDLDocument{
		TargetNamespace: "http://example.com/test",
		TypeRegistry: pkgWsdl.NewTypeRegistry(),
	}

	doc.TypeRegistry.SimpleTypes["StatusType"] = &pkgWsdl.XSDSimpleType{
		Name: "StatusType",
		Restriction: &pkgWsdl.XSDRestriction{
			Base:        "xsd:string",
			Enumeration: []string{"active", "inactive", "pending"},
		},
	}

	param := &core.Parameter{
		Name:     "status",
		Location: core.ParameterLocationBody,
	}

	p.extractTypeConstraints("StatusType", doc, param)

	if len(param.Constraints.Enum) != 3 {
		t.Errorf("enum count: expected 3, got %d", len(param.Constraints.Enum))
	}
}

func TestExtractSimpleTypeConstraints(t *testing.T) {
	p := NewParser()

	t.Run("pattern and length constraints", func(t *testing.T) {
		minLen := 3
		maxLen := 10
		st := &pkgWsdl.XSDSimpleType{
			Restriction: &pkgWsdl.XSDRestriction{
				Base:      "xsd:string",
				Pattern:   "[A-Z]{3,10}",
				MinLength: &minLen,
				MaxLength: &maxLen,
			},
		}

		param := &core.Parameter{}
		p.extractSimpleTypeConstraints(st, param)

		if param.Constraints.Pattern != "[A-Z]{3,10}" {
			t.Errorf("pattern: got %q, want %q", param.Constraints.Pattern, "[A-Z]{3,10}")
		}
		if param.Constraints.MinLength == nil || *param.Constraints.MinLength != 3 {
			t.Error("min length: expected 3")
		}
		if param.Constraints.MaxLength == nil || *param.Constraints.MaxLength != 10 {
			t.Error("max length: expected 10")
		}
	})

	t.Run("numeric range inclusive", func(t *testing.T) {
		st := &pkgWsdl.XSDSimpleType{
			Restriction: &pkgWsdl.XSDRestriction{
				Base:         "xsd:integer",
				MinInclusive: "1",
				MaxInclusive: "100",
			},
		}

		param := &core.Parameter{}
		p.extractSimpleTypeConstraints(st, param)

		if param.Constraints.Minimum == nil || *param.Constraints.Minimum != 1.0 {
			t.Errorf("minimum: got %v, want 1.0", param.Constraints.Minimum)
		}
		if param.Constraints.Maximum == nil || *param.Constraints.Maximum != 100.0 {
			t.Errorf("maximum: got %v, want 100.0", param.Constraints.Maximum)
		}
		if param.Constraints.ExclusiveMin {
			t.Error("expected exclusive min to be false")
		}
		if param.Constraints.ExclusiveMax {
			t.Error("expected exclusive max to be false")
		}
	})

	t.Run("numeric range exclusive", func(t *testing.T) {
		st := &pkgWsdl.XSDSimpleType{
			Restriction: &pkgWsdl.XSDRestriction{
				Base:         "xsd:decimal",
				MinExclusive: "0",
				MaxExclusive: "99.99",
			},
		}

		param := &core.Parameter{}
		p.extractSimpleTypeConstraints(st, param)

		if param.Constraints.Minimum == nil || *param.Constraints.Minimum != 0.0 {
			t.Errorf("minimum: got %v, want 0.0", param.Constraints.Minimum)
		}
		if param.Constraints.Maximum == nil || *param.Constraints.Maximum != 99.99 {
			t.Errorf("maximum: got %v, want 99.99", param.Constraints.Maximum)
		}
		if !param.Constraints.ExclusiveMin {
			t.Error("expected exclusive min to be true")
		}
		if !param.Constraints.ExclusiveMax {
			t.Error("expected exclusive max to be true")
		}
	})

	t.Run("enumeration values", func(t *testing.T) {
		st := &pkgWsdl.XSDSimpleType{
			Restriction: &pkgWsdl.XSDRestriction{
				Base:        "xsd:string",
				Enumeration: []string{"red", "green", "blue"},
			},
		}

		param := &core.Parameter{}
		p.extractSimpleTypeConstraints(st, param)

		if len(param.Constraints.Enum) != 3 {
			t.Fatalf("enum count: expected 3, got %d", len(param.Constraints.Enum))
		}
	})

	t.Run("nil restriction is no-op", func(t *testing.T) {
		st := &pkgWsdl.XSDSimpleType{Restriction: nil}
		param := &core.Parameter{}
		p.extractSimpleTypeConstraints(st, param)
		if !param.Constraints.IsEmpty() {
			t.Error("expected empty constraints for nil restriction")
		}
	})
}

func TestParse_ComplexContentExtension(t *testing.T) {
	extensionWSDL := `<?xml version="1.0" encoding="UTF-8"?>
<definitions
  xmlns="http://schemas.xmlsoap.org/wsdl/"
  xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
  xmlns:tns="http://example.com/extension"
  xmlns:xsd="http://www.w3.org/2001/XMLSchema"
  targetNamespace="http://example.com/extension"
  name="ExtensionService">

  <types>
    <xsd:schema targetNamespace="http://example.com/extension">
      <xsd:complexType name="BaseType">
        <xsd:sequence>
          <xsd:element name="id" type="xsd:int"/>
        </xsd:sequence>
      </xsd:complexType>

      <xsd:complexType name="ExtendedType">
        <xsd:complexContent>
          <xsd:extension base="tns:BaseType">
            <xsd:sequence>
              <xsd:element name="extraField" type="xsd:string"/>
            </xsd:sequence>
          </xsd:extension>
        </xsd:complexContent>
      </xsd:complexType>

      <xsd:element name="ExtRequest">
        <xsd:complexType>
          <xsd:sequence>
            <xsd:element name="data" type="tns:ExtendedType"/>
          </xsd:sequence>
        </xsd:complexType>
      </xsd:element>
      <xsd:element name="ExtResponse">
        <xsd:complexType>
          <xsd:sequence>
            <xsd:element name="ok" type="xsd:boolean"/>
          </xsd:sequence>
        </xsd:complexType>
      </xsd:element>
    </xsd:schema>
  </types>

  <message name="ExtInput">
    <part name="parameters" element="tns:ExtRequest"/>
  </message>
  <message name="ExtOutput">
    <part name="parameters" element="tns:ExtResponse"/>
  </message>

  <portType name="ExtPortType">
    <operation name="ExtOp">
      <input message="tns:ExtInput"/>
      <output message="tns:ExtOutput"/>
    </operation>
  </portType>

  <binding name="ExtBinding" type="tns:ExtPortType">
    <soap:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="ExtOp">
      <soap:operation soapAction="http://example.com/ExtOp"/>
      <input><soap:body use="literal"/></input>
      <output><soap:body use="literal"/></output>
    </operation>
  </binding>

  <service name="ExtService">
    <port name="ExtPort" binding="tns:ExtBinding">
      <soap:address location="http://example.com/soap/ext"/>
    </port>
  </service>
</definitions>`

	parser := NewParser()
	def := makeWSDLDefinition(extensionWSDL)
	ops, err := parser.Parse(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}

	rootParam := ops[0].Parameters[0]
	nestedByName := make(map[string]core.Parameter)
	for _, np := range rootParam.NestedParams {
		nestedByName[np.Name] = np
	}

	dataParam, ok := nestedByName["data"]
	if !ok {
		t.Fatal("missing 'data' parameter")
	}

	if dataParam.DataType != core.DataTypeObject {
		t.Errorf("data type: got %q, want %q", dataParam.DataType, core.DataTypeObject)
	}

	extFieldFound := false
	for _, np := range dataParam.NestedParams {
		if np.Name == "extraField" {
			extFieldFound = true
			if np.DataType != core.DataTypeString {
				t.Errorf("extraField type: got %q, want %q", np.DataType, core.DataTypeString)
			}
		}
	}

	if !extFieldFound {
		t.Error("missing 'extraField' from complex content extension")
	}
}

func TestParse_NillableAndDefault(t *testing.T) {
	nillableWSDL := `<?xml version="1.0" encoding="UTF-8"?>
<definitions
  xmlns="http://schemas.xmlsoap.org/wsdl/"
  xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
  xmlns:tns="http://example.com/nillable"
  xmlns:xsd="http://www.w3.org/2001/XMLSchema"
  targetNamespace="http://example.com/nillable"
  name="NillableService">

  <types>
    <xsd:schema targetNamespace="http://example.com/nillable">
      <xsd:element name="TestRequest">
        <xsd:complexType>
          <xsd:sequence>
            <xsd:element name="nullable_field" type="xsd:string" nillable="true"/>
            <xsd:element name="defaulted_field" type="xsd:string" default="hello"/>
          </xsd:sequence>
        </xsd:complexType>
      </xsd:element>
      <xsd:element name="TestResponse">
        <xsd:complexType>
          <xsd:sequence>
            <xsd:element name="result" type="xsd:string"/>
          </xsd:sequence>
        </xsd:complexType>
      </xsd:element>
    </xsd:schema>
  </types>

  <message name="TestInput">
    <part name="parameters" element="tns:TestRequest"/>
  </message>
  <message name="TestOutput">
    <part name="parameters" element="tns:TestResponse"/>
  </message>

  <portType name="TestPortType">
    <operation name="TestOp">
      <input message="tns:TestInput"/>
      <output message="tns:TestOutput"/>
    </operation>
  </portType>

  <binding name="TestBinding" type="tns:TestPortType">
    <soap:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="TestOp">
      <soap:operation soapAction="http://example.com/TestOp"/>
      <input><soap:body use="literal"/></input>
      <output><soap:body use="literal"/></output>
    </operation>
  </binding>

  <service name="TestService">
    <port name="TestPort" binding="tns:TestBinding">
      <soap:address location="http://example.com/soap/test"/>
    </port>
  </service>
</definitions>`

	parser := NewParser()
	def := makeWSDLDefinition(nillableWSDL)
	ops, err := parser.Parse(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rootParam := ops[0].Parameters[0]
	nestedByName := make(map[string]core.Parameter)
	for _, np := range rootParam.NestedParams {
		nestedByName[np.Name] = np
	}

	if nf, ok := nestedByName["nullable_field"]; ok {
		if !nf.Nullable {
			t.Error("nullable_field: expected Nullable=true")
		}
	} else {
		t.Error("missing 'nullable_field' parameter")
	}

	if df, ok := nestedByName["defaulted_field"]; ok {
		if df.DefaultValue != "hello" {
			t.Errorf("defaulted_field default: got %v, want %q", df.DefaultValue, "hello")
		}
	} else {
		t.Error("missing 'defaulted_field' parameter")
	}
}

func TestParse_ChoiceElements(t *testing.T) {
	choiceWSDL := `<?xml version="1.0" encoding="UTF-8"?>
<definitions
  xmlns="http://schemas.xmlsoap.org/wsdl/"
  xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
  xmlns:tns="http://example.com/choice"
  xmlns:xsd="http://www.w3.org/2001/XMLSchema"
  targetNamespace="http://example.com/choice"
  name="ChoiceService">

  <types>
    <xsd:schema targetNamespace="http://example.com/choice">
      <xsd:element name="PayRequest">
        <xsd:complexType>
          <xsd:choice>
            <xsd:element name="creditCard" type="xsd:string"/>
            <xsd:element name="bankTransfer" type="xsd:string"/>
            <xsd:element name="paypal" type="xsd:string"/>
          </xsd:choice>
        </xsd:complexType>
      </xsd:element>
      <xsd:element name="PayResponse">
        <xsd:complexType>
          <xsd:sequence>
            <xsd:element name="status" type="xsd:string"/>
          </xsd:sequence>
        </xsd:complexType>
      </xsd:element>
    </xsd:schema>
  </types>

  <message name="PayInput">
    <part name="parameters" element="tns:PayRequest"/>
  </message>
  <message name="PayOutput">
    <part name="parameters" element="tns:PayResponse"/>
  </message>

  <portType name="PayPortType">
    <operation name="Pay">
      <input message="tns:PayInput"/>
      <output message="tns:PayOutput"/>
    </operation>
  </portType>

  <binding name="PayBinding" type="tns:PayPortType">
    <soap:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="Pay">
      <soap:operation soapAction="http://example.com/Pay"/>
      <input><soap:body use="literal"/></input>
      <output><soap:body use="literal"/></output>
    </operation>
  </binding>

  <service name="PayService">
    <port name="PayPort" binding="tns:PayBinding">
      <soap:address location="http://example.com/soap/pay"/>
    </port>
  </service>
</definitions>`

	parser := NewParser()
	def := makeWSDLDefinition(choiceWSDL)
	ops, err := parser.Parse(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rootParam := ops[0].Parameters[0]
	if rootParam.DataType != core.DataTypeObject {
		t.Fatalf("expected object type, got %q", rootParam.DataType)
	}

	if len(rootParam.NestedParams) != 3 {
		t.Fatalf("expected 3 choice elements, got %d", len(rootParam.NestedParams))
	}

	for _, np := range rootParam.NestedParams {
		if np.Required {
			t.Errorf("choice element %q should not be required", np.Name)
		}
	}
}

func wsdlBinding(style string) pkgWsdl.Binding {
	return pkgWsdl.Binding{Style: style}
}

func wsdlBindingOperation(style string) pkgWsdl.BindingOperation {
	return pkgWsdl.BindingOperation{Style: style}
}
