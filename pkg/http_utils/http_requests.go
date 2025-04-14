package http_utils

import (
	"bytes"
	"io"
	"net/http"
	"strings"

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
	// This is done so that when you get the request from response.Request,
	// it still has its body
	resp.Request.Body = bodyCopy

	return resp, err
}
