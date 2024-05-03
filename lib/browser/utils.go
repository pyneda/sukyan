package browser

import (
	"io"
	"net/http"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
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
		ctx.MustLoadResponse()
		// history := CreateHistoryFromHijack(ctx.Request, ctx.Response, db.SourceScanner, "Create history from replay in browser", workspaceID, taskID)

	})

	go router.Run()

	return page.Navigate(req.URL.String())
}
