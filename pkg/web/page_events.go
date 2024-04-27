package web

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

type PageEventType string

type PageEvent struct {
	Type        PageEventType
	URL         string
	Description string
	Data        map[string]interface{}
	Issue       db.Issue
}

const (
	JavaScriptDialogOpening          PageEventType = "PageJavascriptDialogOpening"
	BackgroundServiceEventReceived   PageEventType = "BackgroundServiceBackgroundServiceEventReceived"
	StorageIndexedDBContentUpdated   PageEventType = "StorageIndexedDBContentUpdated"
	StorageCacheStorageListUpdated   PageEventType = "StorageCacheStorageListUpdated"
	StorageIndexedDBListUpdated      PageEventType = "StorageIndexedDBListUpdated"
	DatabaseAddDatabase              PageEventType = "DatabaseAddDatabase"
	DebuggerScriptParsed             PageEventType = "DebuggerScriptParsed"
	AuditsIssueAdded                 PageEventType = "AuditsIssueAdded"
	SecuritySecurityStateChanged     PageEventType = "SecuritySecurityStateChanged"
	SecurityHandleCertificateError   PageEventType = "SecurityHandleCertificateError"
	DOMStorageDomStorageItemAdded    PageEventType = "DOMStorageDomStorageItemAdded"
	DOMStorageDomStorageItemRemoved  PageEventType = "DOMStorageDomStorageItemRemoved"
	DOMStorageDomStorageItemsCleared PageEventType = "DOMStorageDomStorageItemsCleared"
	DOMStorageDomStorageItemUpdated  PageEventType = "DOMStorageDomStorageItemUpdated"
	SecurityCertificateError         PageEventType = "SecurityCertificateError"
	NetworkAuthChallenge             PageEventType = "NetworkAuthChallenge"
)

func ListenForPageEvents(ctx context.Context, url string, page *rod.Page, workspaceID, taskID uint, source string) <-chan PageEvent {
	eventChan := make(chan PageEvent)

	go func() {
		defer close(eventChan)

		page.EachEvent(
			func(e *proto.PageJavascriptDialogOpening) (stop bool) {
				pageEvent := PageEvent{
					Type:        JavaScriptDialogOpening,
					URL:         url,
					Description: "Dialog opened on the page",
					Data:        map[string]interface{}{"message": e.Message, "type": e.Type, "url": e.URL, "defaultPrompt": e.DefaultPrompt, "hasBrowserHanlder": e.HasBrowserHandler},
				}
				select {
				case eventChan <- pageEvent:
				case <-ctx.Done():
				}
				err := proto.PageHandleJavaScriptDialog{
					Accept:     true,
					PromptText: "",
				}.Call(page)
				if err != nil {
					log.Error().Err(err).Msg("Could not handle javascript dialog")
					// page.KeyActions().Press(input.Enter).Type(input.Enter).Do()
				} else {
					log.Debug().Msg("Handled javascript dialog")
				}
				return true
			},
			func(e *proto.BackgroundServiceBackgroundServiceEventReceived) {
				var sb strings.Builder
				sb.WriteString("Background service event received in page " + url + "\n\n")
				sb.WriteString("Event data:\n")
				sb.WriteString("Timestamp: " + e.BackgroundServiceEvent.Timestamp.String() + "\n")
				sb.WriteString("Event name: " + e.BackgroundServiceEvent.EventName + "\n")
				sb.WriteString("Origin: " + e.BackgroundServiceEvent.Origin + "\n")
				sb.WriteString("Service worker registration ID: " + string(e.BackgroundServiceEvent.ServiceWorkerRegistrationID) + "\n")
				sb.WriteString("Service: " + string(e.BackgroundServiceEvent.Service) + "\n")
				sb.WriteString("Instance ID: " + e.BackgroundServiceEvent.InstanceID + "\n")
				if len(e.BackgroundServiceEvent.EventMetadata) > 0 {
					sb.WriteString("Event metadata:\n")
					for _, metadata := range e.BackgroundServiceEvent.EventMetadata {
						sb.WriteString("  - " + metadata.Key + ": " + metadata.Value + "\n")
					}
				}
				pageEvent := PageEvent{
					Type:        BackgroundServiceEventReceived,
					URL:         url,
					Description: sb.String(),
					Data: map[string]interface{}{
						"timestamp":                   e.BackgroundServiceEvent.Timestamp,
						"eventName":                   e.BackgroundServiceEvent.EventName,
						"origin":                      e.BackgroundServiceEvent.Origin,
						"serviceWorkerRegistrationID": e.BackgroundServiceEvent.ServiceWorkerRegistrationID,
						"service":                     e.BackgroundServiceEvent.Service,
						"instanceID":                  e.BackgroundServiceEvent.InstanceID,
						"eventMetadata":               e.BackgroundServiceEvent.EventMetadata,
						"storageKey":                  e.BackgroundServiceEvent.StorageKey,
					},
				}
				select {
				case eventChan <- pageEvent:
				case <-ctx.Done():
				}
			},
			func(e *proto.StorageIndexedDBContentUpdated) {
				var sb strings.Builder
				sb.WriteString("IndexedDB content updated in page " + url + "\n\n")
				sb.WriteString("Database name: " + e.DatabaseName + "\n")
				sb.WriteString("Object store name: " + e.ObjectStoreName + "\n")
				sb.WriteString("Storage key: " + e.StorageKey + "\n")
				sb.WriteString("Origin: " + e.Origin + "\n")

				pageEvent := PageEvent{
					Type:        StorageIndexedDBContentUpdated,
					URL:         url,
					Description: sb.String(),
					Data:        map[string]interface{}{"databaseName": e.DatabaseName, "objectStoreName": e.ObjectStoreName, "storageKey": e.StorageKey, "origin": e.Origin},
				}
				select {
				case eventChan <- pageEvent:
				case <-ctx.Done():
				}
			},
			func(e *proto.StorageCacheStorageListUpdated) {
				var sb strings.Builder
				sb.WriteString("Cache storage list updated in page " + url + "\n\n")
				sb.WriteString("Origin: " + e.Origin + "\n")
				sb.WriteString("Storage key: " + e.StorageKey + "\n")
				pageEvent := PageEvent{
					Type:        StorageCacheStorageListUpdated,
					URL:         url,
					Description: sb.String(),
					Data:        map[string]interface{}{"origin": e.Origin, "storageKey": e.StorageKey},
				}
				select {
				case eventChan <- pageEvent:
				case <-ctx.Done():
				}
			},
			func(e *proto.StorageIndexedDBListUpdated) {
				var sb strings.Builder
				sb.WriteString("IndexedDB list updated in page " + url + "\n\n")
				sb.WriteString("Origin: " + e.Origin + "\n")
				sb.WriteString("Storage key: " + e.StorageKey + "\n")
				pageEvent := PageEvent{
					Type:        StorageIndexedDBListUpdated,
					URL:         url,
					Description: sb.String(),
					Data:        map[string]interface{}{"origin": e.Origin, "storageKey": e.StorageKey},
				}
				select {
				case eventChan <- pageEvent:
				case <-ctx.Done():
				}
			},
			func(e *proto.DatabaseAddDatabase) {
				var sb strings.Builder
				sb.WriteString("Database added in page " + url + "\n\n")
				sb.WriteString("Database name: " + e.Database.Name + "\n")
				sb.WriteString("Database ID: " + string(e.Database.ID) + "\n")
				sb.WriteString("Database domain: " + e.Database.Domain + "\n")
				sb.WriteString("Database version: " + e.Database.Version + "\n")

				pageEvent := PageEvent{
					Type:        DatabaseAddDatabase,
					URL:         url,
					Description: sb.String(),
					Data:        map[string]interface{}{"databaseName": e.Database.Name, "databaseId": e.Database.ID, "databaseDomain": e.Database.Domain, "databaseVersion": e.Database.Version},
				}
				select {
				case eventChan <- pageEvent:
				case <-ctx.Done():
				}
			},
			func(e *proto.AuditsIssueAdded) {
				issue := handleBrowserAuditIssues(url, e, workspaceID, taskID)
				if issue.ID != 0 {
					pageEvent := PageEvent{
						Type:        AuditsIssueAdded,
						URL:         url,
						Issue:       issue,
						Description: "Security issue added",
						Data:        map[string]interface{}{"auditIssue": e.Issue},
					}
					select {
					case eventChan <- pageEvent:
					case <-ctx.Done():
					}
				}
			},
			func(e *proto.SecuritySecurityStateChanged) {
				var sb strings.Builder
				sb.WriteString("Security state changed in page " + url + "\n\n")
				sb.WriteString("State: " + string(e.SecurityState) + "\n")
				sb.WriteString("Summary: " + e.Summary + "\n")
				sb.WriteString("Ran mixed content: " + fmt.Sprint(e.InsecureContentStatus.RanMixedContent) + "\n")
				sb.WriteString("Displayed mixed content: " + fmt.Sprint(e.InsecureContentStatus.DisplayedMixedContent) + "\n")
				sb.WriteString("Contained mixed form: " + fmt.Sprint(e.InsecureContentStatus.ContainedMixedForm) + "\n")
				sb.WriteString("Ran content with cert errors: " + fmt.Sprint(e.InsecureContentStatus.RanContentWithCertErrors) + "\n")
				sb.WriteString("Displayed content with cert errors: " + fmt.Sprint(e.InsecureContentStatus.DisplayedContentWithCertErrors) + "\n")
				sb.WriteString("Ran insecure content style: " + fmt.Sprint(e.InsecureContentStatus.RanInsecureContentStyle) + "\n")
				sb.WriteString("Displayed insecure content style: " + fmt.Sprint(e.InsecureContentStatus.DisplayedInsecureContentStyle) + "\n")

				pageEvent := PageEvent{
					Type:        SecuritySecurityStateChanged,
					URL:         url,
					Description: sb.String(),
					Data: map[string]interface{}{
						"state":                          e.SecurityState,
						"summary":                        e.Summary,
						"ranMixedContent":                e.InsecureContentStatus.RanMixedContent,
						"displayedMixedContent":          e.InsecureContentStatus.DisplayedMixedContent,
						"containedMixedForm":             e.InsecureContentStatus.ContainedMixedForm,
						"ranContentWithCertErrors":       e.InsecureContentStatus.RanContentWithCertErrors,
						"displayedContentWithCertErrors": e.InsecureContentStatus.DisplayedContentWithCertErrors,
						"ranInsecureContentStyle":        e.InsecureContentStatus.RanInsecureContentStyle,
						"displayedInsecureContentStyle":  e.InsecureContentStatus.DisplayedInsecureContentStyle,
					},
				}
				select {
				case eventChan <- pageEvent:
				case <-ctx.Done():
				}
			},
			func(e *proto.DOMStorageDomStorageItemAdded) {
				var sb strings.Builder
				sb.WriteString("DOM storage item added in page " + url + "\n\n")
				sb.WriteString("Key: " + e.Key + "\n")
				sb.WriteString("New value: " + e.NewValue + "\n")
				sb.WriteString("Security origin: " + e.StorageID.SecurityOrigin + "\n")
				sb.WriteString("Is local storage: " + fmt.Sprint(e.StorageID.IsLocalStorage) + "\n")
				sb.WriteString("Storage key: " + string(e.StorageID.StorageKey) + "\n")
				pageEvent := PageEvent{
					Type:        DOMStorageDomStorageItemAdded,
					URL:         url,
					Description: sb.String(),
					Data: map[string]interface{}{
						"key":            e.Key,
						"newValue":       e.NewValue,
						"securityOrigin": e.StorageID.SecurityOrigin,
						"isLocalStorage": e.StorageID.IsLocalStorage,
						"storageKey":     e.StorageID.StorageKey,
					},
				}
				select {
				case eventChan <- pageEvent:
				case <-ctx.Done():
				}
			},
			func(e *proto.DOMStorageDomStorageItemRemoved) {
				var sb strings.Builder
				sb.WriteString("DOM storage item removed in page " + url + "\n\n")
				sb.WriteString("Key: " + e.Key + "\n")
				sb.WriteString("Security origin: " + e.StorageID.SecurityOrigin + "\n")
				sb.WriteString("Is local storage: " + fmt.Sprint(e.StorageID.IsLocalStorage) + "\n")
				sb.WriteString("Storage key: " + string(e.StorageID.StorageKey) + "\n")
				pageEvent := PageEvent{
					Type:        DOMStorageDomStorageItemRemoved,
					URL:         url,
					Description: sb.String(),
					Data: map[string]interface{}{
						"key":            e.Key,
						"securityOrigin": e.StorageID.SecurityOrigin,
						"isLocalStorage": e.StorageID.IsLocalStorage,
						"storageKey":     e.StorageID.StorageKey,
					},
				}
				select {
				case eventChan <- pageEvent:
				case <-ctx.Done():
				}
			},
			func(e *proto.DOMStorageDomStorageItemsCleared) {
				var sb strings.Builder
				sb.WriteString("DOM storage items cleared in page " + url + "\n\n")
				sb.WriteString("Security origin: " + e.StorageID.SecurityOrigin + "\n")
				sb.WriteString("Is local storage: " + fmt.Sprint(e.StorageID.IsLocalStorage) + "\n")
				sb.WriteString("Storage key: " + string(e.StorageID.StorageKey) + "\n")
				pageEvent := PageEvent{
					Type:        DOMStorageDomStorageItemsCleared,
					URL:         url,
					Description: "DOM Storage items cleared",
					Data: map[string]interface{}{
						"securityOrigin": e.StorageID.SecurityOrigin,
						"isLocalStorage": e.StorageID.IsLocalStorage,
						"storageKey":     e.StorageID.StorageKey,
					},
				}
				select {
				case eventChan <- pageEvent:
				case <-ctx.Done():
				}
			},
			func(e *proto.DOMStorageDomStorageItemUpdated) {
				var sb strings.Builder
				sb.WriteString("DOM storage item updated in page " + url + "\n\n")
				sb.WriteString("Key: " + e.Key + "\n")
				sb.WriteString("New value: " + e.NewValue + "\n")
				sb.WriteString("Old value: " + e.OldValue + "\n")
				sb.WriteString("Security origin: " + e.StorageID.SecurityOrigin + "\n")
				sb.WriteString("Is local storage: " + fmt.Sprint(e.StorageID.IsLocalStorage) + "\n")
				sb.WriteString("Storage key: " + string(e.StorageID.StorageKey) + "\n")
				pageEvent := PageEvent{
					Type:        DOMStorageDomStorageItemUpdated,
					URL:         url,
					Description: sb.String(),
					Data: map[string]interface{}{
						"key":            e.Key,
						"newValue":       e.NewValue,
						"oldValue":       e.OldValue,
						"securityOrigin": e.StorageID.SecurityOrigin,
						"isLocalStorage": e.StorageID.IsLocalStorage,
						"storageKey":     e.StorageID.StorageKey,
					},
				}
				select {
				case eventChan <- pageEvent:
				case <-ctx.Done():
				}
			},
			func(e *proto.SecurityCertificateError) bool {
				var sb strings.Builder
				sb.WriteString("Security certificate error of type " + e.ErrorType + " has been received")
				pageEvent := PageEvent{
					Type:        SecurityCertificateError,
					URL:         url,
					Description: "A security certificate error has been received",
					Data: map[string]interface{}{
						"errorType": e.ErrorType,
						"eventID":   e.EventID,
						"url":       e.RequestURL,
					},
				}
				select {
				case eventChan <- pageEvent:
				case <-ctx.Done():
				}
				err := proto.SecurityHandleCertificateError{
					EventID: e.EventID,
					Action:  proto.SecurityCertificateErrorActionContinue,
				}.Call(page)
				if err != nil {
					log.Error().Err(err).Msg("Could not handle security certificate error")
				} else {
					log.Debug().Msg("Handled security certificate error")
				}
				return true
			},
		)()
		ListenForWebSocketEvents(page, workspaceID, taskID, source)
		<-ctx.Done()
	}()

	return eventChan
}
