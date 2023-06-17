package active

import (
	"fmt"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
	"net/http"
	"sync"
)

// SNIAudit configuration
type SNIAudit struct {
	HistoryItem *db.History
	InteractionsManager        *integrations.InteractionsManager
}


// Run starts the audit
func (a *SNIAudit) Run() {
	auditLog := log.With().Str("audit", "httpMethods").Interface("auditItem", item).Str("url", a.HistoryItem.URL).Logger()

	interactionData := a.InteractionsManager.GetURL()
	transport := &http.Transport{
		DialContext: (&net.Dialer{}).DialContext,
		TLSClientConfig: &tls.Config{
			ServerName: interactionData.InteractionDomain,
		},
	}
	client := &http.Client{Transport: transport}
	request, err := http.NewRequest(item.method, a.HistoryItem.URL, nil)
	if err != nil {
		auditLog.Error().Err(err).Msg("Error creating the request")
		return
	}

	http_utils.SetRequestHeadersFromHistoryItem(request, a.HistoryItem)
	response, err := client.Do(request)

	if err != nil {
		auditLog.Error().Err(err).Msg("Error during request")
		return
	}
	history, err := http_utils.ReadHttpResponseAndCreateHistory(response, db.SourceScanner)
	if err != nil {
		auditLog.Error().Err(err).Msg("Error reading response")
	}
	issueTemplate := db.GetIssueTemplateByCode(db.SNIInjectionCode)
	oobTest := db.OOBTest{
		Code:              db.SNIInjectionCode,
		TestName:          issueTemplate.Title,
		InteractionDomain: interactionData.InteractionDomain,
		InteractionFullID: interactionData.InteractionFullID,
		Target:            a.URL,
		Payload:           interactionData.InteractionDomain,
		HistoryID:         history.ID,
		InsertionPoint:    "sni",
	}
	db.Connection.CreateOOBTest(oobTest)

	log.Info().Str("url", a.HistoryItem.URL).Msg("SNI Injection audit completed")
}
