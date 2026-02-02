package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/active"
	activegraphql "github.com/pyneda/sukyan/pkg/active/api/graphql"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/scan/control"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

type APIBehaviorJobData struct {
	DefinitionID uuid.UUID           `json:"definition_id"`
	AuthConfigID *uuid.UUID          `json:"auth_config_id,omitempty"`
	Headers      map[string][]string `json:"headers,omitempty"`
	Concurrency  int                 `json:"concurrency,omitempty"`
}

type APIBehaviorExecutor struct {
	interactionsManager *integrations.InteractionsManager
}

func NewAPIBehaviorExecutor(interactionsManager *integrations.InteractionsManager) *APIBehaviorExecutor {
	return &APIBehaviorExecutor{
		interactionsManager: interactionsManager,
	}
}

func (e *APIBehaviorExecutor) JobType() db.ScanJobType {
	return db.ScanJobTypeAPIBehavior
}

func (e *APIBehaviorExecutor) Execute(ctx context.Context, job *db.ScanJob, ctrl *control.ScanControl) error {
	taskLog := log.With().
		Uint("scan_id", job.ScanID).
		Uint("job_id", job.ID).
		Str("job_type", string(job.JobType)).
		Logger()

	taskLog.Info().Msg("Starting API behavior check")

	var jobData APIBehaviorJobData
	if err := json.Unmarshal(job.Payload, &jobData); err != nil {
		return fmt.Errorf("failed to parse job payload: %w", err)
	}

	if !ctrl.CheckpointWithContext(ctx) {
		return context.Canceled
	}

	definition, err := db.Connection().GetAPIDefinitionByID(jobData.DefinitionID)
	if err != nil {
		return fmt.Errorf("failed to get API definition %s: %w", jobData.DefinitionID, err)
	}

	scan, err := db.Connection().GetScanByID(job.ScanID)
	if err != nil {
		return fmt.Errorf("failed to get scan %d: %w", job.ScanID, err)
	}

	httpClient := http_utils.CreateHTTPClientFromConfig(http_utils.HTTPClientConfig{
		Timeout:             scan.Options.HTTPTimeout,
		MaxIdleConns:        scan.Options.HTTPMaxIdleConns,
		MaxIdleConnsPerHost: scan.Options.HTTPMaxIdleConnsPerHost,
		MaxConnsPerHost:     scan.Options.HTTPMaxConnsPerHost,
		DisableKeepAlives:   scan.Options.HTTPDisableKeepAlives,
	})

	historyOpts := http_utils.HistoryCreationOptions{
		Source:              db.SourceScanner,
		WorkspaceID:         job.WorkspaceID,
		ScanID:              job.ScanID,
		ScanJobID:           job.ID,
		CreateNewBodyStream: true,
	}

	baseURL := definition.BaseURL
	if baseURL == "" {
		baseURL = definition.SourceURL
	}

	concurrency := jobData.Concurrency
	if concurrency == 0 {
		concurrency = 5
	}

	scanJobID := job.ID
	result := &db.APIBehaviorResult{
		ScanID:         job.ScanID,
		ScanJobID:      &scanJobID,
		WorkspaceID:    job.WorkspaceID,
		DefinitionID:   definition.ID,
		DefinitionType: definition.Type,
	}
	if *result.ScanJobID == 0 {
		result.ScanJobID = nil
	}

	if !ctrl.CheckpointWithContext(ctx) {
		return context.Canceled
	}

	notFoundFPs := e.fingerprintNotFound(ctx, baseURL, httpClient, historyOpts, concurrency)
	result.SetNotFoundFingerprints(notFoundFPs)

	if !ctrl.CheckpointWithContext(ctx) {
		return context.Canceled
	}

	unauthFPs := e.fingerprintUnauthenticated(ctx, baseURL, definition.Type, httpClient, historyOpts, concurrency)
	result.SetUnauthenticatedFingerprints(unauthFPs)

	if !ctrl.CheckpointWithContext(ctx) {
		return context.Canceled
	}

	invalidCTFPs := e.fingerprintInvalidContentType(ctx, baseURL, definition.Type, httpClient, historyOpts, concurrency)
	result.SetInvalidContentTypeFingerprints(invalidCTFPs)

	if !ctrl.CheckpointWithContext(ctx) {
		return context.Canceled
	}

	malformedFPs := e.fingerprintMalformedBody(ctx, baseURL, definition.Type, httpClient, historyOpts, concurrency)
	result.SetMalformedBodyFingerprints(malformedFPs)

	if !ctrl.CheckpointWithContext(ctx) {
		return context.Canceled
	}

	if _, err := db.Connection().CreateAPIBehaviorResult(result); err != nil {
		return fmt.Errorf("failed to store API behavior result: %w", err)
	}

	taskLog.Info().
		Int("not_found_patterns", len(notFoundFPs)).
		Int("unauth_patterns", len(unauthFPs)).
		Int("invalid_ct_patterns", len(invalidCTFPs)).
		Int("malformed_patterns", len(malformedFPs)).
		Msg("API behavior fingerprinting completed")

	e.runAPILevelSecurityChecks(ctx, definition, httpClient, job, ctrl)

	taskLog.Info().Msg("API behavior check completed")
	return nil
}

func (e *APIBehaviorExecutor) runAPILevelSecurityChecks(
	ctx context.Context,
	definition *db.APIDefinition,
	httpClient *http.Client,
	job *db.ScanJob,
	ctrl *control.ScanControl,
) {
	taskLog := log.With().
		Uint("scan_id", job.ScanID).
		Str("definition_id", definition.ID.String()).
		Str("type", string(definition.Type)).
		Logger()

	if !ctrl.CheckpointWithContext(ctx) {
		return
	}

	switch definition.Type {
	case db.APIDefinitionTypeGraphQL:
		taskLog.Debug().Msg("Running API-level GraphQL security tests")
		graphqlOpts := &activegraphql.GraphQLAuditOptions{
			ActiveModuleOptions: active.ActiveModuleOptions{
				Ctx:         ctx,
				WorkspaceID: job.WorkspaceID,
				ScanID:      job.ScanID,
				ScanJobID:   job.ID,
				HTTPClient:  httpClient,
			},
		}
		activegraphql.ScanGraphQLAPI(definition, graphqlOpts)
	}

	taskLog.Debug().Msg("API-level security tests completed")
}

func (e *APIBehaviorExecutor) fingerprintNotFound(
	ctx context.Context,
	baseURL string,
	httpClient *http.Client,
	historyOpts http_utils.HistoryCreationOptions,
	concurrency int,
) []db.ResponseFingerprint {
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	paths := []string{
		"sukyan-nonexistent-" + lib.GenerateRandomString(12),
		"api-test-404-" + lib.GenerateRandomString(8),
		fmt.Sprintf("does-not-exist-%d", lib.GenerateRandInt(100000, 999999)),
		"random-endpoint-" + lib.GenerateRandomString(10),
	}

	baseURL = strings.TrimSuffix(baseURL, "/")

	p := pool.NewWithResults[*db.ResponseFingerprint]().WithContext(ctx).WithMaxGoroutines(concurrency)

	for _, path := range paths {
		currentPath := path
		p.Go(func(ctx context.Context) (*db.ResponseFingerprint, error) {
			targetURL := baseURL + "/" + currentPath
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
			if err != nil {
				return nil, nil
			}

			result := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
				Client:                 httpClient,
				CreateHistory:          true,
				HistoryCreationOptions: historyOpts,
			})
			if result.Err != nil || result.History == nil {
				return nil, nil
			}

			return historyToFingerprint(result.History), nil
		})
	}

	results, _ := p.Wait()
	return collectFingerprints(results)
}

func (e *APIBehaviorExecutor) fingerprintUnauthenticated(
	ctx context.Context,
	baseURL string,
	defType db.APIDefinitionType,
	httpClient *http.Client,
	historyOpts http_utils.HistoryCreationOptions,
	concurrency int,
) []db.ResponseFingerprint {
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	requests := buildUnauthenticatedProbes(baseURL, defType)

	p := pool.NewWithResults[*db.ResponseFingerprint]().WithContext(ctx).WithMaxGoroutines(concurrency)

	for _, reqData := range requests {
		rd := reqData
		p.Go(func(ctx context.Context) (*db.ResponseFingerprint, error) {
			req, err := http.NewRequestWithContext(ctx, rd.method, rd.url, rd.body)
			if err != nil {
				return nil, nil
			}
			for k, v := range rd.headers {
				req.Header.Set(k, v)
			}

			result := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
				Client:                 httpClient,
				CreateHistory:          true,
				HistoryCreationOptions: historyOpts,
			})
			if result.Err != nil || result.History == nil {
				return nil, nil
			}

			return historyToFingerprint(result.History), nil
		})
	}

	results, _ := p.Wait()
	return collectFingerprints(results)
}

func (e *APIBehaviorExecutor) fingerprintInvalidContentType(
	ctx context.Context,
	baseURL string,
	defType db.APIDefinitionType,
	httpClient *http.Client,
	historyOpts http_utils.HistoryCreationOptions,
	concurrency int,
) []db.ResponseFingerprint {
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	requests := buildInvalidContentTypeProbes(baseURL, defType)

	p := pool.NewWithResults[*db.ResponseFingerprint]().WithContext(ctx).WithMaxGoroutines(concurrency)

	for _, reqData := range requests {
		rd := reqData
		p.Go(func(ctx context.Context) (*db.ResponseFingerprint, error) {
			req, err := http.NewRequestWithContext(ctx, rd.method, rd.url, rd.body)
			if err != nil {
				return nil, nil
			}
			for k, v := range rd.headers {
				req.Header.Set(k, v)
			}

			result := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
				Client:                 httpClient,
				CreateHistory:          true,
				HistoryCreationOptions: historyOpts,
			})
			if result.Err != nil || result.History == nil {
				return nil, nil
			}

			return historyToFingerprint(result.History), nil
		})
	}

	results, _ := p.Wait()
	return collectFingerprints(results)
}

func (e *APIBehaviorExecutor) fingerprintMalformedBody(
	ctx context.Context,
	baseURL string,
	defType db.APIDefinitionType,
	httpClient *http.Client,
	historyOpts http_utils.HistoryCreationOptions,
	concurrency int,
) []db.ResponseFingerprint {
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	requests := buildMalformedBodyProbes(baseURL, defType)

	p := pool.NewWithResults[*db.ResponseFingerprint]().WithContext(ctx).WithMaxGoroutines(concurrency)

	for _, reqData := range requests {
		rd := reqData
		p.Go(func(ctx context.Context) (*db.ResponseFingerprint, error) {
			req, err := http.NewRequestWithContext(ctx, rd.method, rd.url, rd.body)
			if err != nil {
				return nil, nil
			}
			for k, v := range rd.headers {
				req.Header.Set(k, v)
			}

			result := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
				Client:                 httpClient,
				CreateHistory:          true,
				HistoryCreationOptions: historyOpts,
			})
			if result.Err != nil || result.History == nil {
				return nil, nil
			}

			return historyToFingerprint(result.History), nil
		})
	}

	results, _ := p.Wait()
	return collectFingerprints(results)
}

type probeRequest struct {
	method  string
	url     string
	body    *bytes.Reader
	headers map[string]string
}

func buildUnauthenticatedProbes(baseURL string, defType db.APIDefinitionType) []probeRequest {
	baseURL = strings.TrimSuffix(baseURL, "/")
	var probes []probeRequest

	switch defType {
	case db.APIDefinitionTypeGraphQL:
		queries := []string{
			`{"query":"{__typename}"}`,
			`{"query":"{ __type(name: \"Query\") { name } }"}`,
			`{"query":"{__schema{queryType{name}}}"}`,
			`{"query":"query { __typename }"}`,
		}
		for _, q := range queries {
			probes = append(probes, probeRequest{
				method:  "POST",
				url:     baseURL,
				body:    bytes.NewReader([]byte(q)),
				headers: map[string]string{"Content-Type": "application/json"},
			})
		}

	case db.APIDefinitionTypeOpenAPI:
		probes = append(probes,
			probeRequest{method: "GET", url: baseURL, body: bytes.NewReader(nil), headers: map[string]string{}},
			probeRequest{method: "GET", url: baseURL + "/", body: bytes.NewReader(nil), headers: map[string]string{}},
			probeRequest{method: "POST", url: baseURL, body: bytes.NewReader([]byte(`{}`)), headers: map[string]string{"Content-Type": "application/json"}},
			probeRequest{method: "OPTIONS", url: baseURL, body: bytes.NewReader(nil), headers: map[string]string{}},
		)

	case db.APIDefinitionTypeWSDL:
		envelope := `<?xml version="1.0" encoding="UTF-8"?><soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body/></soap:Envelope>`
		probes = append(probes,
			probeRequest{method: "POST", url: baseURL, body: bytes.NewReader([]byte(envelope)), headers: map[string]string{"Content-Type": "text/xml; charset=utf-8"}},
			probeRequest{method: "GET", url: baseURL, body: bytes.NewReader(nil), headers: map[string]string{}},
			probeRequest{method: "POST", url: baseURL, body: bytes.NewReader([]byte(envelope)), headers: map[string]string{"Content-Type": "text/xml; charset=utf-8", "SOAPAction": `""`}},
			probeRequest{method: "POST", url: baseURL, body: bytes.NewReader([]byte(envelope)), headers: map[string]string{"Content-Type": "application/soap+xml; charset=utf-8"}},
		)
	}

	return probes
}

func buildInvalidContentTypeProbes(baseURL string, defType db.APIDefinitionType) []probeRequest {
	baseURL = strings.TrimSuffix(baseURL, "/")
	var probes []probeRequest

	invalidContentTypes := []string{"text/plain", "text/html", "application/xml", "multipart/form-data"}

	switch defType {
	case db.APIDefinitionTypeGraphQL:
		for _, ct := range invalidContentTypes {
			probes = append(probes, probeRequest{
				method:  "POST",
				url:     baseURL,
				body:    bytes.NewReader([]byte(`{"query":"{__typename}"}`)),
				headers: map[string]string{"Content-Type": ct},
			})
		}

	case db.APIDefinitionTypeOpenAPI:
		for _, ct := range invalidContentTypes[:2] {
			probes = append(probes, probeRequest{
				method:  "POST",
				url:     baseURL,
				body:    bytes.NewReader([]byte(`{"test": true}`)),
				headers: map[string]string{"Content-Type": ct},
			})
		}
		probes = append(probes,
			probeRequest{method: "POST", url: baseURL, body: bytes.NewReader([]byte(`<xml/>`)), headers: map[string]string{"Content-Type": "application/json"}},
			probeRequest{method: "POST", url: baseURL, body: bytes.NewReader([]byte(`not-json`)), headers: map[string]string{"Content-Type": "application/json"}},
		)

	case db.APIDefinitionTypeWSDL:
		for _, ct := range []string{"application/json", "text/plain", "text/html", "application/octet-stream"} {
			probes = append(probes, probeRequest{
				method:  "POST",
				url:     baseURL,
				body:    bytes.NewReader([]byte(`not xml`)),
				headers: map[string]string{"Content-Type": ct},
			})
		}
	}

	return probes
}

func buildMalformedBodyProbes(baseURL string, defType db.APIDefinitionType) []probeRequest {
	baseURL = strings.TrimSuffix(baseURL, "/")
	var probes []probeRequest

	switch defType {
	case db.APIDefinitionTypeGraphQL:
		malformedBodies := []string{
			`{malformed`,
			`{"query": }`,
			`not json at all`,
			`{"query": "{ __typename }", "extra": ` + strings.Repeat("a", 1000) + `}`,
		}
		for _, body := range malformedBodies {
			probes = append(probes, probeRequest{
				method:  "POST",
				url:     baseURL,
				body:    bytes.NewReader([]byte(body)),
				headers: map[string]string{"Content-Type": "application/json"},
			})
		}

	case db.APIDefinitionTypeOpenAPI:
		malformedBodies := []string{
			`{broken json`,
			`{"key": undefined}`,
			`[]]]`,
			string(make([]byte, 0)),
		}
		for _, body := range malformedBodies {
			probes = append(probes, probeRequest{
				method:  "POST",
				url:     baseURL,
				body:    bytes.NewReader([]byte(body)),
				headers: map[string]string{"Content-Type": "application/json"},
			})
		}

	case db.APIDefinitionTypeWSDL:
		malformedBodies := []string{
			`<broken xml`,
			`<?xml version="1.0"?><unclosed`,
			`not xml at all`,
			`<soap:Envelope><invalid/></soap:Envelope>`,
		}
		for _, body := range malformedBodies {
			probes = append(probes, probeRequest{
				method:  "POST",
				url:     baseURL,
				body:    bytes.NewReader([]byte(body)),
				headers: map[string]string{"Content-Type": "text/xml; charset=utf-8"},
			})
		}
	}

	return probes
}

func historyToFingerprint(h *db.History) *db.ResponseFingerprint {
	if h == nil {
		return nil
	}
	return &db.ResponseFingerprint{
		StatusCode:   h.StatusCode,
		ResponseHash: h.ResponseHash(),
		ContentType:  h.ResponseContentType,
		BodySize:     h.ResponseBodySize,
	}
}

func collectFingerprints(results []*db.ResponseFingerprint) []db.ResponseFingerprint {
	var fps []db.ResponseFingerprint
	for _, r := range results {
		if r != nil {
			fps = append(fps, *r)
		}
	}
	return db.DeduplicateFingerprints(fps)
}
