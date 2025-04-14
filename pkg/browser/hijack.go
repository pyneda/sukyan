package browser

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/passive"

	"fmt"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
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

		if hj == nil || hj.Request == nil || hj.Request.URL() == nil {
			log.Error().Msg("Invalid hijack object, request, or URL")
			hj.ContinueRequest(&proto.FetchContinueRequest{})
			return
		}

		select {
		case <-ctx.Done():
			router.Stop()
			return // Stop processing if context is cancelled
		default:
			// Continue as usual
		}

		if scheme := hj.Request.URL().Scheme; scheme != "http" && scheme != "https" {
			log.Debug().
				Str("url", hj.Request.URL().String()).
				Str("scheme", scheme).
				Msg("HijackWithContext skipping non-HTTP protocol")
			hj.ContinueRequest(&proto.FetchContinueRequest{})
			return
		}
		err := hj.LoadResponse(http.DefaultClient, true)
		mustSkip := false

		if err != nil {
			log.Error().Err(err).Str("url", hj.Request.URL().String()).Msg("Error loading hijacked response in HijackWithContext")
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
				history := CreateHistoryFromHijack(hj.Request, hj.Response, source, "Create history from hijack", workspaceID, taskID, 0)
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

	go router.Run()
	return router

}

func Hijack(config HijackConfig, browser *rod.Browser, source string, resultsChannel chan HijackResult, workspaceID, taskID uint) {
	router := browser.HijackRequests()
	ignoreKeywords := []string{"google", "pinterest", "facebook", "instagram", "tiktok", "hotjar", "doubleclick", "yandex", "127.0.0.2"}
	httpClient := http_utils.CreateHttpClient()
	router.MustAdd("*", func(ctx *rod.Hijack) {
		if ctx == nil || ctx.Request == nil || ctx.Request.URL() == nil {
			log.Error().Msg("Invalid hijack object, request, or URL")
			ctx.ContinueRequest(&proto.FetchContinueRequest{})
			return
		}
		if scheme := ctx.Request.URL().Scheme; scheme != "http" && scheme != "https" {
			log.Debug().
				Str("url", ctx.Request.URL().String()).
				Str("scheme", scheme).
				Msg("Hijack skipping non-HTTP protocol")
			ctx.ContinueRequest(&proto.FetchContinueRequest{})
			return
		}
		err := ctx.LoadResponse(httpClient, true)
		mustSkip := false

		if err != nil {
			log.Error().Err(err).Str("url", ctx.Request.URL().String()).Msg("Error loading hijacked response in Hijack function")
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
				history := CreateHistoryFromHijack(ctx.Request, ctx.Response, source, "Create history from hijack", workspaceID, taskID, 0)
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

func DumpHijackRequest(req *rod.HijackRequest) (raw string, body string) {
	var dump strings.Builder
	reqUrl := req.URL()
	path := reqUrl.Path
	if reqUrl.RawQuery != "" {
		path += "?" + reqUrl.RawQuery
	}
	if reqUrl.Fragment != "" {
		path += "#" + reqUrl.Fragment
	}
	method := req.Req().Method
	dump.WriteString(fmt.Sprintf("%s %s %s\n", method, path, "HTTP/1.1")) // Using HTTP/1.1 as a placeholder

	// Headers
	for k, v := range req.Headers() {
		dump.WriteString(fmt.Sprintf("%s: %s\n", k, v))
	}

	// Body
	body = req.Body()
	if len(body) > 0 {
		dump.WriteString("\n")
		dump.WriteString(string(body))
	} else {
		reader := req.Req().Body
		if reader != nil {
			bodyBytes, err := io.ReadAll(reader)
			if err != nil {
				log.Error().Err(err).Msg("Error reading request body in DumpHijackRequest")
			}
			body = string(bodyBytes)
			if len(bodyBytes) > 0 {
				dump.WriteString("\n")
				dump.WriteString(body)
			}
		} else {
			log.Warn().Msg("DumpHijackRequest request body is empty")
		}
	}
	raw = dump.String()
	return raw, body
}

func DumpHijackResponse(res *rod.HijackResponse) (rawResponse string, body string) {
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
	body = res.Body()
	if len(body) > 0 {
		dump.WriteString("\n")
		dump.WriteString(body)
	}

	return dump.String(), body
}

// CreateHistoryFromHijack saves a history request from hijack request/response items.
func CreateHistoryFromHijack(request *rod.HijackRequest, response *rod.HijackResponse, source string, note string, workspaceID, taskID, playgroundSessionID uint) *db.History {
	// requestHeaders, err := json.Marshal(request.Headers())
	// if err != nil {
	// 	log.Error().Err(err).Msg("Error converting request headers to json")
	// }
	// responseHeaders, err := json.Marshal(response.Headers())
	// if err != nil {
	// 	log.Error().Err(err).Msg("Error converting response headers to json")
	// }
	rawRequest, reqBody := DumpHijackRequest(request)
	rawResponse, _ := DumpHijackResponse(response)
	historyUrl := request.URL().String()
	history := db.History{
		StatusCode: response.Payload().ResponseCode,
		URL:        historyUrl,
		Depth:      lib.CalculateURLDepth(historyUrl),
		// RequestHeaders:       datatypes.JSON(requestHeaders),
		// RequestBody:          []byte(reqBody),
		RequestBodySize: len(reqBody),
		// RequestContentLength: request.Req().ContentLength,
		RequestContentType: request.Req().Header.Get("Content-Type"),
		// ResponseHeaders:      datatypes.JSON(responseHeaders),
		// ResponseBody:         []byte(responseBody),
		ResponseContentType: response.Headers().Get("Content-Type"),
		Evaluated:           false,
		Method:              request.Req().Method,
		// ParametersCount:      len(request.URL().Query()),
		Note:        note,
		Source:      source,
		RawRequest:  []byte(rawRequest),
		RawResponse: []byte(rawResponse),
		// ResponseContentLength: response.ContentLength,
		WorkspaceID:         &workspaceID,
		TaskID:              &taskID,
		PlaygroundSessionID: &playgroundSessionID,
		Proto:               request.Req().Proto,
	}
	createdHistory, _ := db.Connection.CreateHistory(&history)
	log.Debug().Interface("history", history).Msg("New history record created")

	return createdHistory
}
