package browser

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/passive"

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

func HijackWithContext(config HijackConfig, browser *rod.Browser, source string, resultsChannel chan HijackResult, ctx context.Context, workspaceID, taskID uint) *rod.HijackRouter {
	router := browser.HijackRequests()
	ignoreKeywords := []string{"google", "pinterest", "facebook", "instagram", "tiktok", "hotjar", "doubleclick", "yandex", "127.0.0.2"}
	router.MustAdd("*", func(hj *rod.Hijack) {
		select {
		case <-ctx.Done():
			router.Stop()
			return // Stop processing if context is cancelled
		default:
			// Continue as usual
		}

		err := hj.LoadResponse(http.DefaultClient, true)
		mustSkip := false

		if err != nil {
			log.Error().Err(err).Str("url", hj.Request.URL().String()).Msg("Error loading hijacked response")
			mustSkip = true
		}

		for _, skipWord := range ignoreKeywords {
			if strings.Contains(hj.Request.URL().Host, skipWord) {
				mustSkip = true
			}
		}
		if mustSkip {
			log.Debug().Str("url", hj.Request.URL().String()).Msg("Skipping processing of hijacked response")
		} else {
			go func() {
				defer func() {
					// recover from potential panics (such as sending on closed channel, though it should be avoided by context check)
					if recover() != nil {
						log.Warn().Msg("Recovered from panic in a hijack goroutine, possibly due to closed channel")
					}
				}()
				// Additional check for context cancellation
				history := CreateHistoryFromHijack(hj.Request, hj.Response, source, "Create history from hijack", workspaceID, taskID)
				linksFound := passive.ExtractedURLS{}
				if hj.Request.Type() != "Image" && hj.Request.Type() != "Font" && hj.Request.Type() != "Media" {
					linksFound = passive.ExtractURLsFromHistoryItem(history)
				}
				hijackResult := HijackResult{
					History:        history,
					DiscoveredURLs: linksFound.Web,
				}

				select {
				case resultsChannel <- hijackResult:
				case <-ctx.Done():
					router.Stop()
					return // Stop processing if context is cancelled
				}
			}()
		}
	})

	// go func() {
	// 	defer func() {
	// 		router.Stop()
	// 		close(resultsChannel)
	// 	}()
	// 	router.Run()
	// }()
	go router.Run()
	return router

}

func Hijack(config HijackConfig, browser *rod.Browser, source string, resultsChannel chan HijackResult, workspaceID, taskID uint) {
	router := browser.HijackRequests()
	ignoreKeywords := []string{"google", "pinterest", "facebook", "instagram", "tiktok", "hotjar", "doubleclick", "yandex", "127.0.0.2"}
	httpClient := http_utils.CreateHttpClient()
	router.MustAdd("*", func(ctx *rod.Hijack) {

		err := ctx.LoadResponse(httpClient, true)
		mustSkip := false

		if err != nil {
			log.Error().Err(err).Str("url", ctx.Request.URL().String()).Msg("Error loading hijacked response")
			mustSkip = true
		}

		for _, skipWord := range ignoreKeywords {
			if strings.Contains(ctx.Request.URL().Host, skipWord) {
				mustSkip = true
			}
		}
		if mustSkip {
			log.Debug().Str("url", ctx.Request.URL().String()).Msg("Skipping processing of hijacked response")
		} else {
			go func() {
				history := CreateHistoryFromHijack(ctx.Request, ctx.Response, source, "Create history from hijack", workspaceID, taskID)
				linksFound := passive.ExtractedURLS{}
				if ctx.Request.Type() != "Image" && ctx.Request.Type() != "Font" && ctx.Request.Type() != "Media" {
					linksFound = passive.ExtractURLsFromHistoryItem(history)

				}
				hijackResult := HijackResult{
					History:        history,
					DiscoveredURLs: linksFound.Web,
				}
				if resultsChannel != nil {
					resultsChannel <- hijackResult
				}
			}()
		}

	})
	go router.Run()
}

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
func CreateHistoryFromHijack(request *rod.HijackRequest, response *rod.HijackResponse, source string, note string, workspaceID, taskID uint) *db.History {
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
	reqBody := request.Body()
	history := db.History{
		StatusCode:           response.Payload().ResponseCode,
		URL:                  historyUrl,
		Depth:                lib.CalculateURLDepth(historyUrl),
		RequestHeaders:       datatypes.JSON(requestHeaders),
		RequestBody:          []byte(reqBody),
		RequestBodySize:      len(reqBody),
		RequestContentLength: request.Req().ContentLength,
		RequestContentType:   request.Req().Header.Get("Content-Type"),
		ResponseHeaders:      datatypes.JSON(responseHeaders),
		ResponseBody:         []byte(response.Body()),
		ResponseContentType:  response.Headers().Get("Content-Type"),
		Evaluated:            false,
		Method:               request.Method(),
		// ParametersCount:      len(request.URL().Query()),
		Note:        note,
		Source:      source,
		RawRequest:  []byte(rawRequest),
		RawResponse: []byte(rawResponse),
		// ResponseContentLength: response.ContentLength,
		WorkspaceID:         &workspaceID,
		TaskID:              &taskID,
		PlaygroundSessionID: nil,
		Proto:               request.Req().Proto,
	}
	createdHistory, _ := db.Connection.CreateHistory(&history)
	log.Debug().Interface("history", history).Msg("New history record created")

	return createdHistory
}
