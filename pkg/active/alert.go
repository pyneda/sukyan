package active

import (
	"bufio"
	"context"
	"encoding/json"
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
	Ctx                        context.Context
	requests                   sync.Map
	WorkspaceID                uint
	TaskID                     uint
	TaskJobID                  uint
	ScanID                     uint
	ScanJobID                  uint
	SkipInitialAlertValidation bool
	detectedLocations          sync.Map
}

func (x *AlertAudit) requestHasAlert(history *db.History, browserPool *browser.BrowserPoolManager) bool {
	// Check parent context before acquiring browser
	if x.Ctx != nil {
		select {
		case <-x.Ctx.Done():
			return false
		default:
		}
	}

	b := browserPool.NewBrowser()
	page := b.MustPage("")
	defer browserPool.ReleaseBrowser(b)

	taskLog := log.With().Uint("history", history.ID).Str("method", history.Method).Str("task", "ensure no alert").Str("url", history.URL).Logger()
	hasAlert := false
	done := make(chan struct{})

	taskLog.Debug().Msg("Getting a browser page")
	web.IgnoreCertificateErrors(page)

	taskLog.Debug().Msg("Browser page gathered")

	// Use parent context with timeout, so cancellation propagates
	parentCtx := x.Ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	pageWithCancel := page.Context(ctx)
	defer pageWithCancel.Close()
	defer cancel()
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
		ScanID:              x.ScanID,
		ScanJobID:           x.ScanJobID,
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

	// Get context, defaulting to background if not provided
	ctx := x.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Check context before starting
	select {
	case <-ctx.Done():
		taskLog.Info().Msg("Alert audit cancelled before starting")
		return
	default:
	}

	p := pool.New().WithMaxGoroutines(3)
	browserPool := browser.GetScannerBrowserPoolManager()

	if x.requestHasAlert(history, browserPool) {
		taskLog.Warn().Msg("Skipping XSS tests as the original request triggers an alert dialog")
		return
	}

	for _, payload := range payloads {
		// Check context before each payload
		select {
		case <-ctx.Done():
			taskLog.Info().Msg("Alert audit cancelled during payload iteration")
			p.Wait()
			return
		default:
		}

		p.Go(func() {
			// Check context before testing payload
			select {
			case <-ctx.Done():
				return
			default:
			}
			value := payload.GetValue()
			x.testPayload(browserPool, history, insertionPoints, value, issueCode)
			taskLog.Debug().Str("payload", value).Msg("Finished testing payload")
		})
	}

	p.Wait()
	taskLog.Info().Msg("Completed tests")
}

// RunWithContextAwarePayloads runs the audit using context-aware payload selection
// based on reflection analysis. It selects payloads appropriate for each insertion point's
// reflection context (HTML, script, attribute, etc.) and character encoding behavior.
func (x *AlertAudit) RunWithContextAwarePayloads(history *db.History, insertionPoints []scan.InsertionPoint, issueCode db.IssueCode) {
	taskLog := log.With().Uint("history", history.ID).Str("method", history.Method).Str("url", history.URL).Str("audit", string(issueCode)).Logger()

	// Get context, defaulting to background if not provided
	ctx := x.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Check context before starting
	select {
	case <-ctx.Done():
		taskLog.Info().Msg("Alert audit cancelled before starting")
		return
	default:
	}

	p := pool.New().WithMaxGoroutines(3)
	browserPool := browser.GetScannerBrowserPoolManager()

	if !x.SkipInitialAlertValidation && x.requestHasAlert(history, browserPool) {
		taskLog.Warn().Msg("Skipping XSS tests as the original request triggers an alert dialog")
		return
	}

	var csp *http_utils.CSPPolicy
	if headers, err := history.GetResponseHeadersAsMap(); err == nil {
		csp = http_utils.ParseCSPFromHeaders(http.Header(headers))
		if csp != nil {
			taskLog.Debug().Bool("reportOnly", csp.ReportOnly).Msg("CSP policy detected")
		}
	}

	for _, insertionPoint := range insertionPoints {
		select {
		case <-ctx.Done():
			taskLog.Info().Msg("Alert audit cancelled during insertion point iteration")
			p.Wait()
			return
		default:
		}

		var contextPayloads []payloads.PayloadInterface
		if insertionPoint.Behaviour.ReflectionAnalysis != nil {
			cspResult := payloads.GetCSPAwarePayloadsWithDetails(insertionPoint.Behaviour.ReflectionAnalysis, csp)
			contextPayloads = cspResult.Payloads

			logEvent := taskLog.Info().
				Str("insertionPoint", insertionPoint.Name).
				Int("payloadCount", cspResult.FilteredCount).
				Bool("hasHTMLContext", insertionPoint.Behaviour.ReflectionAnalysis.HasHTMLContext).
				Bool("hasScriptContext", insertionPoint.Behaviour.ReflectionAnalysis.HasScriptContext).
				Bool("hasAttributeContext", insertionPoint.Behaviour.ReflectionAnalysis.HasAttributeContext).
				Bool("hasCSP", csp != nil)

			if csp != nil && cspResult.OriginalCount != cspResult.FilteredCount {
				logEvent = logEvent.
					Int("originalPayloads", cspResult.OriginalCount).
					Int("inlineScriptBlocked", cspResult.InlineScriptBlocked).
					Int("dataURIBlocked", cspResult.DataURIBlocked).
					Bool("cspBlocksInline", cspResult.BlocksInline).
					Bool("cspAllowsData", cspResult.AllowsData)
			}
			logEvent.Msg("Using context-aware payloads")
		} else {
			contextPayloads = payloads.GetXSSPayloads()
			taskLog.Debug().Str("insertionPoint", insertionPoint.Name).Msg("No reflection analysis, using all XSS payloads")
		}

		// Test each payload for this insertion point
		for _, payload := range contextPayloads {
			select {
			case <-ctx.Done():
				taskLog.Info().Msg("Alert audit cancelled during payload iteration")
				p.Wait()
				return
			default:
			}

			ip := insertionPoint // Capture for closure
			pl := payload        // Capture for closure

			p.Go(func() {
				select {
				case <-ctx.Done():
					return
				default:
				}
				value := pl.GetValue()
				x.testPayloadForSingleInsertionPoint(browserPool, history, ip, value, issueCode)
			})
		}
	}

	p.Wait()
	taskLog.Info().Msg("Completed context-aware XSS tests")
}

// testPayloadForSingleInsertionPoint tests a payload against a single insertion point
func (x *AlertAudit) testPayloadForSingleInsertionPoint(browserPool *browser.BrowserPoolManager, history *db.History, insertionPoint scan.InsertionPoint, payload string, issueCode db.IssueCode) {
	// Check parent context before doing work
	if x.Ctx != nil {
		select {
		case <-x.Ctx.Done():
			return
		default:
		}
	}

	if x.isDetectedLocation(history.URL, insertionPoint) {
		log.Debug().Str("url", history.URL).Interface("insertionPoint", insertionPoint.LogSummary()).Str("check", issueCode.String()).Msg("Skipping testing for alert in already detected location")
		return
	}

	b := browserPool.NewBrowser()
	log.Debug().Msg("Got scan browser from the pool")

	hijackResultsChannel := make(chan browser.HijackResult)
	// Use parent context for hijacking so it stops when scan is cancelled
	parentCtx := x.Ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	hijackContext, hijackCancel := context.WithCancel(parentCtx)
	browser.HijackWithContext(browser.HijackConfig{AnalyzeJs: false, AnalyzeHTML: false}, b, db.SourceScanner, hijackResultsChannel, hijackContext, x.WorkspaceID, x.TaskID, x.ScanID, x.ScanJobID)
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

	builders := []scan.InsertionPointBuilder{
		{
			Point:   insertionPoint,
			Payload: payload,
		},
	}
	request, err := scan.CreateRequestFromInsertionPoints(history, builders)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create request from insertion points")
		return
	}
	x.testRequest(request, insertionPoint, payload, b, issueCode)
	log.Debug().Str("payload", payload).Str("insertionPoint", insertionPoint.Name).Msg("Finished testing payload")
}

func (x *AlertAudit) testPayload(browserPool *browser.BrowserPoolManager, history *db.History, insertionPoints []scan.InsertionPoint, payload string, issueCode db.IssueCode) {
	// Check parent context before doing work
	if x.Ctx != nil {
		select {
		case <-x.Ctx.Done():
			return
		default:
		}
	}

	b := browserPool.NewBrowser()
	log.Debug().Msg("Got scan browser from the pool")

	hijackResultsChannel := make(chan browser.HijackResult)
	// Use parent context for hijacking so it stops when scan is cancelled
	parentCtx := x.Ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	hijackContext, hijackCancel := context.WithCancel(parentCtx)
	browser.HijackWithContext(browser.HijackConfig{AnalyzeJs: false, AnalyzeHTML: false}, b, db.SourceScanner, hijackResultsChannel, hijackContext, x.WorkspaceID, x.TaskID, x.ScanID, x.ScanJobID)
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

	// Get context, defaulting to background if not provided
	ctx := x.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Check context before starting
	select {
	case <-ctx.Done():
		taskLog.Info().Msg("Alert audit cancelled before starting")
		return
	default:
	}

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
		// Check context before each payload
		select {
		case <-ctx.Done():
			taskLog.Info().Msg("Alert audit cancelled during scan")
			p.Wait()
			return
		default:
		}

		payload := scanner.Text()
		p.Go(func() {
			// Check context before testing payload
			select {
			case <-ctx.Done():
				return
			default:
			}
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
		if x.isDetectedLocation(history.URL, insertionPoint) {
			log.Debug().Str("url", history.URL).Interface("insertionPoint", insertionPoint.LogSummary()).Str("check", issueCode.String()).Msg("Skipping testing for alert in already detected location")
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
	if !x.markDetectedIfNew(history.URL, insertionPoint) {
		log.Debug().Str("url", history.URL).Interface("insertionPoint", insertionPoint.LogSummary()).Msg("Skipping duplicate XSS report")
		return
	}

	log.Warn().Str("url", history.URL).Interface("insertionPoint", insertionPoint.LogSummary()).Str("payload", payload).Str("audit", string(issueCode)).Msg("Reflected XSS detected")
	testurl := scanRequest.URL.String()
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
	if err == nil && len(body) > 0 {
		sb.WriteString("\n\nThe request body:\n```\n" + string(body) + "\n```\n")
	}
	db.CreateIssueFromHistoryAndTemplate(history, issueCode, sb.String(), 90, "", &x.WorkspaceID, &x.TaskID, &x.TaskJobID, &x.ScanID, &x.ScanJobID)

	x.saveBrowserDialogEvent(history, e, insertionPoint, payload, issueCode)
}

func (x *AlertAudit) saveBrowserDialogEvent(history *db.History, e proto.PageJavascriptDialogOpening, insertionPoint scan.InsertionPoint, payload string, issueCode db.IssueCode) {
	eventData := map[string]interface{}{
		"dialog_type":         string(e.Type),
		"message":             e.Message,
		"default_prompt":      e.DefaultPrompt,
		"has_browser_handler": e.HasBrowserHandler,
		"insertion_point":     insertionPoint.Name,
		"insertion_type":      string(insertionPoint.Type),
		"payload":             payload,
		"issue_code":          string(issueCode),
	}

	dataJSON, err := json.Marshal(eventData)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal dialog event data")
		return
	}

	var historyID *uint
	if history != nil && history.ID > 0 {
		historyID = &history.ID
	}

	var scanID *uint
	if x.ScanID > 0 {
		scanID = &x.ScanID
	}
	var scanJobID *uint
	if x.ScanJobID > 0 {
		scanJobID = &x.ScanJobID
	}
	var taskID *uint
	if x.TaskID > 0 {
		taskID = &x.TaskID
	}

	browserEvent := &db.BrowserEvent{
		EventType:   db.BrowserEventDialog,
		Category:    db.BrowserEventCategoryRuntime,
		URL:         e.URL,
		Description: fmt.Sprintf("JavaScript %s dialog triggered", e.Type),
		Data:        dataJSON,
		WorkspaceID: x.WorkspaceID,
		ScanID:      scanID,
		ScanJobID:   scanJobID,
		HistoryID:   historyID,
		TaskID:      taskID,
		Source:      "xss_audit",
	}

	if err := db.Connection().SaveBrowserEvent(browserEvent); err != nil {
		log.Error().Err(err).Msg("Failed to save browser dialog event")
	}
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

	taskLog := log.With().Str("method", scanRequest.Method).Str("url", testurl).Interface("insertionPoint", insertionPoint.LogSummary()).Str("payload", payload).Str("audit", string(issueCode)).Logger()

	taskLog.Debug().Msg("Getting a browser page")
	page := b.MustPage("")
	web.IgnoreCertificateErrors(page)

	taskLog.Debug().Msg("Browser page gathered")

	alertOpenEventChan := make(chan *proto.PageJavascriptDialogOpening, 1)

	// Create context with 60s timeout, using parent context so cancellation propagates
	parentCtx := x.Ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(parentCtx, 60*time.Second)
	pageWithCancel := page.Context(ctx)
	defer pageWithCancel.Close()
	defer cancel()
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
		ScanID:              x.ScanID,
		ScanJobID:           x.ScanJobID,
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
	// Close done channel when context is cancelled to stop event triggering
	go func() {
		select {
		case <-ctx.Done():
			close(done)
		case <-done:
			// Already closed by someone else
		}
	}()

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

func (x *AlertAudit) buildDeduplicationKey(url string, insertionPoint scan.InsertionPoint) string {
	normalizedUrl, err := lib.NormalizeURL(url)
	if err != nil {
		normalizedUrl = url
	}
	return normalizedUrl + ":" + insertionPoint.String()
}

func (x *AlertAudit) markDetectedIfNew(url string, insertionPoint scan.InsertionPoint) bool {
	key := x.buildDeduplicationKey(url, insertionPoint)
	_, alreadyExists := x.detectedLocations.LoadOrStore(key, true)
	return !alreadyExists
}

func (x *AlertAudit) isDetectedLocation(url string, insertionPoint scan.InsertionPoint) bool {
	key := x.buildDeduplicationKey(url, insertionPoint)
	_, ok := x.detectedLocations.Load(key)
	return ok
}
