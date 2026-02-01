package soap

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/pkg/api/core"
)

func makeSOAPOperation(name, baseURL, targetNS, soapVersion, soapAction string, params []core.Parameter) core.Operation {
	return core.Operation{
		APIType: core.APITypeSOAP,
		Name:    name,
		Method:  "POST",
		BaseURL: baseURL,
		SOAP: &core.SOAPMetadata{
			ServiceName: "TestService",
			PortName:    "TestPort",
			SOAPAction:  soapAction,
			SOAPVersion: soapVersion,
			TargetNS:    targetNS,
		},
		Parameters: params,
	}
}

func TestSOAPEnvelopeNamespacePrefix(t *testing.T) {
	builder := NewRequestBuilder()
	op := makeSOAPOperation(
		"GetUser",
		"http://example.com/soap",
		"http://example.com/test",
		"1.1",
		"http://example.com/GetUser",
		[]core.Parameter{
			{
				Name:     "userId",
				Location: core.ParameterLocationBody,
				DataType: core.DataTypeInteger,
			},
		},
	)

	req, err := builder.Build(context.Background(), op, map[string]any{"userId": 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	bodyStr := string(body)

	checks := []struct {
		name     string
		contains string
	}{
		{"soap:Envelope open tag", "<soap:Envelope"},
		{"soap:Envelope close tag", "</soap:Envelope>"},
		{"soap:Body open tag", "<soap:Body>"},
		{"soap:Body close tag", "</soap:Body>"},
		{"XML declaration", `<?xml version="1.0" encoding="UTF-8"?>`},
		{"xmlns:soap attribute", `xmlns:soap="`},
		{"operation element", "<GetUser"},
		{"parameter value", "<userId>42</userId>"},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if !strings.Contains(bodyStr, check.contains) {
				t.Errorf("body missing %q\nbody:\n%s", check.contains, bodyStr)
			}
		})
	}

	t.Run("no unprefixed Envelope tag", func(t *testing.T) {
		if strings.Contains(bodyStr, "<Envelope") && !strings.Contains(bodyStr, "<soap:Envelope") {
			t.Errorf("body contains unprefixed <Envelope> tag\nbody:\n%s", bodyStr)
		}
	})
}

func TestSOAPEnvelopeSOAP11Namespace(t *testing.T) {
	builder := NewRequestBuilder()
	op := makeSOAPOperation(
		"TestOp",
		"http://example.com/soap",
		"http://example.com/test",
		"1.1",
		"",
		nil,
	)

	req, err := builder.Build(context.Background(), op, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	bodyStr := string(body)

	if !strings.Contains(bodyStr, SOAP11Namespace) {
		t.Errorf("SOAP 1.1 body missing namespace %q\nbody:\n%s", SOAP11Namespace, bodyStr)
	}

	if strings.Contains(bodyStr, SOAP12Namespace) {
		t.Errorf("SOAP 1.1 body should not contain 1.2 namespace\nbody:\n%s", bodyStr)
	}

	contentType := req.Header.Get("Content-Type")
	if contentType != "text/xml; charset=utf-8" {
		t.Errorf("content type: got %q, want %q", contentType, "text/xml; charset=utf-8")
	}
}

func TestSOAPEnvelopeSOAP12Namespace(t *testing.T) {
	builder := NewRequestBuilder()
	op := makeSOAPOperation(
		"TestOp",
		"http://example.com/soap",
		"http://example.com/test",
		"1.2",
		"http://example.com/TestAction",
		nil,
	)

	req, err := builder.Build(context.Background(), op, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	bodyStr := string(body)

	if !strings.Contains(bodyStr, SOAP12Namespace) {
		t.Errorf("SOAP 1.2 body missing namespace %q\nbody:\n%s", SOAP12Namespace, bodyStr)
	}

	if strings.Contains(bodyStr, SOAP11Namespace) {
		t.Errorf("SOAP 1.2 body should not contain 1.1 namespace\nbody:\n%s", bodyStr)
	}

	contentType := req.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/soap+xml") {
		t.Errorf("content type should contain application/soap+xml, got %q", contentType)
	}

	if !strings.Contains(contentType, `action="http://example.com/TestAction"`) {
		t.Errorf("SOAP 1.2 content type should include action, got %q", contentType)
	}
}

func TestSOAPEnvelopeWSSecurityHeader(t *testing.T) {
	builder := NewRequestBuilder().WithAuth(&AuthConfig{
		Username:   "admin",
		Password:   "secret",
		WSSecurity: true,
	})

	op := makeSOAPOperation(
		"SecureOp",
		"http://example.com/soap",
		"http://example.com/test",
		"1.1",
		"",
		nil,
	)

	req, err := builder.Build(context.Background(), op, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	bodyStr := string(body)

	checks := []struct {
		name     string
		contains string
	}{
		{"soap:Header open tag", "<soap:Header>"},
		{"soap:Header close tag", "</soap:Header>"},
		{"wsse:Security element", "<wsse:Security"},
		{"wsse:Username", "<wsse:Username>admin</wsse:Username>"},
		{"wsse:Password", "<wsse:Password>secret</wsse:Password>"},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if !strings.Contains(bodyStr, check.contains) {
				t.Errorf("body missing %q\nbody:\n%s", check.contains, bodyStr)
			}
		})
	}

	headerIdx := strings.Index(bodyStr, "<soap:Header>")
	bodyIdx := strings.Index(bodyStr, "<soap:Body>")
	if headerIdx >= bodyIdx {
		t.Errorf("soap:Header should appear before soap:Body in the envelope")
	}
}

func TestSOAPEnvelopeNoHeaderWithoutWSSecurity(t *testing.T) {
	builder := NewRequestBuilder()
	op := makeSOAPOperation(
		"SimpleOp",
		"http://example.com/soap",
		"http://example.com/test",
		"1.1",
		"",
		nil,
	)

	req, err := builder.Build(context.Background(), op, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	bodyStr := string(body)

	if strings.Contains(bodyStr, "<soap:Header>") {
		t.Errorf("body should not contain soap:Header when WSSecurity is not enabled\nbody:\n%s", bodyStr)
	}
}

func TestSOAPEnvelopeTargetNamespace(t *testing.T) {
	builder := NewRequestBuilder()
	targetNS := "http://example.com/myservice"
	op := makeSOAPOperation(
		"MyOperation",
		"http://example.com/soap",
		targetNS,
		"1.1",
		"",
		[]core.Parameter{
			{
				Name:         "name",
				Location:     core.ParameterLocationBody,
				DataType:     core.DataTypeString,
				ExampleValue: "test",
			},
		},
	)

	req, err := builder.Build(context.Background(), op, map[string]any{"name": "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	bodyStr := string(body)

	if !strings.Contains(bodyStr, `xmlns="`+targetNS+`"`) {
		t.Errorf("body missing target namespace\nbody:\n%s", bodyStr)
	}
}

func TestSOAPEnvelopeVersionFromBuilder(t *testing.T) {
	builder := NewRequestBuilder().WithSOAPVersion("1.2")
	op := makeSOAPOperation(
		"TestOp",
		"http://example.com/soap",
		"",
		"",
		"",
		nil,
	)

	req, err := builder.Build(context.Background(), op, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	bodyStr := string(body)

	if !strings.Contains(bodyStr, SOAP12Namespace) {
		t.Errorf("builder version 1.2 should use SOAP 1.2 namespace\nbody:\n%s", bodyStr)
	}
}

func TestSOAPEnvelopeOperationVersionOverridesBuilder(t *testing.T) {
	builder := NewRequestBuilder().WithSOAPVersion("1.1")
	op := makeSOAPOperation(
		"TestOp",
		"http://example.com/soap",
		"",
		"1.2",
		"",
		nil,
	)

	req, err := builder.Build(context.Background(), op, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	bodyStr := string(body)

	if !strings.Contains(bodyStr, SOAP12Namespace) {
		t.Errorf("operation version should override builder version\nbody:\n%s", bodyStr)
	}
}

func TestSOAPEnvelopeNilSOAPMetadata(t *testing.T) {
	builder := NewRequestBuilder()
	op := core.Operation{
		APIType: core.APITypeSOAP,
		Name:    "TestOp",
		Method:  "POST",
		BaseURL: "http://example.com/soap",
		SOAP:    nil,
	}

	_, err := builder.Build(context.Background(), op, nil)
	if err == nil {
		t.Fatal("expected error for nil SOAP metadata")
	}
}

func TestSOAPEnvelopeSOAPActionHeader(t *testing.T) {
	builder := NewRequestBuilder()
	op := makeSOAPOperation(
		"TestOp",
		"http://example.com/soap",
		"",
		"1.1",
		"http://example.com/TestAction",
		nil,
	)

	req, err := builder.Build(context.Background(), op, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	soapAction := req.Header.Get("SOAPAction")
	if soapAction != `"http://example.com/TestAction"` {
		t.Errorf("SOAPAction header: got %q, want %q", soapAction, `"http://example.com/TestAction"`)
	}
}

func TestSOAPEnvelopeXMLEscaping(t *testing.T) {
	builder := NewRequestBuilder().WithAuth(&AuthConfig{
		Username:   "user<>&\"'",
		Password:   "pass<>&\"'",
		WSSecurity: true,
	})

	op := makeSOAPOperation(
		"TestOp",
		"http://example.com/soap",
		"",
		"1.1",
		"",
		nil,
	)

	req, err := builder.Build(context.Background(), op, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	bodyStr := string(body)

	if strings.Contains(bodyStr, "<wsse:Username>user<>") {
		t.Errorf("XML special characters not properly escaped in username\nbody:\n%s", bodyStr)
	}

	if !strings.Contains(bodyStr, "user&lt;&gt;&amp;&quot;&apos;") {
		t.Errorf("expected escaped username in body\nbody:\n%s", bodyStr)
	}
}

func TestSOAPEnvelopeBuildWithModifiedParam(t *testing.T) {
	builder := NewRequestBuilder()
	op := makeSOAPOperation(
		"GetUser",
		"http://example.com/soap",
		"http://example.com/test",
		"1.1",
		"",
		[]core.Parameter{
			{
				Name:     "userId",
				Location: core.ParameterLocationBody,
				DataType: core.DataTypeInteger,
			},
			{
				Name:     "name",
				Location: core.ParameterLocationBody,
				DataType: core.DataTypeString,
			},
		},
	)

	originalValues := map[string]any{"userId": 1, "name": "original"}

	req, err := builder.BuildWithModifiedParam(context.Background(), op, "name", "modified", originalValues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "<name>modified</name>") {
		t.Errorf("modified param not found in body\nbody:\n%s", bodyStr)
	}

	if originalValues["name"] != "original" {
		t.Error("BuildWithModifiedParam should not modify original map")
	}
}
