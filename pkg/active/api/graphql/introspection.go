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

// IntrospectionAudit tests for GraphQL introspection exposure
type IntrospectionAudit struct {
	Options     *GraphQLAuditOptions
	Definition  *db.APIDefinition
	BaseHistory *db.History
}

// introspectionPayload represents different ways to query introspection
type introspectionPayload struct {
	name        string
	query       string
	description string
}

// getIntrospectionPayloads returns various introspection query variants
func getIntrospectionPayloads() []introspectionPayload {
	return []introspectionPayload{
		{
			name:        "basic_schema",
			query:       `{"query":"{__schema{types{name}}}"}`,
			description: "Basic __schema query",
		},
		{
			name:        "full_introspection",
			query:       `{"query":"query IntrospectionQuery{__schema{queryType{name}mutationType{name}subscriptionType{name}types{...FullType}directives{name description locations args{...InputValue}}}}fragment FullType on __Type{kind name description fields(includeDeprecated:true){name description args{...InputValue}type{...TypeRef}isDeprecated deprecationReason}inputFields{...InputValue}interfaces{...TypeRef}enumValues(includeDeprecated:true){name description isDeprecated deprecationReason}possibleTypes{...TypeRef}}fragment InputValue on __InputValue{name description type{...TypeRef}defaultValue}fragment TypeRef on __Type{kind name ofType{kind name ofType{kind name ofType{kind name ofType{kind name ofType{kind name ofType{kind name ofType{kind name}}}}}}}}"}`,
			description: "Full introspection query (standard)",
		},
		{
			name:        "type_query",
			query:       `{"query":"{__type(name:\"Query\"){name fields{name type{name}}}}"}`,
			description: "__type query for Query type",
		},
		{
			name:        "get_method",
			query:       ``,
			description: "GET request introspection (query param)",
		},
		{
			name:        "aliased_introspection",
			query:       `{"query":"{a:__schema{types{name}} b:__type(name:\"Query\"){name}}"}`,
			description: "Aliased introspection to bypass naive blocking",
		},
		{
			name:        "newline_bypass",
			query:       `{"query":"{\n__schema\n{\ntypes\n{\nname\n}\n}\n}"}`,
			description: "Newline-separated introspection",
		},
		{
			name:        "whitespace_bypass",
			query:       `{"query":"{  __schema  {  types  {  name  }  }  }"}`,
			description: "Extra whitespace introspection",
		},
		{
			name:        "fragment_introspection",
			query:       `{"query":"query{...F}fragment F on Query{__schema{types{name}}}"}`,
			description: "Fragment-based introspection",
		},
	}
}

// Run executes the introspection audit
func (a *IntrospectionAudit) Run() {
	auditLog := log.With().
		Str("audit", "graphql-introspection").
		Uint("workspace", a.Options.WorkspaceID).
		Logger()

	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			auditLog.Debug().Msg("Context cancelled, skipping introspection audit")
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

	auditLog.Info().Str("url", baseURL).Msg("Starting GraphQL introspection audit")

	client := a.Options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	payloads := getIntrospectionPayloads()
	foundIntrospection := false

	for _, payload := range payloads {
		if a.Options.Ctx != nil {
			select {
			case <-a.Options.Ctx.Done():
				return
			default:
			}
		}

		// Skip GET method test for now, handle separately
		if payload.name == "get_method" {
			if a.testGETIntrospection(baseURL, client) {
				foundIntrospection = true
			}
			continue
		}

		req, err := http.NewRequestWithContext(a.Options.Ctx, "POST", baseURL, bytes.NewBufferString(payload.query))
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
		bodyStr := string(body)

		// Check for successful introspection
		if (strings.Contains(bodyStr, "__schema") || strings.Contains(bodyStr, "__type")) &&
			(strings.Contains(bodyStr, "types") || strings.Contains(bodyStr, "fields") || strings.Contains(bodyStr, "name")) &&
			!strings.Contains(strings.ToLower(bodyStr), "error") {

			confidence := 95
			if payload.name != "basic_schema" && payload.name != "full_introspection" {
				confidence = 85 // Bypass techniques get slightly lower confidence
			}

			details := fmt.Sprintf(`Introspection query variant: %s (%s)

Query: %s
Response Status: %d`, payload.name, payload.description, truncateQuery(payload.query, 200), result.History.StatusCode)

			reportIssue(result.History, db.GraphqlIntrospectionEnabledCode, details, confidence, a.Options)
			foundIntrospection = true

			// If basic introspection works, no need to test bypasses
			if payload.name == "basic_schema" || payload.name == "full_introspection" {
				break
			}
		}
	}

	if !foundIntrospection {
		auditLog.Debug().Msg("No introspection vulnerability found")
	}

	auditLog.Info().Msg("Completed GraphQL introspection audit")
}

// testGETIntrospection tests if introspection works via GET request
func (a *IntrospectionAudit) testGETIntrospection(baseURL string, client *http.Client) bool {
	// Build URL with query parameter
	queryParam := `{__schema{types{name}}}`
	url := baseURL
	if strings.Contains(url, "?") {
		url += "&query=" + queryParam
	} else {
		url += "?query=" + queryParam
	}

	req, err := http.NewRequestWithContext(a.Options.Ctx, "GET", url, nil)
	if err != nil {
		return false
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
		return false
	}

	body, _ := result.History.ResponseBody()
	bodyStr := string(body)

	if strings.Contains(bodyStr, "__schema") && strings.Contains(bodyStr, "types") {
		details := fmt.Sprintf(`Introspection query variant: GET method with query parameter

The endpoint accepts introspection queries via GET method, which may bypass WAF rules that only inspect POST bodies.

Response Status: %d`, result.History.StatusCode)

		reportIssue(result.History, db.GraphqlIntrospectionEnabledCode, details, 90, a.Options)
		return true
	}

	return false
}

// truncateQuery truncates a query string for display
func truncateQuery(query string, maxLen int) string {
	if len(query) <= maxLen {
		return query
	}
	return query[:maxLen] + "..."
}
