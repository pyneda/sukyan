package http_utils

import (
	"github.com/rs/zerolog/log"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
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
