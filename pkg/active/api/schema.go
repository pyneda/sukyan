package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/db"
	pkgapi "github.com/pyneda/sukyan/pkg/api"
	"github.com/pyneda/sukyan/pkg/api/core"
	"github.com/pyneda/sukyan/pkg/api/payloads"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

func RunSchemaValidationTests(opts APITestOptions) []APITestResult {
	var results []APITestResult

	if opts.Operation == nil {
		return results
	}

	taskLog := log.With().
		Str("module", "schema-validation-tests").
		Uint("workspace", opts.WorkspaceID).
		Logger()

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			taskLog.Debug().Msg("Context cancelled, skipping schema validation tests")
			return results
		default:
		}
	}

	for _, param := range opts.Operation.Parameters {
		if !param.HasConstraints() {
			continue
		}

		boundaryPayloads := core.GenerateBoundaryPayloads(param)
		for _, payload := range boundaryPayloads {
			if opts.Ctx != nil {
				select {
				case <-opts.Ctx.Done():
					return results
				default:
				}
			}

			result := testSchemaPayload(opts, param, payload)
			if result != nil {
				results = append(results, *result)
			}
		}
	}

	taskLog.Debug().Int("results", len(results)).Msg("Schema validation tests completed")
	return results
}

func RunTypeConfusionTests(opts APITestOptions) []APITestResult {
	var results []APITestResult

	if opts.Operation == nil {
		return results
	}

	taskLog := log.With().
		Str("module", "type-confusion-tests").
		Uint("workspace", opts.WorkspaceID).
		Logger()

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			taskLog.Debug().Msg("Context cancelled, skipping type confusion tests")
			return results
		default:
		}
	}

	for _, param := range opts.Operation.Parameters {
		confusionPayloads := payloads.GenerateTypeConfusionPayloads(param)
		for _, payload := range confusionPayloads {
			if opts.Ctx != nil {
				select {
				case <-opts.Ctx.Done():
					return results
				default:
				}
			}

			result := testTypeConfusion(opts, param, payload)
			if result != nil {
				results = append(results, *result)
			}
		}
	}

	taskLog.Debug().Int("results", len(results)).Msg("Type confusion tests completed")
	return results
}

func testSchemaPayload(opts APITestOptions, param core.Parameter, payload core.BoundaryPayload) *APITestResult {
	req, err := buildSchemaTestRequest(opts, param.Name, payload.Value)
	if err != nil {
		return nil
	}

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
		return nil
	}

	if isValidationMissing(result.History) {
		return &APITestResult{
			Vulnerable: true,
			IssueCode:  db.SchemaValidationMissingCode,
			Details:    buildSchemaDetails(param, payload, result.History),
			Confidence: 70,
			History:    result.History,
		}
	}

	return nil
}

func testTypeConfusion(opts APITestOptions, param core.Parameter, payload payloads.TypeConfusionPayload) *APITestResult {
	req, err := buildSchemaTestRequest(opts, param.Name, payload.Value)
	if err != nil {
		return nil
	}

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
		return nil
	}

	if isValidationMissing(result.History) {
		return &APITestResult{
			Vulnerable: true,
			IssueCode:  db.ApiTypeConfusionCode,
			Details:    buildTypeConfusionDetails(param, payload, result.History),
			Confidence: 60,
			History:    result.History,
		}
	}

	return nil
}

func buildSchemaTestRequest(opts APITestOptions, paramName string, value any) (*http.Request, error) {
	if opts.Operation == nil {
		return nil, fmt.Errorf("operation is nil")
	}

	paramValues := make(map[string]any)
	for _, param := range opts.Operation.Parameters {
		paramValues[param.Name] = param.GetEffectiveValue()
	}
	paramValues[paramName] = value

	return pkgapi.BuildRequest(opts.Ctx, opts.Operation.APIType, *opts.Operation, paramValues)
}

func isValidationMissing(history *db.History) bool {
	statusCode := history.StatusCode

	if statusCode >= 500 {
		return true
	}

	if statusCode >= 400 && statusCode < 500 {
		return false
	}

	if statusCode >= 200 && statusCode < 300 {
		body, err := history.ResponseBody()
		if err != nil || len(body) == 0 {
			return true
		}

		bodyLower := strings.ToLower(string(body))
		validationIndicators := []string{
			"\"error\"", "\"errors\"", "validation failed", "invalid",
			"\"status\":\"error\"", "\"success\":false",
		}
		for _, indicator := range validationIndicators {
			if strings.Contains(bodyLower, indicator) {
				return false
			}
		}

		return true
	}

	return false
}

func buildSchemaDetails(param core.Parameter, payload core.BoundaryPayload, history *db.History) string {
	statusExplanation := "Success"
	if history.StatusCode >= 500 {
		statusExplanation = "Server Error - the server crashed processing invalid input"
	}

	return fmt.Sprintf(`Schema validation is missing for parameter '%s'.

The API accepted an invalid value that should have been rejected based on the API specification.

Parameter Details:
- Name: %s
- Location: %s
- Expected Type: %s
- Violation: %s

Payload Used:
%v

Expected Behavior:
%s

Actual Behavior:
Response Status: %d (%s)

Impact:
- Data integrity issues
- Business logic bypass
- Injection attacks if the value reaches backend systems

Remediation:
- Enforce server-side validation that matches the API specification.
- Do not rely solely on client-side validation.`,
		param.Name,
		param.Name,
		param.Location,
		param.DataType,
		payload.ViolationType,
		payload.Value,
		payload.ExpectedResult,
		history.StatusCode,
		statusExplanation,
	)
}

func buildTypeConfusionDetails(param core.Parameter, payload payloads.TypeConfusionPayload, history *db.History) string {
	statusExplanation := "Success"
	if history.StatusCode >= 500 {
		statusExplanation = "Server Error - the server crashed processing mistyped input"
	}

	return fmt.Sprintf(`Type confusion detected for parameter '%s'.

The API accepted a value of incorrect type without proper validation.

Parameter Details:
- Name: %s
- Expected Type: %s
- Actual Type Sent: %s

Payload:
%v

Description:
%s

Response Status:
%d (%s)

Impact:
- Unexpected application behavior
- Security bypass in type-checking code
- Data corruption

Remediation:
- Implement strict type checking on the server side.`,
		param.Name,
		param.Name,
		payload.ExpectedType,
		payload.ActualType,
		payload.Value,
		payload.Description,
		history.StatusCode,
		statusExplanation,
	)
}
