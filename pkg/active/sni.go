package active

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/http_utils"

	"crypto/tls"
	"github.com/rs/zerolog/log"
	"net"
	"net/http"
	"strings"
)

// SNIAudit configuration
type SNIAudit struct {
	HistoryItem         *db.History
	InteractionsManager *integrations.InteractionsManager
	WorkspaceID         uint
}

// Run starts the audit
func (a *SNIAudit) Run() {
	auditLog := log.With().Str("audit", "sni-injection").Str("url", a.HistoryItem.URL).Logger()

	if !strings.HasPrefix(a.HistoryItem.URL, "https://") {
		auditLog.Info().Msg("URL is not HTTPS, skipping audit")
		return
	}
	auditLog.Info().Msg("Starting SNI Injection audit")
	interactionData := a.InteractionsManager.GetURL()
	transport := &http.Transport{
		DialContext: (&net.Dialer{}).DialContext,
		TLSClientConfig: &tls.Config{
			ServerName: interactionData.URL,
		},
	}
	client := &http.Client{Transport: transport}
	request, err := http_utils.BuildRequestFromHistoryItem(a.HistoryItem)

	if err != nil {
		auditLog.Error().Err(err).Msg("Error creating the request")
		return
	}

	response, err := client.Do(request)

	if err != nil {
		auditLog.Error().Err(err).Msg("Error during request")
		return
	}
	history, err := http_utils.ReadHttpResponseAndCreateHistory(response, db.SourceScanner, a.WorkspaceID)

	if err != nil {
		auditLog.Error().Err(err).Msg("Error reading response")
	}
	issueTemplate := db.GetIssueTemplateByCode(db.SniInjectionCode)
	oobTest := db.OOBTest{
		Code:              db.SniInjectionCode,
		TestName:          issueTemplate.Title,
		InteractionDomain: interactionData.URL,
		InteractionFullID: interactionData.ID,
		Target:            a.HistoryItem.URL,
		Payload:           interactionData.URL,
		HistoryID:         &history.ID,
		InsertionPoint:    "sni",
		WorkspaceID:       &a.WorkspaceID,
	}
	db.Connection.CreateOOBTest(oobTest)

	log.Info().Str("url", a.HistoryItem.URL).Msg("SNI Injection audit completed")
}
