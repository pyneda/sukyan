package active

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

type HttpVersionScanResults struct {
	Http2 bool
	Http3 bool
}

func HttpVersionsScan(history *db.History, options ActiveModuleOptions) (HttpVersionScanResults, error) {
	results := HttpVersionScanResults{}
	auditLog := log.With().Str("audit", "http-versions").Str("url", history.URL).Uint("workspace", options.WorkspaceID).Logger()

	// Get context, defaulting to background if not provided
	ctx := options.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Check context before starting
	select {
	case <-ctx.Done():
		auditLog.Info().Msg("HTTP versions scan cancelled before starting")
		return results, ctx.Err()
	default:
	}

	http2Client := http_utils.CreateHttp2Client()
	http2History, err := sendRequestWithContext(ctx, http2Client, history, options)
	if err == nil && http2History != nil && history.ID > 0 {
		auditLog.Info().Msg("HTTP/2 is supported")
		results.Http2 = true
		details := fmt.Sprintf("The server responded to an HTTP/2 request with status code %d", http2History.StatusCode)
		db.CreateIssueFromHistoryAndTemplate(history, db.Http2DetectedCode, details, 90, "", &options.WorkspaceID, &options.TaskID, &options.TaskJobID, &options.ScanID, &options.ScanJobID)
	} else if err != nil {
		auditLog.Debug().Err(err).Msg("Failed to send HTTP/2 request")
	}

	// Check context before second request
	select {
	case <-ctx.Done():
		auditLog.Info().Msg("HTTP versions scan cancelled before HTTP/3 check")
		return results, ctx.Err()
	default:
	}

	http3Client := http_utils.CreateHttp3Client()
	http3History, err := sendRequestWithContext(ctx, http3Client, history, options)
	if err == nil && http3History != nil && history.ID > 0 {
		auditLog.Info().Msg("HTTP/3 is supported")
		results.Http3 = true
		details := fmt.Sprintf("The server responded to an HTTP/3 request with status code %d", http3History.StatusCode)
		db.CreateIssueFromHistoryAndTemplate(history, db.Http3DetectedCode, details, 90, "", &options.WorkspaceID, &options.TaskID, &options.TaskJobID, &options.ScanID, &options.ScanJobID)
	} else if err != nil {
		auditLog.Debug().Err(err).Msg("Failed to send HTTP/3 request")
	}

	return results, nil
}

func sendRequest(client *http.Client, history *db.History, options ActiveModuleOptions) (*db.History, error) {
	return sendRequestWithContext(context.Background(), client, history, options)
}

func sendRequestWithContext(ctx context.Context, client *http.Client, history *db.History, options ActiveModuleOptions) (*db.History, error) {
	request, err := http_utils.BuildRequestFromHistoryItem(history)
	if err != nil {
		return nil, err
	}
	request = request.WithContext(ctx)

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
		return nil, executionResult.Err
	}

	return executionResult.History, nil
}
