package http_utils

import (
	"github.com/rs/zerolog/log"

	"errors"
	"io"
	"net/http"
	"net/http/httputil"

	"gorm.io/datatypes"

	"bytes"
	"encoding/json"

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
	if response == nil || response.Request == nil {
		return nil, errors.New("response or request is nil")
	}

	requestHeaders, err := json.Marshal(response.Request.Header)
	if err != nil {
		log.Error().Err(err).Msg("Error converting request headers to json")
	}
	responseHeaders, err := json.Marshal(response.Header)
	if err != nil {
		log.Error().Err(err).Msg("Error converting response headers to json")
	}

	requestDump, err := httputil.DumpRequestOut(response.Request, true)
	if err != nil {
		log.Error().Err(err).Msg("Error dumping request")
	}
	var requestBody []byte
	if response.Request.Body != nil {
		requestBody, _ = io.ReadAll(response.Request.Body)
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
		RequestHeaders:      datatypes.JSON(requestHeaders),
		RequestBody:         requestBody,
		RequestBodySize:     len(requestBody),
		ResponseHeaders:     datatypes.JSON(responseHeaders),
		ResponseBody:        responseData.Body,
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
	}
	return db.Connection.CreateHistory(&record)
}
