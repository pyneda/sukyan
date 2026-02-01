package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

func RunSOAPTests(opts APITestOptions) []APITestResult {
	var results []APITestResult

	taskLog := log.With().
		Str("module", "soap-tests").
		Uint("workspace", opts.WorkspaceID).
		Logger()

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			taskLog.Debug().Msg("Context cancelled, skipping SOAP tests")
			return results
		default:
		}
	}

	if opts.Definition == nil || opts.Definition.Type != db.APIDefinitionTypeWSDL {
		return results
	}

	taskLog.Debug().Msg("Running SOAP/WSDL-specific security tests")

	spoofResults := testSOAPActionSpoofing(opts)
	results = append(results, spoofResults...)

	return results
}

func testSOAPActionSpoofing(opts APITestOptions) []APITestResult {
	var results []APITestResult

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			return results
		default:
		}
	}

	if opts.Endpoint == nil || opts.Endpoint.SOAPAction == "" {
		return results
	}

	baseURL := opts.Definition.BaseURL
	if baseURL == "" {
		baseURL = opts.Definition.SourceURL
	}

	fakeAction := "http://example.com/FakeOperation"

	operationName := opts.Endpoint.OperationID
	if operationName == "" {
		operationName = opts.Endpoint.Name
	}
	targetNS := "http://tempuri.org/"
	if opts.Definition.WSDLTargetNamespace != nil && *opts.Definition.WSDLTargetNamespace != "" {
		targetNS = *opts.Definition.WSDLTargetNamespace
	}

	soapEnvelope := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">
  <soap:Body>
    <%s xmlns="%s"/>
  </soap:Body>
</soap:Envelope>`, operationName, targetNS)

	req, err := http.NewRequestWithContext(opts.Ctx, "POST", baseURL, strings.NewReader(soapEnvelope))
	if err != nil {
		return results
	}

	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", `"`+fakeAction+`"`)

	result := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		Client:        opts.HTTPClient,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: opts.WorkspaceID,
			ScanID:      opts.ScanID,
			ScanJobID:   opts.ScanJobID,
		},
	})

	if result.Err != nil || result.History == nil {
		return results
	}

	if result.History.StatusCode >= 200 && result.History.StatusCode < 300 {
		body, _ := result.History.ResponseBody()
		bodyStr := strings.ToLower(string(body))

		if !strings.Contains(bodyStr, "fault") && !strings.Contains(bodyStr, "error") {
			results = append(results, APITestResult{
				Vulnerable: true,
				IssueCode:  db.SoapActionSpoofingCode,
				Details: fmt.Sprintf(`SOAP Action spoofing may be possible.

The SOAP service accepted a request where the SOAPAction header (%s)
did not match the actual operation in the SOAP body.

This could indicate that:
- The service relies on SOAPAction for routing without validation
- Action-based access controls could be bypassed
- Different authorization levels might be accessible

Original SOAPAction: %s
Spoofed SOAPAction: %s
Response Status: %d

Further testing is recommended to verify the impact.`, fakeAction, opts.Endpoint.SOAPAction, fakeAction, result.History.StatusCode),
				Confidence: 70,
				Evidence:   body,
				History:    result.History,
			})
		}
	}

	return results
}
