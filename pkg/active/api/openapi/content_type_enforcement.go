package openapi

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

// ContentTypeEnforcementAudit tests if endpoints accept content types
// not declared in the OpenAPI specification.
type ContentTypeEnforcementAudit struct {
	Options     *OpenAPIAuditOptions
	Definition  *db.APIDefinition
	Endpoint    *db.APIEndpoint
	BaseHistory *db.History
}

// contentTypeTest represents a test for an undocumented content type
type contentTypeTest struct {
	contentType string
	transform   func([]byte) ([]byte, error)
	name        string
	riskNote    string
}

// getContentTypeTests returns tests for common content types that might be
// processed unexpectedly
func getContentTypeTests(originalBody []byte) []contentTypeTest {
	tests := []contentTypeTest{
		{
			contentType: "application/xml",
			transform:   lib.JSONToXML,
			name:        "XML",
			riskNote:    "XML acceptance may enable XXE injection attacks",
		},
		{
			contentType: "text/xml",
			transform:   lib.JSONToXML,
			name:        "Text/XML",
			riskNote:    "XML acceptance may enable XXE injection attacks",
		},
		{
			contentType: "application/x-www-form-urlencoded",
			transform:   lib.JSONToFormURLEncoded,
			name:        "Form URL Encoded",
			riskNote:    "Form data parsing may have different validation rules",
		},
	}
	return tests
}

// Run executes the content type enforcement audit
func (a *ContentTypeEnforcementAudit) Run() {
	auditLog := log.With().
		Str("audit", "openapi-content-type-enforcement").
		Uint("workspace", a.Options.WorkspaceID).
		Logger()

	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			auditLog.Debug().Msg("Context cancelled, skipping content type enforcement audit")
			return
		default:
		}
	}

	// Only test endpoints that accept request bodies
	method := strings.ToUpper(a.Endpoint.Method)
	if method != "POST" && method != "PUT" && method != "PATCH" {
		auditLog.Debug().Msg("Skipping content type audit for non-body method")
		return
	}

	// Get declared content types from the operation
	declaredTypes := a.getDeclaredContentTypes()
	if len(declaredTypes) == 0 {
		auditLog.Debug().Msg("No content types declared in spec, skipping")
		return
	}

	auditLog.Info().
		Str("path", a.Endpoint.Path).
		Str("method", a.Endpoint.Method).
		Strs("declared_types", declaredTypes).
		Msg("Starting content type enforcement audit")

	client := a.Options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	// Get original request body
	originalBody, err := a.BaseHistory.RequestBody()
	if err != nil {
		auditLog.Debug().Err(err).Msg("Could not read original request body")
		originalBody = []byte{}
	}

	tests := getContentTypeTests(originalBody)
	for _, test := range tests {
		if a.Options.Ctx != nil {
			select {
			case <-a.Options.Ctx.Done():
				return
			default:
			}
		}

		// Skip if this content type is declared in the spec
		if a.isContentTypeDeclared(test.contentType, declaredTypes) {
			continue
		}

		a.testContentType(client, test, originalBody, declaredTypes)
	}

	auditLog.Info().Msg("Completed content type enforcement audit")
}

// getDeclaredContentTypes extracts declared request content types from the operation
func (a *ContentTypeEnforcementAudit) getDeclaredContentTypes() []string {
	if a.Options.Operation == nil {
		return nil
	}
	return a.Options.Operation.ContentTypes.Request
}

// equivalentContentTypes maps content types that are semantically identical
// and should not be flagged when either variant is declared in the spec.
var equivalentContentTypes = map[string][]string{
	"application/xml":        {"text/xml"},
	"text/xml":               {"application/xml"},
	"application/json":       {"text/json"},
	"text/json":              {"application/json"},
	"application/javascript": {"text/javascript"},
	"text/javascript":        {"application/javascript"},
}

// isContentTypeDeclared checks if a content type (or a semantically equivalent one)
// matches any declared type
func (a *ContentTypeEnforcementAudit) isContentTypeDeclared(contentType string, declared []string) bool {
	baseType := strings.TrimSpace(strings.Split(strings.ToLower(contentType), ";")[0])

	for _, d := range declared {
		declaredBase := strings.TrimSpace(strings.Split(strings.ToLower(d), ";")[0])
		if declaredBase == baseType {
			return true
		}
	}

	if equivalents, ok := equivalentContentTypes[baseType]; ok {
		for _, equiv := range equivalents {
			for _, d := range declared {
				declaredBase := strings.TrimSpace(strings.Split(strings.ToLower(d), ";")[0])
				if declaredBase == equiv {
					return true
				}
			}
		}
	}

	return false
}

// testContentType tests if the server accepts an undocumented content type
func (a *ContentTypeEnforcementAudit) testContentType(client *http.Client, test contentTypeTest, originalBody []byte, declaredTypes []string) {
	// Transform the body to the test content type
	transformedBody, err := test.transform(originalBody)
	if err != nil {
		log.Debug().Err(err).Str("type", test.name).Msg("Failed to transform body")
		return
	}

	req, err := http_utils.BuildRequestFromHistoryItem(a.BaseHistory)
	if err != nil {
		log.Error().Err(err).Msg("Failed to build request from history")
		return
	}

	req = req.WithContext(a.Options.Ctx)
	req.Header.Set("Content-Type", test.contentType)
	req.Body = io.NopCloser(bytes.NewReader(transformedBody))
	req.ContentLength = int64(len(transformedBody))

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

	// Check if the response indicates the content type was processed
	if a.wasContentTypeAccepted(result.History, test) {
		a.reportVulnerability(result.History, test, declaredTypes)
	}
}

// wasContentTypeAccepted determines if the server processed the undocumented content type
func (a *ContentTypeEnforcementAudit) wasContentTypeAccepted(history *db.History, test contentTypeTest) bool {
	// First check against behavior fingerprints if available
	if a.Options.BehaviorResult != nil {
		body, _ := history.ResponseBody()
		bodyHash := fmt.Sprintf("%x", sha256.Sum256(body))

		// Check if it matches the invalid content type fingerprint
		fps := a.Options.BehaviorResult.GetInvalidContentTypeFingerprints()
		for _, fp := range fps {
			if fp.ResponseHash != "" && fp.ResponseHash == bodyHash {
				return false // Known rejection response
			}
			if fp.StatusCode == history.StatusCode && fp.BodySize == len(body) {
				return false
			}
		}
	}

	// 415 Unsupported Media Type is the correct rejection
	if history.StatusCode == 415 {
		return false
	}

	// 4xx with content type error messages indicate rejection
	if history.StatusCode >= 400 && history.StatusCode < 500 {
		body, err := history.ResponseBody()
		if err == nil {
			bodyLower := strings.ToLower(string(body))
			rejectionIndicators := []string{
				"unsupported media type", "content type", "content-type",
				"invalid content", "unexpected content", "wrong content",
				"only accepts", "must be", "expected", "application/json",
			}
			for _, indicator := range rejectionIndicators {
				if strings.Contains(bodyLower, indicator) {
					return false
				}
			}
		}
	}

	// 2xx responses indicate the content type was processed
	if history.StatusCode >= 200 && history.StatusCode < 300 {
		// Additional check: ensure it's not an error wrapped in 200
		body, err := history.ResponseBody()
		if err == nil {
			bodyLower := strings.ToLower(string(body))
			errorIndicators := []string{
				"\"error\":", "\"errors\":", "\"status\":\"error\"",
				"\"success\":false", "unsupported", "invalid",
			}
			for _, indicator := range errorIndicators {
				if strings.Contains(bodyLower, indicator) {
					return false
				}
			}
		}
		return true
	}

	return false
}

// reportVulnerability creates the issue with technical details
func (a *ContentTypeEnforcementAudit) reportVulnerability(history *db.History, test contentTypeTest, declaredTypes []string) {
	details := fmt.Sprintf(`The endpoint accepted a request with an undocumented Content-Type.

Endpoint: %s %s
Operation ID: %s

Documented Content-Types:
%s

Accepted Undocumented Content-Type: %s

Test Details:
- Sent request with Content-Type: %s
- Response Status: %d
- Response Size: %d bytes

Security Implications:
%s

The server processed a content type not declared in the OpenAPI specification,
indicating that request parsing is more permissive than documented.`,
		a.Endpoint.Method,
		a.Endpoint.Path,
		a.Endpoint.OperationID,
		"- "+strings.Join(declaredTypes, "\n- "),
		test.contentType,
		test.contentType,
		history.StatusCode,
		history.ResponseBodySize,
		test.riskNote,
	)

	reportIssue(history, db.ApiUndocumentedContentTypeAcceptedCode, details, 70, a.Options)
}
