package api

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

func RunRESTTests(opts APITestOptions) []APITestResult {
	var results []APITestResult

	taskLog := log.With().
		Str("module", "rest-tests").
		Uint("workspace", opts.WorkspaceID).
		Logger()

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			taskLog.Debug().Msg("Context cancelled, skipping REST tests")
			return results
		default:
		}
	}

	if opts.Definition == nil || opts.Definition.Type != db.APIDefinitionTypeOpenAPI {
		return results
	}

	taskLog.Debug().Msg("Running REST API-specific security tests")

	overrideResults := testMethodOverride(opts)
	results = append(results, overrideResults...)

	massAssignmentResults := testMassAssignment(opts)
	results = append(results, massAssignmentResults...)

	return results
}

func testMethodOverride(opts APITestOptions) []APITestResult {
	var results []APITestResult

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			return results
		default:
		}
	}

	if opts.Endpoint == nil || opts.Endpoint.Method != "GET" {
		return results
	}

	baseURL := opts.Definition.BaseURL
	if baseURL == "" {
		baseURL = opts.Definition.SourceURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	path := opts.Endpoint.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	fullURL := baseURL + path

	overrideHeaders := []struct {
		header string
		method string
	}{
		{"X-HTTP-Method-Override", "DELETE"},
		{"X-Method-Override", "DELETE"},
		{"X-HTTP-Method", "DELETE"},
	}

	for _, oh := range overrideHeaders {
		if opts.Ctx != nil {
			select {
			case <-opts.Ctx.Done():
				return results
			default:
			}
		}

		req, err := http.NewRequestWithContext(opts.Ctx, "GET", fullURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set(oh.header, oh.method)

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
			continue
		}

		if result.History.StatusCode != 405 && result.History.StatusCode != 400 {
			if opts.BaseHistory != nil && result.History.StatusCode != opts.BaseHistory.StatusCode {
				results = append(results, APITestResult{
					Vulnerable: true,
					IssueCode:  db.HttpMethodOverrideCode,
					Details: fmt.Sprintf(`HTTP method override is accepted via the %s header.

Original request method: GET
Override header: %s: %s

The server processed the request differently when the override header was present,
suggesting the method may have been overridden to %s.

Original response status: %d
Override response status: %d

This could potentially be used to bypass method-based access controls.

URL: %s`, oh.header, oh.header, oh.method, oh.method, opts.BaseHistory.StatusCode, result.History.StatusCode, fullURL),
					Confidence: 80,
					History:    result.History,
				})
			}
		}
	}

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			return results
		default:
		}
	}

	queryOverrideURL := fullURL
	if strings.Contains(fullURL, "?") {
		queryOverrideURL += "&_method=DELETE"
	} else {
		queryOverrideURL += "?_method=DELETE"
	}

	req, err := http.NewRequestWithContext(opts.Ctx, "GET", queryOverrideURL, nil)
	if err == nil {
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

		if result.Err == nil && result.History != nil {
			if result.History.StatusCode != 405 && result.History.StatusCode != 400 {
				if opts.BaseHistory != nil && result.History.StatusCode != opts.BaseHistory.StatusCode {
					results = append(results, APITestResult{
						Vulnerable: true,
						IssueCode:  db.HttpMethodOverrideCode,
						Details: fmt.Sprintf(`HTTP method override is accepted via _method query parameter.

Original request: GET %s
Override request: GET %s

The server processed the request differently when the _method parameter was present.

Original response status: %d
Override response status: %d

This could potentially be used to bypass method-based access controls.`, fullURL, queryOverrideURL, opts.BaseHistory.StatusCode, result.History.StatusCode),
						Confidence: 80,
						History:    result.History,
					})
				}
			}
		}
	}

	return results
}

func testMassAssignment(opts APITestOptions) []APITestResult {
	var results []APITestResult

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			return results
		default:
		}
	}

	if opts.Endpoint == nil {
		return results
	}

	method := strings.ToUpper(opts.Endpoint.Method)
	if method != "POST" && method != "PUT" && method != "PATCH" {
		return results
	}

	baseURL := opts.Definition.BaseURL
	if baseURL == "" {
		baseURL = opts.Definition.SourceURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	path := opts.Endpoint.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	fullURL := baseURL + path

	massAssignmentPayload := `{"name":"test","admin":true,"isAdmin":true,"role":"admin","verified":true,"active":true}`

	req, err := http.NewRequestWithContext(opts.Ctx, method, fullURL, bytes.NewBufferString(massAssignmentPayload))
	if err != nil {
		return results
	}
	req.Header.Set("Content-Type", "application/json")

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

	body, _ := result.History.ResponseBody()
	bodyStr := strings.ToLower(string(body))

	if result.History.StatusCode >= 200 && result.History.StatusCode < 300 {
		sensitiveFieldsInResponse := []string{"admin", "isadmin", "role", "verified"}
		foundFields := []string{}

		for _, field := range sensitiveFieldsInResponse {
			if strings.Contains(bodyStr, `"`+field+`"`) || strings.Contains(bodyStr, `'`+field+`'`) {
				foundFields = append(foundFields, field)
			}
		}

		if len(foundFields) > 0 {
			results = append(results, APITestResult{
				Vulnerable: true,
				IssueCode:  db.ApiMassAssignmentCode,
				Details: fmt.Sprintf(`The API endpoint may be vulnerable to mass assignment.

The endpoint accepted a request containing additional privileged fields
and the response includes some of these fields: %s

Request: %s %s
Payload: %s
Response Status: %d

This requires manual verification to determine if the fields were actually
processed and stored by the server.

Note: This is a heuristic detection. Verify whether the fields were actually
persisted and affected authorization.`, strings.Join(foundFields, ", "), method, fullURL, massAssignmentPayload, result.History.StatusCode),
				Confidence: 60,
				Evidence:   body,
				History:    result.History,
			})
		}
	}

	return results
}
