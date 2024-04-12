package active

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/sourcegraph/conc/pool"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/browser"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
)

// TestXSS : Tests xss
func TestXSS(targetUrl string, params []string, wordlist string, urlEncode bool) error {
	parsedURL, err := url.ParseRequestURI(targetUrl)
	if err != nil {
		log.Error().Err(err).Msg("Invalid URL")
		return err
	}

	query, err := url.ParseQuery(parsedURL.RawQuery)
	if err != nil {
		log.Warn().Err(err).Msg("Could not parse URL query")
		return err
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

	f, err := os.Open(wordlist)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to open wordlist file")
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	p := pool.New().WithMaxGoroutines(3)

	for scanner.Scan() {
		payload := scanner.Text()
		for _, param := range testQueryParams {
			param := param // create a new instance for closure
			payload := payload
			p.Go(func() {
				TestUrlParamWithAlertPayload(lib.ParameterAuditItem{
					Parameter: param,
					URL:       targetUrl,
					Payload:   payload,
					URLEncode: urlEncode,
				})
			})
		}
	}

	if err := scanner.Err(); err != nil {
		log.Error().Err(err).Msg("Error reading from scanner")
		return err
	}

	p.Wait()
	log.Info().Str("url", targetUrl).Msg("Completed XSS tests")
	return err
}

// TestUrlParamWithAlertPayload opens a browser and sends a payload to a param and check if alert has opened
func TestUrlParamWithAlertPayload(item lib.ParameterAuditItem) error {
	log.Debug().Msg("Launching browser to connect to page")
	l := browser.GetBrowserLauncher()
	controlURL := l.MustLaunch()
	b := rod.New().ControlURL(controlURL).MustConnect()
	defer b.MustClose()
	testurl, err := lib.BuildURLWithParam(item.URL, item.Parameter, item.Payload, item.URLEncode)
	if err != nil {
		return err
	}
	log.Debug().Msg("Getting a browser page")
	page := b.MustIncognito().MustPage("")
	log.Debug().Msg("Browser page gathered")

	//wait := page.MustWaitNavigation()
	ctx, cancel := context.WithCancel(context.Background())
	pageWithCancel := page.Context(ctx)
	//restore := pageWithCancel.EnableDomain(&proto.PageEnable{})

	//page.MustNavigate(testurl)
	go func() {
		// Cancel timeout
		time.Sleep(60 * time.Second)
		cancel()
	}()
	log.Debug().Str("url", testurl).Msg("Navigating to the page")
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

			log.Warn().Str("url", testurl).Str("browser_url", e.URL).Str("payload", item.Payload).Str("param", item.Parameter).Str("type", string(e.Type)).Str("dialog_text", e.Message).Bool("has_browser_handler", e.HasBrowserHandler).Msg("Reflected XSS Verified")
			issueDescription := fmt.Sprintf("A reflected XSS has been detected affecting `%s` parameter. The POC verified an alert dialog of type %s that has been triggered with text `%s`\n", item.Parameter, e.Type, e.Message)
			xssIssue := db.Issue{
				Title:         "Reflected Cross-Site Scripting (XSS)",
				Description:   issueDescription,
				Code:          "xss-reflected",
				Cwe:           79,
				Payload:       item.Payload,
				URL:           e.URL,
				StatusCode:    200,
				HTTPMethod:    "GET",
				Request:       []byte("not implemented"),
				Response:      []byte("Not implemented"),
				FalsePositive: false,
				Confidence:    99,
				Severity:      "High",
			}
			db.Connection.CreateIssue(xssIssue)

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
				log.Error().Err(err).Msg("Error handling javascript dialog")
				// return true
			} else {
				log.Debug().Msg("PageHandleJavaScriptDialog succedded")
			}

			return true
		})()
	navigationErr := pageWithCancel.Navigate(testurl)
	if navigationErr != nil {
		log.Error().Str("url", testurl).Msg("Navigation error")
	}
	log.Debug().Str("url", testurl).Msg("Navigated to the page completed")
	loadError := pageWithCancel.WaitLoad()
	if loadError != nil {
		log.Error().Err(err).Msg("Error waiting for page complete load")
	} else {
		log.Debug().Str("url", testurl).Msg("Page fully loaded on browser")
	}

	return nil
}
