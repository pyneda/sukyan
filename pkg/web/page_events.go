package web

import (
	"fmt"
	"github.com/pyneda/sukyan/db"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
)

func ListenForPageEvents(url string, page *rod.Page) {

	go page.EachEvent(
		func(e *proto.PageJavascriptDialogOpening) (stop bool) {
			// Here we could make configurable if we want to accept or not the dialog
			// And could even allow to receive a callback function
			log.Warn().Interface("event", e).Msg("Received PageJavascriptDialogOpening event (alert, prompt, confirm)")
			page.Activate()
			err := proto.PageHandleJavaScriptDialog{
				Accept: true,
				// PromptText: "",
			}.Call(page)
			if err != nil {
				log.Error().Err(err).Msg("Could not handle javascript dialog")
			} else {
				log.Debug().Msg("Handled javascript dialog")
			}
			return true
		},
		func(e *proto.BackgroundServiceBackgroundServiceEventReceived) {
			log.Warn().Interface("event", e).Msg("Received background service event")
		},
		func(e *proto.StorageIndexedDBContentUpdated) {
			log.Warn().Interface("event", e).Msg("Received StorageIndexedDBContentUpdated event")
		},
		func(e *proto.StorageCacheStorageListUpdated) {
			log.Warn().Interface("event", e).Msg("Received StorageCacheStorageListUpdated event")
		},
		func(e *proto.StorageIndexedDBContentUpdated) {
			log.Warn().Interface("event", e).Msg("Received StorageIndexedDBContentUpdated event")
		},
		func(e *proto.StorageIndexedDBListUpdated) {
			log.Warn().Interface("event", e).Msg("Received StorageIndexedDBListUpdated event")
		},
		func(e *proto.DatabaseAddDatabase) {
			log.Warn().Interface("database", e).Msg("Client side database has been added")
			dbIssueDescription := fmt.Sprintf("Dynamic analisis has detected that a client side database with name %s and ID %s has been registered on domain %s. This is not an issue bug might require further investigation.", e.Database.Name, e.Database.ID, e.Database.Domain)
			dbAddedIssue := db.Issue{
				Code:        "client-db-added",
				URL:         url,
				Title:       "A client side database event has been triggered",
				Cwe:         1,
				StatusCode:  200,
				HTTPMethod:  "GET?",
				Description: dbIssueDescription,
				Payload:     "N/A",
				Confidence:  99,
				Severity:    "Info",
			}
			db.Connection.CreateIssue(dbAddedIssue)
		},
		// func(e *proto.DebuggerScriptParsed) {
		// 	log.Debug().Interface("parsed script data", e).Msg("Debugger script parsed")
		// },
		func(e *proto.AuditsIssueAdded) {
			// log.Warn().Interface("issue", e.Issue).Str("url", url).Msg("Received a new browser audits issue")
			handleBrowserAuditIssues(url, e)
		},
		func(e *proto.SecuritySecurityStateChanged) (stop bool) {
			if e.Summary == "all served securely" {
				log.Warn().Interface("state", e).Str("url", url).Msg("Received a new browser SecuritySecurityStateChanged event without issues")
				return true
			} else {
				log.Warn().Interface("state", e).Str("url", url).Msg("Received a new browser SecuritySecurityStateChanged event")
			}
			return false
		},
		// func(e *proto.SecurityHandleCertificateError) {
		// 	log.Warn().Interface("issue", e).Str("url", url).Msg("Received a new browser SecurityHandleCertificateError")
		// },
		func(e *proto.DOMStorageDomStorageItemAdded) {
			log.Warn().Interface("dom_storage_item_added", e).Str("url", url).Msg("Received a new DOMStorageDomStorageItemAdded event")
		},
		func(e *proto.DOMStorageDomStorageItemRemoved) {
			log.Warn().Interface("dom_storage_item_removed", e).Str("url", url).Msg("Received a new DOMStorageDomStorageItemRemoved event")
		},
		func(e *proto.DOMStorageDomStorageItemsCleared) {
			log.Warn().Interface("dom_storage_items_cleared", e).Str("url", url).Msg("Received a new DOMStorageDomStorageItemsCleared event")
		},
		func(e *proto.DOMStorageDomStorageItemUpdated) {
			log.Warn().Interface("dom_storage_item_updated", e).Str("url", url).Msg("Received a new DOMStorageDomStorageItemUpdated event")
		},
		func(e *proto.SecurityCertificateError) bool {
			// If IgnoreCertificateErrors are permanently added, this can be deleted
			log.Warn().Interface("issue", e).Str("url", url).Msg("Received a new browser SecurityCertificateError")

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
	// func(e *proto.NetworkAuthChallenge) {
	// Should probably listen to: proto.FetchAuthRequired
	// 	log.Warn().Str("source", string(e.Source)).Str("origin", e.Origin).Str("realm", e.Realm).Str("scheme", e.Scheme).Msg("Network auth challange received")
	// }
	)()
	ListenForWebSocketEvents(page)
}
