package active

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pyneda/sukyan/pkg/scan"
	"github.com/pyneda/sukyan/pkg/web"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/browser"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

type NewXSSAudit struct {
	requests          sync.Map
	WorkspaceID       uint
	TaskID            uint
	TaskJobID         uint
	detectedLocations sync.Map
}

func (x *NewXSSAudit) Run(history db.History, insertionPoints []scan.InsertionPoint, wordlistPath string, urlEncode bool) {
	targetUrl := history.URL
	taskLog := log.With().Str("url", targetUrl).Str("audit", "xss-reflected").Logger()

	f, err := os.Open(wordlistPath)
	if err != nil {
		taskLog.Fatal().Err(err).Msg("Failed to open wordlist file")
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	p := pool.New().WithMaxGoroutines(3)
	browserPool := browser.GetScannerBrowserPoolManager()

	for scanner.Scan() {
		payload := scanner.Text()
		p.Go(func() {
			b := browserPool.NewBrowser()
			taskLog.Debug().Msg("Got scan browser from the pool")
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
			x.testPayloadInInsertionPoint(history, insertionPoints, payload, b)
			taskLog.Debug().Msg("Scan browser released")

			// close(hijackResultsChannel)

		})
	}

	p.Wait()
	if err := scanner.Err(); err != nil {
		taskLog.Error().Err(err).Msg("Error reading from scanner")
	}

	taskLog.Info().Str("url", targetUrl).Msg("Completed XSS tests")

}

func (x *NewXSSAudit) testPayloadInInsertionPoint(history db.History, insertionPoints []scan.InsertionPoint, payload string, b *rod.Browser) {
	for _, insertionPoint := range insertionPoints {
		if x.isDetecteLocation(history.URL, insertionPoint) {
			log.Warn().Str("url", history.URL).Interface("insertionPoint", insertionPoint).Msg("Skipping testing reflected XSS in already detected location")
			continue
		}
		builders := []scan.InsertionPointBuilder{
			{
				Point:   insertionPoint,
				Payload: payload,
			},
		}
		request, err := scan.CreateRequestFromInsertionPoints(&history, builders)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create request from insertion points")
			continue
		}
		x.testRequest(request, insertionPoint, payload, b)

	}
}

func (x *NewXSSAudit) testRequest(scanRequest *http.Request, insertionPoint scan.InsertionPoint, payload string, b *rod.Browser) error {
	testurl := scanRequest.URL.String()
	taskLog := log.With().Str("url", testurl).Interface("insertionPoint", insertionPoint).Str("payload", payload).Str("audit", "xss-reflected").Logger()

	taskLog.Debug().Msg("Getting a browser page")
	page := b.MustPage("")
	web.IgnoreCertificateErrors(page)

	taskLog.Debug().Msg("Browser page gathered")

	ctx, cancel := context.WithCancel(context.Background())
	pageWithCancel := page.Context(ctx)

	go func() {
		time.Sleep(30 * time.Second)
		cancel()
	}()
	taskLog.Debug().Str("url", testurl).Msg("Navigating to the page")
	go pageWithCancel.EachEvent(
		func(e *proto.PageJavascriptDialogOpening) (stop bool) {
			//log.Printf("XSS Verified on url: %s - PageHandleJavaScriptDialog triggered!!!", testurl)
			//log.Printf("[XSS Verified] - Dialog type: %s - Message: %s - Has Browser Handler: %t - URL: %s", e.Type, e.Message, e.HasBrowserHandler, e.URL)

			taskLog.Warn().Str("browser_url", e.URL).Str("type", string(e.Type)).Str("dialog_text", e.Message).Bool("has_browser_handler", e.HasBrowserHandler).Msg("Reflected XSS Verified")
			history := x.GetHistory(e.URL)
			if history.ID == 0 {
				history = x.GetHistory(testurl)
			}

			if history.ID == 0 {
				taskLog.Warn().Str("url", testurl).Msg("Could not find history for XSS, sleeping and trying again")
				time.Sleep(4 * time.Second)
				history = x.GetHistory(e.URL)
				if history.ID == 0 {
					history.URL = e.URL
					taskLog.Warn().Str("url", testurl).Msg("Couldn't find history for XSS after sleep")
				} else {
					taskLog.Warn().Str("url", testurl).Msg("Found history for XSS after sleep")
				}
			}

			// Save as detected location to avoid duplicate alerts
			x.storeDetectedLocation(e.URL, insertionPoint)
			var sb strings.Builder

			sb.WriteString("A reflected XSS has been detected affecting the `" + insertionPoint.Name + "` " + string(insertionPoint.Type) + ". The POC verified an alert dialog of type " + string(e.Type) + " that has been triggered.\n\n")

			if e.Message != "" {
				sb.WriteString("The alert contained the following text `" + e.Message + "`\n")
			}

			if e.URL != testurl {
				sb.WriteString("\nThe original request performed has been to the URL:\n" + testurl + "\n\n")
				sb.WriteString("The payload has probably been encoded by the browser and the alert has been triggered at URL:\n" + e.URL + "\n")
			} else {
				sb.WriteString("\nThe following URL can be used to reproduce the issue: " + testurl)
			}
			db.CreateIssueFromHistoryAndTemplate(history, db.XssReflectedCode, sb.String(), 90, "", &x.WorkspaceID, &x.TaskID, &x.TaskJobID)
			err := proto.PageHandleJavaScriptDialog{
				Accept: true,
				// PromptText: "",
			}.Call(pageWithCancel)
			if err != nil {
				//log.Printf("Dialog from %s was already closed when attempted to close: %s", e.URL, err)
				taskLog.Error().Err(err).Msg("Error handling javascript dialog")
				// return true
			} else {
				taskLog.Debug().Msg("PageHandleJavaScriptDialog succedded")
			}

			return true
		})()
	navigationErr := browser.ReplayRequestInBrowser(pageWithCancel, scanRequest)
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
	pageWithCancel.MustClose()
	return nil
}

func (x *NewXSSAudit) GetHistory(url string) *db.History {
	// NOTE: This won't work anmyore if trying to also test insertion points outside of the URL (ex. headers, cookies, etc)
	history, ok := x.requests.Load(url)
	if ok {
		return history.(*db.History)
	}
	return &db.History{}
}

func (x *NewXSSAudit) storeDetectedLocation(url string, insertionPoint scan.InsertionPoint) {
	normalizedUrl, err := lib.NormalizeURL(url)
	if err != nil {
		return
	}
	key := normalizedUrl + ":" + insertionPoint.String()
	x.detectedLocations.Store(key, true)
}

func (x *NewXSSAudit) isDetecteLocation(url string, insertionPoint scan.InsertionPoint) bool {
	normalizedUrl, err := lib.NormalizeURL(url)
	if err != nil {
		return false
	}
	key := normalizedUrl + ":" + insertionPoint.String()
	_, ok := x.detectedLocations.Load(key)
	return ok
}
