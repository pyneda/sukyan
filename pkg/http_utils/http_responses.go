package http_utils

import (
	"github.com/rs/zerolog/log"

	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
)

type ResponseBodyData struct {
	Content string
	Size    int
	err     error
}

// ReadResponseBodyData reads an http response body and returns it as string + its length as bytes
func ReadResponseBodyData(response *http.Response) (body []byte, size int, err error) {
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		log.Error().Err(err).Msg("Error reading response body in ReadResponseBodyData")
	}
	defer response.Body.Close()

	size = len(bodyBytes) // Should check if its better to do the len on bytes or when converted to string
	body = bodyBytes
	return body, size, err
}

type FullResponseData struct {
	Body      []byte
	BodySize  int
	Raw       []byte
	RawString string
	RawSize   int
	err       error
}

// ReadResponseBodyData should be replaced by this
func ReadFullResponse(response *http.Response, createNewBodyStream bool) (FullResponseData, io.ReadCloser, error) {
	if response == nil {
		return FullResponseData{}, nil, errors.New("response is nil")
	}
	if response.Body == nil {
		return FullResponseData{}, nil, errors.New("response.Body is nil")
	}
	defer response.Body.Close()

	responseDump, err := httputil.DumpResponse(response, true)
	if err != nil {
		log.Error().Err(err).Msg("Error dumping response")
		return FullResponseData{}, nil, err
	}

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		log.Error().Err(err).Msg("Error reading response body in ReadFullResponse")
		return FullResponseData{}, nil, err
	}

	var newBody io.ReadCloser
	if createNewBodyStream {
		newBody = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	return FullResponseData{
		Body:      bodyBytes,
		BodySize:  len(bodyBytes),
		Raw:       responseDump,
		RawString: string(responseDump),
		RawSize:   len(responseDump),
	}, newBody, nil
}

type HistoryCreationOptions struct {
	Source              string
	WorkspaceID         uint
	TaskID              uint
	CreateNewBodyStream bool
	PlaygroundSessionID uint
	TaskJobID           uint
	IsWebSocketUpgrade  bool
}

func ReadHttpResponseAndCreateHistory(response *http.Response, options HistoryCreationOptions) (*db.History, error) {
	if response == nil || response.Request == nil {
		return nil, errors.New("response or request is nil")
	}

	responseData, newBody, err := ReadFullResponse(response, options.CreateNewBodyStream)
	if err != nil {
		log.Error().Err(err).Msg("Error reading response body in ReadHttpResponseAndCreateHistory")
		return nil, err
	}

	if options.CreateNewBodyStream {
		response.Body = newBody
	}

	return CreateHistoryFromHttpResponse(response, responseData, options)
}

func CreateHistoryFromHttpResponse(response *http.Response, responseData FullResponseData, options HistoryCreationOptions) (*db.History, error) {

	logger := log.With().
		Str("source", options.Source).
		Uint("workspace", options.WorkspaceID).
		Logger()

	if response == nil || response.Request == nil {
		return nil, errors.New("response or request is nil")
	}

	// requestHeaders, err := json.Marshal(response.Request.Header)
	// if err != nil {
	// 	logger.Error().Err(err).Msg("Error converting request headers to json")
	// }
	// responseHeaders, err := json.Marshal(response.Header)
	// if err != nil {
	// 	logger.Error().Err(err).Msg("Error converting response headers to json")
	// }

	var requestBody []byte
	if response.Request.Body != nil {
		requestBody, _ = io.ReadAll(response.Request.Body)
		response.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		response.Request.ContentLength = int64(len(requestBody))

		response.Request.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewBuffer(requestBody)), nil
		}

	}

	// Create a fresh request with background context for dumping to avoid "context canceled" errors
	requestForDump := response.Request.Clone(context.Background())
	if response.Request.Body != nil {
		requestForDump.Body = io.NopCloser(bytes.NewBuffer(requestBody))
	}

	requestDump, err := httputil.DumpRequestOut(requestForDump, true)
	if err != nil {
		logger.Error().Err(err).Msg("Error dumping request")
	}

	if response.Request.Body != nil {
		defer response.Request.Body.Close()
	}

	var playgroundSessionID *uint
	if options.PlaygroundSessionID > 0 {
		playgroundSessionID = &options.PlaygroundSessionID
	}

	record := db.History{
		URL:                 response.Request.URL.String(),
		Depth:               lib.CalculateURLDepth(response.Request.URL.String()),
		StatusCode:          response.StatusCode,
		RequestBodySize:     len(requestBody),
		ResponseBodySize:    responseData.BodySize,
		Method:              response.Request.Method,
		ResponseContentType: response.Header.Get("Content-Type"),
		RequestContentType:  response.Request.Header.Get("Content-Type"),
		Evaluated:           false,
		Source:              options.Source,
		RawRequest:          requestDump,
		RawResponse:         responseData.Raw,
		WorkspaceID:         &options.WorkspaceID,
		TaskID:              &options.TaskID,
		// TaskJobID:           &options.TaskJobID,
		PlaygroundSessionID: playgroundSessionID,
		Proto:               response.Proto,
		IsWebSocketUpgrade:  options.IsWebSocketUpgrade || response.StatusCode == http.StatusSwitchingProtocols,
	}
	return db.Connection().CreateHistory(&record)
}

// CreateTimeoutHistory creates a history record for requests that timed out
func CreateTimeoutHistory(req *http.Request, duration time.Duration, timeoutErr error, options HistoryCreationOptions) (*db.History, error) {
	logger := log.With().
		Str("source", options.Source).
		Uint("workspace", options.WorkspaceID).
		Str("url", req.URL.String()).
		Str("method", req.Method).
		Dur("duration", duration).
		Logger()

	if req == nil {
		return nil, errors.New("request is nil")
	}

	var requestBody []byte
	if req.Body != nil {
		requestBody, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		req.ContentLength = int64(len(requestBody))

		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewBuffer(requestBody)), nil
		}
	}

	// Create a fresh request with background context for dumping to avoid "context canceled" errors
	requestForDump := req.Clone(context.Background())
	if req.Body != nil {
		requestForDump.Body = io.NopCloser(bytes.NewBuffer(requestBody))
	}

	requestDump, err := httputil.DumpRequestOut(requestForDump, true)
	if err != nil {
		logger.Error().Err(err).Msg("Error dumping timeout request")
		requestDump = []byte(fmt.Sprintf("Error dumping timeout request: %v", err))
	}

	if req.Body != nil {
		defer req.Body.Close()
	}

	var playgroundSessionID *uint
	if options.PlaygroundSessionID > 0 {
		playgroundSessionID = &options.PlaygroundSessionID
	}

	note := fmt.Sprintf("Request timed out after %s. Error: %v", duration, timeoutErr)

	record := db.History{
		URL:                 req.URL.String(),
		Depth:               lib.CalculateURLDepth(req.URL.String()),
		StatusCode:          0,
		RequestBodySize:     len(requestBody),
		ResponseBodySize:    0,
		Method:              req.Method,
		ResponseContentType: "",
		RequestContentType:  req.Header.Get("Content-Type"),
		Evaluated:           false,
		Source:              options.Source,
		RawRequest:          requestDump,
		RawResponse:         nil,
		Note:                note,
		WorkspaceID:         &options.WorkspaceID,
		TaskID:              &options.TaskID,
		PlaygroundSessionID: playgroundSessionID,
		Proto:               "HTTP/1.1",
		IsWebSocketUpgrade:  options.IsWebSocketUpgrade,
	}

	logger.Debug().
		Int("request_body_size", len(requestBody)).
		Str("request_content_type", req.Header.Get("Content-Type")).
		Msg("Creating timeout history record")

	return db.Connection().CreateHistory(&record)
}
