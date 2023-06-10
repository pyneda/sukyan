package active

import (
	"bufio"
	"context"
	"fmt"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
)


// TestXSS : Tests xss
func TestXSS(targetUrl string, params []string, wordlist string, urlEncode bool) error {
	parsedURL, err := url.ParseRequestURI(targetUrl)
	testQueryParams := make([]string, len(params))
	var wg sync.WaitGroup
	if err != nil {
		fmt.Printf("Error Invalid URL: %s\n", targetUrl)
		return err
	}
	fmt.Println(parsedURL.RawQuery)
	query, err := url.ParseQuery(parsedURL.RawQuery)
	if err != nil {
		log.Warn().Str("url", targetUrl).Msg("Could not parse url query")
	}
	// fmt.Println("Query: ", query)
	for key, element := range query {
		fmt.Println("Key: ", key, "=>", "Element", element)
		if len(params) > 0 {
			if lib.Contains(params, key) == true {
				testQueryParams = append(testQueryParams, key)
			}
		} else {
			// If no params to test, we test all
			testQueryParams = append(testQueryParams, key)
		}
	}
	//log.Printf("URL Params to test: %s\n", testQueryParams)
	log.Info().Strs("params", testQueryParams).Str("url", targetUrl).Msg("testing url params")
	f, err := os.Open(wordlist)
	if err != nil {
		log.Fatal().Err(err)
	}
	defer func() {
		if err = f.Close(); err != nil {
			log.Fatal().Err(err)
		}
	}()
	s := bufio.NewScanner(f)

	// Create channel to transfer audit items to workers
	auditItemsChannel := make(chan lib.ParameterAuditItem)
	pendingChannel := make(chan int)
	go XSSParameterAuditMonitor(pendingChannel, auditItemsChannel)
	// Start 4 hard coded XSS audit workers
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go XSSParameterAuditWorker(auditItemsChannel, pendingChannel, &wg)
	}

	for s.Scan() {
		// log.Printf("Testing payload: %s\n", s.Text())
		log.Debug().Str("url", targetUrl).Str("payload", s.Text()).Msg("Testing payload on url parameters")
		for _, tp := range testQueryParams {
			auditItem := lib.ParameterAuditItem{
				Parameter: tp,
				URL:       targetUrl,
				Payload:   s.Text(),
				URLEncode: urlEncode,
			}
			pendingChannel <- 1
			auditItemsChannel <- auditItem
		}
	}
	wg.Wait()
	err = s.Err()
	if err != nil {
		log.Fatal().Err(err)
	}
	log.Info().Str("url", targetUrl).Msg("Completed XSS tests")
	return nil
}

func XSSParameterAuditMonitor(pendingChannel chan int, auditItemsChanell chan lib.ParameterAuditItem) {
	count := 0
	log.Debug().Msg("Crawl monitor started")
	for c := range pendingChannel {
		log.Debug().Int("count", count).Int("received", c).Msg("XSSParameterAuditMonitor received from pendingChannel")
		count += c
		if count == 0 {
			log.Debug().Msg("XSS evaluation finished, closing communication channels")
			close(auditItemsChanell)
			close(pendingChannel)
		}
	}
}

func XSSParameterAuditWorker(auditItems chan lib.ParameterAuditItem, pendingChannel chan int, wg *sync.WaitGroup) {
	for auditItem := range auditItems {
		TestUrlParamWithAlertPayload(auditItem)
		pendingChannel <- -1
	}
	wg.Done()
}

// TestUrlParamWithAlertPayload opens a browser and sends a payload to a param and check if alert has opened
func TestUrlParamWithAlertPayload(item lib.ParameterAuditItem) error {
	log.Debug().Msg("Launching browser to connect to page")
	browser := rod.New().MustConnect()
	defer browser.MustClose()
	testurl, err := lib.BuildURLWithParam(item.URL, item.Parameter, item.Payload, item.URLEncode)
	if err != nil {
		return err
	}
	//log.Printf("Testing XSS on parameter `%s` with final url: %s", param, testurl)
	log.Debug().Msg("Getting a browser page")
	page := browser.MustIncognito().MustPage("")
	//log.Printf("page created: %s", testurl)
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
				Request:       "Not implemented",
				Response:      "Not implemented",
				FalsePositive: false,
				Confidence:    99,
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
	//wait()

	return nil
}
