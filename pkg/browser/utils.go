package browser

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
	"github.com/ysmood/gson"
)

// ConvertToNetworkHeaders converts map[string][]string to NetworkHeaders
func ConvertToNetworkHeaders(headersMap map[string][]string) proto.NetworkHeaders {
	networkHeaders := make(proto.NetworkHeaders)
	for key, values := range headersMap {
		// Join multiple header values into a single string separated by commas
		combinedValues := strings.Join(values, ", ")
		networkHeaders[key] = gson.New(combinedValues)
	}
	return networkHeaders
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
	PlaygroundSessionID uint
	Note                string
	Source              string
}

func ReplayRequestInBrowserAndCreateHistory(opts ReplayAndCreateHistoryOptions) (history *db.History, err error) {

	router := opts.Page.HijackRequests()
	defer router.Stop()
	requestHandled := false
	if opts.Note == "" {
		opts.Note = "Create history from replay in browser"
	}

	router.MustAdd("*", func(ctx *rod.Hijack) {
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

		if opts.Request.Body != nil {
			bodyBytes, err := io.ReadAll(opts.Request.Body)
			if err != nil {
				ctx.OnError(err)
				opts.Request.Body.Close()
				return
			}
			opts.Request.Body.Close()

			// Set the new body on the context and the original request for future use
			newBodyReader := bytes.NewReader(bodyBytes)
			opts.Request.Body = io.NopCloser(newBodyReader)
			ctx.Request.Req().Body = io.NopCloser(bytes.NewReader(bodyBytes))
			ctx.Request.SetBody(bodyBytes)
		}
		client := http_utils.CreateHttpClient()
		err := ctx.LoadResponse(client, true)
		if err != nil {
			log.Error().Err(err).Msg("Error loading hijacked response in replay function")
		}
		history = CreateHistoryFromHijack(ctx.Request, ctx.Response, opts.Source, opts.Note, opts.WorkspaceID, opts.TaskID, opts.PlaygroundSessionID)
		// NOTE: This shouldn't be necessary, but it seems that the body is not being set on the history object when replaying the request
		// if history.RequestBody == nil && len(reqBody) > 0 {
		// 	history.RequestBody = reqBody
		// 	history, _ = db.Connection.UpdateHistory(history)
		// }

	})

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
