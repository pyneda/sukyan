package http_utils

import (
	"github.com/rs/zerolog/log"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"gorm.io/datatypes"

	"encoding/json"
	"github.com/pyneda/sukyan/db"
)

type ResponseBodyData struct {
	Content string
	Size    int
	err     error
}

// ReadResponseBodyData reads an http response body and returns it as string + its length as bytes
func ReadResponseBodyData(response *http.Response) (body string, size int, err error) {
	bodyBytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Error().Err(err).Msg("Error reading response body")
	}
	defer response.Body.Close()

	size = len(bodyBytes) // Should check if its better to do the len on bytes or when converted to string
	body = string(bodyBytes)
	return body, size, err
}

// ReadResponseBodyDataAsStruct same as ReadResponseBodyData, but returns the data as a struct.
func ReadResponseBodyDataAsStruct(response *http.Response) ResponseBodyData {
	body, size, err := ReadResponseBodyData(response)
	return ResponseBodyData{
		Content: body,
		Size:    size,
		err:     err,
	}
}

type FullResponseData struct {
	Body     string
	BodySize int
	Raw      string
	RawSize  int
	err      error
}

// If it works, both ReadResponseBodyData and ReadResponseBodyDataAsStruct should be replaced by this
func ReadFullResponse(response *http.Response) FullResponseData {
	responseDump, err := httputil.DumpResponse(response, true)
	bodyBytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Error().Err(err).Msg("Error reading response body")
	}
	defer response.Body.Close()
	return FullResponseData{
		Body:     string(bodyBytes),
		BodySize: len(string(bodyBytes)),
		Raw:      string(responseDump),
		RawSize:  len(string(responseDump)),
	}
}

func ReadHttpResponseAndCreateHistory(response *http.Response, source string) (*db.History, error) {
	responseData := ReadFullResponse(response)
	return CreateHistoryFromHttpResponse(response, responseData, source)
}


func CreateHistoryFromHttpResponse(response *http.Response, responseData FullResponseData, source string) (*db.History, error) {
	requestHeaders, err := json.Marshal(response.Request.Header)
	if err != nil {
		log.Error().Err(err).Msg("Error converting request headers to json")
	}
	responseHeaders, err := json.Marshal(response.Header)
	if err != nil {
		log.Error().Err(err).Msg("Error converting response headers to json")
	}
	requestDump, _ := httputil.DumpRequestOut(response.Request, true)

	record := db.History{
		URL:            response.Request.URL.String(),
		StatusCode:     response.StatusCode,
		RequestHeaders: datatypes.JSON(requestHeaders),
		// RequestContentLength int64
		ResponseHeaders:  datatypes.JSON(responseHeaders),
		ResponseBody:     responseData.Body,
		ResponseBodySize: responseData.BodySize,
		Method:           response.Request.Method,
		ContentType:      response.Header.Get("Content-Type"),
		Evaluated:        false,
		Source:           source,
		RawRequest:       string(requestDump),
		RawResponse:      string(responseData.Raw),
		// Note                 string
	}
	return db.Connection.CreateHistory(&record)
}