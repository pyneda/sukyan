package soap

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/pkg/api/core"
)

type RequestBuilder struct {
	DefaultHeaders map[string]string
	AuthConfig     *AuthConfig
	SOAPVersion    string
}

type AuthConfig struct {
	Username      string
	Password      string
	WSSecurity    bool
	CustomHeaders map[string]string
}

const (
	SOAP11Namespace = "http://schemas.xmlsoap.org/soap/envelope/"
	SOAP12Namespace = "http://www.w3.org/2003/05/soap-envelope"
)

func NewRequestBuilder() *RequestBuilder {
	return &RequestBuilder{
		DefaultHeaders: map[string]string{
			"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36",
		},
		SOAPVersion: "1.1",
	}
}

func (b *RequestBuilder) WithAuth(config *AuthConfig) *RequestBuilder {
	b.AuthConfig = config
	return b
}

func (b *RequestBuilder) WithSOAPVersion(version string) *RequestBuilder {
	b.SOAPVersion = version
	return b
}

func (b *RequestBuilder) Build(ctx context.Context, op core.Operation, paramValues map[string]any) (*http.Request, error) {
	if op.SOAP == nil {
		return nil, fmt.Errorf("operation is not a SOAP operation")
	}

	fullBody := b.buildSOAPEnvelopeXML(op, paramValues)

	req, err := http.NewRequestWithContext(ctx, "POST", op.BaseURL, bytes.NewReader([]byte(fullBody)))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	soapVersion := b.SOAPVersion
	if op.SOAP.SOAPVersion != "" {
		soapVersion = op.SOAP.SOAPVersion
	}

	if soapVersion == "1.2" {
		req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	} else {
		req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	}

	if op.SOAP.SOAPAction != "" {
		if soapVersion == "1.2" {
			contentType := req.Header.Get("Content-Type")
			req.Header.Set("Content-Type", contentType+`; action="`+op.SOAP.SOAPAction+`"`)
		} else {
			req.Header.Set("SOAPAction", `"`+op.SOAP.SOAPAction+`"`)
		}
	}

	b.addDefaultHeaders(req)
	b.applyAuth(req)

	return req, nil
}

func (b *RequestBuilder) BuildWithModifiedParam(ctx context.Context, op core.Operation, paramName string, modifiedValue any, paramValues map[string]any) (*http.Request, error) {
	modifiedValues := make(map[string]any)
	for k, v := range paramValues {
		modifiedValues[k] = v
	}
	modifiedValues[paramName] = modifiedValue
	return b.Build(ctx, op, modifiedValues)
}

func (b *RequestBuilder) GetDefaultParamValues(op core.Operation) map[string]any {
	values := make(map[string]any)
	for _, param := range op.Parameters {
		values[param.Name] = param.GetEffectiveValue()
	}
	return values
}

func (b *RequestBuilder) buildSOAPEnvelopeXML(op core.Operation, paramValues map[string]any) string {
	soapVersion := b.SOAPVersion
	if op.SOAP != nil && op.SOAP.SOAPVersion != "" {
		soapVersion = op.SOAP.SOAPVersion
	}

	namespace := SOAP11Namespace
	if soapVersion == "1.2" {
		namespace = SOAP12Namespace
	}

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString(`<soap:Envelope xmlns:soap="`)
	sb.WriteString(namespace)
	sb.WriteString(`">` + "\n")

	if b.AuthConfig != nil && b.AuthConfig.WSSecurity {
		sb.WriteString("  <soap:Header>")
		sb.WriteString(b.buildWSSecurityHeader())
		sb.WriteString("</soap:Header>\n")
	}

	sb.WriteString("  <soap:Body>")
	sb.WriteString(b.buildBodyContent(op, paramValues))
	sb.WriteString("</soap:Body>\n")

	sb.WriteString("</soap:Envelope>")

	return sb.String()
}

func (b *RequestBuilder) buildBodyContent(op core.Operation, paramValues map[string]any) string {
	var sb strings.Builder

	targetNS := ""
	if op.SOAP != nil && op.SOAP.TargetNS != "" {
		targetNS = op.SOAP.TargetNS
	}

	sb.WriteString("\n    <")
	sb.WriteString(op.Name)
	if targetNS != "" {
		sb.WriteString(` xmlns="`)
		sb.WriteString(targetNS)
		sb.WriteString(`"`)
	}
	sb.WriteString(">\n")

	for _, param := range op.Parameters {
		value := paramValues[param.Name]
		if value == nil {
			value = param.GetEffectiveValue()
		}

		sb.WriteString("      ")
		sb.WriteString(b.buildElement(param.Name, value, param.NestedParams, 3))
		sb.WriteString("\n")
	}

	sb.WriteString("    </")
	sb.WriteString(op.Name)
	sb.WriteString(">\n  ")

	return sb.String()
}

func (b *RequestBuilder) buildElement(name string, value any, nestedParams []core.Parameter, depth int) string {
	if depth > 10 {
		return ""
	}

	indent := strings.Repeat("  ", depth)

	if value == nil {
		return fmt.Sprintf("<%s/>", name)
	}

	switch v := value.(type) {
	case map[string]any:
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("<%s>\n", name))
		for k, val := range v {
			var nestedParam []core.Parameter
			for _, np := range nestedParams {
				if np.Name == k {
					nestedParam = np.NestedParams
					break
				}
			}
			sb.WriteString(indent)
			sb.WriteString("  ")
			sb.WriteString(b.buildElement(k, val, nestedParam, depth+1))
			sb.WriteString("\n")
		}
		sb.WriteString(indent)
		sb.WriteString(fmt.Sprintf("</%s>", name))
		return sb.String()
	case []any:
		var sb strings.Builder
		for _, item := range v {
			sb.WriteString(b.buildElement(name, item, nestedParams, depth))
		}
		return sb.String()
	default:
		escapedValue := escapeXML(fmt.Sprintf("%v", v))
		return fmt.Sprintf("<%s>%s</%s>", name, escapedValue, name)
	}
}

func (b *RequestBuilder) buildWSSecurityHeader() string {
	if b.AuthConfig == nil {
		return ""
	}

	return fmt.Sprintf(`
    <wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
      <wsse:UsernameToken>
        <wsse:Username>%s</wsse:Username>
        <wsse:Password>%s</wsse:Password>
      </wsse:UsernameToken>
    </wsse:Security>
  `, escapeXML(b.AuthConfig.Username), escapeXML(b.AuthConfig.Password))
}

func (b *RequestBuilder) addDefaultHeaders(req *http.Request) {
	for k, v := range b.DefaultHeaders {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}
}

func (b *RequestBuilder) applyAuth(req *http.Request) {
	if b.AuthConfig == nil {
		return
	}

	if b.AuthConfig.Username != "" && b.AuthConfig.Password != "" && !b.AuthConfig.WSSecurity {
		req.SetBasicAuth(b.AuthConfig.Username, b.AuthConfig.Password)
	}

	for k, v := range b.AuthConfig.CustomHeaders {
		req.Header.Set(k, v)
	}
}

func escapeXML(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			sb.WriteString("&amp;")
		case '<':
			sb.WriteString("&lt;")
		case '>':
			sb.WriteString("&gt;")
		case '"':
			sb.WriteString("&quot;")
		case '\'':
			sb.WriteString("&apos;")
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func BuildRequest(ctx context.Context, op core.Operation, paramValues map[string]any) (*http.Request, error) {
	builder := NewRequestBuilder()
	return builder.Build(ctx, op, paramValues)
}
