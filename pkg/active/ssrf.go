package active

import (
	"context"
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/scan"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// ssrfCanaryMatch reports whether the response body proves the server fetched our
// canary endpoint. It requires BOTH the fixed marker (which only appears if the
// target actually retrieved the canary response body) and the request-specific
// token. The marker is the load-bearing, echo-immune signal: an endpoint that
// merely echoes the injected URL reflects the token but never the marker.
func ssrfCanaryMatch(body, marker, token string) bool {
	if marker == "" || token == "" {
		return false
	}
	return strings.Contains(body, marker) && strings.Contains(body, token)
}

func SSRFCanaryScan(history *db.History, options ActiveModuleOptions, insertionPoints []scan.InsertionPoint) (bool, error) {
	auditLog := log.With().Str("audit", "ssrf-canary").Str("url", history.URL).Uint("workspace", options.WorkspaceID).Logger()

	canaryBase := strings.TrimRight(viper.GetString("scan.ssrf.canary_url"), "/")
	marker := viper.GetString("scan.ssrf.canary_marker")
	if canaryBase == "" || marker == "" {
		auditLog.Debug().Msg("SSRF canary scan is dormant (no canary_url/canary_marker configured)")
		return false, nil
	}

	ctx := options.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		auditLog.Info().Msg("SSRF canary scan cancelled before starting")
		return false, ctx.Err()
	default:
	}

	scanInsertionPoints := []scan.InsertionPoint{}
	for _, insertionPoint := range insertionPoints {
		if scan.IsCommonSSRFParameter(insertionPoint.Name) || insertionPoint.ValueType == lib.TypeURL || insertionPoint.Behaviour.IsReflected {
			scanInsertionPoints = append(scanInsertionPoints, insertionPoint)
		}
	}

	if len(scanInsertionPoints) == 0 {
		auditLog.Debug().Msg("No interesting insertion points to test for SSRF")
		return false, nil
	}

	client := options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	for _, insertionPoint := range scanInsertionPoints {
		select {
		case <-ctx.Done():
			auditLog.Info().Msg("SSRF canary scan cancelled during testing")
			return false, ctx.Err()
		default:
		}

		token := lib.GenerateRandomLowercaseString(12)
		canaryURL := canaryBase + "/ssrf/" + token

		auditLog.Info().Str("insertionPoint", insertionPoint.String()).Str("canary", canaryURL).Msg("Testing insertion point for SSRF via canary reflection")
		builders := []scan.InsertionPointBuilder{
			{
				Point:   insertionPoint,
				Payload: canaryURL,
			},
		}
		req, err := scan.CreateRequestFromInsertionPoints(history, builders)
		if err != nil {
			auditLog.Error().Err(err).Msg("Failed to create request from insertion points")
			continue
		}

		req = req.WithContext(ctx)

		executionResult := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
			Client:        client,
			CreateHistory: true,
			HistoryCreationOptions: http_utils.HistoryCreationOptions{
				Source:              db.SourceScanner,
				WorkspaceID:         options.WorkspaceID,
				TaskID:              options.TaskID,
				TaskJobID:           options.TaskJobID,
				ScanID:              options.ScanID,
				ScanJobID:           options.ScanJobID,
				CreateNewBodyStream: true,
			},
		})
		if executionResult.Err != nil {
			auditLog.Error().Err(executionResult.Err).Msg("Failed to send request")
			continue
		}

		new := executionResult.History
		responseBody, err := new.ResponseBody()
		if err != nil {
			auditLog.Error().Err(err).Msg("Failed to read response body")
			continue
		}

		if ssrfCanaryMatch(string(responseBody), marker, token) {
			auditLog.Info().Str("insertionPoint", insertionPoint.String()).Str("canary", canaryURL).Msg("SSRF found via canary reflection")

			details := fmt.Sprintf("Injecting the canary URL %s into the insertion point %s caused the server to fetch it: the canary marker %q was reflected in the response together with the request-specific token %s.\n\nThe marker is only present in the canary endpoint's own response body, so it cannot be produced by the sink merely echoing the injected URL. This proves the server-side request was actually made (full-response SSRF).", canaryURL, insertionPoint.String(), marker, token)
			db.CreateIssueFromHistoryAndTemplate(new, db.SsrfCode, details, 90, "", &options.WorkspaceID, &options.TaskID, &options.TaskJobID, &options.ScanID, &options.ScanJobID)

			return true, nil
		}
	}

	return false, nil
}
