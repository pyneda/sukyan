package graphql

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

// FieldSuggestionsAudit tests for GraphQL field suggestion leakage
type FieldSuggestionsAudit struct {
	Options     *GraphQLAuditOptions
	Definition  *db.APIDefinition
	BaseHistory *db.History
}

// suggestionTestCase represents a field suggestion test
type suggestionTestCase struct {
	name         string
	query        string
	targetField  string
	expectedHint string // What we expect to find in suggestions
}

// getSuggestionTestCases returns test cases for field suggestions
func getSuggestionTestCases() []suggestionTestCase {
	return []suggestionTestCase{
		{
			name:         "typename_typo",
			query:        `{"query":"{__typenameXYZ}"}`,
			targetField:  "__typenameXYZ",
			expectedHint: "__typename",
		},
		{
			name:         "schema_typo",
			query:        `{"query":"{__schem}"}`,
			targetField:  "__schem",
			expectedHint: "__schema",
		},
		{
			name:         "user_probe",
			query:        `{"query":"{usr}"}`,
			targetField:  "usr",
			expectedHint: "user",
		},
		{
			name:         "users_probe",
			query:        `{"query":"{usrs}"}`,
			targetField:  "usrs",
			expectedHint: "users",
		},
		{
			name:         "admin_probe",
			query:        `{"query":"{admi}"}`,
			targetField:  "admi",
			expectedHint: "admin",
		},
		{
			name:         "query_probe",
			query:        `{"query":"{quer}"}`,
			targetField:  "quer",
			expectedHint: "query",
		},
		{
			name:         "mutation_arg_probe",
			query:        `{"query":"mutation{createUser(passwrd:\"test\")}"}`,
			targetField:  "passwrd",
			expectedHint: "password",
		},
		{
			name:         "id_probe",
			query:        `{"query":"{user(i:1){name}}"}`,
			targetField:  "i",
			expectedHint: "id",
		},
	}
}

// Run executes the field suggestions audit
func (a *FieldSuggestionsAudit) Run() {
	auditLog := log.With().
		Str("audit", "graphql-field-suggestions").
		Uint("workspace", a.Options.WorkspaceID).
		Logger()

	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			auditLog.Debug().Msg("Context cancelled, skipping field suggestions audit")
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

	auditLog.Info().Str("url", baseURL).Msg("Starting GraphQL field suggestions audit")

	client := a.Options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	// Track if suggestions are enabled
	suggestionsFound := false
	var firstHistory *db.History
	discoveredFields := make(map[string]bool)

	testCases := getSuggestionTestCases()

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
		bodyStr := strings.ToLower(string(body))

		// Check for suggestion indicators
		hasSuggestion := strings.Contains(bodyStr, "did you mean") ||
			strings.Contains(bodyStr, "suggestion") ||
			strings.Contains(bodyStr, "perhaps you meant") ||
			strings.Contains(bodyStr, "similar")

		if hasSuggestion {
			suggestionsFound = true

			// Extract suggested fields from response
			if strings.Contains(bodyStr, tc.expectedHint) {
				discoveredFields[tc.expectedHint] = true
			}

			// Store first history for reporting
			if firstHistory == nil {
				firstHistory = result.History
			}
		}
	}

	// If suggestions found, create comprehensive report
	if suggestionsFound && firstHistory != nil {
		var discoveredList []string
		for field := range discoveredFields {
			discoveredList = append(discoveredList, field)
		}

		details := fmt.Sprintf(`GraphQL field suggestions are enabled.

When querying for invalid/misspelled fields, the error response includes suggestions
for valid field names. This aids in schema enumeration even when introspection is disabled.

Request URL: %s

Schema Enumeration Impact:
- Attackers can discover field names through typo probing
- Internal/hidden fields may be revealed
- Bypasses introspection disabling as a security measure

Fields discovered through suggestion probing:
%s

Techniques used:
- Typo-based probing (e.g., "usr" -> suggests "user")
- Partial name probing (e.g., "admi" -> suggests "admin")
- Argument name probing

Remediation:
- Disable field suggestions in production (library-specific setting)
- Use allowlisting instead of suggestions
- Implement query allowlisting for production`, baseURL, formatDiscoveredFields(discoveredList))

		reportIssue(firstHistory, db.GraphqlFieldSuggestionsCode, details, 85, a.Options)
	}

	auditLog.Info().Bool("suggestions_enabled", suggestionsFound).Msg("Completed GraphQL field suggestions audit")
}

// formatDiscoveredFields formats the list of discovered fields
func formatDiscoveredFields(fields []string) string {
	if len(fields) == 0 {
		return "  (No specific fields enumerated in this test)"
	}
	var sb strings.Builder
	for _, f := range fields {
		sb.WriteString(fmt.Sprintf("  - %s\n", f))
	}
	return sb.String()
}
