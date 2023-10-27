package active

import (
	"fmt"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
	"net/http"
)

type HttpVersionScanResults struct {
	Http2 bool
	Http3 bool
}

func HttpVersionsScan(history *db.History, options ActiveModuleOptions) (HttpVersionScanResults, error) {
	results := HttpVersionScanResults{}
	auditLog := log.With().Str("audit", "http-versions").Str("url", history.URL).Uint("workspace", options.WorkspaceID).Logger()
	http2Client := http_utils.CreateHttp2Client()
	http2History, err := sendRequest(http2Client, history, options)
	if err == nil && history.ID > 0 {
		auditLog.Info().Msg("HTTP/2 is supported")
		results.Http2 = true
		details := fmt.Sprintf("The server responded to an HTTP/2 request with status code %d", http2History.StatusCode)
		db.CreateIssueFromHistoryAndTemplate(history, db.Http2DetectedCode, details, 90, "", &options.WorkspaceID, &options.TaskID, &options.TaskJobID)
	}

	http3Client := http_utils.CreateHttp3Client()
	http3History, err := sendRequest(http3Client, history, options)
	if err == nil && history.ID > 0 {
		auditLog.Info().Msg("HTTP/3 is supported")
		results.Http3 = true
		details := fmt.Sprintf("The server responded to an HTTP/3 request with status code %d", http3History.StatusCode)
		db.CreateIssueFromHistoryAndTemplate(history, db.Http3DetectedCode, details, 90, "", &options.WorkspaceID, &options.TaskID, &options.TaskJobID)
	}
	return results, nil
}

func sendRequest(client *http.Client, history *db.History, options ActiveModuleOptions) (*db.History, error) {
	request, err := http_utils.BuildRequestFromHistoryItem(history)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	new, err := http_utils.ReadHttpResponseAndCreateHistory(response, http_utils.HistoryCreationOptions{
		Source:              db.SourceScanner,
		WorkspaceID:         options.WorkspaceID,
		TaskID:              options.TaskID,
		CreateNewBodyStream: false,
	})
	return new, nil
}
