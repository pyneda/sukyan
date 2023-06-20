package web

import (
	"encoding/json"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/passive"
	"net/http"
	"strings"

	"fmt"
	"github.com/go-rod/rod"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
)

// HijackConfig represents a hijack configuration to apply when using the browser
type HijackConfig struct {
	AnalyzeJs   bool
	AnalyzeHTML bool
}

type HijackResult struct {
	History        *db.History
	DiscoveredURLs []string
}

func Hijack(config HijackConfig, browser *rod.Browser, resultsChannel chan HijackResult) {
	router := browser.HijackRequests()
	ignoreKeywords := []string{"google", "pinterest", "facebook", "instagram", "127.0.0.2"}

	router.MustAdd("*", func(ctx *rod.Hijack) {

		// ctx.MustLoadResponse()
		err := ctx.LoadResponse(http.DefaultClient, true)

		if err != nil {
			log.Error().Err(err).Str("url", ctx.Request.URL().String()).Msg("Error loading hijacked response")
		}

		// contentType := ctx.Response.Headers().Get("Content-Type")
		mustSkip := false
		for _, skipWord := range ignoreKeywords {
			if strings.Contains(ctx.Request.URL().Host, skipWord) == true {
				mustSkip = true
			}
		}
		if mustSkip {
			log.Debug().Str("url", ctx.Request.URL().String()).Msg("Skipping processing of hijacked response")
		} else {
			go func() {
				history := CreateHistoryFromHijack(ctx.Request, ctx.Response, "Create history from hijack")
				passive.ScanHistoryItem(history)
				linksFound := passive.ExtractAndAnalyzeURLS(history.RawResponse, history.URL)
				hijackResult := HijackResult{
					History:        history,
					DiscoveredURLs: linksFound.Web,
				}
				// log.Info().Interface("hijackResult", hijackResult).Msg("Hijack result")
				if resultsChannel != nil {
					resultsChannel <- hijackResult
				}
			}()
		}

	})
	go router.Run()
}

// func Hijack(config HijackConfig, browser *rod.Browser) {
// 	router := browser.HijackRequests()
// 	ignoreKeywords := []string{"google", "pinterest", "facebook", "instagram", "127.0.0.2"}

// 	router.MustAdd("*", func(ctx *rod.Hijack) {

// 		// ctx.MustLoadResponse()
// 		err := ctx.LoadResponse(http.DefaultClient, true)

// 		if err != nil {
// 			log.Error().Err(err).Str("url", ctx.Request.URL().String()).Msg("Error loading hijacked response")
// 		}

// 		contentType := ctx.Response.Headers().Get("Content-Type")
// 		mustSkip := false
// 		for _, skipWord := range ignoreKeywords {
// 			if strings.Contains(ctx.Request.URL().Host, skipWord) == true {
// 				mustSkip = true
// 			}
// 		}
// 		if mustSkip {
// 			log.Debug().Str("url", ctx.Request.URL().String()).Msg("Skipping processing of hijacked response")
// 		} else {
// 			history := CreateHistoryFromHijack(ctx.Request, ctx.Response, "Create history from hijack")
// 			passive.ScanHistoryItem(history)
// 			linksFound := passive.ExtractAndAnalyzeURLS(history.RawResponse, history.URL)
// 			hijackResult := HijackResult{
// 				History: history,
// 				DiscoveredURLs: linksFound.Web,
// 			}
// 		}

// 	})
// 	go router.Run()
// }

func DumpHijackRequest(req *rod.HijackRequest) string {
	var dump strings.Builder

	// Request Line
	dump.WriteString(fmt.Sprintf("%s %s %s\n", req.Method(), req.URL(), "HTTP/1.1")) // Using HTTP/1.1 as a placeholder

	// Headers
	for k, v := range req.Headers() {
		dump.WriteString(fmt.Sprintf("%s: %s\n", k, v))
	}

	// Body
	body := req.Body()
	if len(body) > 0 {
		dump.WriteString("\n")
		dump.WriteString(string(body))
	}

	return dump.String()
}

func DumpHijackResponse(res *rod.HijackResponse) string {
	var dump strings.Builder

	// Status Line
	dump.WriteString(fmt.Sprintf("HTTP/1.1 %d %s\n", res.Payload().ResponseCode, http.StatusText(res.Payload().ResponseCode))) // Using HTTP/1.1 as a placeholder

	// Headers
	for k, values := range res.Headers() {
		for _, v := range values {
			dump.WriteString(fmt.Sprintf("%s: %s\n", k, v))
		}
	}

	// Body
	body := res.Body()
	if len(body) > 0 {
		dump.WriteString("\n")
		dump.WriteString(string(body))
	}

	return dump.String()
}

// CreateHistoryFromHijack saves a history request from hijack request/response items.
func CreateHistoryFromHijack(request *rod.HijackRequest, response *rod.HijackResponse, note string) *db.History {
	requestHeaders, err := json.Marshal(request.Headers())
	if err != nil {
		log.Error().Err(err).Msg("Error converting request headers to json")
	}
	responseHeaders, err := json.Marshal(response.Headers())
	if err != nil {
		log.Error().Err(err).Msg("Error converting response headers to json")
	}
	rawRequest := DumpHijackRequest(request)
	rawResponse := DumpHijackResponse(response)
	historyUrl := request.URL().String()
	history := db.History{
		StatusCode:           response.Payload().ResponseCode,
		URL:                  historyUrl,
		Depth:                lib.CalculateURLDepth(historyUrl),
		RequestHeaders:       datatypes.JSON(requestHeaders),
		RequestContentLength: request.Req().ContentLength,
		ResponseHeaders:      datatypes.JSON(responseHeaders),
		ResponseBody:         response.Body(),
		ResponseContentType:  response.Headers().Get("Content-Type"),
		RequestContentType:   request.Req().Header.Get("Content-Type"),
		Evaluated:            false,
		Method:               request.Method(),
		Note:                 note,
		Source:               db.SourceHijack,
		RawRequest:           rawRequest,
		RawResponse:          rawResponse,
		// ResponseContentLength: response.ContentLength,

	}
	createdHistory, _ := db.Connection.CreateHistory(&history)
	log.Debug().Interface("history", history).Msg("New history record created")

	return createdHistory
}
