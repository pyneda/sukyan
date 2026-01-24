package browser

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

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

type RedirectTracker struct {
	mu            sync.Mutex
	urlCounts     map[string]int
	lastReset     time.Time
	resetInterval time.Duration
}

func NewRedirectTracker() *RedirectTracker {
	return &RedirectTracker{
		urlCounts:     make(map[string]int),
		lastReset:     time.Now(),
		resetInterval: 5 * time.Second,
	}
}

func (rt *RedirectTracker) IsRedirectLoop(url string) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if time.Since(rt.lastReset) > rt.resetInterval {
		rt.urlCounts = make(map[string]int)
		rt.lastReset = time.Now()
	}

	rt.urlCounts[url]++
	return rt.urlCounts[url] > 3
}

func HijackWithContext(config HijackConfig, browser *rod.Browser, httpClient *http.Client, source string, resultsChannel chan HijackResult, ctx context.Context, workspaceID, taskID, scanID, scanJobID uint) *rod.HijackRouter {
	router := browser.HijackRequests()
	ignoreKeywords := []string{"google", "pinterest", "facebook", "instagram", "tiktok", "hotjar", "doubleclick", "yandex", "127.0.0.2"}
	redirectTracker := NewRedirectTracker()
	if httpClient == nil {
		httpClient = http_utils.CreateHttpClient()
	}
	router.MustAdd("*", func(hj *rod.Hijack) {

		if hj == nil || hj.Request == nil || hj.Request.URL() == nil {
			log.Error().Msg("Invalid hijack object, request, or URL")
			hj.ContinueRequest(&proto.FetchContinueRequest{})
			return
		}

		select {
		case <-ctx.Done():
			router.Stop()
			return
		default:
		}

		url := hj.Request.URL().String()
		isRedirectLoop := redirectTracker.IsRedirectLoop(url)

		if scheme := hj.Request.URL().Scheme; scheme != "http" && scheme != "https" {
			log.Debug().
				Str("url", url).
				Str("scheme", scheme).
				Msg("HijackWithContext skipping non-HTTP protocol")
			hj.ContinueRequest(&proto.FetchContinueRequest{})
			return
		}
		err := hj.LoadResponse(httpClient, true)
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

		if isRedirectLoop {
			log.Warn().Str("url", url).Msg("Redirect loop detected, failing request")
			hj.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			return
		}

		if mustSkip {
			log.Debug().Str("url", url).Msg("Skipping processing of hijacked response")
		} else {
			go func() {
				defer func() {
					// recover from potential panics (such as sending on closed channel, though it should be avoided by context check)
					if recover() != nil {
						log.Warn().Msg("Recovered from panic in a hijack goroutine, possibly due to closed channel")
					}
				}()
				// Additional check for context cancellation
				history := CreateHistoryFromHijack(hj.Request, hj.Response, source, "Create history from hijack", workspaceID, taskID, scanID, scanJobID, 0)
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

func Hijack(config HijackConfig, browser *rod.Browser, httpClient *http.Client, source string, resultsChannel chan HijackResult, workspaceID, taskID, scanID, scanJobID uint) {
	router := browser.HijackRequests()
	ignoreKeywords := []string{"google", "twitter", "pinterest", "facebook", "instagram", "tiktok", "hotjar", "doubleclick", "yandex", "127.0.0.2"}
	if httpClient == nil {
		httpClient = http_utils.CreateHttpClient()
	}
	redirectTracker := NewRedirectTracker()
	router.MustAdd("*", func(ctx *rod.Hijack) {
		if ctx == nil || ctx.Request == nil || ctx.Request.URL() == nil {
			log.Error().Msg("Invalid hijack object, request, or URL")
			ctx.ContinueRequest(&proto.FetchContinueRequest{})
			return
		}

		url := ctx.Request.URL().String()
		isRedirectLoop := redirectTracker.IsRedirectLoop(url)

		if scheme := ctx.Request.URL().Scheme; scheme != "http" && scheme != "https" {
			log.Debug().
				Str("url", url).
				Str("scheme", scheme).
				Msg("Hijack skipping non-HTTP protocol")
			ctx.ContinueRequest(&proto.FetchContinueRequest{})
			return
		}
		err := ctx.LoadResponse(httpClient, true)
		mustSkip := false

		if err != nil {
			log.Error().Err(err).Str("url", url).Msg("Error loading hijacked response in Hijack function")
			mustSkip = true
		}

		for _, skipWord := range ignoreKeywords {
			if strings.Contains(ctx.Request.URL().Host, skipWord) {
				mustSkip = true
			}
		}

		if isRedirectLoop {
			log.Warn().Str("url", url).Msg("Redirect loop detected, failing request")
			ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			return
		}

		if mustSkip {
			log.Debug().Str("url", url).Msg("Skipping processing of hijacked response")
		} else {
			go func() {
				defer func() {
					// recover from potential panics (such as sending on closed channel)
					if recover() != nil {
						log.Warn().Msg("Recovered from panic in a hijack goroutine, possibly due to closed channel")
					}
				}()
				history := CreateHistoryFromHijack(ctx.Request, ctx.Response, source, "Create history from hijack", workspaceID, taskID, scanID, scanJobID, 0)
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

// DumpHijackRequest creates a raw HTTP request dump from a hijack request
// If originalBody is provided, it will be used instead of trying to read from the request streams
func DumpHijackRequest(req *rod.HijackRequest, originalBody ...[]byte) (raw string, body string) {
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
	dump.WriteString(fmt.Sprintf("%s %s %s\n", method, path, "HTTP/1.1"))

	// Headers
	for k, v := range req.Headers() {
		dump.WriteString(fmt.Sprintf("%s: %s\n", k, v))
	}

	// Body - use provided originalBody if available, otherwise fall back to normal retrieval
	if len(originalBody) > 0 && len(originalBody[0]) > 0 {
		body = string(originalBody[0])
	} else {
		// Fall back to normal body retrieval
		body = req.Body()
		if len(body) == 0 {
			reader := req.Req().Body
			if reader != nil {
				bodyBytes, err := io.ReadAll(reader)
				if err != nil {
					log.Debug().Err(err).Msg("Error reading request body from consumed stream")
				} else {
					body = string(bodyBytes)
				}
			}
		}
	}

	// Add body to dump if we have any
	if len(body) > 0 {
		dump.WriteString("\n")
		dump.WriteString(body)
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
func CreateHistoryFromHijack(request *rod.HijackRequest, response *rod.HijackResponse, source string, note string, workspaceID, taskID, scanID, scanJobID, playgroundSessionID uint) *db.History {
	rawRequest, reqBody := DumpHijackRequest(request)
	rawResponse, _ := DumpHijackResponse(response)
	historyUrl := request.URL().String()
	history := db.History{
		StatusCode:          response.Payload().ResponseCode,
		URL:                 historyUrl,
		Depth:               lib.CalculateURLDepth(historyUrl),
		RequestBodySize:     len(reqBody),
		RequestContentType:  request.Req().Header.Get("Content-Type"),
		ResponseContentType: response.Headers().Get("Content-Type"),
		Evaluated:           false,
		Method:              request.Req().Method,
		Note:                note,
		Source:              source,
		RawRequest:          []byte(rawRequest),
		RawResponse:         []byte(rawResponse),
		WorkspaceID:         &workspaceID,
		TaskID:              &taskID,
		PlaygroundSessionID: &playgroundSessionID,
		Proto:               request.Req().Proto,
	}
	if scanID > 0 {
		history.ScanID = &scanID
	}
	if scanJobID > 0 {
		history.ScanJobID = &scanJobID
	}
	createdHistory, _ := db.Connection().CreateHistory(&history)
	// log.Debug().Interface("history", history).Msg("New history record created")

	return createdHistory
}

// CreateHistoryFromHijackWithBody saves a history request from hijack request/response items with optional original body
func CreateHistoryFromHijackWithBody(request *rod.HijackRequest, response *rod.HijackResponse, source string, note string, workspaceID, taskID, scanID, scanJobID, playgroundSessionID uint, originalBody ...[]byte) *db.History {
	rawRequest, reqBody := DumpHijackRequest(request, originalBody...)
	rawResponse, _ := DumpHijackResponse(response)
	historyUrl := request.URL().String()
	history := db.History{
		StatusCode:          response.Payload().ResponseCode,
		URL:                 historyUrl,
		Depth:               lib.CalculateURLDepth(historyUrl),
		RequestBodySize:     len(reqBody),
		RequestContentType:  request.Req().Header.Get("Content-Type"),
		ResponseContentType: response.Headers().Get("Content-Type"),
		Evaluated:           false,
		Method:              request.Req().Method,
		Note:                note,
		Source:              source,
		RawRequest:          []byte(rawRequest),
		RawResponse:         []byte(rawResponse),
		WorkspaceID:         &workspaceID,
		TaskID:              &taskID,
		PlaygroundSessionID: &playgroundSessionID,
		Proto:               request.Req().Proto,
	}
	if scanID > 0 {
		history.ScanID = &scanID
	}
	if scanJobID > 0 {
		history.ScanJobID = &scanJobID
	}
	createdHistory, _ := db.Connection().CreateHistory(&history)

	return createdHistory
}
