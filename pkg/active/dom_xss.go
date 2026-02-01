package active

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/browser"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/passive"
	"github.com/pyneda/sukyan/pkg/payloads"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/pyneda/sukyan/pkg/web"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// CrawlEventContext contains browser event data from crawl phase for optimization
type CrawlEventContext struct {
	ScanID                  uint
	HasLocalStorageEvents   bool
	HasSessionStorageEvents bool
	LocalStorageKeys        []string
	SessionStorageKeys      []string
	Loaded                  bool // True if data was successfully loaded from database
}

// DOMXSSAudit performs DOM-based XSS vulnerability detection
type DOMXSSAudit struct {
	Options           ActiveModuleOptions
	HistoryItem       *db.History
	detectedSources   sync.Map              // Track detected sources to avoid duplicate testing/reporting
	SkipPreFiltering  bool                  // Skip static pre-filtering
	csp               *http_utils.CSPPolicy // Parsed CSP policy for payload filtering
	CrawlEventContext *CrawlEventContext    // Optional crawl phase data for optimization
}

// DOMXSSDetection represents a detected DOM XSS vulnerability
type DOMXSSDetection struct {
	Source       web.DOMXSSSource
	Sink         string
	Payload      payloads.DOMXSSPayload
	TestURL      string
	Confidence   int
	AlertMessage string
	TaintFlow    bool // Was this detected via taint tracking vs alert
}

// Run executes the DOM XSS audit
func (a *DOMXSSAudit) Run() {
	ctx := a.Options.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	taskLog := log.With().
		Uint("history", a.HistoryItem.ID).
		Str("url", a.HistoryItem.URL).
		Str("audit", "dom-xss").
		Logger()

	select {
	case <-ctx.Done():
		taskLog.Info().Msg("DOM XSS audit cancelled before starting")
		return
	default:
	}

	taskLog.Info().Msg("Starting DOM XSS audit")

	// Load crawl event context for optimization (if in orchestrator mode with scan ID)
	if a.Options.ScanID > 0 && a.CrawlEventContext == nil {
		a.CrawlEventContext = a.LoadCrawlEventContext()
	}

	if !a.SkipPreFiltering && viper.GetBool("scan.dom_xss.skip_if_no_patterns") {
		if !a.hasSourceSinkPatterns() {
			taskLog.Debug().Msg("No DOM XSS source/sink patterns detected, skipping audit")
			return
		}
	}

	if viper.GetBool("scan.dom_xss.csp_aware") {
		if headers, err := a.HistoryItem.GetResponseHeadersAsMap(); err == nil {
			a.csp = http_utils.ParseCSPFromHeaders(http.Header(headers))
			if a.csp != nil {
				taskLog.Debug().
					Bool("reportOnly", a.csp.ReportOnly).
					Bool("blocksInline", a.csp.BlocksInlineScripts()).
					Bool("blocksEval", a.csp.BlocksEval()).
					Msg("CSP policy detected for DOM XSS filtering")
			}
		}
	}

	browserPool := browser.GetScannerBrowserPoolManager()
	b := browserPool.NewBrowser()
	defer browserPool.ReleaseBrowser(b)

	overallTimeout := time.Duration(viper.GetInt("scan.dom_xss.total_timeout")) * time.Second
	if overallTimeout == 0 {
		overallTimeout = 2 * time.Minute
	}
	overallCtx, overallCancel := context.WithTimeout(ctx, overallTimeout)
	defer overallCancel()

	incognito, err := b.Incognito()
	if err != nil {
		taskLog.Warn().Err(err).Msg("Failed to create incognito browser context")
		return
	}
	defer incognito.Close()

	for _, source := range web.GetURLBasedSources() {
		select {
		case <-overallCtx.Done():
			taskLog.Info().Msg("DOM XSS audit timeout reached")
			return
		default:
		}
		a.testURLSource(overallCtx, incognito, source)
	}

	// Test storage sources with crawl-informed optimization
	if a.shouldTestStorageSources() {
		for _, source := range web.GetStorageSources() {
			select {
			case <-overallCtx.Done():
				taskLog.Info().Msg("DOM XSS audit timeout reached")
				return
			default:
			}
			// Skip specific storage source if no crawl evidence for it
			if !a.shouldTestSpecificStorageSource(source) {
				continue
			}
			a.testStorageSource(overallCtx, incognito, source)
		}
	} else {
		taskLog.Debug().Msg("Skipping all storage sources - no storage events detected during crawl")
	}

	for _, source := range web.DOMXSSSources() {
		if source.Type == web.SourceTypeDocument || source.Type == web.SourceTypeWindow {
			select {
			case <-overallCtx.Done():
				taskLog.Info().Msg("DOM XSS audit timeout reached")
				return
			default:
			}
			a.testOtherSource(overallCtx, incognito, source)
		}
	}

	if viper.GetBool("scan.dom_xss.test_postmessage") {
		select {
		case <-overallCtx.Done():
			taskLog.Info().Msg("DOM XSS audit timeout reached")
			return
		default:
		}
		for _, source := range web.GetMessageSources() {
			a.testPostMessageSource(overallCtx, incognito, source)
		}
	}

	taskLog.Info().Msg("DOM XSS audit completed")
}

// getPayloads returns DOM XSS payloads, filtered by CSP if enabled
func (a *DOMXSSAudit) getPayloads() []payloads.DOMXSSPayload {
	if a.csp == nil {
		return payloads.GetDOMXSSPayloads()
	}

	result := payloads.GetCSPAwareDOMXSSPayloads(a.csp)
	if result.OriginalCount != result.FilteredCount {
		result.LogCSPFilterStats(a.HistoryItem.URL)
	}
	return result.Payloads
}

func (a *DOMXSSAudit) testURLSource(ctx context.Context, browser *rod.Browser, source web.DOMXSSSource) {
	taskLog := log.With().
		Str("source", source.Name).
		Str("url", a.HistoryItem.URL).
		Logger()

	if a.isDetectedSource(a.HistoryItem.URL, source) {
		taskLog.Debug().Msg("Skipping already detected source")
		return
	}

	sourceTimeout := time.Duration(viper.GetInt("scan.dom_xss.source_timeout")) * time.Second
	if sourceTimeout == 0 {
		sourceTimeout = 30 * time.Second
	}

	payloads := a.getPayloads()

	if source.Name == "location.search" {
		a.testLocationSearchSource(ctx, browser, source, payloads, sourceTimeout)
		return
	}

	for _, payload := range payloads {
		select {
		case <-ctx.Done():
			return
		default:
		}

		testURL, err := web.InjectPayloadIntoURL(a.HistoryItem.URL, source, payload.Value)
		if err != nil {
			taskLog.Debug().Err(err).Str("payload", payload.Value).Msg("Failed to inject payload into URL")
			continue
		}

		sourceCtx, cancel := context.WithTimeout(ctx, sourceTimeout)

		detection := a.testPayloadInBrowser(sourceCtx, browser, source, payload, testURL)
		cancel()

		if detection != nil {
			a.reportDOMXSS(detection)
			return
		}
	}
}

func (a *DOMXSSAudit) testLocationSearchSource(ctx context.Context, rodBrowser *rod.Browser, source web.DOMXSSSource, plds []payloads.DOMXSSPayload, sourceTimeout time.Duration) {
	taskLog := log.With().
		Str("source", source.Name).
		Str("url", a.HistoryItem.URL).
		Logger()

	paramsToTest := a.getQueryParamsToTest(ctx, rodBrowser)
	taskLog.Debug().
		Int("paramCount", len(paramsToTest)).
		Strs("params", paramsToTest).
		Msg("Testing query parameters for location.search source")

	if len(paramsToTest) == 0 {
		taskLog.Debug().Msg("No query parameters to test for location.search")
		return
	}

	for _, paramName := range paramsToTest {
		select {
		case <-ctx.Done():
			return
		default:
		}

		for _, payload := range plds {
			select {
			case <-ctx.Done():
				return
			default:
			}

			testURL := buildURLWithParam(a.HistoryItem.URL, paramName, payload.Value)

			sourceCtx, cancel := context.WithTimeout(ctx, sourceTimeout)
			detection := a.testPayloadInBrowser(sourceCtx, rodBrowser, source, payload, testURL)
			cancel()

			if detection != nil {
				a.reportDOMXSS(detection)
				taskLog.Info().
					Str("testURL", testURL).
					Str("param", paramName).
					Msg("DOM XSS detected via location.search")
				return
			}
		}
	}
}

func (a *DOMXSSAudit) testStorageSource(ctx context.Context, rodBrowser *rod.Browser, source web.DOMXSSSource) {
	taskLog := log.With().
		Str("source", source.Name).
		Str("url", a.HistoryItem.URL).
		Logger()

	sourceTimeout := time.Duration(viper.GetInt("scan.dom_xss.source_timeout")) * time.Second
	if sourceTimeout == 0 {
		sourceTimeout = 30 * time.Second
	}

	payloads := a.getPayloads()
	storageKeys := a.getStorageKeysToTest(ctx, rodBrowser, source)
	taskLog.Debug().Int("keyCount", len(storageKeys)).Strs("keys", storageKeys).Msg("Testing storage keys")

	for _, storageKey := range storageKeys {
		if a.isDetectedStorageSource(a.HistoryItem.URL, source, storageKey) {
			continue
		}

		for _, payload := range payloads {
			select {
			case <-ctx.Done():
				return
			default:
			}

			sourceCtx, cancel := context.WithTimeout(ctx, sourceTimeout)

			detection := a.testStoragePayloadInBrowser(sourceCtx, rodBrowser, source, payload, storageKey)
			cancel()

			if detection != nil {
				a.markDetectedStorageIfNew(a.HistoryItem.URL, source, storageKey)
				a.reportDOMXSS(detection)
				taskLog.Info().Str("key", storageKey).Msg("DOM XSS detected via storage source")
				return
			}
		}
	}
}

func (a *DOMXSSAudit) testOtherSource(ctx context.Context, browser *rod.Browser, source web.DOMXSSSource) {
	taskLog := log.With().
		Str("source", source.Name).
		Str("url", a.HistoryItem.URL).
		Logger()

	if a.isDetectedSource(a.HistoryItem.URL, source) {
		taskLog.Debug().Msg("Skipping already detected source")
		return
	}

	sourceTimeout := time.Duration(viper.GetInt("scan.dom_xss.source_timeout")) * time.Second
	if sourceTimeout == 0 {
		sourceTimeout = 30 * time.Second
	}

	payloads := a.getPayloads()

	for _, payload := range payloads {
		select {
		case <-ctx.Done():
			return
		default:
		}

		sourceCtx, cancel := context.WithTimeout(ctx, sourceTimeout)

		var detection *DOMXSSDetection
		switch source.Name {
		case "window.name":
			detection = a.testWindowNamePayload(sourceCtx, browser, source, payload)
		case "document.referrer":
			// Requires navigation from another page with crafted Referer header
			taskLog.Debug().Msg("Skipping document.referrer - requires referrer injection")
		case "document.cookie":
			detection = a.testCookiePayload(sourceCtx, browser, source, payload)
		}
		cancel()

		if detection != nil {
			a.reportDOMXSS(detection)
			return
		}
	}
}

func (a *DOMXSSAudit) testPostMessageSource(ctx context.Context, rodBrowser *rod.Browser, source web.DOMXSSSource) {
	taskLog := log.With().
		Str("source", source.Name).
		Str("url", a.HistoryItem.URL).
		Logger()

	if a.isDetectedSource(a.HistoryItem.URL, source) {
		taskLog.Debug().Msg("Skipping already detected source")
		return
	}

	sourceTimeout := time.Duration(viper.GetInt("scan.dom_xss.source_timeout")) * time.Second
	if sourceTimeout == 0 {
		sourceTimeout = 30 * time.Second
	}

	payloads := a.getPayloads()

	for _, payload := range payloads {
		select {
		case <-ctx.Done():
			return
		default:
		}

		sourceCtx, cancel := context.WithTimeout(ctx, sourceTimeout)

		detection := a.testPostMessagePayloadInBrowser(sourceCtx, rodBrowser, source, payload)
		cancel()

		if detection != nil {
			a.reportDOMXSS(detection)
			taskLog.Info().Msg("DOM XSS detected via postMessage source")
			return
		}
	}
}

func (a *DOMXSSAudit) testPostMessagePayloadInBrowser(ctx context.Context, rodBrowser *rod.Browser, source web.DOMXSSSource, payload payloads.DOMXSSPayload) *DOMXSSDetection {
	taskLog := log.With().
		Str("source", source.Name).
		Str("marker", payload.Marker).
		Logger()

	page, err := rodBrowser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		taskLog.Debug().Err(err).Msg("Failed to create browser page")
		return nil
	}
	defer page.Close()

	web.IgnoreCertificateErrors(page)
	pageWithCtx := page.Context(ctx)

	alertChan := make(chan *proto.PageJavascriptDialogOpening, 1)
	taintChan := make(chan string, 10)

	go pageWithCtx.EachEvent(
		func(e *proto.PageJavascriptDialogOpening) (stop bool) {
			if payloads.ContainsMarker(e.Message, payload.Marker) {
				select {
				case alertChan <- e:
				default:
				}
			}
			proto.PageHandleJavaScriptDialog{Accept: true}.Call(pageWithCtx)
			return true
		},
		func(e *proto.RuntimeConsoleAPICalled) {
			for _, arg := range e.Args {
				valStr := arg.Value.String()
				if payloads.ContainsMarker(valStr, payload.Marker) || payloads.ContainsTaintMarker(valStr) {
					select {
					case taintChan <- valStr:
					default:
					}
				}
			}
		},
	)()

	err = pageWithCtx.Navigate(a.HistoryItem.URL)
	if err != nil {
		taskLog.Debug().Err(err).Msg("Navigation error")
		return nil
	}
	pageWithCtx.WaitLoad()

	a.injectTaintTracking(pageWithCtx, payload.Marker)
	time.Sleep(300 * time.Millisecond)

	// Send postMessage with payload in different formats (string, object, JSON)
	escapedPayload := web.EscapeJSString(payload.Value)

	postMessageScript := fmt.Sprintf(`
		try {
			window.postMessage('%s', '*');
		} catch(e) {}
	`, escapedPayload)
	pageWithCtx.Eval(postMessageScript)

	postMessageObjScript := fmt.Sprintf(`
		try {
			window.postMessage({data: '%s', message: '%s', content: '%s', html: '%s', text: '%s', value: '%s'}, '*');
		} catch(e) {}
	`, escapedPayload, escapedPayload, escapedPayload, escapedPayload, escapedPayload, escapedPayload)
	pageWithCtx.Eval(postMessageObjScript)

	postMessageJSONScript := fmt.Sprintf(`
		try {
			window.postMessage(JSON.stringify({payload: '%s'}), '*');
		} catch(e) {}
	`, escapedPayload)
	pageWithCtx.Eval(postMessageJSONScript)

	detectionTimeout := a.getDetectionTimeout(pageWithCtx)

	select {
	case alertEvent := <-alertChan:
		taskLog.Info().Str("message", alertEvent.Message).Msg("Alert triggered via postMessage - DOM XSS detected")
		return &DOMXSSDetection{
			Source:       source,
			Payload:      payload,
			TestURL:      a.HistoryItem.URL,
			Confidence:   90,
			AlertMessage: alertEvent.Message,
			TaintFlow:    false,
		}
	case taintMsg := <-taintChan:
		taskLog.Info().Str("taint", taintMsg).Msg("Taint flow detected via postMessage")
		return &DOMXSSDetection{
			Source:     source,
			Payload:    payload,
			TestURL:    a.HistoryItem.URL,
			Confidence: 70,
			TaintFlow:  true,
		}
	case <-time.After(detectionTimeout):
	case <-ctx.Done():
		return nil
	}

	if viper.GetBool("scan.dom_xss.trigger_events") {
		eventTypes := browser.EventTypesForAlertPayload(payload.Value)
		if eventTypes.HasEventTypesToCheck() {
			done := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					close(done)
				case <-time.After(5 * time.Second):
					close(done)
				}
			}()

			browser.TriggerAllEvents(pageWithCtx, eventTypes, &browser.FastMovementOptions, done)

			select {
			case alertEvent := <-alertChan:
				taskLog.Info().Str("message", alertEvent.Message).Msg("Alert triggered via postMessage event - DOM XSS detected")
				return &DOMXSSDetection{
					Source:       source,
					Payload:      payload,
					TestURL:      a.HistoryItem.URL,
					Confidence:   85,
					AlertMessage: alertEvent.Message,
					TaintFlow:    false,
				}
			case taintMsg := <-taintChan:
				taskLog.Info().Str("taint", taintMsg).Msg("Taint flow detected via postMessage event")
				return &DOMXSSDetection{
					Source:     source,
					Payload:    payload,
					TestURL:    a.HistoryItem.URL,
					Confidence: 65,
					TaintFlow:  true,
				}
			case <-time.After(500 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return nil
			}
		}
	}

	return nil
}

func (a *DOMXSSAudit) testPayloadInBrowser(ctx context.Context, rodBrowser *rod.Browser, source web.DOMXSSSource, payload payloads.DOMXSSPayload, testURL string) *DOMXSSDetection {
	taskLog := log.With().
		Str("source", source.Name).
		Str("testURL", testURL).
		Str("marker", payload.Marker).
		Logger()

	page, err := rodBrowser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		taskLog.Debug().Err(err).Msg("Failed to create browser page")
		return nil
	}
	defer page.Close()

	web.IgnoreCertificateErrors(page)
	pageWithCtx := page.Context(ctx)

	alertChan := make(chan *proto.PageJavascriptDialogOpening, 1)
	taintChan := make(chan string, 10)

	go pageWithCtx.EachEvent(
		func(e *proto.PageJavascriptDialogOpening) (stop bool) {
			if payloads.ContainsMarker(e.Message, payload.Marker) {
				select {
				case alertChan <- e:
				default:
				}
			}
			proto.PageHandleJavaScriptDialog{Accept: true}.Call(pageWithCtx)
			return true
		},
		func(e *proto.RuntimeConsoleAPICalled) {
			for _, arg := range e.Args {
				valStr := arg.Value.String()
				if payloads.ContainsMarker(valStr, payload.Marker) || payloads.ContainsTaintMarker(valStr) {
					select {
					case taintChan <- valStr:
					default:
					}
				}
			}
		},
	)()

	// EvalOnNewDocument ensures hooks are in place before page scripts execute
	if err := a.injectTaintTrackingOnNewDocument(pageWithCtx, payload.Marker); err != nil {
		taskLog.Debug().Err(err).Msg("Failed to inject taint tracking on new document")
	}

	taskLog.Debug().Msg("Navigating to test URL")
	err = pageWithCtx.Navigate(testURL)
	if err != nil {
		taskLog.Debug().Err(err).Msg("Navigation error")
		return nil
	}

	err = pageWithCtx.WaitLoad()
	if err != nil {
		taskLog.Debug().Err(err).Msg("Error waiting for page load")
	}

	time.Sleep(500 * time.Millisecond)
	detectionTimeout := a.getDetectionTimeout(pageWithCtx)

	select {
	case alertEvent := <-alertChan:
		taskLog.Info().Str("message", alertEvent.Message).Msg("Alert triggered - DOM XSS detected")
		return &DOMXSSDetection{
			Source:       source,
			Payload:      payload,
			TestURL:      testURL,
			Confidence:   95,
			AlertMessage: alertEvent.Message,
			TaintFlow:    false,
		}
	case taintMsg := <-taintChan:
		taskLog.Info().Str("taint", taintMsg).Msg("Taint flow detected - potential DOM XSS")
		return &DOMXSSDetection{
			Source:     source,
			Payload:    payload,
			TestURL:    testURL,
			Confidence: 75,
			TaintFlow:  true,
		}
	case <-time.After(detectionTimeout):
		// No immediate detection, try event triggering if enabled
	case <-ctx.Done():
		return nil
	}

	// Try event triggering for interactive payloads (onclick, onmouseover, onfocus, etc.)
	if viper.GetBool("scan.dom_xss.trigger_events") {
		eventTypes := browser.EventTypesForAlertPayload(payload.Value)
		if eventTypes.HasEventTypesToCheck() {
			taskLog.Debug().
				Bool("click", eventTypes.Click).
				Bool("hover", eventTypes.Hover).
				Bool("focus", eventTypes.Focus).
				Bool("keyboard", eventTypes.Keyboard).
				Msg("Triggering events for interactive payload")

			done := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					close(done)
				case <-time.After(5 * time.Second):
					close(done)
				}
			}()

			browser.TriggerAllEvents(pageWithCtx, eventTypes, &browser.FastMovementOptions, done)

			select {
			case alertEvent := <-alertChan:
				taskLog.Info().Str("message", alertEvent.Message).Msg("Alert triggered via event - DOM XSS detected")
				return &DOMXSSDetection{
					Source:       source,
					Payload:      payload,
					TestURL:      testURL,
					Confidence:   90, // Slightly lower confidence for event-triggered
					AlertMessage: alertEvent.Message,
					TaintFlow:    false,
				}
			case taintMsg := <-taintChan:
				taskLog.Info().Str("taint", taintMsg).Msg("Taint flow detected via event")
				return &DOMXSSDetection{
					Source:     source,
					Payload:    payload,
					TestURL:    testURL,
					Confidence: 70,
					TaintFlow:  true,
				}
			case <-time.After(500 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return nil
			}
		}
	}

	return nil
}

func (a *DOMXSSAudit) testStoragePayloadInBrowser(ctx context.Context, rodBrowser *rod.Browser, source web.DOMXSSSource, payload payloads.DOMXSSPayload, storageKey string) *DOMXSSDetection {
	taskLog := log.With().
		Str("source", source.Name).
		Str("key", storageKey).
		Str("marker", payload.Marker).
		Logger()

	page, err := rodBrowser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		taskLog.Debug().Err(err).Msg("Failed to create browser page")
		return nil
	}
	defer page.Close()

	web.IgnoreCertificateErrors(page)
	pageWithCtx := page.Context(ctx)

	// Navigate to target origin first to set storage (same-origin requirement)
	err = pageWithCtx.Navigate(a.HistoryItem.URL)
	if err != nil {
		taskLog.Debug().Err(err).Msg("Initial navigation error")
		return nil
	}
	pageWithCtx.WaitLoad()

	alertChan := make(chan *proto.PageJavascriptDialogOpening, 1)
	taintChan := make(chan string, 10)

	go pageWithCtx.EachEvent(
		func(e *proto.PageJavascriptDialogOpening) (stop bool) {
			if payloads.ContainsMarker(e.Message, payload.Marker) {
				select {
				case alertChan <- e:
				default:
				}
			}
			proto.PageHandleJavaScriptDialog{Accept: true}.Call(pageWithCtx)
			return true
		},
		func(e *proto.RuntimeConsoleAPICalled) {
			for _, arg := range e.Args {
				valStr := arg.Value.String()
				if payloads.ContainsMarker(valStr, payload.Marker) || payloads.ContainsTaintMarker(valStr) {
					select {
					case taintChan <- valStr:
					default:
					}
				}
			}
		},
	)()

	setupScript := web.GetBrowserSetupScript(source, payload.Value, storageKey)
	if setupScript == "" {
		return nil
	}

	_, err = pageWithCtx.Eval(setupScript)
	if err != nil {
		taskLog.Debug().Err(err).Msg("Failed to set storage value")
		return nil
	}

	a.injectTaintTracking(pageWithCtx, payload.Marker)

	// Reload to trigger storage-reading JavaScript
	err = pageWithCtx.Reload()
	if err != nil {
		taskLog.Debug().Err(err).Msg("Reload error")
		return nil
	}
	pageWithCtx.WaitLoad()
	time.Sleep(500 * time.Millisecond)
	detectionTimeout := a.getDetectionTimeout(pageWithCtx)

	select {
	case alertEvent := <-alertChan:
		taskLog.Info().Str("message", alertEvent.Message).Msg("Alert triggered via storage - DOM XSS detected")
		return &DOMXSSDetection{
			Source:       source,
			Payload:      payload,
			TestURL:      a.HistoryItem.URL,
			Confidence:   90,
			AlertMessage: alertEvent.Message,
			TaintFlow:    false,
		}
	case taintMsg := <-taintChan:
		taskLog.Info().Str("taint", taintMsg).Msg("Taint flow detected via storage")
		return &DOMXSSDetection{
			Source:     source,
			Payload:    payload,
			TestURL:    a.HistoryItem.URL,
			Confidence: 70,
			TaintFlow:  true,
		}
	case <-time.After(detectionTimeout):
	case <-ctx.Done():
		return nil
	}

	if viper.GetBool("scan.dom_xss.trigger_events") {
		eventTypes := browser.EventTypesForAlertPayload(payload.Value)
		if eventTypes.HasEventTypesToCheck() {
			done := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					close(done)
				case <-time.After(5 * time.Second):
					close(done)
				}
			}()

			browser.TriggerAllEvents(pageWithCtx, eventTypes, &browser.FastMovementOptions, done)

			select {
			case alertEvent := <-alertChan:
				taskLog.Info().Str("message", alertEvent.Message).Msg("Alert triggered via storage event - DOM XSS detected")
				return &DOMXSSDetection{
					Source:       source,
					Payload:      payload,
					TestURL:      a.HistoryItem.URL,
					Confidence:   85,
					AlertMessage: alertEvent.Message,
					TaintFlow:    false,
				}
			case taintMsg := <-taintChan:
				taskLog.Info().Str("taint", taintMsg).Msg("Taint flow detected via storage event")
				return &DOMXSSDetection{
					Source:     source,
					Payload:    payload,
					TestURL:    a.HistoryItem.URL,
					Confidence: 65,
					TaintFlow:  true,
				}
			case <-time.After(500 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return nil
			}
		}
	}

	return nil
}

func (a *DOMXSSAudit) testWindowNamePayload(ctx context.Context, browser *rod.Browser, source web.DOMXSSSource, payload payloads.DOMXSSPayload) *DOMXSSDetection {
	taskLog := log.With().
		Str("source", source.Name).
		Str("marker", payload.Marker).
		Logger()

	page, err := browser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		taskLog.Debug().Err(err).Msg("Failed to create browser page")
		return nil
	}
	defer page.Close()

	web.IgnoreCertificateErrors(page)
	pageWithCtx := page.Context(ctx)

	alertChan := make(chan *proto.PageJavascriptDialogOpening, 1)

	go pageWithCtx.EachEvent(
		func(e *proto.PageJavascriptDialogOpening) (stop bool) {
			if payloads.ContainsMarker(e.Message, payload.Marker) {
				select {
				case alertChan <- e:
				default:
				}
			}
			proto.PageHandleJavaScriptDialog{Accept: true}.Call(pageWithCtx)
			return true
		},
	)()

	_, err = pageWithCtx.Eval(web.GetBrowserSetupScript(source, payload.Value, ""))
	if err != nil {
		taskLog.Debug().Err(err).Msg("Failed to set window.name")
		return nil
	}

	err = pageWithCtx.Navigate(a.HistoryItem.URL)
	if err != nil {
		taskLog.Debug().Err(err).Msg("Navigation error")
		return nil
	}
	pageWithCtx.WaitLoad()
	time.Sleep(500 * time.Millisecond)
	detectionTimeout := a.getDetectionTimeout(pageWithCtx)

	select {
	case alertEvent := <-alertChan:
		taskLog.Info().Str("message", alertEvent.Message).Msg("Alert triggered via window.name - DOM XSS detected")
		return &DOMXSSDetection{
			Source:       source,
			Payload:      payload,
			TestURL:      a.HistoryItem.URL,
			Confidence:   90,
			AlertMessage: alertEvent.Message,
			TaintFlow:    false,
		}
	case <-time.After(detectionTimeout):
		return nil
	case <-ctx.Done():
		return nil
	}
}

func (a *DOMXSSAudit) testCookiePayload(ctx context.Context, browser *rod.Browser, source web.DOMXSSSource, payload payloads.DOMXSSPayload) *DOMXSSDetection {
	taskLog := log.With().
		Str("source", source.Name).
		Str("marker", payload.Marker).
		Logger()

	page, err := browser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		taskLog.Debug().Err(err).Msg("Failed to create browser page")
		return nil
	}
	defer page.Close()

	web.IgnoreCertificateErrors(page)
	pageWithCtx := page.Context(ctx)

	// Navigate to target origin first to set cookie (same-origin requirement)
	err = pageWithCtx.Navigate(a.HistoryItem.URL)
	if err != nil {
		taskLog.Debug().Err(err).Msg("Initial navigation error")
		return nil
	}
	pageWithCtx.WaitLoad()

	alertChan := make(chan *proto.PageJavascriptDialogOpening, 1)

	go pageWithCtx.EachEvent(
		func(e *proto.PageJavascriptDialogOpening) (stop bool) {
			if payloads.ContainsMarker(e.Message, payload.Marker) {
				select {
				case alertChan <- e:
				default:
				}
			}
			proto.PageHandleJavaScriptDialog{Accept: true}.Call(pageWithCtx)
			return true
		},
	)()

	escapedPayload := web.EscapeJSString(payload.Value)
	_, err = pageWithCtx.Eval(fmt.Sprintf(`document.cookie = 'dom_xss_test=%s'`, escapedPayload))
	if err != nil {
		taskLog.Debug().Err(err).Msg("Failed to set cookie")
		return nil
	}

	err = pageWithCtx.Reload()
	if err != nil {
		taskLog.Debug().Err(err).Msg("Reload error")
		return nil
	}
	pageWithCtx.WaitLoad()
	time.Sleep(500 * time.Millisecond)
	detectionTimeout := a.getDetectionTimeout(pageWithCtx)

	select {
	case alertEvent := <-alertChan:
		taskLog.Info().Str("message", alertEvent.Message).Msg("Alert triggered via cookie - DOM XSS detected")
		return &DOMXSSDetection{
			Source:       source,
			Payload:      payload,
			TestURL:      a.HistoryItem.URL,
			Confidence:   85,
			AlertMessage: alertEvent.Message,
			TaintFlow:    false,
		}
	case <-time.After(detectionTimeout):
		return nil
	case <-ctx.Done():
		return nil
	}
}

// injectTaintTrackingOnNewDocument must be called BEFORE navigation to ensure hooks
// are in place when page scripts execute.
func (a *DOMXSSAudit) injectTaintTrackingOnNewDocument(page *rod.Page, marker string) error {
	script := browser.GetTaintTrackingScript(marker)
	_, err := page.EvalOnNewDocument(script)
	return err
}

// injectTaintTracking injects into an already loaded page (for storage/postMessage tests).
func (a *DOMXSSAudit) injectTaintTracking(page *rod.Page, marker string) {
	script := browser.GetTaintTrackingScript(marker)
	page.Eval(script)
}

func (a *DOMXSSAudit) hasSourceSinkPatterns() bool {
	body, err := a.HistoryItem.ResponseBody()
	if err != nil {
		return true // Assume patterns exist if we can't read body
	}

	bodyStr := string(body)
	hasSources, hasSinks := passive.HasDOMXSSIndicators(bodyStr)

	taskLog := log.With().
		Str("url", a.HistoryItem.URL).
		Bool("hasSources", hasSources).
		Bool("hasSinks", hasSinks).
		Logger()

	taskLog.Debug().Msg("DOM XSS pattern pre-filtering result")

	return hasSources && hasSinks
}

// LoadCrawlEventContext loads storage event data from the crawl phase
func (a *DOMXSSAudit) LoadCrawlEventContext() *CrawlEventContext {
	scanID := a.Options.ScanID
	if scanID == 0 {
		return nil
	}

	// Check if crawl events optimization is enabled
	if !viper.GetBool("scan.dom_xss.use_crawl_events") {
		return nil
	}

	summary, err := db.Connection().GetScanStorageEventSummary(scanID)
	if err != nil {
		log.Debug().
			Err(err).
			Uint("scan_id", scanID).
			Msg("Failed to load crawl event context for DOM XSS optimization")
		return nil
	}

	ctx := &CrawlEventContext{
		ScanID:                  scanID,
		HasLocalStorageEvents:   summary.HasLocalStorageEvents,
		HasSessionStorageEvents: summary.HasSessionStorageEvents,
		LocalStorageKeys:        summary.LocalStorageKeys,
		SessionStorageKeys:      summary.SessionStorageKeys,
		Loaded:                  true,
	}

	log.Debug().
		Uint("scan_id", scanID).
		Bool("has_local", ctx.HasLocalStorageEvents).
		Bool("has_session", ctx.HasSessionStorageEvents).
		Int("local_keys", len(ctx.LocalStorageKeys)).
		Int("session_keys", len(ctx.SessionStorageKeys)).
		Msg("Loaded crawl event context for DOM XSS optimization")

	return ctx
}

func (a *DOMXSSAudit) shouldTestStorageSources() bool {
	if !viper.GetBool("scan.dom_xss.skip_storage_without_crawl_evidence") {
		return true
	}

	if a.Options.ScanMode == options.ScanModeFuzz {
		return true
	}

	if a.CrawlEventContext == nil || !a.CrawlEventContext.Loaded {
		return true
	}

	if !a.CrawlEventContext.HasLocalStorageEvents && !a.CrawlEventContext.HasSessionStorageEvents {
		log.Debug().
			Uint("scan_id", a.CrawlEventContext.ScanID).
			Str("mode", a.Options.ScanMode.String()).
			Msg("Skipping storage source testing - no storage events detected during crawl")
		return false
	}

	return true
}

func (a *DOMXSSAudit) shouldTestSpecificStorageSource(source web.DOMXSSSource) bool {
	if !viper.GetBool("scan.dom_xss.skip_storage_without_crawl_evidence") {
		return true
	}

	if a.Options.ScanMode == options.ScanModeFuzz {
		return true
	}

	if a.CrawlEventContext == nil || !a.CrawlEventContext.Loaded {
		return true
	}

	if source.Name == "localStorage" && !a.CrawlEventContext.HasLocalStorageEvents {
		log.Debug().Str("source", source.Name).Msg("Skipping localStorage - no crawl evidence")
		return false
	}
	if source.Name == "sessionStorage" && !a.CrawlEventContext.HasSessionStorageEvents {
		log.Debug().Str("source", source.Name).Msg("Skipping sessionStorage - no crawl evidence")
		return false
	}

	return true
}

func (a *DOMXSSAudit) getCrawlStorageKeys(storageType string) []string {
	if a.CrawlEventContext == nil || !a.CrawlEventContext.Loaded {
		return nil
	}

	if storageType == "localStorage" {
		return a.CrawlEventContext.LocalStorageKeys
	} else if storageType == "sessionStorage" {
		return a.CrawlEventContext.SessionStorageKeys
	}

	return nil
}

func (a *DOMXSSAudit) reportDOMXSS(detection *DOMXSSDetection) {
	taskLog := log.With().
		Str("source", detection.Source.Name).
		Str("url", detection.TestURL).
		Int("confidence", detection.Confidence).
		Logger()

	if !a.markDetectedIfNew(detection.TestURL, detection.Source) {
		taskLog.Debug().Msg("Skipping duplicate DOM XSS report for this source")
		return
	}

	taskLog.Warn().Msg("DOM XSS vulnerability detected")

	var sb strings.Builder
	sb.WriteString("A DOM-based XSS vulnerability has been detected.\n\n")
	sb.WriteString(fmt.Sprintf("Source: %s\n", detection.Source.Name))
	sb.WriteString(fmt.Sprintf("Source Type: %s\n", detection.Source.Type.String()))
	sb.WriteString(fmt.Sprintf("Payload: %s\n\n", detection.Payload.Value))

	if detection.TaintFlow {
		sb.WriteString("This vulnerability was detected via taint flow tracking. ")
		sb.WriteString("The payload was observed flowing from the source to a dangerous sink.\n\n")
	} else {
		sb.WriteString("This vulnerability was confirmed by triggering a JavaScript alert dialog.\n")
		if detection.AlertMessage != "" {
			sb.WriteString(fmt.Sprintf("Alert Message: %s\n\n", detection.AlertMessage))
		}
	}

	sb.WriteString(fmt.Sprintf("Test URL: %s\n\n", detection.TestURL))
	sb.WriteString(fmt.Sprintf("Source Description: %s\n", detection.Source.Description))

	issue, err := db.CreateIssueFromHistoryAndTemplate(
		a.HistoryItem,
		db.DomXssCode,
		sb.String(),
		detection.Confidence,
		"", // Use default severity from template
		&a.Options.WorkspaceID,
		&a.Options.TaskID,
		&a.Options.TaskJobID,
		&a.Options.ScanID,
		&a.Options.ScanJobID,
	)

	if err != nil {
		taskLog.Error().Err(err).Msg("Failed to create DOM XSS issue")
		return
	}

	a.saveBrowserEvent(detection, &issue)
}

func (a *DOMXSSAudit) saveBrowserEvent(detection *DOMXSSDetection, issue *db.Issue) {
	eventData := map[string]interface{}{
		"source":        detection.Source.Name,
		"source_type":   detection.Source.Type.String(),
		"payload":       detection.Payload.Value,
		"marker":        detection.Payload.Marker,
		"test_url":      detection.TestURL,
		"confidence":    detection.Confidence,
		"taint_flow":    detection.TaintFlow,
		"alert_message": detection.AlertMessage,
	}

	dataJSON, err := json.Marshal(eventData)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal DOM XSS event data")
		return
	}

	var historyID *uint
	if a.HistoryItem != nil && a.HistoryItem.ID > 0 {
		historyID = &a.HistoryItem.ID
	}

	var scanID *uint
	if a.Options.ScanID > 0 {
		scanID = &a.Options.ScanID
	}
	var scanJobID *uint
	if a.Options.ScanJobID > 0 {
		scanJobID = &a.Options.ScanJobID
	}
	var taskID *uint
	if a.Options.TaskID > 0 {
		taskID = &a.Options.TaskID
	}

	browserEvent := &db.BrowserEvent{
		EventType:   db.BrowserEventDialog,
		Category:    db.BrowserEventCategoryRuntime,
		URL:         detection.TestURL,
		Description: fmt.Sprintf("DOM XSS detected via %s source", detection.Source.Name),
		Data:        dataJSON,
		WorkspaceID: a.Options.WorkspaceID,
		ScanID:      scanID,
		ScanJobID:   scanJobID,
		HistoryID:   historyID,
		TaskID:      taskID,
		Source:      db.SourceScanner,
	}

	if err := db.Connection().SaveBrowserEvent(browserEvent); err != nil {
		log.Error().Err(err).Msg("Failed to save DOM XSS browser event")
	}
}

func (a *DOMXSSAudit) getDetectionTimeout(page *rod.Page) time.Duration {
	baseTimeout := time.Duration(viper.GetInt("scan.dom_xss.detection_timeout")) * time.Second
	if baseTimeout == 0 {
		baseTimeout = 2 * time.Second
	}

	if !viper.GetBool("scan.dom_xss.adaptive_timeout") {
		return baseTimeout
	}

	if page != nil {
		scriptCount, err := page.Eval(`() => document.scripts.length`)
		if err == nil && scriptCount.Value.Int() > 20 {
			return baseTimeout * 2
		}

		asyncCount, err := page.Eval(`() => document.querySelectorAll('script[async], script[defer]').length`)
		if err == nil && asyncCount.Value.Int() > 5 {
			return baseTimeout + (baseTimeout / 2)
		}
	}

	return baseTimeout
}

func (a *DOMXSSAudit) getStorageKeysToTest(ctx context.Context, rodBrowser *rod.Browser, source web.DOMXSSSource) []string {
	keySet := make(map[string]bool)

	storageType := "localStorage"
	if source.Name == "sessionStorage" {
		storageType = "sessionStorage"
	}

	// Keys from crawl CDP events (most reliable)
	if viper.GetBool("scan.dom_xss.prioritize_crawl_keys") {
		crawlKeys := a.getCrawlStorageKeys(storageType)
		for _, k := range crawlKeys {
			keySet[k] = true
		}
		if len(crawlKeys) > 0 {
			log.Debug().
				Str("storage_type", storageType).
				Int("crawl_keys", len(crawlKeys)).
				Strs("keys", crawlKeys).
				Msg("Added storage keys from crawl CDP events")
		}
	}

	// Static discovery from response body
	discoveredKeys := a.discoverStorageKeysFromSource(source)
	for _, k := range discoveredKeys {
		keySet[k] = true
	}

	// Dynamic discovery via browser hooks
	if viper.GetBool("scan.dom_xss.discover_storage_keys") {
		browserKeys := a.discoverStorageKeysInBrowser(ctx, rodBrowser, storageType)
		for _, k := range browserKeys {
			keySet[k] = true
		}
	}

	// Common keys fallback in fuzz mode only
	if len(keySet) == 0 && a.Options.ScanMode == options.ScanModeFuzz {
		defaultKeys := []string{
			"user", "username", "userId", "user_id", "currentUser",
			"token", "accessToken", "access_token", "refreshToken",
			"authToken", "auth_token", "jwt", "session", "sessionId",
			"config", "settings", "preferences", "theme", "language",
			"data", "state", "appState", "cache", "store",
			"input", "value", "payload", "query", "search", "filter",
			"redirect", "redirectUrl", "callback", "next", "target", "url",
			"message", "notification", "alert", "error", "name", "content", "html",
		}
		for _, k := range defaultKeys {
			keySet[k] = true
		}
	}

	result := make([]string, 0, len(keySet))
	for k := range keySet {
		result = append(result, k)
	}
	return result
}

func (a *DOMXSSAudit) discoverStorageKeysFromSource(source web.DOMXSSSource) []string {
	body, err := a.HistoryItem.ResponseBody()
	if err != nil {
		return nil
	}

	storageType := "localStorage"
	if source.Name == "sessionStorage" {
		storageType = "sessionStorage"
	}

	return http_utils.DiscoverStorageKeysFromBody(string(body), storageType)
}

// buildDeduplicationKey creates a normalized key for issue deduplication
// Follows the same pattern as AlertAudit for consistency
func (a *DOMXSSAudit) buildDeduplicationKey(url string, source web.DOMXSSSource) string {
	// Use lib.NormalizeURL for proper normalization (replaces path segments and query values with X)
	normalizedURL, err := lib.NormalizeURL(url)
	if err != nil {
		// Fallback to basic normalization if lib fails
		normalizedURL = url
		if idx := strings.Index(url, "#"); idx != -1 {
			normalizedURL = url[:idx]
		}
	}
	return normalizedURL + ":" + source.Name
}

// buildDeduplicationKeyWithStorageKey creates a key for storage-based sources that includes the storage key
func (a *DOMXSSAudit) buildDeduplicationKeyWithStorageKey(url string, source web.DOMXSSSource, storageKey string) string {
	normalizedURL, err := lib.NormalizeURL(url)
	if err != nil {
		normalizedURL = url
		if idx := strings.Index(url, "#"); idx != -1 {
			normalizedURL = url[:idx]
		}
	}
	return normalizedURL + ":" + source.Name + ":" + storageKey
}

// isDetectedSource checks if we've already detected XSS for this source (pre-check before testing)
func (a *DOMXSSAudit) isDetectedSource(url string, source web.DOMXSSSource) bool {
	key := a.buildDeduplicationKey(url, source)
	_, ok := a.detectedSources.Load(key)
	return ok
}

// isDetectedStorageSource checks if we've already detected XSS for this storage source+key
func (a *DOMXSSAudit) isDetectedStorageSource(url string, source web.DOMXSSSource, storageKey string) bool {
	key := a.buildDeduplicationKeyWithStorageKey(url, source, storageKey)
	_, ok := a.detectedSources.Load(key)
	return ok
}

// markDetectedIfNew marks a source as detected and returns true if this is new (race-safe)
func (a *DOMXSSAudit) markDetectedIfNew(url string, source web.DOMXSSSource) bool {
	key := a.buildDeduplicationKey(url, source)
	_, alreadyExists := a.detectedSources.LoadOrStore(key, true)
	return !alreadyExists
}

// markDetectedStorageIfNew marks a storage source+key as detected and returns true if new
func (a *DOMXSSAudit) markDetectedStorageIfNew(urlStr string, source web.DOMXSSSource, storageKey string) bool {
	key := a.buildDeduplicationKeyWithStorageKey(urlStr, source, storageKey)
	_, alreadyExists := a.detectedSources.LoadOrStore(key, true)
	return !alreadyExists
}

// discoverQueryParamsInBrowser loads the page with JavaScript hooks to intercept
// URL parameter access and returns the list of parameter names the application reads.
// This is more accurate than static analysis as it catches runtime behavior.
func (a *DOMXSSAudit) discoverQueryParamsInBrowser(ctx context.Context, rodBrowser *rod.Browser) []string {
	params, err := web.DiscoverQueryParamsInBrowser(ctx, rodBrowser, a.HistoryItem.URL, nil)
	if err != nil {
		log.Debug().Err(err).Str("url", a.HistoryItem.URL).Msg("Failed to discover query params in browser")
		return nil
	}
	return params
}

// discoverStorageKeysInBrowser loads the page with JavaScript hooks to intercept
// storage access and returns the keys the application reads from localStorage/sessionStorage.
func (a *DOMXSSAudit) discoverStorageKeysInBrowser(ctx context.Context, rodBrowser *rod.Browser, storageType string) []string {
	keys, err := web.DiscoverStorageKeysInBrowser(ctx, rodBrowser, a.HistoryItem.URL, storageType, nil)
	if err != nil {
		log.Debug().Err(err).Str("url", a.HistoryItem.URL).Str("storageType", storageType).Msg("Failed to discover storage keys in browser")
		return nil
	}
	return keys
}

// getQueryParamsToTest returns the list of query parameters to test for location.search source.
// Static discovery (from response body) always runs. Dynamic discovery (via browser) is optional.
func (a *DOMXSSAudit) getQueryParamsToTest(ctx context.Context, rodBrowser *rod.Browser) []string {
	paramSet := make(map[string]bool)

	// Always include existing URL query parameters
	if parsedURL, err := url.Parse(a.HistoryItem.URL); err == nil {
		for paramName := range parsedURL.Query() {
			paramSet[paramName] = true
		}
	}

	// Static discovery always runs (cheap - just parses response body)
	bodyParams := a.discoverQueryParamsFromSource()
	for _, param := range bodyParams {
		paramSet[param] = true
	}

	// Dynamic discovery is optional (requires browser page load)
	if viper.GetBool("scan.dom_xss.discover_query_params") {
		discoveredParams := a.discoverQueryParamsInBrowser(ctx, rodBrowser)
		for _, param := range discoveredParams {
			paramSet[param] = true
		}
	}

	// Add common fallback params only in fuzz mode (expensive - tests many params blindly)
	if len(paramSet) == 0 && a.Options.ScanMode == options.ScanModeFuzz {
		for _, param := range http_utils.CommonQueryParamFallbacks {
			paramSet[param] = true
		}
	}

	result := make([]string, 0, len(paramSet))
	for param := range paramSet {
		result = append(result, param)
	}
	return result
}

// discoverQueryParamsFromSource extracts query parameter names from the page source
// by looking for patterns like urlParams.get('name') or searchParams.get('name')
func (a *DOMXSSAudit) discoverQueryParamsFromSource() []string {
	body, err := a.HistoryItem.ResponseBody()
	if err != nil {
		return nil
	}
	return http_utils.DiscoverQueryParamsFromBody(string(body))
}

// buildURLWithParam creates a URL with the given parameter set to the payload
func buildURLWithParam(baseURL string, paramName string, payload string) string {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	q := parsedURL.Query()
	q.Set(paramName, payload)
	parsedURL.RawQuery = q.Encode()
	return parsedURL.String()
}
