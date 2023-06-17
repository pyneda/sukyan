package http_utils

import (
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"strings"
)

// BuildRequestFromHistoryItem gets a history item and returns an http.Request with the same data
func BuildRequestFromHistoryItem(historyItem *db.History) (*http.Request, error) {
	var body io.Reader
	method := strings.ToUpper(historyItem.Method)

	if historyItem.RequestBody != "" {
		body = strings.NewReader(historyItem.RequestBody)
	}

	request, err := http.NewRequest(method, historyItem.URL, body)
	if err != nil {
		log.Info().Err(err).Msg("Error creating the request")
		return nil, err
	}
	SetRequestHeadersFromHistoryItem(request, historyItem)
	return request, nil
}
