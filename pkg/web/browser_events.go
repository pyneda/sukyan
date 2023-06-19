package web

import (
	"encoding/json"
	"github.com/pyneda/sukyan/db"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
)

type BrowserEventsHandler struct {
	ListenBackgroundServiceEvents bool
	ListenIndexedDBEvents         bool
	ListenDOMStorageEvents        bool
}

func (h *BrowserEventsHandler) GetCallbacks() (callbacks []interface{}) {
	if h.ListenBackgroundServiceEvents {
		callbacks = append(callbacks, GetBackgroundServiceCallbacks()...)
	}
	if h.ListenIndexedDBEvents {
		callbacks = append(callbacks, GetIndexedDBCallbacks()...)
	}
	if h.ListenDOMStorageEvents {
		callbacks = append(callbacks, GetDOMStorageCallbacks()...)
	}
	return callbacks
}

func (h *BrowserEventsHandler) RunOnBrowser(browser *rod.Browser) {
	go browser.EachEvent(
		h.GetCallbacks(),
	)
}

func RunOnPage(page *rod.Page) {

	go page.EachEvent(

		func(e *proto.DebuggerScriptParsed) {
			log.Debug().Interface("parsed script data", e).Msg("Debugger script parsed")
		},
		func(e *proto.AuditsIssueAdded) {
			log.Warn().Interface("issue", e.Issue).Msg("Received a new browser audits issue")
			jsonDetails, err := json.Marshal(e.Issue.Details)
			if err != nil {
				log.Error().Err(err).Msg("Could not convert browser audit issue event details to JSON")
			}

			browserAuditIssue := db.Issue{
				Code:           "browser-audit-" + string(e.Issue.Code),
				URL:            "url",
				Title:          "Browser audit issue (classification needed)",
				Cwe:            1,
				StatusCode:     200,
				HTTPMethod:     "GET?",
				Description:    string(jsonDetails),
				Payload:        "N/A",
				Confidence:     80,
				AdditionalInfo: jsonDetails,
				Severity:       "Info",
			}
			db.Connection.CreateIssue(browserAuditIssue)
			// }

		},
		func(e *proto.SecuritySecurityStateChanged) (stop bool) {
			if e.Summary == "all served securely" {
				log.Warn().Interface("state", e).Str("url", "url").Msg("Received a new browser SecuritySecurityStateChanged event without issues")
				return true
			} else {
				log.Warn().Interface("state", e).Str("url", "url").Msg("Received a new browser SecuritySecurityStateChanged event")
			}
			return false
		},
		// func(e *proto.SecurityHandleCertificateError) {
		// 	log.Warn().Interface("issue", e).Str("url", url).Msg("Received a new browser SecurityHandleCertificateError")
		// },

		func(e *proto.SecurityCertificateError) bool {
			// If IgnoreCertificateErrors are permanently added, this can be deleted
			log.Warn().Interface("issue", e).Str("url", "url").Msg("Received a new browser SecurityCertificateError")

			err := proto.SecurityHandleCertificateError{
				EventID: e.EventID,
				Action:  proto.SecurityCertificateErrorActionContinue,
			}.Call(page)
			if err != nil {
				log.Error().Err(err).Msg("Could not handle security certificate error")
			} else {
				log.Debug().Msg("Handled security certificate error")
			}

			// certificate, err := proto.NetworkGetCertificate{}.Call(page)
			// if err != nil {
			// 	log.Warn().Str("url", url).Msg("Error getting certificate data")
			// } else {
			// 	log.Info().Msgf("Certificate data gathered: %s", certificate)
			// }
			return true

		},
	)()

}
