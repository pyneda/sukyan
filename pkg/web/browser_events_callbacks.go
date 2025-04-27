package web

import (
	"fmt"

	"github.com/pyneda/sukyan/db"

	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
)

func GetBackgroundServiceCallbacks() (callbacks []interface{}) {
	callbacks = append(callbacks, func(e *proto.BackgroundServiceBackgroundServiceEventReceived) {
		log.Warn().Interface("event", e).Msg("Received background service event")
	})
	return callbacks
}

func GetIndexedDBCallbacks() (callbacks []interface{}) {
	callbacks = append(callbacks, func(e *proto.StorageIndexedDBContentUpdated) {
		log.Warn().Interface("event", e).Msg("Received StorageIndexedDBContentUpdated event")
	})
	callbacks = append(callbacks, func(e *proto.StorageCacheStorageListUpdated) {
		log.Warn().Interface("event", e).Msg("Received StorageCacheStorageListUpdated event")
	})
	callbacks = append(callbacks, func(e *proto.StorageCacheStorageListUpdated) {
		log.Warn().Interface("event", e).Msg("Received StorageCacheStorageListUpdated event")
	})
	callbacks = append(callbacks, func(e *proto.StorageIndexedDBContentUpdated) {
		log.Warn().Interface("event", e).Msg("Received StorageIndexedDBContentUpdated event")
	})
	callbacks = append(callbacks, func(e *proto.StorageIndexedDBListUpdated) {
		log.Warn().Interface("event", e).Msg("Received StorageIndexedDBListUpdated event")
	})
	return callbacks
}

func GetDOMStorageCallbacks() (callbacks []interface{}) {
	callbacks = append(callbacks, func(e *proto.DOMStorageDomStorageItemAdded) {
		log.Warn().Interface("dom_storage_item_added", e).Msg("Received a new DOMStorageDomStorageItemAdded event")
	})
	callbacks = append(callbacks, func(e *proto.DOMStorageDomStorageItemRemoved) {
		log.Warn().Interface("dom_storage_item_removed", e).Msg("Received a new DOMStorageDomStorageItemRemoved event")
	})
	callbacks = append(callbacks, func(e *proto.DOMStorageDomStorageItemsCleared) {
		log.Warn().Interface("dom_storage_items_cleared", e).Msg("Received a new DOMStorageDomStorageItemsCleared event")
	})
	callbacks = append(callbacks, func(e *proto.DOMStorageDomStorageItemUpdated) {
		log.Warn().Interface("dom_storage_item_updated", e).Msg("Received a new DOMStorageDomStorageItemUpdated event")
	})
	return callbacks
}

func GetBrowserDatabaseCallbacks() (callbacks []interface{}) {
	callbacks = append(callbacks, func(e *proto.DatabaseAddDatabase) {
		log.Warn().Interface("database", e).Msg("Client side database has been added")
		dbIssueDescription := fmt.Sprintf("Dynamic analisis has detected that a client side database with name %s and ID %s has been registered on domain %s. This is not an issue bug might require further investigation.", e.Database.Name, e.Database.ID, e.Database.Domain)
		dbAddedIssue := db.Issue{
			Code:        "client-db-added",
			URL:         "",
			Title:       "A client side database event has been triggered",
			Cwe:         1,
			StatusCode:  200,
			HTTPMethod:  "GET?",
			Description: dbIssueDescription,
			Payload:     "N/A",
			Confidence:  99,
			Severity:    "Info",
		}
		db.Connection().CreateIssue(dbAddedIssue)
	})
	return callbacks
}
