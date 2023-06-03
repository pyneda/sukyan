package http_utils

import (
	"io/ioutil"
	"net/http"

	"github.com/rs/zerolog/log"
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
