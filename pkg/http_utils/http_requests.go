package http_utils

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// BuildRequestFromHistoryItem gets a history item and returns an http.Request with the same data
func BuildRequestFromHistoryItem(historyItem *db.History) (*http.Request, error) {
	method := strings.ToUpper(historyItem.Method)

	requestBody, err := historyItem.RequestBody()
	if err != nil {
		log.Info().Err(err).Msg("Error extracting request body")
		return nil, err
	}

	var body io.Reader
	if len(requestBody) > 0 {
		body = bytes.NewReader(requestBody)
	} else {
		body = bytes.NewReader([]byte{})
	}

	request, err := http.NewRequest(method, historyItem.URL, body)
	if err != nil {
		log.Info().Err(err).Msg("Error creating the request")
		return nil, err
	}

	SetRequestHeadersFromHistoryItem(request, historyItem)
	return request, nil
}

// SendRequest sends an http request and returns the response ensuring that the Request body is still readable so we can dump it
func SendRequest(client *http.Client, req *http.Request) (*http.Response, error) {
	var bodyCopy io.ReadCloser
	if req.Body != nil {
		// Create copy of the body
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}

		// Ensure the original body can be read again
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Create a copy for after the Do
		bodyCopy = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	resp, err := client.Do(req)

	if err != nil {
		return nil, err
	}

	// After the client.Do call, replace the request body in the response
	// This is done so that the request from response.Request, still has its body
	resp.Request.Body = bodyCopy

	return resp, err
}

func SendRequestWithTimeout(client *http.Client, req *http.Request, timeout time.Duration) (*http.Response, error) {
	// Note: Not using client.Timeout as also applies after the request is sent
	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()
	req = req.WithContext(ctx)

	return SendRequest(client, req)
}
