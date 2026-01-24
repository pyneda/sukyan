package active

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/scan"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
)

const openRedirecTestDomain = "sukyan.com"

func OpenRedirectScan(history *db.History, options ActiveModuleOptions, insertionPoints []scan.InsertionPoint) (bool, error) {
	auditLog := log.With().Str("audit", "open-redirect").Str("url", history.URL).Uint("workspace", options.WorkspaceID).Logger()

	// Get context, defaulting to background if not provided
	ctx := options.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Check context before starting
	select {
	case <-ctx.Done():
		auditLog.Info().Msg("Open redirect scan cancelled before starting")
		return false, ctx.Err()
	default:
	}

	payloads := []string{
		"https://" + openRedirecTestDomain,
		"//" + openRedirecTestDomain,
		"https%3A%2F%2F" + openRedirecTestDomain,
		"//%5c" + openRedirecTestDomain,
	}

	scanInsertionPoints := []scan.InsertionPoint{}
	switch options.ScanMode {

	case scan_options.ScanModeFuzz:
		scanInsertionPoints = insertionPoints

	default:
		headers, err := history.GetResponseHeadersAsMap()
		if err != nil {
			auditLog.Error().Err(err).Msg("Failed to get response headers")
			return false, err
		}
		locations := headers["Location"]
		for _, insertionPoint := range insertionPoints {
			if lib.SliceContains(locations, insertionPoint.Value) || lib.SliceContains(locations, insertionPoint.OriginalData) || scan.IsCommonOpenRedirectParameter(insertionPoint.Name) || insertionPoint.ValueType == lib.TypeURL {
				auditLog.Info().Str("insertionPoint", insertionPoint.Value).Msg("Found an interesting insertion point to test for open redirect")
				scanInsertionPoints = append(scanInsertionPoints, insertionPoint)
			}
		}

	}

	if len(scanInsertionPoints) == 0 {
		auditLog.Info().Msg("No interesting insertion points to test for open redirect")
		return false, nil
	}
	client := options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	for _, insertionPoint := range scanInsertionPoints {
		for _, payload := range payloads {
			// Check context before each test
			select {
			case <-ctx.Done():
				auditLog.Info().Msg("Open redirect scan cancelled during testing")
				return false, ctx.Err()
			default:
			}

			auditLog.Info().Str("insertionPoint", insertionPoint.Value).Str("payload", payload).Msg("Testing insertion point for open redirect")
			builders := []scan.InsertionPointBuilder{
				{
					Point:   insertionPoint,
					Payload: payload,
				},
			}
			req, err := scan.CreateRequestFromInsertionPoints(history, builders)
			if err != nil {
				auditLog.Error().Err(err).Msg("Failed to create request from insertion points")
				continue
			}

			// Add context to request
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

			if new.StatusCode >= 300 && new.StatusCode < 400 {
				newLocation := executionResult.Response.Header.Get("Location")
				if newLocation != "" && newLocation == payload || strings.Contains(newLocation, openRedirecTestDomain) {
					auditLog.Info().Str("insertionPoint", insertionPoint.String()).Str("payload", payload).Msg("Open redirect found")

					details := fmt.Sprintf("Using the payload %s in the insertion point %s, the server redirected the request to %s.", payload, insertionPoint.String(), newLocation)
					db.CreateIssueFromHistoryAndTemplate(new, db.OpenRedirectCode, details, 90, "", &options.WorkspaceID, &options.TaskID, &options.TaskJobID, &options.ScanID, &options.ScanJobID)

					return true, nil

				}

			}

		}
	}
	return false, nil
}
