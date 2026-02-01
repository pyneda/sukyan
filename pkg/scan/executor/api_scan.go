package executor

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/active"
	"github.com/pyneda/sukyan/pkg/active/api"
	pkgapi "github.com/pyneda/sukyan/pkg/api"
	apicore "github.com/pyneda/sukyan/pkg/api/core"
	apigraphql "github.com/pyneda/sukyan/pkg/api/graphql"
	apiopenapi "github.com/pyneda/sukyan/pkg/api/openapi"
	apisoap "github.com/pyneda/sukyan/pkg/api/soap"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/passive"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan/control"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
)

type APIScanJobData struct {
	DefinitionID        uuid.UUID                    `json:"definition_id"`
	EndpointID          uuid.UUID                    `json:"endpoint_id"`
	APIScanID           uuid.UUID                    `json:"api_scan_id"`
	Mode                scan_options.ScanMode        `json:"mode"`
	AuditCategories     scan_options.AuditCategories `json:"audit_categories"`
	RunAPISpecificTests bool                         `json:"run_api_specific_tests"`
	RunStandardTests    bool                         `json:"run_standard_tests"`
	RunSchemaTests      bool                         `json:"run_schema_tests"`
	AuthConfigID        *uuid.UUID                   `json:"auth_config_id,omitempty"`
	FingerprintTags     []string                     `json:"fingerprint_tags,omitempty"`
	MaxRetries          int                          `json:"max_retries,omitempty"`
}

type APIScanExecutor struct {
	interactionsManager *integrations.InteractionsManager
	payloadGenerators   []*generation.PayloadGenerator
}

func NewAPIScanExecutor(
	interactionsManager *integrations.InteractionsManager,
	payloadGenerators []*generation.PayloadGenerator,
) *APIScanExecutor {
	return &APIScanExecutor{
		interactionsManager: interactionsManager,
		payloadGenerators:   payloadGenerators,
	}
}

func (e *APIScanExecutor) JobType() db.ScanJobType {
	return db.ScanJobTypeAPIScan
}

func (e *APIScanExecutor) Execute(ctx context.Context, job *db.ScanJob, ctrl *control.ScanControl) error {
	taskLog := log.With().
		Uint("scan_id", job.ScanID).
		Uint("job_id", job.ID).
		Str("job_type", string(job.JobType)).
		Logger()

	taskLog.Info().Msg("Starting API scan job execution")

	var jobData APIScanJobData
	if err := json.Unmarshal(job.Payload, &jobData); err != nil {
		return fmt.Errorf("failed to parse job payload: %w", err)
	}

	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled before starting")
		return context.Canceled
	}

	definition, err := db.Connection().GetAPIDefinitionByID(jobData.DefinitionID)
	if err != nil {
		return fmt.Errorf("failed to get API definition %s: %w", jobData.DefinitionID, err)
	}

	endpoint, err := db.Connection().GetAPIEndpointByIDWithRelations(jobData.EndpointID)
	if err != nil {
		return fmt.Errorf("failed to get API endpoint %s: %w", jobData.EndpointID, err)
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

	var authConfig *db.APIAuthConfig
	if jobData.AuthConfigID != nil {
		authConfig, err = db.Connection().GetAPIAuthConfigByIDWithHeaders(*jobData.AuthConfigID)
		if err != nil {
			taskLog.Warn().Err(err).Msg("Failed to get auth config, proceeding without authentication")
		}
	}

	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled before building request")
		return context.Canceled
	}

	history, err := e.buildAndExecuteRequest(ctx, definition, endpoint, authConfig, httpClient, job)
	if err != nil {
		return fmt.Errorf("failed to execute API request: %w", err)
	}

	if history == nil {
		taskLog.Warn().Msg("No history created from API request")
		return nil
	}

	taskLog.Debug().Uint("history_id", history.ID).Msg("API request executed, history created")

	historyID := history.ID
	if updateErr := db.Connection().LinkHistoryToScanJob(job.ID, historyID); updateErr != nil {
		taskLog.Warn().Err(updateErr).Msg("Failed to link base request history to scan job")
	}
	job.HistoryID = &historyID

	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled before scanning")
		return context.Canceled
	}

	if jobData.AuditCategories.Passive {
		taskLog.Debug().Msg("Running passive scan on API endpoint")
		passive.ScanHistoryItem(history)
	}

	if jobData.RunStandardTests && (jobData.AuditCategories.ServerSide || jobData.AuditCategories.ClientSide) {
		taskLog.Debug().Msg("Running standard active scan on API endpoint")

		options := scan_options.HistoryItemScanOptions{
			Ctx:             ctx,
			WorkspaceID:     job.WorkspaceID,
			ScanID:          job.ScanID,
			ScanJobID:       job.ID,
			Mode:            jobData.Mode,
			AuditCategories: jobData.AuditCategories,
			FingerprintTags: jobData.FingerprintTags,
			MaxRetries:      jobData.MaxRetries,
			HTTPClient:      httpClient,
		}

		active.ScanHistoryItem(history, e.interactionsManager, e.payloadGenerators, options)
	}

	var operation *apicore.Operation
	if jobData.RunAPISpecificTests || jobData.RunSchemaTests {
		parsed, parseErr := e.parseOperationForEndpoint(definition, endpoint)
		if parseErr != nil {
			taskLog.Debug().Err(parseErr).Msg("Failed to parse operation for endpoint")
		}
		operation = parsed
	}

	if jobData.RunAPISpecificTests {
		taskLog.Debug().Msg("Running API-specific security tests")
		e.runAPISpecificTests(ctx, history, definition, endpoint, operation, httpClient, job, &jobData, ctrl)
	}

	if jobData.RunSchemaTests {
		taskLog.Debug().Msg("Running schema-based security tests")
		e.runSchemaBasedTests(ctx, history, definition, endpoint, operation, httpClient, job, &jobData, ctrl)
	}

	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled after scanning")
		return context.Canceled
	}

	if err := db.Connection().IncrementAPIScanCompletedEndpoints(jobData.APIScanID); err != nil {
		taskLog.Warn().Err(err).Msg("Failed to update API scan progress")
	}

	var issueCount int64
	db.Connection().DB().Model(&db.Issue{}).Where("api_endpoint_id = ?", endpoint.ID).Count(&issueCount)
	if err := db.Connection().MarkAPIEndpointScanned(endpoint.ID, int(issueCount)); err != nil {
		taskLog.Warn().Err(err).Msg("Failed to mark endpoint as scanned")
	}

	taskLog.Info().Msg("API scan job completed successfully")
	return nil
}

func (e *APIScanExecutor) buildAndExecuteRequest(
	ctx context.Context,
	definition *db.APIDefinition,
	endpoint *db.APIEndpoint,
	authConfig *db.APIAuthConfig,
	httpClient *http.Client,
	job *db.ScanJob,
) (*db.History, error) {
	var req *http.Request
	var err error

	if len(endpoint.RequestVariations) > 0 {
		req, err = e.buildRequestFromVariation(ctx, &endpoint.RequestVariations[0])
		if err != nil {
			log.Warn().Err(err).
				Str("endpoint", endpoint.Path).
				Str("method", endpoint.Method).
				Msg("Failed to build request from stored variation, falling back to operation parsing")
			req = nil
		} else {
			log.Debug().
				Str("endpoint", endpoint.Path).
				Str("method", endpoint.Method).
				Msg("Using stored request variation")
		}
	}

	if req == nil {
		operation, parseErr := e.parseOperationForEndpoint(definition, endpoint)
		if parseErr == nil && operation != nil {
			req, err = e.buildRequestFromOperation(ctx, definition.Type, operation)
			if err != nil {
				log.Warn().Err(err).
					Str("endpoint", endpoint.Path).
					Str("method", endpoint.Method).
					Msg("Failed to build request from parsed operation")
				return nil, fmt.Errorf("failed to create request: %w", err)
			}
		} else if parseErr != nil {
			log.Debug().Err(parseErr).
				Str("endpoint", endpoint.Path).
				Msg("Failed to parse operation for request building")
			return nil, fmt.Errorf("failed to parse operation: %w", parseErr)
		} else {
			return nil, fmt.Errorf("no matching operation found for endpoint %s %s", endpoint.Method, endpoint.Path)
		}
	}

	if req.Body != nil {
		bodyBytes, readErr := io.ReadAll(req.Body)
		if readErr != nil {
			return nil, fmt.Errorf("failed to read request body: %w", readErr)
		}
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		log.Debug().
			Int("body_size", len(bodyBytes)).
			Str("method", req.Method).
			Str("url", req.URL.String()).
			Str("content_type", req.Header.Get("Content-Type")).
			Msg("API scan request built")
	} else {
		log.Debug().
			Str("method", req.Method).
			Str("url", req.URL.String()).
			Msg("API scan request built with no body")
	}

	if authConfig != nil {
		e.applyAuthToRequest(req, authConfig)
	}

	historyOptions := http_utils.HistoryCreationOptions{
		Source:              db.SourceScanner,
		WorkspaceID:         job.WorkspaceID,
		ScanID:              job.ScanID,
		ScanJobID:           job.ID,
		CreateNewBodyStream: false,
	}

	execOptions := http_utils.RequestExecutionOptions{
		Client:                 httpClient,
		CreateHistory:          true,
		HistoryCreationOptions: historyOptions,
	}

	result := http_utils.ExecuteRequest(req, execOptions)
	if result.Err != nil {
		return result.History, fmt.Errorf("API request failed: %w", result.Err)
	}

	if result.History == nil {
		return nil, fmt.Errorf("no history created from request")
	}

	defID := definition.ID
	endpointID := endpoint.ID
	result.History.APIDefinitionID = &defID
	result.History.APIEndpointID = &endpointID

	_, err = db.Connection().UpdateHistory(result.History)
	if err != nil {
		return result.History, fmt.Errorf("failed to update history with API references: %w", err)
	}

	return result.History, nil
}

func (e *APIScanExecutor) buildRequestFromVariation(ctx context.Context, variation *db.APIRequestVariation) (*http.Request, error) {
	if variation.URL == "" {
		return nil, fmt.Errorf("variation has empty URL")
	}

	var bodyReader io.Reader
	if len(variation.Body) > 0 {
		bodyReader = bytes.NewReader(variation.Body)
	}

	method := variation.Method
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequestWithContext(ctx, method, variation.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request from variation: %w", err)
	}

	if len(variation.Headers) > 0 {
		var headers http.Header
		if err := json.Unmarshal(variation.Headers, &headers); err != nil {
			log.Warn().Err(err).
				Str("endpoint_id", variation.EndpointID.String()).
				Msg("Failed to unmarshal stored headers, proceeding without them")
		} else {
			for name, values := range headers {
				for _, v := range values {
					req.Header.Add(name, v)
				}
			}
		}
	}

	if variation.ContentType != "" {
		req.Header.Set("Content-Type", variation.ContentType)
	}

	return req, nil
}

func (e *APIScanExecutor) buildRequestFromOperation(ctx context.Context, defType db.APIDefinitionType, operation *apicore.Operation) (*http.Request, error) {
	return pkgapi.BuildDefaultRequest(ctx, defType, operation)
}

func (e *APIScanExecutor) applyAuthToRequest(req *http.Request, authConfig *db.APIAuthConfig) {
	switch authConfig.Type {
	case db.APIAuthTypeBasic:
		if authConfig.Username != "" || authConfig.Password != "" {
			auth := authConfig.Username + ":" + authConfig.Password
			encoded := base64.StdEncoding.EncodeToString([]byte(auth))
			req.Header.Set("Authorization", "Basic "+encoded)
		}

	case db.APIAuthTypeBearer:
		if authConfig.Token != "" {
			prefix := authConfig.TokenPrefix
			if prefix == "" {
				prefix = "Bearer"
			}
			req.Header.Set("Authorization", prefix+" "+authConfig.Token)
		}

	case db.APIAuthTypeAPIKey:
		if authConfig.APIKeyName != "" && authConfig.APIKeyValue != "" {
			switch authConfig.APIKeyLocation {
			case db.APIKeyLocationHeader:
				req.Header.Set(authConfig.APIKeyName, authConfig.APIKeyValue)
			case db.APIKeyLocationQuery:
				q := req.URL.Query()
				q.Set(authConfig.APIKeyName, authConfig.APIKeyValue)
				req.URL.RawQuery = q.Encode()
			case db.APIKeyLocationCookie:
				req.AddCookie(&http.Cookie{
					Name:  authConfig.APIKeyName,
					Value: authConfig.APIKeyValue,
				})
			}
		}

	case db.APIAuthTypeOAuth2:
		if authConfig.Token != "" {
			prefix := authConfig.TokenPrefix
			if prefix == "" {
				prefix = "Bearer"
			}
			req.Header.Set("Authorization", prefix+" "+authConfig.Token)
		} else {
			log.Warn().Msg("OAuth2 auth config has no token set, skipping authentication")
		}
	}

	for _, header := range authConfig.CustomHeaders {
		req.Header.Set(header.HeaderName, header.HeaderValue)
	}
}

func (e *APIScanExecutor) buildAPITestOptions(
	ctx context.Context,
	job *db.ScanJob,
	jobData *APIScanJobData,
	definition *db.APIDefinition,
	endpoint *db.APIEndpoint,
	history *db.History,
	operation *apicore.Operation,
	httpClient *http.Client,
) api.APITestOptions {
	defID := definition.ID
	endpointID := endpoint.ID

	return api.APITestOptions{
		ActiveModuleOptions: active.ActiveModuleOptions{
			Ctx:         ctx,
			WorkspaceID: job.WorkspaceID,
			TaskID:      0,
			TaskJobID:   0,
			ScanID:      job.ScanID,
			ScanJobID:   job.ID,
			ScanMode:    jobData.Mode,
			HTTPClient:  httpClient,
			APIContext: &scan_options.APIContext{
				DefinitionType: string(definition.Type),
				DefinitionID:   &defID,
				EndpointID:     &endpointID,
			},
		},
		Definition:  definition,
		Endpoint:    endpoint,
		BaseHistory: history,
		Operation:   operation,
	}
}

func (e *APIScanExecutor) runAPISpecificTests(
	ctx context.Context,
	history *db.History,
	definition *db.APIDefinition,
	endpoint *db.APIEndpoint,
	operation *apicore.Operation,
	httpClient *http.Client,
	job *db.ScanJob,
	jobData *APIScanJobData,
	ctrl *control.ScanControl,
) {
	taskLog := log.With().
		Uint("scan_id", job.ScanID).
		Uint("job_id", job.ID).
		Str("endpoint", endpoint.Path).
		Logger()

	opts := e.buildAPITestOptions(ctx, job, jobData, definition, endpoint, history, operation, httpClient)

	var results []api.APITestResult

	switch definition.Type {
	case db.APIDefinitionTypeGraphQL:
		taskLog.Debug().Msg("Running GraphQL-specific tests")
		results = api.RunGraphQLTests(opts)

	case db.APIDefinitionTypeOpenAPI:
		taskLog.Debug().Msg("Running REST API-specific tests")
		results = api.RunRESTTests(opts)

	case db.APIDefinitionTypeWSDL:
		taskLog.Debug().Msg("Running SOAP/WSDL-specific tests")
		results = api.RunSOAPTests(opts)
	}

	for _, result := range results {
		if result.Vulnerable {
			issueHistory := history
			if result.History != nil {
				issueHistory = result.History
			}
			_, issueErr := db.CreateIssueFromHistoryAndTemplate(
				issueHistory,
				result.IssueCode,
				result.Details,
				result.Confidence,
				"",
				&job.WorkspaceID,
				nil,
				nil,
				&job.ScanID,
				&job.ID,
			)
			if issueErr != nil {
				taskLog.Error().Err(issueErr).
					Str("issue_code", string(result.IssueCode)).
					Msg("Failed to create issue from API-specific test result")
			} else {
				taskLog.Info().
					Str("issue", string(result.IssueCode)).
					Int("confidence", result.Confidence).
					Msg("API-specific issue found")
			}
		}
	}

	taskLog.Debug().Int("results", len(results)).Msg("API-specific tests completed")
}

func (e *APIScanExecutor) runSchemaBasedTests(
	ctx context.Context,
	history *db.History,
	definition *db.APIDefinition,
	endpoint *db.APIEndpoint,
	operation *apicore.Operation,
	httpClient *http.Client,
	job *db.ScanJob,
	jobData *APIScanJobData,
	ctrl *control.ScanControl,
) {
	taskLog := log.With().
		Uint("scan_id", job.ScanID).
		Uint("job_id", job.ID).
		Str("endpoint", endpoint.Path).
		Logger()

	if operation == nil {
		taskLog.Debug().Msg("No operation found for schema testing")
		return
	}

	opts := e.buildAPITestOptions(ctx, job, jobData, definition, endpoint, history, operation, httpClient)

	var results []api.APITestResult

	if !ctrl.CheckpointWithContext(ctx) {
		return
	}

	schemaResults := api.RunSchemaValidationTests(opts)
	results = append(results, schemaResults...)

	if !ctrl.CheckpointWithContext(ctx) {
		return
	}

	typeConfusionResults := api.RunTypeConfusionTests(opts)
	results = append(results, typeConfusionResults...)

	for _, result := range results {
		if result.Vulnerable {
			issueHistory := history
			if result.History != nil {
				issueHistory = result.History
			}

			_, err := db.CreateIssueFromHistoryAndTemplate(
				issueHistory,
				result.IssueCode,
				result.Details,
				result.Confidence,
				"",
				&job.WorkspaceID,
				nil,
				nil,
				&job.ScanID,
				&job.ID,
			)

			if err != nil {
				taskLog.Error().Err(err).
					Str("issue_code", string(result.IssueCode)).
					Msg("Failed to create issue from schema test result")
			} else {
				taskLog.Info().
					Str("issue", string(result.IssueCode)).
					Int("confidence", result.Confidence).
					Msg("Schema validation issue found")
			}
		}
	}

	taskLog.Debug().Int("results", len(results)).Msg("Schema-based tests completed")
}

func (e *APIScanExecutor) parseOperationForEndpoint(
	definition *db.APIDefinition,
	endpoint *db.APIEndpoint,
) (*apicore.Operation, error) {
	var operations []apicore.Operation
	var err error

	switch definition.Type {
	case db.APIDefinitionTypeOpenAPI:
		parser := apiopenapi.NewParser()
		operations, err = parser.Parse(definition)
	case db.APIDefinitionTypeGraphQL:
		parser := apigraphql.NewParser()
		operations, err = parser.Parse(definition)
	case db.APIDefinitionTypeWSDL:
		parser := apisoap.NewParser()
		operations, err = parser.Parse(definition)
	default:
		return nil, fmt.Errorf("unsupported API definition type: %s", definition.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse definition: %w", err)
	}

	if endpoint.OperationID != "" {
		for i := range operations {
			if operations[i].OperationID == endpoint.OperationID || operations[i].Name == endpoint.OperationID {
				return &operations[i], nil
			}
		}
	}

	for i := range operations {
		if operations[i].Path == endpoint.Path && strings.EqualFold(operations[i].Method, endpoint.Method) {
			return &operations[i], nil
		}
	}

	return nil, nil
}
