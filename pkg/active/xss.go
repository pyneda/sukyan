package active

import (
	"bufio"
	"context"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pyneda/sukyan/pkg/web"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/browser"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

type XSSAudit struct {
	requests          sync.Map
	WorkspaceID       uint
	TaskID            uint
	TaskJobID         uint
	detectedLocations sync.Map
}

func (x *XSSAudit) Run(targetUrl string, params []string, wordlistPath string, urlEncode bool) {
	taskLog := log.With().Str("url", targetUrl).Str("audit", "xss-reflected").Logger()
	parsedURL, err := url.ParseRequestURI(targetUrl)
	if err != nil {
		taskLog.Error().Err(err).Msg("Invalid URL")
		return
	}

	query, err := url.ParseQuery(parsedURL.RawQuery)
	if err != nil {
		taskLog.Warn().Err(err).Msg("Could not parse URL query")
		return
	}

	var testQueryParams []string
	if len(params) > 0 {
		for _, key := range params {
			if _, exists := query[key]; exists {
				testQueryParams = append(testQueryParams, key)
			}
		}
	} else {
		for key := range query {
			testQueryParams = append(testQueryParams, key)
		}
	}
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
			x.testPayload(targetUrl, testQueryParams, payload, urlEncode, b)
			taskLog.Debug().Msg("Scan browser released")
			browserPool.ReleaseBrowser(b)
			hijackCancel()
			// close(hijackResultsChannel)

		})
	}

	p.Wait()
	if err := scanner.Err(); err != nil {
		taskLog.Error().Err(err).Msg("Error reading from scanner")
	}

	taskLog.Info().Str("url", targetUrl).Msg("Completed XSS tests")

}

func (x *XSSAudit) testPayload(targetUrl string, params []string, payload string, urlEncode bool, b *rod.Browser) {
	for _, param := range params {
		if x.IsDetectedLocation(targetUrl, param) {
			log.Warn().Str("url", targetUrl).Str("param", param).Msg("Skipping testing reflected XSS in already detected location")
			continue
		}
		item := lib.ParameterAuditItem{
			Parameter: param,
			URL:       targetUrl,
			Payload:   payload,
			URLEncode: urlEncode,
		}
		err := x.TestUrlParamWithAlertPayload(item, b)
		if err != nil {
			log.Error().Err(err).Msg("Failed to test URL with payload")
		}
	}
}

func (x *XSSAudit) GetHistory(url string) *db.History {
	history, ok := x.requests.Load(url)
	if ok {
		return history.(*db.History)
	}
	return &db.History{}
}

// TestUrlParamWithAlertPayload opens a browser and sends a payload to a param and check if alert has opened
func (x *XSSAudit) TestUrlParamWithAlertPayload(item lib.ParameterAuditItem, b *rod.Browser) error {
	taskLog := log.With().Str("url", item.URL).Str("param", item.Parameter).Str("payload", item.Payload).Str("audit", "xss-reflected").Logger()

	testurl, err := lib.BuildURLWithParam(item.URL, item.Parameter, item.Payload, item.URLEncode)
	if err != nil {
		return err
	}
	taskLog.Debug().Msg("Getting a browser page")
	page := b.MustPage("")
	web.IgnoreCertificateErrors(page)

	taskLog.Debug().Msg("Browser page gathered")

	//wait := page.MustWaitNavigation()
	ctx, cancel := context.WithCancel(context.Background())
	pageWithCancel := page.Context(ctx)
	//restore := pageWithCancel.EnableDomain(&proto.PageEnable{})

	go func() {
		// Cancel timeout
		time.Sleep(30 * time.Second)
		cancel()
	}()
	taskLog.Debug().Str("url", testurl).Msg("Navigating to the page")
	go pageWithCancel.EachEvent(func(e *proto.RuntimeConsoleAPICalled) {
		// consoleData := pageWithCancel.MustObjectsToJSON(e.Args)

		// if strings.Contains(consoleData.String(), "Uncaught") {
		// 	// log.Printf("[Uncaught console exception - %s] %s", testurl, consoleData)
		// 	log.Warn().Str("url", testurl).Str("data", consoleData.String()).Msg("Uncaught browser console exception")
		// } else if strings.Contains(consoleData.String(), "404 (Not found)") {
		// 	log.Warn().Str("url", testurl).Str("data", consoleData.String()).Msg("Console logged 404 resouce")
		// 	// log.Printf("[Console logged 404 resouce - %s] %s", testurl, consoleData)
		// }
	},
		func(e *proto.PageJavascriptDialogOpening) (stop bool) {
			//log.Printf("XSS Verified on url: %s - PageHandleJavaScriptDialog triggered!!!", testurl)
			//log.Printf("[XSS Verified] - Dialog type: %s - Message: %s - Has Browser Handler: %t - URL: %s", e.Type, e.Message, e.HasBrowserHandler, e.URL)

			taskLog.Warn().Str("browser_url", e.URL).Str("param", item.Parameter).Str("type", string(e.Type)).Str("dialog_text", e.Message).Bool("has_browser_handler", e.HasBrowserHandler).Msg("Reflected XSS Verified")
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
			x.StoreDetectedLocation(e.URL, item.Parameter)
			var sb strings.Builder

			sb.WriteString("A reflected XSS has been detected affecting the `" + item.Parameter + "` parameter. The POC verified an alert dialog of type " + string(e.Type) + " that has been triggered.\n\n")

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

			// urlSlug := slug.Make(testurl)
			// screenshot := fmt.Sprintf("%s.png", urlSlug)
			// log.Printf("Taking screenshot of XSS and saving to: %s", screenshot)
			// pageWithCancel.MustScreenshot(screenshot)
			//defer restore()
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
	navigationErr := pageWithCancel.Navigate(testurl)
	if navigationErr != nil {
		taskLog.Error().Str("url", testurl).Msg("Navigation error")
	}
	taskLog.Debug().Str("url", testurl).Msg("Navigated to the page completed")
	loadError := pageWithCancel.WaitLoad()
	if loadError != nil {
		taskLog.Error().Err(err).Msg("Error waiting for page complete load")
	} else {
		taskLog.Debug().Str("url", testurl).Msg("Page fully loaded on browser")
	}
	pageWithCancel.MustClose()
	return nil
}

func (x *XSSAudit) StoreDetectedLocation(url, parameter string) {
	normalizedUrl, err := lib.NormalizeURLParams(url)
	if err != nil {
		return
	}
	key := normalizedUrl + ":" + parameter
	x.detectedLocations.Store(key, true)
}

func (x *XSSAudit) IsDetectedLocation(url, parameter string) bool {
	normalizedUrl, err := lib.NormalizeURLParams(url)
	if err != nil {
		return false
	}
	key := normalizedUrl + ":" + parameter
	_, ok := x.detectedLocations.Load(key)
	return ok
}
