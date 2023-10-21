package manual

import (
	"bytes"
	"github.com/projectdiscovery/rawhttp"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
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
	MaxRedirects        int  `json:"max_redirects" validate:"min=0"`
	UpdateHostHeader    bool `json:"update_host_header"`
	UpdateContentLength bool `json:"update_content_length"`
}

func (o *RequestOptions) ToRawHTTPOptions() rawhttp.Options {
	requestOptions := rawhttp.DefaultOptions
	requestOptions.FollowRedirects = o.FollowRedirects
	if o.MaxRedirects == 0 && o.FollowRedirects {
		requestOptions.MaxRedirects = viper.GetInt("navigation.max_redirects")
	} else {
		requestOptions.MaxRedirects = o.MaxRedirects
	}
	requestOptions.AutomaticHostHeader = o.UpdateHostHeader
	requestOptions.AutomaticContentLength = o.UpdateContentLength
	return requestOptions
}

type RequestReplayOptions struct {
	Request Request              `json:"request" validate:"required"`
	Session db.PlaygroundSession `json:"session" validate:"required"`
	Options RequestOptions       `json:"options"`
}

type ReplayResult struct {
	Result *db.History `json:"result"`
}

func Replay(input RequestReplayOptions) (ReplayResult, error) {
	client := rawhttp.NewClient(rawhttp.DefaultOptions)
	requestOptions := input.Options.ToRawHTTPOptions()
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
	taskID := input.Session.TaskID
	if taskID == nil {
		taskID = new(uint)
	}

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
