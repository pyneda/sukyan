package active

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/scan"
	"github.com/rs/zerolog/log"
)

const openRedirecTestDomain = "sukyan.com"

func OpenRedirectScan(history *db.History, options ActiveModuleOptions, insertionPoints []scan.InsertionPoint) (bool, error) {
	auditLog := log.With().Str("audit", "open-redirect").Str("url", history.URL).Uint("workspace", options.WorkspaceID).Logger()
	payloads := []string{
		"https://" + openRedirecTestDomain,
		"//" + openRedirecTestDomain,
		"https%3A%2F%2F" + openRedirecTestDomain,
	}

	scanInsertionPoints := []scan.InsertionPoint{}
	switch options.ScanMode {

	case scan.ScanModeFuzz:
		scanInsertionPoints = insertionPoints
		return false, nil
	default:
		headers, err := history.GetResponseHeadersAsMap()
		if err != nil {
			auditLog.Error().Err(err).Msg("Failed to get response headers")
			return false, err
		}
		locations := headers["Location"]
		if len(locations) == 0 {
			auditLog.Info().Msg("Testing all insertion points for open redirect as no Location header was found")
			scanInsertionPoints = append(scanInsertionPoints, insertionPoints...)
		} else {
			for _, insertionPoint := range insertionPoints {
				if lib.SliceContains(locations, insertionPoint.Value) || lib.SliceContains(locations, insertionPoint.OriginalData) || insertionPoint.ValueType == lib.TypeURL {
					auditLog.Info().Str("insertionPoint", insertionPoint.Value).Msg("Found an interesting insertion point to test for open redirect")
					scanInsertionPoints = append(scanInsertionPoints, insertionPoint)
				}
			}
		}

	}

	if len(scanInsertionPoints) == 0 {
		auditLog.Info().Msg("No interesting insertion points to test for open redirect")
		return false, nil
	}
	client := http_utils.CreateHttpClient()
	// ensure that the client does not follow redirects
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	for _, insertionPoint := range scanInsertionPoints {
		for _, payload := range payloads {
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
			response, err := client.Do(req)
			if err != nil {
				auditLog.Error().Err(err).Msg("Failed to send request")
				continue
			}
			new, err := http_utils.ReadHttpResponseAndCreateHistory(response, http_utils.HistoryCreationOptions{
				Source:              db.SourceScanner,
				WorkspaceID:         options.WorkspaceID,
				TaskID:              options.TaskID,
				CreateNewBodyStream: true,
			})

			if err != nil {
				auditLog.Error().Err(err).Msg("Failed to create history from response")
				continue
			}

			if new.StatusCode >= 300 && new.StatusCode < 400 {
				newLocation := response.Header.Get("Location")
				if newLocation != "" && newLocation == payload || strings.Contains(newLocation, openRedirecTestDomain) {
					auditLog.Info().Str("insertionPoint", insertionPoint.String()).Str("payload", payload).Msg("Open redirect found")

					details := fmt.Sprintf("Using the payload %s in the insertion point %s, the server redirected the request to %s.", payload, insertionPoint.String(), newLocation)
					db.CreateIssueFromHistoryAndTemplate(new, db.OpenRedirectCode, details, 90, "", &options.WorkspaceID, &options.TaskID, &options.TaskJobID)

					return true, nil

				}

			}

		}
	}
	return false, nil
}
