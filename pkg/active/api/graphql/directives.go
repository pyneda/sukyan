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

// DirectivesAudit tests for GraphQL directive abuse vulnerabilities
type DirectivesAudit struct {
	Options     *GraphQLAuditOptions
	Definition  *db.APIDefinition
	BaseHistory *db.History
}

// directiveTestCase represents a directive abuse test
type directiveTestCase struct {
	name        string
	query       string
	checkFunc   func(body []byte, statusCode int) bool
	description string
	severity    string
}

// getDirectiveTestCases returns directive abuse test cases
func getDirectiveTestCases() []directiveTestCase {
	return []directiveTestCase{
		{
			name:  "repeated_include",
			query: `{"query":"query{__typename @include(if:true) @include(if:true) @include(if:true)}"}`,
			checkFunc: func(body []byte, statusCode int) bool {
				if statusCode != 200 {
					return false
				}
				var resp map[string]interface{}
				if err := json.Unmarshal(body, &resp); err != nil {
					return false
				}
				_, hasErrors := resp["errors"]
				return !hasErrors
			},
			description: "Multiple @include directives on same field accepted without error",
			severity:    "low",
		},
		{
			name:  "repeated_skip",
			query: `{"query":"query{__typename @skip(if:false) @skip(if:false) @skip(if:false)}"}`,
			checkFunc: func(body []byte, statusCode int) bool {
				if statusCode != 200 {
					return false
				}
				var resp map[string]interface{}
				if err := json.Unmarshal(body, &resp); err != nil {
					return false
				}
				_, hasErrors := resp["errors"]
				return !hasErrors
			},
			description: "Multiple @skip directives on same field accepted",
			severity:    "low",
		},
		{
			name:  "skip_include_conflict",
			query: `{"query":"query{__typename @skip(if:true) @include(if:true)}"}`,
			checkFunc: func(body []byte, statusCode int) bool {
				if statusCode != 200 {
					return false
				}
				var resp map[string]interface{}
				if err := json.Unmarshal(body, &resp); err != nil {
					return false
				}
				_, hasData := resp["data"]
				_, hasErrors := resp["errors"]
				return hasData && !hasErrors
			},
			description: "Conflicting @skip(if:true) and @include(if:true) accepted - undefined behavior",
			severity:    "medium",
		},
		{
			name:  "skip_false_include_false",
			query: `{"query":"query{__typename @skip(if:false) @include(if:false)}"}`,
			checkFunc: func(body []byte, statusCode int) bool {
				if statusCode != 200 {
					return false
				}
				var resp map[string]interface{}
				if err := json.Unmarshal(body, &resp); err != nil {
					return false
				}
				// Field should be excluded but some impls might include it
				if data, ok := resp["data"].(map[string]interface{}); ok {
					_, hasTypename := data["__typename"]
					return hasTypename // Vulnerable if field is returned
				}
				return false
			},
			description: "@skip(if:false) @include(if:false) should exclude field but it's included",
			severity:    "medium",
		},
		{
			name:  "unknown_directive_ignored",
			query: `{"query":"query{__typename @customDirective(arg:\"test\")}"}`,
			checkFunc: func(body []byte, statusCode int) bool {
				if statusCode != 200 {
					return false
				}
				var resp map[string]interface{}
				if err := json.Unmarshal(body, &resp); err != nil {
					return false
				}
				if data, ok := resp["data"].(map[string]interface{}); ok {
					_, hasTypename := data["__typename"]
					return hasTypename
				}
				return false
			},
			description: "Unknown directives are silently ignored instead of rejected",
			severity:    "medium",
		},
		{
			name:  "deprecated_directive_with_args",
			query: `{"query":"query{__typename @deprecated(reason:\"test\")}"}`,
			checkFunc: func(body []byte, statusCode int) bool {
				if statusCode != 200 {
					return false
				}
				var resp map[string]interface{}
				if err := json.Unmarshal(body, &resp); err != nil {
					return false
				}
				// @deprecated shouldn't be usable on query fields
				if data, ok := resp["data"].(map[string]interface{}); ok {
					_, hasTypename := data["__typename"]
					return hasTypename
				}
				return false
			},
			description: "@deprecated directive accepted in query context (should only be in schema)",
			severity:    "low",
		},
		{
			name:  "directive_injection",
			query: `{"query":"query{__typename @include(if:$var)}","variables":{"var":"true OR 1=1"}}`,
			checkFunc: func(body []byte, statusCode int) bool {
				// This shouldn't work, but check if it doesn't error properly
				if statusCode == 200 {
					var resp map[string]interface{}
					if err := json.Unmarshal(body, &resp); err != nil {
						return false
					}
					if data, ok := resp["data"].(map[string]interface{}); ok {
						_, hasTypename := data["__typename"]
						return hasTypename
					}
				}
				return false
			},
			description: "Directive argument injection via variables not properly validated",
			severity:    "high",
		},
		{
			name:  "directive_on_fragment_spread",
			query: `{"query":"query{...F @include(if:true) @skip(if:true)}fragment F on Query{__typename}"}`,
			checkFunc: func(body []byte, statusCode int) bool {
				if statusCode != 200 {
					return false
				}
				var resp map[string]interface{}
				if err := json.Unmarshal(body, &resp); err != nil {
					return false
				}
				// Conflicting directives on fragment spread
				_, hasData := resp["data"]
				_, hasErrors := resp["errors"]
				return hasData && !hasErrors
			},
			description: "Conflicting directives on fragment spread accepted",
			severity:    "low",
		},
	}
}

// Run executes the directives audit
func (a *DirectivesAudit) Run() {
	auditLog := log.With().
		Str("audit", "graphql-directives").
		Uint("workspace", a.Options.WorkspaceID).
		Logger()

	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			auditLog.Debug().Msg("Context cancelled, skipping directives audit")
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

	auditLog.Info().Str("url", baseURL).Msg("Starting GraphQL directives audit")

	client := a.Options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	testCases := getDirectiveTestCases()
	issuesFound := make(map[string]bool)

	for _, tc := range testCases {
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

		if tc.checkFunc(body, result.History.StatusCode) {
			// Avoid duplicate issue types
			if issuesFound[tc.severity] {
				continue
			}
			issuesFound[tc.severity] = true

			confidence := 60
			if tc.severity == "high" {
				confidence = 80
			} else if tc.severity == "medium" {
				confidence = 70
			}

			details := fmt.Sprintf(`GraphQL directive handling vulnerability detected.

Test: %s
Severity: %s
Description: %s

Query: %s
Response Status: %d

Improper directive handling can lead to:
- Query logic bypass (skip authorization checks via directives)
- Unexpected data exposure
- Security control circumvention
- Undefined behavior exploitation

Request URL: %s

Remediation:
- Implement strict directive validation
- Reject unknown directives
- Validate directive combinations
- Follow GraphQL specification for directive behavior`, tc.name, strings.ToUpper(tc.severity), tc.description, tc.query, result.History.StatusCode, baseURL)

			reportIssue(result.History, db.GraphqlFieldSuggestionsCode, details, confidence, a.Options)
		}
	}

	auditLog.Info().Msg("Completed GraphQL directives audit")
}
