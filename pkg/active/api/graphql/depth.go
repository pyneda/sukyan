package graphql

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

// DepthLimitAudit tests for GraphQL query depth limit vulnerabilities
type DepthLimitAudit struct {
	Options     *GraphQLAuditOptions
	Definition  *db.APIDefinition
	BaseHistory *db.History
}

// depthTestCase represents a depth test configuration
type depthTestCase struct {
	name        string
	query       string
	depth       int
	description string
}

// getDepthTestCases returns various depth test queries
func getDepthTestCases() []depthTestCase {
	return []depthTestCase{
		{
			name:        "introspection_depth_8",
			query:       `{"query":"{__schema{types{fields{type{fields{type{fields{type{fields{type{name}}}}}}}}}}}"}`,
			depth:       8,
			description: "8-level nested introspection query",
		},
		{
			name:        "introspection_depth_12",
			query:       `{"query":"{__schema{types{fields{type{fields{type{fields{type{fields{type{fields{type{fields{type{name}}}}}}}}}}}}}}}"}`,
			depth:       12,
			description: "12-level nested introspection query",
		},
		{
			name:        "fragment_depth",
			query:       `{"query":"query{...A}fragment A on Query{__schema{...B}}fragment B on __Schema{types{...C}}fragment C on __Type{fields{...D}}fragment D on __Field{type{...E}}fragment E on __Type{fields{type{name}}}}"}`,
			depth:       7,
			description: "Fragment-based depth (harder to detect)",
		},
		{
			name:        "inline_fragment_depth",
			query:       `{"query":"{__schema{...on __Schema{types{...on __Type{fields{...on __Field{type{...on __Type{name}}}}}}}}}"}`,
			depth:       6,
			description: "Inline fragment depth",
		},
	}
}

// Run executes the depth limit audit
func (a *DepthLimitAudit) Run() {
	auditLog := log.With().
		Str("audit", "graphql-depth-limit").
		Uint("workspace", a.Options.WorkspaceID).
		Logger()

	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			auditLog.Debug().Msg("Context cancelled, skipping depth limit audit")
			return
		default:
		}
	}

	if a.Definition == nil {
		return
	}

	baseURL := a.Definition.BaseURL
	if baseURL == "" {
		baseURL = a.Definition.SourceURL
	}

	auditLog.Info().Str("url", baseURL).Msg("Starting GraphQL depth limit audit")

	client := a.Options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	// Test various depth levels
	testCases := getDepthTestCases()
	maxSuccessfulDepth := 0

	for _, tc := range testCases {
		if a.Options.Ctx != nil {
			select {
			case <-a.Options.Ctx.Done():
				return
			default:
			}
		}

		if a.testDepth(baseURL, client, tc) && tc.depth > maxSuccessfulDepth {
			maxSuccessfulDepth = tc.depth
		}
	}

	// Test circular/recursive patterns (more aggressive)
	if a.Options.ScanMode.String() == "fuzz" {
		a.testCircularFragments(baseURL, client)
	}

	auditLog.Info().Int("max_depth", maxSuccessfulDepth).Msg("Completed GraphQL depth limit audit")
}

// testDepth tests a specific depth query
func (a *DepthLimitAudit) testDepth(baseURL string, client *http.Client, tc depthTestCase) bool {
	req, err := http.NewRequestWithContext(a.Options.Ctx, "POST", baseURL, bytes.NewBufferString(tc.query))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")

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
		return false
	}

	body, _ := result.History.ResponseBody()
	bodyStr := strings.ToLower(string(body))

	// Check if query was rejected due to depth
	depthRejected := strings.Contains(bodyStr, "depth") ||
		strings.Contains(bodyStr, "too deep") ||
		strings.Contains(bodyStr, "max") ||
		strings.Contains(bodyStr, "exceeded") ||
		strings.Contains(bodyStr, "limit")

	if depthRejected {
		return false
	}

	// Check if query executed successfully
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		return false
	}

	// Check for data or successful execution
	_, hasData := response["data"]
	errors, hasErrors := response["errors"]

	// If we have data or no depth-related errors, the query succeeded
	if hasData || !hasErrors {
		confidence := 70
		if tc.depth >= 10 {
			confidence = 85
		} else if tc.depth >= 8 {
			confidence = 80
		}

		details := fmt.Sprintf(`GraphQL query depth limit is not enforced or is too permissive.

Test: %s
Query Depth: %d levels
Description: %s

A deeply nested query was executed without being rejected.

Attack vectors:
- Recursive query attacks causing exponential resource consumption
- DoS via deeply nested selections on list fields
- Database query amplification

Example attack pattern:
  query { users { friends { friends { friends { ... } } } } }

Request URL: %s

Recommended limits:
- Maximum depth: 5-7 for most applications
- Combined with query cost analysis for complex schemas`, tc.name, tc.depth, tc.description, baseURL)

		reportIssue(result.History, db.GraphqlDepthLimitMissingCode, details, confidence, a.Options)
		return true
	}

	// Check if errors are depth-related
	if hasErrors {
		errStr := fmt.Sprintf("%v", errors)
		if !strings.Contains(strings.ToLower(errStr), "depth") {
			// Errors exist but not depth-related - query might have partially succeeded
			return false
		}
	}

	return false
}

// testCircularFragments tests for circular fragment reference handling
func (a *DepthLimitAudit) testCircularFragments(baseURL string, client *http.Client) {
	circularTests := []struct {
		name  string
		query string
		desc  string
	}{
		{
			name:  "self_reference",
			query: `{"query":"query{...F}fragment F on Query{...F}"}`,
			desc:  "Direct self-referencing fragment",
		},
		{
			name:  "mutual_reference",
			query: `{"query":"query{...A}fragment A on Query{...B}fragment B on Query{...A}"}`,
			desc:  "Mutually referencing fragments (A->B->A)",
		},
		{
			name:  "chain_reference",
			query: `{"query":"query{...A}fragment A on Query{...B}fragment B on Query{...C}fragment C on Query{...A}"}`,
			desc:  "Chain of fragments forming a cycle (A->B->C->A)",
		},
	}

	for _, tc := range circularTests {
		if a.Options.Ctx != nil {
			select {
			case <-a.Options.Ctx.Done():
				return
			default:
			}
		}

		req, err := http.NewRequestWithContext(a.Options.Ctx, "POST", baseURL, bytes.NewBufferString(tc.query))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")

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
			continue
		}

		body, _ := result.History.ResponseBody()
		bodyStr := strings.ToLower(string(body))

		// Check if circular reference was properly rejected
		circularRejected := strings.Contains(bodyStr, "circular") ||
			strings.Contains(bodyStr, "cycle") ||
			strings.Contains(bodyStr, "recursive") ||
			strings.Contains(bodyStr, "fragment")

		if !circularRejected && result.History.StatusCode == 200 {
			details := fmt.Sprintf(`GraphQL circular fragment reference may not be properly validated.

Test: %s
Description: %s
Query: %s

The query with circular fragment references was not explicitly rejected.

This can cause:
- Infinite loops during query execution
- Stack overflow errors
- Complete service denial

Request URL: %s
Response Status: %d

GraphQL servers should validate fragment cycles during query parsing.`, tc.name, tc.desc, tc.query, baseURL, result.History.StatusCode)

			reportIssue(result.History, db.GraphqlDepthLimitMissingCode, details, 55, a.Options)
		}
	}
}
