package active

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

var jsonpCallbackParameters = []string{
	"callback",
	"jsonp",
	"cb",
	"json",
	"jquery",
	"jsonpcallback",
	"jcb",
	"call",
}

func getCallbacksForMode(mode scan_options.ScanMode, hasJsonParam bool) []string {
	switch mode {
	case scan_options.ScanModeFuzz:
		return jsonpCallbackParameters
	case scan_options.ScanModeSmart:
		if hasJsonParam {
			return jsonpCallbackParameters
		}
		return jsonpCallbackParameters[:5]
	case scan_options.ScanModeFast:
		if hasJsonParam {
			return jsonpCallbackParameters
		}
		return jsonpCallbackParameters[:2]
	default:
		return jsonpCallbackParameters[:2]
	}
}

func JSONPCallbackScan(history *db.History, options ActiveModuleOptions) {
	auditLog := log.With().Str("audit", "jsonp").Str("url", history.URL).Uint("workspace", options.WorkspaceID).Logger()

	// Get context, defaulting to background if not provided
	ctx := options.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Check context before starting
	select {
	case <-ctx.Done():
		auditLog.Info().Msg("JSONP scan cancelled before starting")
		return
	default:
	}

	if options.Concurrency == 0 {
		options.Concurrency = 5
	}
	originalBody, err := history.ResponseBody()
	if err != nil && isJsonpResponse(string(originalBody)) {
		createJSONPIssue(history, "Endpoint returns JSONP response by default", 90, options)
		return
	}

	hasJsonParam := hasJsonpParameter(history)
	callbacksToTest := getCallbacksForMode(options.ScanMode, hasJsonParam)

	client := http_utils.CreateHttpClient()
	p := pool.New().WithMaxGoroutines(options.Concurrency)

	for _, param := range callbacksToTest {
		// Check context before scheduling each test
		select {
		case <-ctx.Done():
			auditLog.Info().Msg("JSONP scan cancelled during parameter iteration")
			p.Wait()
			return
		default:
		}

		callbackParam := param
		p.Go(func() {
			// Check context before making request
			select {
			case <-ctx.Done():
				return
			default:
			}

			testCallback := lib.GenerateRandomString(8)
			request, err := http_utils.BuildRequestFromHistoryItem(history)
			if err != nil {
				auditLog.Error().Err(err).Msg("Error creating request")
				return
			}

			// Add context to request
			request = request.WithContext(ctx)

			q := request.URL.Query()
			q.Add(callbackParam, testCallback)
			request.URL.RawQuery = q.Encode()

			executionResult := http_utils.ExecuteRequest(request, http_utils.RequestExecutionOptions{
				Client:        client,
				CreateHistory: true,
				HistoryCreationOptions: http_utils.HistoryCreationOptions{
					Source:              db.SourceScanner,
					WorkspaceID:         options.WorkspaceID,
					TaskID:              options.TaskID,
					ScanID:              options.ScanID,
					ScanJobID:           options.ScanJobID,
					CreateNewBodyStream: false,
				},
			})

			if executionResult.Err != nil {
				auditLog.Error().Err(executionResult.Err).Msg("Error during request")
				return
			}

			newHistory := executionResult.History
			bodyStr := string(executionResult.ResponseData.Body)
			if isJsonpResponse(bodyStr) {
				isControllable := strings.Contains(bodyStr, testCallback+"(")

				callbackType := "possible"
				if isControllable {
					callbackType = "controllable"
				}

				paramDiscovery := "No JSONP parameter was initially present"
				if hasJsonParam {
					paramDiscovery = "JSONP parameter was already present"
				}

				controlDetails := "A JSONP response was received but the callback function name might not be fully controllable"
				if isControllable {
					controlDetails = "The callback function name is fully controllable via URL parameter"
				}

				details := fmt.Sprintf(`
JSONP endpoint detected with %s callback function.

Test Details:
- Tested Parameter: %s
- Test Value: %s
- URL: %s
- Scan Mode: %s
- Parameter Discovery: %s

%s

Original Response:
%s

Modified Response:
%s
`,
					callbackType,
					callbackParam,
					testCallback,
					request.URL.String(),
					options.ScanMode,
					paramDiscovery,
					controlDetails,
					string(originalBody),
					bodyStr)

				confidence := 75
				if isControllable {
					confidence = 90
				}
				createJSONPIssue(newHistory, details, confidence, options)
			}
		})
	}

	p.Wait()
	auditLog.Info().
		Str("scan_mode", string(options.ScanMode)).
		Bool("has_jsonp_param", hasJsonParam).
		Int("callbacks_tested", len(callbacksToTest)).
		Msg("Finished JSONP scan")
}

func isJsonpResponse(body string) bool {
	// Remove trailing semicolon if present
	body = strings.TrimRight(body, ";")
	body = strings.TrimSpace(body)

	// Basic format check: should end with ) and contain (
	if !strings.HasSuffix(body, ")") || !strings.Contains(body, "(") {
		return false
	}

	// Split into function name and content
	parts := strings.SplitN(body, "(", 2)
	if len(parts) != 2 {
		return false
	}

	funcName := strings.TrimSpace(parts[0])
	content := strings.TrimSpace(parts[1])

	// Function name validation
	if funcName == "" || strings.ContainsAny(funcName, "(){}[]<>") {
		return false
	}

	// Remove trailing ) from content
	content = strings.TrimSuffix(content, ")")

	// Validate JSON content
	var js interface{}
	if err := json.Unmarshal([]byte(content), &js); err != nil {
		return false
	}

	return true
}
func createJSONPIssue(history *db.History, details string, confidence int, options ActiveModuleOptions) {
	db.CreateIssueFromHistoryAndTemplate(
		history,
		db.JsonpEndpointDetectedCode,
		details,
		confidence,
		"",
		&options.WorkspaceID,
		&options.TaskID,
		&options.TaskJobID,
		&options.ScanID,
		&options.ScanJobID,
	)
}

func hasJsonpParameter(history *db.History) bool {
	u, err := url.Parse(history.URL)
	if err != nil {
		return false
	}

	queryParams := u.Query()

	for _, param := range jsonpCallbackParameters {
		if queryParams.Has(param) {
			return true
		}
	}

	jsonpSubstrings := []string{"callback", "jsonp", "json"}
	for paramName := range queryParams {
		paramLower := strings.ToLower(paramName)
		for _, substr := range jsonpSubstrings {
			if strings.Contains(paramLower, substr) {
				return true
			}
		}
	}

	return false
}
