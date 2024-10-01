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
	"github.com/rs/zerolog/log"
)

type RequestReplayOptions struct {
	Mode    string               `json:"mode" validate:"required,oneof=raw browser"`
	Request Request              `json:"request" validate:"required"`
	Session db.PlaygroundSession `json:"session" validate:"required"`
	Options RequestOptions       `json:"options"`
}

type ReplayResult struct {
	Result *db.History `json:"result"`
}

func Replay(input RequestReplayOptions) (ReplayResult, error) {
	if input.Mode == "raw" {
		return ReplayRaw(input)
	}
	return ReplayInBrowser(input)
}

func ReplayRaw(input RequestReplayOptions) (ReplayResult, error) {
	client := rawhttp.NewClient(rawhttp.DefaultOptions)
	requestOptions := input.Options.ToRawHTTPOptions()
	bodyReader := bytes.NewReader([]byte(input.Request.Body))
	fullURL := input.Request.URL
	if input.Request.URI != "" {
		fullURL = input.Request.URL + input.Request.URI
	}
	resp, err := client.DoRawWithOptions(input.Request.Method, fullURL, "", input.Request.Headers, bodyReader, requestOptions)

	if err != nil {
		log.Error().Err(err).Msg("Error sending request")
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
	// taskID := input.Session.TaskID
	// if taskID == nil {
	// 	taskID = new(uint)
	// }
	taskID := new(uint)

	options := http_utils.HistoryCreationOptions{
		Source:              db.SourceRepeater,
		WorkspaceID:         input.Session.WorkspaceID,
		TaskID:              *taskID,
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
	history, navigationErr := browser.ReplayRequestInBrowserAndCreateHistory(pageWithCancel, request, input.Session.WorkspaceID, 0, input.Session.ID, "Browser replay", db.SourceRepeater)
	if navigationErr != nil {
		log.Error().Err(navigationErr).Msg("Error replaying request in browser")
		return ReplayResult{}, navigationErr
	}

	result := ReplayResult{
		Result: history,
	}
	return result, nil
}
