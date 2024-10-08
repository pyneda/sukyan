package manual

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/projectdiscovery/rawhttp"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/browser"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/web"
	"github.com/rs/zerolog/log"
)

type RequestReplayOptions struct {
	Mode    string               `json:"mode" validate:"required,oneof=raw browser"`
	Request Request              `json:"request" validate:"required"`
	Session db.PlaygroundSession `json:"session" validate:"required"`
	Options RequestOptions       `json:"options"`
}

type ReplayResult struct {
	Result        *db.History     `json:"result"`
	BrowserEvents []web.PageEvent `json:"browser_events"`
}

func Replay(input RequestReplayOptions) (ReplayResult, error) {
	log.Info().Str("mode", input.Mode).Msg("Replaying request")
	if input.Mode == "raw" {
		return ReplayRaw(input)
	}
	return ReplayInBrowser(input)
}

func ReplayRaw(input RequestReplayOptions) (ReplayResult, error) {
	parsedUrl, err := url.Parse(input.Request.URL)
	if err != nil {
		return ReplayResult{}, err
	}
	pipeOptions := input.Options.toRawHTTPPipelineOptions(parsedUrl.Host)
	pipeOptions.MaxConnections = 1
	pipeOptions.MaxPendingRequests = 10

	if input.Options.Timeout > 0 {
		pipeOptions.Timeout = time.Duration(input.Options.Timeout) * time.Second
	}
	pipeClient := rawhttp.NewPipelineClient(pipeOptions)
	bodyReader := bytes.NewReader([]byte(input.Request.Body))
	fullURL := input.Request.URL
	if input.Request.URI != "" {
		fullURL = input.Request.URL + input.Request.URI
	}
	resp, err := pipeClient.DoRawWithOptions(input.Request.Method, fullURL, "", input.Request.Headers, bodyReader, pipeOptions)

	if err != nil {
		log.Error().Str("method", input.Request.Method).Str("url", fullURL).Interface("options", pipeOptions).Err(err).Msg("Error sending request")
		return ReplayResult{}, err
	}

	parsed, err := url.Parse(fullURL)
	if err != nil {
		log.Error().Msgf("Error parsing URL: %s", err)
		return ReplayResult{}, err
	}
	resp.Request = &http.Request{
		Method: input.Request.Method,
		URL:    parsed,
		Header: input.Request.Headers,
		Body:   io.NopCloser(bytes.NewReader([]byte(input.Request.Body))),
	}

	options := http_utils.HistoryCreationOptions{
		Source:              db.SourceRepeater,
		WorkspaceID:         input.Session.WorkspaceID,
		TaskID:              0,
		CreateNewBodyStream: false,
		PlaygroundSessionID: input.Session.ID,
	}
	history, err := http_utils.ReadHttpResponseAndCreateHistory(resp, options)
	if err != nil {
		log.Error().Msgf("Error creating history item: %s", err)
		return ReplayResult{}, err
	}
	result := ReplayResult{
		Result: history,
	}
	return result, nil
}

func ReplayInBrowser(input RequestReplayOptions) (ReplayResult, error) {
	request, err := input.Request.toHTTPRequest()
	if err != nil {
		log.Error().Err(err).Msg("Error converting request to http.Request")
		return ReplayResult{}, err
	}
	log.Info().Str("url", request.URL.String()).Msg("Replaying request in browser")

	browserPool := browser.GetScannerBrowserPoolManager()
	b := browserPool.NewBrowser()
	page := b.MustPage("")
	defer browserPool.ReleaseBrowser(b)
	ctx, cancel := context.WithCancel(context.Background())
	pageWithCancel := page.Context(ctx)
	defer pageWithCancel.Close()
	go func() {
		time.Sleep(30 * time.Second)
		cancel()
	}()

	eventStream := web.ListenForPageEvents(ctx, input.Request.URL, pageWithCancel, input.Session.WorkspaceID, 0, db.SourceRepeater)
	events := []web.PageEvent{}
	go func() {
		for {
			select {
			case event, ok := <-eventStream:
				if !ok {
					return // exit if channel is closed
				}
				log.Info().Str("url", input.Request.URL).Interface("event", event).Msg("Browser repeater received page event")
				events = append(events, event)
			case <-ctx.Done():
				return
			}
		}
	}()
	defer cancel()

	history, navigationErr := browser.ReplayRequestInBrowserAndCreateHistory(pageWithCancel, request, input.Session.WorkspaceID, 0, input.Session.ID, "Browser replay", db.SourceRepeater)
	if navigationErr != nil {
		log.Error().Err(navigationErr).Msg("Error replaying request in browser")
		return ReplayResult{}, navigationErr
	}
	// Wait for 1 second after navigation to gather more events
	log.Info().Msg("Waiting for 2 second after navigation to gather more events")
	time.Sleep(2 * time.Second)

	result := ReplayResult{
		Result:        history,
		BrowserEvents: events,
	}
	return result, nil
}
