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

type BatchingAudit struct {
	Options     *GraphQLAuditOptions
	Definition  *db.APIDefinition
	BaseHistory *db.History
}

type batchTestResult struct {
	history    *db.History
	testName   string
	details    string
	confidence int
}

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

	var results []batchTestResult

	if r := a.testArrayBatching(baseURL, client); r != nil {
		results = append(results, *r)
	}

	if r := a.testAliasAmplification(baseURL, client); r != nil {
		results = append(results, *r)
	}

	if r := a.testBatchLimits(baseURL, client); r != nil {
		results = append(results, *r)
	}

	if a.Options.ScanMode.String() == "fuzz" {
		if r := a.testBatchTiming(baseURL, client); r != nil {
			results = append(results, *r)
		}
	}

	if len(results) == 0 {
		auditLog.Info().Msg("No batching issues detected")
		return
	}

	bestIdx := 0
	for i, r := range results {
		if r.confidence > results[bestIdx].confidence {
			bestIdx = i
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Batching tests passed: %d\n\n", len(results)))
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("- %s (confidence: %d%%)\n  %s\n\n", r.testName, r.confidence, r.details))
	}
	details := sb.String()

	issue, err := db.CreateIssueFromHistoryAndTemplate(
		results[bestIdx].history,
		db.GraphqlBatchingAllowedCode,
		details,
		results[bestIdx].confidence,
		"",
		&a.Options.WorkspaceID,
		&a.Options.TaskID,
		&a.Options.TaskJobID,
		&a.Options.ScanID,
		&a.Options.ScanJobID,
	)
	if err != nil {
		auditLog.Error().Err(err).Msg("Failed to create batching issue")
		return
	}

	if len(results) > 1 {
		var additionalHistories []*db.History
		for i, r := range results {
			if i != bestIdx {
				additionalHistories = append(additionalHistories, r.history)
			}
		}
		if err := issue.AppendHistories(additionalHistories); err != nil {
			auditLog.Warn().Err(err).Uint("issue_id", issue.ID).Msg("Failed to link additional histories")
		}
	}

	auditLog.Info().Uint("issue_id", issue.ID).Int("tests_passed", len(results)).Msg("Created consolidated batching issue")
}

func (a *BatchingAudit) testArrayBatching(baseURL string, client *http.Client) *batchTestResult {
	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			return nil
		default:
		}
	}

	batchQuery := `[{"query":"{__typename}"},{"query":"{__typename}"},{"query":"{__typename}"},{"query":"{__typename}"},{"query":"{__typename}"}]`

	req, err := http.NewRequestWithContext(a.Options.Ctx, "POST", baseURL, bytes.NewBufferString(batchQuery))
	if err != nil {
		return nil
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
		return nil
	}

	body, _ := result.History.ResponseBody()
	var batchResponse []interface{}
	if err := json.Unmarshal(body, &batchResponse); err == nil && len(batchResponse) >= 5 {
		return &batchTestResult{
			history:    result.History,
			testName:   "Array batching",
			details:    fmt.Sprintf("Batch of 5 queries processed, returned %d responses.", len(batchResponse)),
			confidence: 85,
		}
	}
	return nil
}

func (a *BatchingAudit) testAliasAmplification(baseURL string, client *http.Client) *batchTestResult {
	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			return nil
		default:
		}
	}

	var aliases []string
	for i := 0; i < 50; i++ {
		aliases = append(aliases, fmt.Sprintf("a%d:__typename", i))
	}
	aliasQuery := fmt.Sprintf(`{"query":"query{%s}"}`, strings.Join(aliases, " "))

	req, err := http.NewRequestWithContext(a.Options.Ctx, "POST", baseURL, bytes.NewBufferString(aliasQuery))
	if err != nil {
		return nil
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
		return nil
	}

	body, _ := result.History.ResponseBody()
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err == nil {
		if data, ok := response["data"].(map[string]interface{}); ok {
			if len(data) >= 45 {
				return &batchTestResult{
					history:    result.History,
					testName:   "Alias amplification",
					details:    fmt.Sprintf("Query with 50 field aliases returned %d results. This often bypasses batch query limits.", len(data)),
					confidence: 80,
				}
			}
		}
	}
	return nil
}

func (a *BatchingAudit) testBatchLimits(baseURL string, client *http.Client) *batchTestResult {
	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			return nil
		default:
		}
	}

	var queries []string
	for i := 0; i < 100; i++ {
		queries = append(queries, `{"query":"{__typename}"}`)
	}
	largeBatchQuery := "[" + strings.Join(queries, ",") + "]"

	req, err := http.NewRequestWithContext(a.Options.Ctx, "POST", baseURL, bytes.NewBufferString(largeBatchQuery))
	if err != nil {
		return nil
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
		return nil
	}

	body, _ := result.History.ResponseBody()
	var batchResponse []interface{}
	if err := json.Unmarshal(body, &batchResponse); err == nil && len(batchResponse) >= 100 {
		return &batchTestResult{
			history:    result.History,
			testName:   "Large batch (no limit)",
			details:    fmt.Sprintf("Batch of 100 queries processed, returned %d responses. No batch size limit enforced.", len(batchResponse)),
			confidence: 90,
		}
	}
	return nil
}

func (a *BatchingAudit) testBatchTiming(baseURL string, client *http.Client) *batchTestResult {
	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			return nil
		default:
		}
	}

	singleQuery := `{"query":"{__typename}"}`
	req1, err := http.NewRequestWithContext(a.Options.Ctx, "POST", baseURL, bytes.NewBufferString(singleQuery))
	if err != nil {
		return nil
	}
	req1.Header.Set("Content-Type", "application/json")

	start1 := time.Now()
	result1 := http_utils.ExecuteRequest(req1, http_utils.RequestExecutionOptions{
		Client:        client,
		CreateHistory: false,
	})
	singleTime := time.Since(start1)

	if result1.Err != nil {
		return nil
	}

	var queries []string
	for i := 0; i < 20; i++ {
		queries = append(queries, `{"query":"{__typename}"}`)
	}
	batchQuery := "[" + strings.Join(queries, ",") + "]"

	req2, err := http.NewRequestWithContext(a.Options.Ctx, "POST", baseURL, bytes.NewBufferString(batchQuery))
	if err != nil {
		return nil
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
		return nil
	}

	if batchTime < singleTime*5 && batchTime > 0 {
		return &batchTestResult{
			history:  result2.History,
			testName: "Parallel execution timing",
			details: fmt.Sprintf("Single query: %v, batch of 20: %v (ratio: %.2fx, expected ~20x if sequential). "+
				"Suggests parallel execution.", singleTime, batchTime, float64(batchTime)/float64(singleTime)),
			confidence: 60,
		}
	}
	return nil
}
