package browser

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
	"github.com/ysmood/gson"
)

// ConvertToNetworkHeaders converts map[string][]string to NetworkHeaders, excluding cookies
func ConvertToNetworkHeaders(headersMap map[string][]string) proto.NetworkHeaders {
	networkHeaders := make(proto.NetworkHeaders)
	for key, values := range headersMap {
		if strings.ToLower(key) == "cookie" {
			continue
		}
		combinedValues := strings.Join(values, ", ")
		networkHeaders[key] = gson.New(combinedValues)
	}
	return networkHeaders
}

// ConvertToNetworkHeadersAndCookies separates cookies from headers and returns both
func ConvertToNetworkHeadersAndCookies(headersMap map[string][]string) (proto.NetworkHeaders, []*proto.NetworkCookieParam) {
	networkHeaders := make(proto.NetworkHeaders)
	var cookies []*proto.NetworkCookieParam

	for key, values := range headersMap {
		if strings.ToLower(key) == "cookie" {
			for _, cookieHeader := range values {
				parsedCookies := parseCookieHeader(cookieHeader)
				cookies = append(cookies, parsedCookies...)
			}
		} else {
			combinedValues := strings.Join(values, ", ")
			networkHeaders[key] = gson.New(combinedValues)
		}
	}

	return networkHeaders, cookies
}

// ConvertToNetworkHeadersAndCookiesWithDomain separates cookies from headers and returns both with domain set
func ConvertToNetworkHeadersAndCookiesWithDomain(headersMap map[string][]string, domain string) (proto.NetworkHeaders, []*proto.NetworkCookieParam) {
	networkHeaders := make(proto.NetworkHeaders)
	var cookies []*proto.NetworkCookieParam

	for key, values := range headersMap {
		if strings.ToLower(key) == "cookie" {
			for _, cookieHeader := range values {
				parsedCookies := parseCookieHeaderWithDomain(cookieHeader, domain)
				cookies = append(cookies, parsedCookies...)
			}
		} else {
			combinedValues := strings.Join(values, ", ")
			networkHeaders[key] = gson.New(combinedValues)
		}
	}

	return networkHeaders, cookies
}

// parseCookieHeader parses a Cookie header string into NetworkCookieParam objects
func parseCookieHeader(cookieHeader string) []*proto.NetworkCookieParam {
	return parseCookieHeaderWithDomain(cookieHeader, "")
}

// parseCookieHeaderWithDomain parses a Cookie header string into NetworkCookieParam objects with domain
func parseCookieHeaderWithDomain(cookieHeader string, domain string) []*proto.NetworkCookieParam {
	var cookies []*proto.NetworkCookieParam

	cookiePairs := strings.Split(cookieHeader, ";")
	for _, pair := range cookiePairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if name == "" {
			continue
		}

		cookie := &proto.NetworkCookieParam{
			Name:  name,
			Value: value,
		}

		if domain != "" {
			cookie.Domain = domain
		}

		cookies = append(cookies, cookie)
	}

	return cookies
}

// SetPageHeadersAndCookies sets headers and cookies on a page from a headers map
func SetPageHeadersAndCookies(page *rod.Page, headersMap map[string][]string, targetURL string) error {
	if headersMap == nil {
		return nil
	}

	var headers proto.NetworkHeaders
	var cookies []*proto.NetworkCookieParam

	if targetURL != "" {
		if parsedURL, err := url.Parse(targetURL); err == nil {
			headers, cookies = ConvertToNetworkHeadersAndCookiesWithDomain(headersMap, parsedURL.Host)
		} else {
			headers, cookies = ConvertToNetworkHeadersAndCookies(headersMap)
		}
	} else {
		headers, cookies = ConvertToNetworkHeadersAndCookies(headersMap)
	}

	if len(cookies) > 0 {
		err := page.SetCookies(cookies)
		if err != nil {
			log.Error().Err(err).Msg("Error setting cookies")
			return err
		}
	}

	if len(headers) > 0 {
		page.EnableDomain(&proto.NetworkEnable{})
		err := proto.NetworkSetExtraHTTPHeaders{Headers: headers}.Call(page)
		if err != nil {
			log.Error().Err(err).Interface("headers", headers).Msg("Error setting extra HTTP headers")
			return err
		}
	}

	return nil
}

// ReplayRequestInBrowser takes a rod.Page and an http.Request, it loads the URL of the input request in the browser,
// but hijacks it and updates the headers, method, etc. to match the input request.
func ReplayRequestInBrowser(page *rod.Page, req *http.Request) error {
	router := page.HijackRequests()
	defer router.Stop()
	requestHandled := false

	router.MustAdd("*", func(ctx *rod.Hijack) {
		// https://github.com/go-rod/rod/blob/4c4ccbecdd8110a434de73de08bdbb72e8c47cb0/examples_test.go#L473-L477
		if requestHandled {
			defer router.Stop()
			return
		}
		requestHandled = true

		ctx.Request.Req().Method = req.Method
		for key, values := range req.Header {
			for _, value := range values {
				ctx.Request.Req().Header.Add(key, value)
			}
		}

		if req.Body != nil {
			bodyBytes, err := io.ReadAll(req.Body)
			if err != nil {
				ctx.OnError(err)
				return
			}
			ctx.Request.SetBody(bodyBytes)
		}
		// ctx.MustLoadResponse()
		client := http_utils.CreateHttpClient()
		err := ctx.LoadResponse(client, true)
		if err != nil {
			log.Error().Err(err).Msg("Error loading hijacked response in replay function")
		}
		// history := CreateHistoryFromHijack(ctx.Request, ctx.Response, db.SourceScanner, "Create history from replay in browser", workspaceID, taskID)

	})

	go router.Run()

	return page.Navigate(req.URL.String())
}

type ReplayAndCreateHistoryOptions struct {
	Page                *rod.Page
	Request             *http.Request
	RawURL              string
	WorkspaceID         uint
	TaskID              uint
	ScanID              uint
	ScanJobID           uint
	PlaygroundSessionID uint
	Note                string
	Source              string
}

func ReplayRequestInBrowserAndCreateHistory(opts ReplayAndCreateHistoryOptions) (history *db.History, err error) {
	// Check if the page context is already cancelled before starting
	if opts.Page.GetContext().Err() != nil {
		return nil, opts.Page.GetContext().Err()
	}

	router := opts.Page.HijackRequests()
	defer router.Stop()
	requestHandled := false
	if opts.Note == "" {
		opts.Note = "Create history from replay in browser"
	}

	err = router.Add("*", "", func(ctx *rod.Hijack) {
		// https://github.com/go-rod/rod/blob/4c4ccbecdd8110a434de73de08bdbb72e8c47cb0/examples_test.go#L473-L477
		if requestHandled {
			defer router.Stop()
			return
		}
		requestHandled = true

		ctx.Request.Req().Method = opts.Request.Method
		for key, values := range opts.Request.Header {
			for _, value := range values {
				ctx.Request.Req().Header.Add(key, value)
			}
		}

		// Store the request body before LoadResponse consumes it
		var requestBodyBytes []byte
		if opts.Request.Body != nil {
			bodyBytes, err := io.ReadAll(opts.Request.Body)
			if err != nil {
				ctx.OnError(err)
				log.Err(err).Msg("Error reading request body in replay function")
				return
			}
			opts.Request.Body.Close()
			requestBodyBytes = bodyBytes

			// Set the new body on the context and the original request for future use
			opts.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			ctx.Request.Req().Body = io.NopCloser(bytes.NewReader(bodyBytes))

			ctx.Request.SetBody(bodyBytes)

			// Set the Content-Length header to the length of the new body
			contentLength := len(bodyBytes)
			ctx.Request.Req().Header.Set("Content-Length", strconv.Itoa(contentLength))
		}
		client := http_utils.CreateHttpClient()
		err := ctx.LoadResponse(client, true)
		if err != nil {
			log.Error().Err(err).Msg("Error loading hijacked response in replay function")
		}

		// Pass the original body to preserve it in the history record
		if len(requestBodyBytes) > 0 {
			history = CreateHistoryFromHijackWithBody(ctx.Request, ctx.Response, opts.Source, opts.Note, opts.WorkspaceID, opts.TaskID, opts.ScanID, opts.ScanJobID, opts.PlaygroundSessionID, requestBodyBytes)
		} else {
			history = CreateHistoryFromHijack(ctx.Request, ctx.Response, opts.Source, opts.Note, opts.WorkspaceID, opts.TaskID, opts.ScanID, opts.ScanJobID, opts.PlaygroundSessionID)
		}

	})
	if err != nil {
		log.Debug().Err(err).Msg("Failed to add hijack handler (context may be cancelled)")
		return nil, err
	}

	go router.Run()

	requestURL := opts.RawURL

	if requestURL == "" {
		requestURL = opts.Request.URL.String()
	}

	err = opts.Page.Navigate(requestURL)

	if history == nil || history.ID == 0 {
		time.Sleep(2 * time.Second)
	}

	return history, err
}
