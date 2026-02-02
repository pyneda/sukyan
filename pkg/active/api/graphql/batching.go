package graphql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

// BatchingAudit tests for GraphQL batching vulnerabilities
type BatchingAudit struct {
	Options     *GraphQLAuditOptions
	Definition  *db.APIDefinition
	BaseHistory *db.History
}

// Run executes the batching audit with multiple attack vectors
func (a *BatchingAudit) Run() {
	auditLog := log.With().
		Str("audit", "graphql-batching").
		Uint("workspace", a.Options.WorkspaceID).
		Logger()

	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			auditLog.Debug().Msg("Context cancelled, skipping batching audit")
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

	auditLog.Info().Str("url", baseURL).Msg("Starting GraphQL batching audit")

	client := a.Options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	// Test array-based batching
	a.testArrayBatching(baseURL, client)

	// Test alias-based amplification
	a.testAliasAmplification(baseURL, client)

	// Test large batch to find limits
	a.testBatchLimits(baseURL, client)

	// Test timing-based detection for expensive operations
	if a.Options.ScanMode.String() == "fuzz" {
		a.testBatchTiming(baseURL, client)
	}

	auditLog.Info().Msg("Completed GraphQL batching audit")
}

// testArrayBatching tests standard array-based query batching
func (a *BatchingAudit) testArrayBatching(baseURL string, client *http.Client) {
	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			return
		default:
		}
	}

	// Test with 5 queries
	batchQuery := `[{"query":"{__typename}"},{"query":"{__typename}"},{"query":"{__typename}"},{"query":"{__typename}"},{"query":"{__typename}"}]`

	req, err := http.NewRequestWithContext(a.Options.Ctx, "POST", baseURL, bytes.NewBufferString(batchQuery))
	if err != nil {
		return
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
		return
	}

	body, _ := result.History.ResponseBody()
	var batchResponse []interface{}
	if err := json.Unmarshal(body, &batchResponse); err == nil && len(batchResponse) >= 5 {
		details := fmt.Sprintf(`GraphQL array-based query batching is allowed.

The endpoint processed a batch of 5 queries in a single request and returned %d responses.

Attack vectors enabled:
- Bypass per-request rate limiting
- Amplify expensive operations (N queries for 1 request cost)
- Efficient brute-force attacks (e.g., password guessing)
- Circumvent CSRF tokens (one token, multiple mutations)

Request URL: %s
Batch size tested: 5 queries

Consider implementing:
- Maximum operations per request limit
- Query cost analysis
- Per-operation rate limiting`, len(batchResponse), baseURL)

		reportIssue(result.History, db.GraphqlBatchingAllowedCode, details, 85, a.Options)
	}
}

// testAliasAmplification tests alias-based query amplification
func (a *BatchingAudit) testAliasAmplification(baseURL string, client *http.Client) {
	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			return
		default:
		}
	}

	// Test with 50 aliases
	var aliases []string
	for i := 0; i < 50; i++ {
		aliases = append(aliases, fmt.Sprintf("a%d:__typename", i))
	}
	aliasQuery := fmt.Sprintf(`{"query":"query{%s}"}`, strings.Join(aliases, " "))

	req, err := http.NewRequestWithContext(a.Options.Ctx, "POST", baseURL, bytes.NewBufferString(aliasQuery))
	if err != nil {
		return
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
		return
	}

	body, _ := result.History.ResponseBody()
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err == nil {
		if data, ok := response["data"].(map[string]interface{}); ok {
			if len(data) >= 45 {
				details := fmt.Sprintf(`GraphQL alias-based query amplification is allowed.

A query with 50 field aliases was executed successfully, returning %d results.

This attack differs from array batching:
- Single query with multiple aliases for the same field
- Often bypasses batch query limits
- Can target expensive resolvers (e.g., a1:user(id:1) a2:user(id:2) ...)

Attack vectors:
- Bypass per-field rate limiting
- Amplify database queries
- Resource exhaustion via resolver multiplication

Request URL: %s
Aliases tested: 50

Consider implementing alias limits or query cost analysis.`, len(data), baseURL)

				reportIssue(result.History, db.GraphqlBatchingAllowedCode, details, 80, a.Options)
			}
		}
	}
}

// testBatchLimits tests for batch size limits
func (a *BatchingAudit) testBatchLimits(baseURL string, client *http.Client) {
	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			return
		default:
		}
	}

	// Test with 100 queries to see if there's a limit
	var queries []string
	for i := 0; i < 100; i++ {
		queries = append(queries, `{"query":"{__typename}"}`)
	}
	largeBatchQuery := "[" + strings.Join(queries, ",") + "]"

	req, err := http.NewRequestWithContext(a.Options.Ctx, "POST", baseURL, bytes.NewBufferString(largeBatchQuery))
	if err != nil {
		return
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
		return
	}

	body, _ := result.History.ResponseBody()
	var batchResponse []interface{}
	if err := json.Unmarshal(body, &batchResponse); err == nil && len(batchResponse) >= 100 {
		details := fmt.Sprintf(`GraphQL allows large batch queries without apparent limits.

A batch of 100 queries was executed successfully, returning %d responses.
No batch size limit appears to be enforced.

This significantly amplifies the attack surface:
- 100x amplification of expensive operations
- Severe DoS potential
- Efficient large-scale enumeration

Request URL: %s
Batch size tested: 100 queries

Strongly recommend implementing strict batch limits (e.g., max 10 operations).`, len(batchResponse), baseURL)

		reportIssue(result.History, db.GraphqlBatchingAllowedCode, details, 90, a.Options)
	}
}

// testBatchTiming tests for timing-based DoS via batching
func (a *BatchingAudit) testBatchTiming(baseURL string, client *http.Client) {
	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			return
		default:
		}
	}

	// Single query timing
	singleQuery := `{"query":"{__typename}"}`
	req1, err := http.NewRequestWithContext(a.Options.Ctx, "POST", baseURL, bytes.NewBufferString(singleQuery))
	if err != nil {
		return
	}
	req1.Header.Set("Content-Type", "application/json")

	start1 := time.Now()
	result1 := http_utils.ExecuteRequest(req1, http_utils.RequestExecutionOptions{
		Client:        client,
		CreateHistory: false,
	})
	singleTime := time.Since(start1)

	if result1.Err != nil {
		return
	}

	// Batch query timing (20 queries)
	var queries []string
	for i := 0; i < 20; i++ {
		queries = append(queries, `{"query":"{__typename}"}`)
	}
	batchQuery := "[" + strings.Join(queries, ",") + "]"

	req2, err := http.NewRequestWithContext(a.Options.Ctx, "POST", baseURL, bytes.NewBufferString(batchQuery))
	if err != nil {
		return
	}
	req2.Header.Set("Content-Type", "application/json")

	start2 := time.Now()
	result2 := http_utils.ExecuteRequest(req2, http_utils.RequestExecutionOptions{
		Client:        client,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: a.Options.WorkspaceID,
			ScanID:      a.Options.ScanID,
			ScanJobID:   a.Options.ScanJobID,
		},
	})
	batchTime := time.Since(start2)

	if result2.Err != nil || result2.History == nil {
		return
	}

	// If batch of 20 takes less than 5x single query time, queries run in parallel
	// This could indicate better DoS potential
	if batchTime < singleTime*5 && batchTime > 0 {
		details := fmt.Sprintf(`GraphQL batch queries appear to execute efficiently (potential parallel execution).

Timing analysis:
- Single query: %v
- Batch of 20 queries: %v
- Ratio: %.2fx (expected ~20x if sequential)

This suggests batch queries may execute in parallel or with significant optimization,
which could amplify DoS attacks as the server processes many operations simultaneously.

Request URL: %s`, singleTime, batchTime, float64(batchTime)/float64(singleTime), baseURL)

		reportIssue(result2.History, db.GraphqlBatchingAllowedCode, details, 60, a.Options)
	}
}
