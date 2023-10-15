package manual

import (
	"bytes"
	"github.com/projectdiscovery/rawhttp"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
	"io/ioutil"
	"net/http"
	"net/url"
)

type Request struct {
	URL     string              `json:"url" validate:"required"`
	URI     string              `json:"uri" validate:"omitempty"`
	Method  string              `json:"method" validate:"required"`
	Headers map[string][]string `json:"headers" validate:"required"`
	Body    string              `json:"body" validate:"omitempty"`
}

type RequestOptions struct {
	FollowRedirects     bool `json:"follow_redirects"`
	MaxRedirects        int  `json:"max_redirects" validate:"min=0,de"`
	UpdateHostHeader    bool `json:"update_host_header"`
	UpdateContentLength bool `json:"update_content_length"`
}

type RequestReplayOptions struct {
	Request Request              `json:"request" validate:"required"`
	Session db.PlaygroundSession `json:"session" validate:"required"`
	Options RequestOptions       `json:"options"`
}

type ReplayResult struct {
	Result *db.
		History `json:"result"`
}

var defaultMaxRedirects = 10

func Replay(input RequestReplayOptions) (ReplayResult, error) {
	client := rawhttp.NewClient(rawhttp.DefaultOptions)
	requestOptions := rawhttp.DefaultOptions
	requestOptions.FollowRedirects = input.Options.FollowRedirects
	if input.Options.MaxRedirects == 0 && input.Options.FollowRedirects {
		requestOptions.MaxRedirects = defaultMaxRedirects
	} else {
		requestOptions.MaxRedirects = input.Options.MaxRedirects
	}
	requestOptions.AutomaticHostHeader = input.Options.UpdateHostHeader
	requestOptions.AutomaticContentLength = input.Options.UpdateContentLength
	bodyReader := bytes.NewReader([]byte(input.Request.Body))
	resp, err := client.DoRawWithOptions(input.Request.Method, input.Request.URL, input.Request.URI, input.Request.Headers, bodyReader, requestOptions)

	if err != nil {
		log.Error().Msgf("Error sending request: %s", err)
		return ReplayResult{}, err
	}

	// NOTE: rawhttp doesn't set the http.Response.Request field, so we need to do it manually
	parsed, err := url.Parse(input.Request.URL)
	if err != nil {
		log.Error().Msgf("Error parsing URL: %s", err)
		return ReplayResult{}, err
	}
	resp.Request = &http.Request{
		Method: input.Request.Method,
		URL:    parsed,
		Header: input.Request.Headers,
		Body:   ioutil.NopCloser(bytes.NewReader([]byte(input.Request.Body))),
	}

	// NOTE: Could probably use rawhttp.DumpRequestRaw to dump a better representation of the request
	history, err := http_utils.ReadHttpResponseAndCreateHistory(resp, db.SourceRepeater, input.Session.WorkspaceID, *input.Session.TaskID, false)
	if err != nil {
		log.Error().Msgf("Error creating history item: %s", err)
		return ReplayResult{}, err
	}
	result := ReplayResult{
		Result: history,
	}
	return result, nil
}
