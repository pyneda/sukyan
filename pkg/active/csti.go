package active

import (
	"context"
	"fmt"
	"net/http"
	"time"
	"sukyan/db"
	"sukyan/pkg/fuzz"
	"sukyan/pkg/payloads"
	"sukyan/pkg/web"

	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
)

type CSTIAudit struct {
	URL                        string
	Concurrency                int
	Params                     []string
	Platform                   string
	StopAfterSuccess           bool
	OnlyCommonVulnerableParams bool
	PageLoadTimeout            uint16
	// HeuristicRecords           []fuzz.HeuristicRecord
	// ExpectedResponses          fuzz.ExpectedResponses
}

type CSTIAuditItem struct {
	payload        payloads.PayloadInterface
	injectionPoint fuzz.InjectionPoint
}

func (a *CSTIAudit) checkConfig() {
	if a.Concurrency == 0 {
		log.Info().Interface("audit", a).Msg("Concurrency is not set, setting 4 as default")
		a.Concurrency = 4
	}
	if a.PageLoadTimeout == 0 {
		log.Info().Interface("audit", a).Msg("CSTI page load timeout is not set, setting 30 seconds as default")
		a.PageLoadTimeout = 30
	}
}

func (a *CSTIAudit) Run() {
	a.checkConfig()
	payloads := payloads.GetCSTIPayloads()
	log.Info().Int("count", len(payloads)).Msg("Gathered CSTI payloads to test")
	injectionPointGatherer := fuzz.InjectionPointGatherer{
		ParamsExtensive: false,
	}
	for _, injectionPoint := range injectionPointGatherer.GetFromURL(a.URL) {
		// could consider doing an initial validation to check its value is really used in the DOM before sending payloads
		// even though probably this should be done before launching active audits and active audits be configured based on that knowledge
		for _, payload := range payloads {
			item := CSTIAuditItem{payload: payload, injectionPoint: injectionPoint}
			a.processAuditItem(item)
		}
	}
}

func (a *CSTIAudit) processAuditItem(item CSTIAuditItem) (issues []db.Issue, err error) {
	pageLoader := web.PageLoader{
		IgnoreCertCerrors: true,
		HijackEnabled:     false,
	}
	browser, page, err := pageLoader.GetPage()
	defer browser.MustClose()
	testURL := item.injectionPoint.GetWithPayload(item.payload)
	auditLog := log.With().Str("audit", "csti").Interface("auditItem", item).Str("url", testURL).Logger()
	ctx, cancel := context.WithCancel(context.Background())
	pageWithCancel := page.Context(ctx)

	go func() {
		// Cancel timeout
		time.Sleep(time.Duration(a.PageLoadTimeout) * time.Second)
		cancel()
	}()
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
		func(e *proto.PageJavascriptDialogOpening) {
			log.Warn().Str("browser_url", e.URL).Str("type", string(e.Type)).Str("dialog_text", e.Message).Bool("has_browser_handler", e.HasBrowserHandler).Msg("CSTI verified via alert box")
			issueDescription := fmt.Sprintf("A CSTI has been detected affecting  `%s`. The POC verified that appending the following payload %s an alert dialog of type %s that has been triggered with text `%s`\n", item.injectionPoint.GetTitle(), item.payload.GetValue(), e.Type, e.Message)
			cstiIssue := db.Issue{
				Title:         "Client Side Template Injection (CSTI)",
				Description:   issueDescription,
				Code:          "csti",
				Cwe:           79,
				Payload:       item.payload.GetValue(),
				URL:           testURL,
				StatusCode:    200,
				HTTPMethod:    "GET",
				Request:       "Not implemented",
				Response:      "Not implemented",
				FalsePositive: false,
				Confidence:    99,
			}
			db.Connection.CreateIssue(cstiIssue)
			issues = append(issues, cstiIssue)

			err := proto.PageHandleJavaScriptDialog{
				Accept: true,
				// PromptText: "",
			}.Call(pageWithCancel)
			if err != nil {
				log.Error().Err(err).Msg("Error handling javascript dialog")
			} else {
				log.Debug().Msg("PageHandleJavaScriptDialog succedded")
			}

		})()

	// Could listen for network response received event
	auditLog.Debug().Msg("Navigating to the page")
	var networkResponse proto.NetworkResponseReceived
	waitNetworkResponseReceived := page.WaitEvent(&networkResponse)

	// pageWithCancel.MustNavigate(testURL).MustWaitLoad()
	navigateError := pageWithCancel.Navigate(testURL)

	if navigateError != nil {
		auditLog.Warn().Err(navigateError).Msg("Error navigating to URL")
	} else {
		auditLog.Debug().Msg("Navigated to the page completed")
	}

	waitNetworkResponseReceived()
	loadError := page.WaitLoad()
	if loadError != nil {
		auditLog.Error().Err(err).Msg("Error waiting for page complete load")
	} else {
		auditLog.Debug().Msg("Page fully loaded on browser")
	}

	if networkResponse.Response.Status == http.StatusNotFound {
		auditLog.Debug().Msg("404 not found received trying a SSTI probe")
	} else if networkResponse.Response.Status == http.StatusUnprocessableEntity {
		auditLog.Debug().Msg("422 - Unprocessable entity response received trying a SSTI probe")
	} else if networkResponse.Response.Status == http.StatusInternalServerError {
		auditLog.Warn().Interface("response", networkResponse).Msg("An CSTI test caused an Internal Server Error, might be worth checking why")
	} else if networkResponse.Response.Status == http.StatusOK {
		auditLog.Debug().Interface("response", networkResponse).Msg("200 network response received on CSTI")
	} else {
		auditLog.Warn().Int("status", networkResponse.Response.Status).Interface("response", networkResponse).Msg("Non catched response status code received during CSTI probe")
	}
	return issues, err
}
