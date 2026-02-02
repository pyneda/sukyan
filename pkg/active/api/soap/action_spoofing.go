package soap

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

// ActionSpoofingAudit tests for SOAP Action spoofing vulnerabilities
type ActionSpoofingAudit struct {
	Options     *SOAPAuditOptions
	Definition  *db.APIDefinition
	Endpoint    *db.APIEndpoint
	BaseHistory *db.History
}

// spoofingTest represents a SOAP action spoofing test configuration
type spoofingTest struct {
	name        string
	fakeAction  string
	description string
}

// getSpoofingTests returns various SOAP action spoofing test configurations
func getSpoofingTests() []spoofingTest {
	return []spoofingTest{
		{
			name:        "fake_operation",
			fakeAction:  "http://example.com/FakeOperation",
			description: "Completely fake operation action",
		},
		{
			name:        "admin_operation",
			fakeAction:  "http://tempuri.org/AdminOperation",
			description: "Potential admin operation",
		},
		{
			name:        "delete_operation",
			fakeAction:  "http://tempuri.org/Delete",
			description: "Destructive delete operation",
		},
		{
			name:        "internal_operation",
			fakeAction:  "http://internal.example.com/InternalOp",
			description: "Internal/private operation",
		},
		{
			name:        "empty_action",
			fakeAction:  "",
			description: "Empty SOAPAction header",
		},
	}
}

// Run executes the SOAP action spoofing audit
func (a *ActionSpoofingAudit) Run() {
	auditLog := log.With().
		Str("audit", "soap-action-spoofing").
		Uint("workspace", a.Options.WorkspaceID).
		Logger()

	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			auditLog.Debug().Msg("Context cancelled, skipping SOAP action spoofing audit")
			return
		default:
		}
	}

	if a.Endpoint == nil || a.Endpoint.SOAPAction == "" {
		return
	}

	baseURL := a.Definition.BaseURL
	if baseURL == "" {
		baseURL = a.Definition.SourceURL
	}

	auditLog.Info().
		Str("url", baseURL).
		Str("original_action", a.Endpoint.SOAPAction).
		Msg("Starting SOAP action spoofing audit")

	client := a.Options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	tests := getSpoofingTests()
	for _, test := range tests {
		if a.Options.Ctx != nil {
			select {
			case <-a.Options.Ctx.Done():
				return
			default:
			}
		}

		a.testSpoofedAction(baseURL, client, test)
	}

	auditLog.Info().Msg("Completed SOAP action spoofing audit")
}

// testSpoofedAction tests a specific spoofed SOAP action
func (a *ActionSpoofingAudit) testSpoofedAction(baseURL string, client *http.Client, test spoofingTest) {
	operationName := a.Endpoint.OperationID
	if operationName == "" {
		operationName = a.Endpoint.Name
	}

	targetNS := "http://tempuri.org/"
	if a.Definition.WSDLTargetNamespace != nil && *a.Definition.WSDLTargetNamespace != "" {
		targetNS = *a.Definition.WSDLTargetNamespace
	}

	soapEnvelope := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">
  <soap:Body>
    <%s xmlns="%s"/>
  </soap:Body>
</soap:Envelope>`, operationName, targetNS)

	req, err := http.NewRequestWithContext(a.Options.Ctx, "POST", baseURL, strings.NewReader(soapEnvelope))
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	if test.fakeAction != "" {
		req.Header.Set("SOAPAction", `"`+test.fakeAction+`"`)
	} else {
		req.Header.Set("SOAPAction", `""`)
	}

	result := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		Client:        client,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: a.Options.WorkspaceID,
			ScanID:      a.Options.ScanID,
			ScanJobID:   a.Options.ScanJobID,
		},
	})

	if result.Err != nil || result.History == nil {
		return
	}

	if result.History.StatusCode >= 200 && result.History.StatusCode < 300 {
		body, _ := result.History.ResponseBody()
		bodyStr := strings.ToLower(string(body))

		// Check for SOAP faults which would indicate the server properly validated
		if !strings.Contains(bodyStr, "fault") && !strings.Contains(bodyStr, "error") {
			details := fmt.Sprintf(`SOAP Action spoofing may be possible.

Test: %s
Description: %s

The SOAP service accepted a request where the SOAPAction header did not match
the actual operation in the SOAP body.

Original SOAPAction: %s
Spoofed SOAPAction: %s
Response Status: %d

Attack vectors:
- Bypass action-based access controls
- Execute privileged operations by spoofing their action
- Access internal/admin operations
- Circumvent WAF rules that filter by SOAPAction

Technical details:
- SOAP body operation: %s
- Target namespace: %s

Impact:
The service may route requests based on SOAPAction without validating it matches
the actual operation. An attacker could potentially:
1. Call any operation while claiming a different action
2. Bypass authorization checks tied to SOAPAction
3. Access operations that should be restricted

Remediation:
- Validate SOAPAction header matches the operation in SOAP body
- Implement operation-level authorization regardless of SOAPAction
- Use WS-Security for message-level authentication
- Consider disabling SOAPAction-based routing`, test.name, test.description, a.Endpoint.SOAPAction, test.fakeAction, result.History.StatusCode, operationName, targetNS)

			reportIssue(result.History, db.SoapActionSpoofingCode, details, 70, a.Options)
		}
	}
}
