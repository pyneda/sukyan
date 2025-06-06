package active

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads"
	"github.com/pyneda/sukyan/pkg/scan"
	"github.com/pyneda/sukyan/pkg/web"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/browser"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

type AlertAudit struct {
	requests                   sync.Map
	WorkspaceID                uint
	TaskID                     uint
	TaskJobID                  uint
	SkipInitialAlertValidation bool
	detectedLocations          sync.Map
}

func (x *AlertAudit) requestHasAlert(history *db.History, browserPool *browser.BrowserPoolManager) bool {
	b := browserPool.NewBrowser()
	page := b.MustPage("")
	defer browserPool.ReleaseBrowser(b)

	taskLog := log.With().Uint("history", history.ID).Str("method", history.Method).Str("task", "ensure no alert").Str("url", history.URL).Logger()
	hasAlert := false
	done := make(chan struct{})

	taskLog.Debug().Msg("Getting a browser page")
	web.IgnoreCertificateErrors(page)

	taskLog.Debug().Msg("Browser page gathered")

	ctx, cancel := context.WithCancel(context.Background())
	pageWithCancel := page.Context(ctx)
	defer pageWithCancel.Close()
	go func() {
		time.Sleep(30 * time.Second)
		cancel()
	}()
	go pageWithCancel.EachEvent(
		func(e *proto.PageJavascriptDialogOpening) (stop bool) {
			hasAlert = true
			close(done)
			return true
		})()

	request, err := http_utils.BuildRequestFromHistoryItem(history)
	if err != nil {
		taskLog.Error().Err(err).Msg("Failed to create request from history item")
		return hasAlert
	}
	note := "This request has been replayed in browser without payloads to avoid FPs by ensuring that it does not trigger an alert dialog"

	_, navigationErr := browser.ReplayRequestInBrowserAndCreateHistory(browser.ReplayAndCreateHistoryOptions{
		Page:                pageWithCancel,
		Request:             request,
		RawURL:              "",
		WorkspaceID:         x.WorkspaceID,
		TaskID:              x.TaskID,
		PlaygroundSessionID: 0,
		Note:                note,
		Source:              db.SourceScanner,
	})
	if navigationErr != nil {
		taskLog.Error().Msg("Navigation error")
	}
	taskLog.Debug().Msg("Navigated to the page completed")
	loadError := pageWithCancel.WaitLoad()
	if loadError != nil {
		taskLog.Error().Err(loadError).Msg("Error waiting for page complete load")
	} else {
		taskLog.Debug().Msg("Page fully loaded on browser")
	}

	return hasAlert
}

// RunWithPayloads runs the audit using the given payloads
func (x *AlertAudit) RunWithPayloads(history *db.History, insertionPoints []scan.InsertionPoint, payloads []payloads.PayloadInterface, issueCode db.IssueCode) {
	taskLog := log.With().Uint("history", history.ID).Str("method", history.Method).Str("url", history.URL).Str("audit", string(issueCode)).Logger()

	p := pool.New().WithMaxGoroutines(3)
	browserPool := browser.GetScannerBrowserPoolManager()

	if x.requestHasAlert(history, browserPool) {
		taskLog.Warn().Msg("Skipping XSS tests as the original request triggers an alert dialog")
		return
	}

	for _, payload := range payloads {
		p.Go(func() {
			value := payload.GetValue()
			x.testPayload(browserPool, history, insertionPoints, value, issueCode)
			taskLog.Debug().Str("payload", value).Msg("Finished testing payload")
		})
	}

	p.Wait()
	taskLog.Info().Msg("Completed tests")
}

func (x *AlertAudit) testPayload(browserPool *browser.BrowserPoolManager, history *db.History, insertionPoints []scan.InsertionPoint, payload string, issueCode db.IssueCode) {
	b := browserPool.NewBrowser()
	log.Debug().Msg("Got scan browser from the pool")

	hijackResultsChannel := make(chan browser.HijackResult)
	hijackContext, hijackCancel := context.WithCancel(context.Background())
	browser.HijackWithContext(browser.HijackConfig{AnalyzeJs: false, AnalyzeHTML: false}, b, db.SourceScanner, hijackResultsChannel, hijackContext, x.WorkspaceID, x.TaskID)
	defer browserPool.ReleaseBrowser(b)
	defer hijackCancel()
	go func() {
		for {
			select {
			case hijackResult, ok := <-hijackResultsChannel:
				if !ok {
					return
				}

				x.requests.Store(hijackResult.History.URL, hijackResult.History)
			case <-hijackContext.Done():
				return
			}
		}
	}()
	x.testPayloadInInsertionPoint(history, insertionPoints, payload, b, issueCode)
	log.Debug().Msg("Scan browser released")
}

// Run runs the audit using the given filesytem path to a wordlist
func (x *AlertAudit) Run(history *db.History, insertionPoints []scan.InsertionPoint, wordlistPath string, issueCode db.IssueCode) {
	taskLog := log.With().Uint("history", history.ID).Str("method", history.Method).Str("url", history.URL).Str("audit", string(issueCode)).Logger()

	p := pool.New().WithMaxGoroutines(3)
	browserPool := browser.GetScannerBrowserPoolManager()

	if x.requestHasAlert(history, browserPool) {
		taskLog.Warn().Msg("Skipping XSS tests as the original request triggers an alert dialog")
		return
	}
	f, err := os.Open(wordlistPath)
	if err != nil {
		taskLog.Fatal().Err(err).Msg("Failed to open wordlist file")
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	taskLog.Info().Msg("Starting XSS tests")

	for scanner.Scan() {
		payload := scanner.Text()
		p.Go(func() {
			x.testPayload(browserPool, history, insertionPoints, payload, issueCode)
			taskLog.Debug().Str("payload", payload).Msg("Finished testing payload")
		})
	}

	p.Wait()
	if err := scanner.Err(); err != nil {
		taskLog.Error().Err(err).Msg("Error reading from scanner")
	}

	taskLog.Info().Msg("Completed XSS tests")

}

func (x *AlertAudit) testPayloadInInsertionPoint(history *db.History, insertionPoints []scan.InsertionPoint, payload string, b *rod.Browser, issueCode db.IssueCode) {
	for _, insertionPoint := range insertionPoints {
		if x.isDetecteLocation(history.URL, insertionPoint) {
			log.Debug().Str("url", history.URL).Interface("insertionPoint", insertionPoint).Str("check", issueCode.String()).Msg("Skipping testing for alert in already detected location")
			continue
		}
		builders := []scan.InsertionPointBuilder{
			{
				Point:   insertionPoint,
				Payload: payload,
			},
		}
		request, err := scan.CreateRequestFromInsertionPoints(history, builders)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create request from insertion points")
			continue
		}
		x.testRequest(request, insertionPoint, payload, b, issueCode)

	}
}

func (x *AlertAudit) reportIssue(history *db.History, scanRequest *http.Request, e proto.PageJavascriptDialogOpening, insertionPoint scan.InsertionPoint, payload string, issueCode db.IssueCode) {

	log.Warn().Str("url", history.URL).Interface("insertionPoint", insertionPoint).Str("payload", payload).Str("audit", string(issueCode)).Msg("Reflected XSS detected")
	testurl := scanRequest.URL.String()
	x.storeDetectedLocation(e.URL, insertionPoint)
	var sb strings.Builder

	sb.WriteString("A " + issueCode.Name() + " has been detected affecting the `" + insertionPoint.Name + "` " + string(insertionPoint.Type) + ". The POC submitted a " + history.Method + " request to the URL below and verified that an alert dialog of type " + string(e.Type) + " has been triggered.\n\n")

	if e.Message != "" {
		sb.WriteString("The alert contained the following text `" + e.Message + "`\n")
	}

	if e.URL != testurl {
		sb.WriteString("\nThe original request performed has been to the URL:\n" + testurl + "\n\n")
		sb.WriteString("The payload has probably been encoded by the browser and the alert has been triggered at URL:\n" + e.URL + "\n")
	} else {
		sb.WriteString("\nThe following URL can be used to reproduce the issue: " + testurl)
	}

	body, err := history.RequestBody()
	if err != nil && string(body) != "" {
		sb.WriteString("\n\nThe request body:\n```\n" + string(body) + "\n```\n")
	}
	db.CreateIssueFromHistoryAndTemplate(history, issueCode, sb.String(), 90, "", &x.WorkspaceID, &x.TaskID, &x.TaskJobID)
}

func (x *AlertAudit) testRequest(scanRequest *http.Request, insertionPoint scan.InsertionPoint, payload string, b *rod.Browser, issueCode db.IssueCode) error {

	var testurl string
	var err error
	if insertionPoint.Type == scan.InsertionPointTypeParameter {
		testurl, err = lib.BuildURLWithParam(scanRequest.URL.String(), insertionPoint.Name, payload, false)
		if err != nil {
			testurl = scanRequest.URL.String()
		}
	}

	if testurl == "" {
		testurl = scanRequest.URL.String()
	}

	taskLog := log.With().Str("method", scanRequest.Method).Str("url", testurl).Interface("insertionPoint", insertionPoint).Str("payload", payload).Str("audit", string(issueCode)).Logger()

	taskLog.Debug().Msg("Getting a browser page")
	page := b.MustPage("")
	web.IgnoreCertificateErrors(page)

	taskLog.Debug().Msg("Browser page gathered")

	alertOpenEventChan := make(chan *proto.PageJavascriptDialogOpening, 1)

	ctx, cancel := context.WithCancel(context.Background())
	pageWithCancel := page.Context(ctx)
	defer pageWithCancel.Close()
	go func() {
		time.Sleep(60 * time.Second)
		cancel()
	}()
	taskLog.Debug().Str("url", testurl).Msg("Navigating to the page")

	go func() {
		defer close(alertOpenEventChan)
		pageWithCancel.EachEvent(
			func(e *proto.PageJavascriptDialogOpening) (stop bool) {
				alertOpenEventChan <- e
				taskLog.Warn().Str("browser_url", e.URL).Str("type", string(e.Type)).Str("dialog_text", e.Message).Bool("has_browser_handler", e.HasBrowserHandler).Msg("Reflected XSS Verified")

				disableDialogErr := browser.CloseAllJSDialogs(pageWithCancel)
				if disableDialogErr != nil {
					taskLog.Error().Err(disableDialogErr).Msg("Error disabling javascript dialogs")
				}
				err := proto.PageHandleJavaScriptDialog{
					Accept: true,
					// PromptText: "",
				}.Call(pageWithCancel)
				if err != nil {
					taskLog.Error().Err(err).Msg("Error handling javascript dialog")
				} else {
					taskLog.Debug().Msg("PageHandleJavaScriptDialog succedded")
				}

				return true
			})()
	}()

	note := fmt.Sprintf(
		"Replaying request in browser to test for  %s\nInsertion point: %s\nType: %s\nOriginal data: %s\nCurrent value: %s\nPayload: %s",
		issueCode, insertionPoint.Name, insertionPoint.Type, insertionPoint.OriginalData, insertionPoint.Value, payload,
	)

	history, navigationErr := browser.ReplayRequestInBrowserAndCreateHistory(browser.ReplayAndCreateHistoryOptions{
		Page:                pageWithCancel,
		Request:             scanRequest,
		RawURL:              testurl,
		WorkspaceID:         x.WorkspaceID,
		TaskID:              x.TaskID,
		PlaygroundSessionID: 0,
		Note:                note,
		Source:              db.SourceScanner,
	})
	if navigationErr != nil {
		taskLog.Error().Str("url", testurl).Msg("Navigation error")
	}
	taskLog.Debug().Str("url", testurl).Msg("Navigated to the page completed")
	loadError := pageWithCancel.WaitLoad()
	if loadError != nil {
		taskLog.Error().Err(loadError).Msg("Error waiting for page complete load")
	} else {
		taskLog.Debug().Str("url", testurl).Msg("Page fully loaded on browser")
	}

	// select {
	// case alertOpenEvent, ok := <-alertOpenEventChan:
	// 	if !ok {
	// 		return fmt.Errorf("no events received before channel was closed")
	// 	}
	// 	x.reportIssue(history, scanRequest, *alertOpenEvent, insertionPoint, payload, issueCode)
	// 	return nil
	// case <-time.After(3 * time.Second):
	// 	return fmt.Errorf("operation timed out while waiting for events")

	// }
	select {
	case alertOpenEvent, ok := <-alertOpenEventChan:
		if !ok {
			return fmt.Errorf("no events received before channel was closed")
		}
		x.reportIssue(history, scanRequest, *alertOpenEvent, insertionPoint, payload, issueCode)
		return nil
	case <-time.After(500 * time.Millisecond):
	}

	done := make(chan struct{})
	eventTypes := browser.EventTypesForAlertPayload(payload)
	if eventTypes.HasEventTypesToCheck() {
		taskLog.Info().Str("url", testurl).Msg("No alert triggered, trying to trigger with mouse events")
		err = browser.TriggerMouseEvents(
			pageWithCancel,
			eventTypes,
			&browser.DefaultMovementOptions,
			done,
		)
		if err != nil {
			taskLog.Error().Err(err).Msg("Failed to trigger mouse events")
		} else {
			taskLog.Info().Str("url", testurl).Str("payload", payload).Msg("Mouse events triggering completed")
		}
	}

	select {
	case alertOpenEvent, ok := <-alertOpenEventChan:
		if !ok {
			return fmt.Errorf("no events received before channel was closed")
		}
		x.reportIssue(history, scanRequest, *alertOpenEvent, insertionPoint, payload, issueCode)
		return nil
	case <-time.After(3 * time.Second):
		return fmt.Errorf("operation timed out while waiting for events")
	}

}

func (x *AlertAudit) GetHistory(id string) *db.History {
	history, ok := x.requests.Load(id)
	if ok {
		return history.(*db.History)
	}
	return &db.History{}
}

func (x *AlertAudit) storeDetectedLocation(url string, insertionPoint scan.InsertionPoint) {
	normalizedUrl, err := lib.NormalizeURL(url)
	if err != nil {
		return
	}
	key := normalizedUrl + ":" + insertionPoint.String()
	x.detectedLocations.Store(key, true)
}

func (x *AlertAudit) isDetecteLocation(url string, insertionPoint scan.InsertionPoint) bool {
	normalizedUrl, err := lib.NormalizeURL(url)
	if err != nil {
		return false
	}
	key := normalizedUrl + ":" + insertionPoint.String()
	_, ok := x.detectedLocations.Load(key)
	return ok
}
