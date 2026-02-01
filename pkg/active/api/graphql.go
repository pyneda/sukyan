package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

func RunGraphQLTests(opts APITestOptions) []APITestResult {
	return nil
}

func RunGraphQLAPILevelTests(opts APITestOptions) []APITestResult {
	var results []APITestResult

	taskLog := log.With().
		Str("module", "graphql-api-level-tests").
		Uint("workspace", opts.WorkspaceID).
		Logger()

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			taskLog.Debug().Msg("Context cancelled, skipping GraphQL API-level tests")
			return results
		default:
		}
	}

	if opts.Definition == nil || opts.Definition.Type != db.APIDefinitionTypeGraphQL {
		return results
	}

	taskLog.Debug().Msg("Running GraphQL API-level security tests")

	introspectionResults := testGraphQLIntrospection(opts)
	results = append(results, introspectionResults...)

	batchingResults := testGraphQLBatching(opts)
	results = append(results, batchingResults...)

	suggestionResults := testGraphQLFieldSuggestions(opts)
	results = append(results, suggestionResults...)

	depthResults := testGraphQLQueryDepth(opts)
	results = append(results, depthResults...)

	return results
}

func testGraphQLIntrospection(opts APITestOptions) []APITestResult {
	var results []APITestResult

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			return results
		default:
		}
	}

	introspectionQuery := `{"query":"{__schema{types{name}}}"}`

	baseURL := opts.Definition.BaseURL
	if baseURL == "" {
		baseURL = opts.Definition.SourceURL
	}

	req, err := http.NewRequestWithContext(opts.Ctx, "POST", baseURL, bytes.NewBufferString(introspectionQuery))
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
	bodyStr := string(body)

	if strings.Contains(bodyStr, "__schema") && strings.Contains(bodyStr, "types") {
		results = append(results, APITestResult{
			Vulnerable: true,
			IssueCode:  db.GraphqlIntrospectionEnabledCode,
			Details: fmt.Sprintf(`GraphQL introspection is enabled on this endpoint.

The introspection query returned the API schema, allowing discovery of:
- All types defined in the schema
- Available queries, mutations, and subscriptions
- Field definitions and their arguments

Request URL: %s
Response contains __schema data indicating full introspection is available.`, baseURL),
			Confidence: 95,
			Evidence:   body,
			History:    result.History,
		})
	}

	return results
}

func testGraphQLBatching(opts APITestOptions) []APITestResult {
	var results []APITestResult

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			return results
		default:
		}
	}

	batchQuery := `[{"query":"{__typename}"},{"query":"{__typename}"},{"query":"{__typename}"},{"query":"{__typename}"},{"query":"{__typename}"}]`

	baseURL := opts.Definition.BaseURL
	if baseURL == "" {
		baseURL = opts.Definition.SourceURL
	}

	req, err := http.NewRequestWithContext(opts.Ctx, "POST", baseURL, bytes.NewBufferString(batchQuery))
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

	var batchResponse []interface{}
	if err := json.Unmarshal(body, &batchResponse); err == nil && len(batchResponse) >= 5 {
		results = append(results, APITestResult{
			Vulnerable: true,
			IssueCode:  db.GraphqlBatchingAllowedCode,
			Details: fmt.Sprintf(`GraphQL query batching is allowed without apparent limits.

The endpoint processed a batch of 5 queries in a single request and returned %d responses.

This could potentially be exploited for:
- Bypassing rate limiting
- Amplifying expensive operations
- More efficient brute-force attacks

Request URL: %s`, len(batchResponse), baseURL),
			Confidence: 80,
			Evidence:   body,
			History:    result.History,
		})
	}

	return results
}

func testGraphQLFieldSuggestions(opts APITestOptions) []APITestResult {
	var results []APITestResult

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			return results
		default:
		}
	}

	suggestionQuery := `{"query":"{__typenameXYZ123}"}`

	baseURL := opts.Definition.BaseURL
	if baseURL == "" {
		baseURL = opts.Definition.SourceURL
	}

	req, err := http.NewRequestWithContext(opts.Ctx, "POST", baseURL, bytes.NewBufferString(suggestionQuery))
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

	if strings.Contains(bodyStr, "did you mean") || strings.Contains(bodyStr, "suggestion") {
		results = append(results, APITestResult{
			Vulnerable: true,
			IssueCode:  db.GraphqlFieldSuggestionsCode,
			Details: fmt.Sprintf(`GraphQL field suggestions are enabled.

When querying for an invalid field, the error response includes suggestions
for valid field names. This aids in schema enumeration even when introspection
is disabled.

Request URL: %s
Query: %s

The response contains field suggestions that could help enumerate the schema.`, baseURL, suggestionQuery),
			Confidence: 85,
			Evidence:   body,
			History:    result.History,
		})
	}

	return results
}

func testGraphQLQueryDepth(opts APITestOptions) []APITestResult {
	var results []APITestResult

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			return results
		default:
		}
	}

	deepQuery := `{"query":"{__schema{types{fields{type{fields{type{fields{type{fields{type{name}}}}}}}}}}}"}`

	baseURL := opts.Definition.BaseURL
	if baseURL == "" {
		baseURL = opts.Definition.SourceURL
	}

	req, err := http.NewRequestWithContext(opts.Ctx, "POST", baseURL, bytes.NewBufferString(deepQuery))
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

	if !strings.Contains(bodyStr, "depth") && !strings.Contains(bodyStr, "too deep") &&
		!strings.Contains(bodyStr, "max") && result.History.StatusCode == 200 {
		var response map[string]interface{}
		if err := json.Unmarshal(body, &response); err == nil {
			if _, hasData := response["data"]; hasData {
				results = append(results, APITestResult{
					Vulnerable: true,
					IssueCode:  db.GraphqlDepthLimitMissingCode,
					Details: fmt.Sprintf(`GraphQL query depth limit does not appear to be enforced.

A deeply nested query (8+ levels) was successfully executed without being
rejected. This could allow denial of service through recursive query attacks.

Request URL: %s

Consider implementing query depth limits to prevent resource exhaustion.`, baseURL),
					Confidence: 70,
					Evidence:   body,
					History:    result.History,
				})
			}
		}
	}

	return results
}
