package active

import (
	"context"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/http_utils"

	"crypto/tls"
	"net"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

// SNIScan performs SNI injection audit
func SNIScan(historyItem *db.History, options ActiveModuleOptions, interactionsManager *integrations.InteractionsManager) {
	auditLog := log.With().Str("audit", "sni-injection").Str("url", historyItem.URL).Logger()

	ctx := options.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		auditLog.Info().Msg("SNI audit cancelled before starting")
		return
	default:
	}

	if !strings.HasPrefix(historyItem.URL, "https://") {
		auditLog.Info().Msg("URL is not HTTPS, skipping audit")
		return
	}
	auditLog.Info().Msg("Starting SNI Injection audit")
	interactionData := interactionsManager.GetURL()
	transport := &http.Transport{
		DialContext: (&net.Dialer{}).DialContext,
		TLSClientConfig: &tls.Config{
			ServerName: interactionData.URL,
		},
	}
	client := &http.Client{Transport: transport}
	request, err := http_utils.BuildRequestFromHistoryItem(historyItem)

	if err != nil {
		auditLog.Error().Err(err).Msg("Error creating the request")
		return
	}

	request = request.WithContext(ctx)

	issueTemplate := db.GetIssueTemplateByCode(db.SniInjectionCode)
	oobTest, err := db.Connection().CreateOOBTest(db.OOBTest{
		Code:              db.SniInjectionCode,
		TestName:          issueTemplate.Title,
		InteractionDomain: interactionData.URL,
		InteractionFullID: interactionData.ID,
		Target:            historyItem.URL,
		Payload:           interactionData.URL,
		InsertionPoint:    "sni",
		WorkspaceID:       &options.WorkspaceID,
		TaskID:            &options.TaskID,
		TaskJobID:         &options.TaskJobID,
		ScanID:            &options.ScanID,
		ScanJobID:         &options.ScanJobID,
	})
	if err != nil {
		auditLog.Error().Err(err).Msg("Failed to create OOB test")
		return
	}

	executionResult := http_utils.ExecuteRequest(request, http_utils.RequestExecutionOptions{
		Client:        client,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:              db.SourceScanner,
			WorkspaceID:         options.WorkspaceID,
			TaskID:              options.TaskID,
			TaskJobID:           options.TaskJobID,
			ScanID:              options.ScanID,
			ScanJobID:           options.ScanJobID,
			CreateNewBodyStream: false,
		},
	})

	if executionResult.Err != nil {
		auditLog.Error().Err(executionResult.Err).Msg("Error during request")
		return
	}

	history := executionResult.History

	if history != nil && history.ID != 0 {
		db.Connection().UpdateOOBTestHistoryID(oobTest.ID, &history.ID)
	}

	log.Info().Str("url", historyItem.URL).Msg("SNI Injection audit completed")
}
