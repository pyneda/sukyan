package http_utils

import (
	"encoding/json"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"net/http"
)

type RequestHeaders map[string][]string

func SetRequestHeadersFromHistoryItem(request *http.Request, historyItem *db.History) error {
	if historyItem.RequestHeaders != nil {
		var headers RequestHeaders
		err := json.Unmarshal(historyItem.RequestHeaders, &headers)
		if err != nil {
			return err
		}

		for key, values := range headers {
			for _, value := range values {
				log.Debug().Str("key", key).Str("value", value).Msg("Setting header")
				request.Header.Set(key, value)
			}
		}
	}

	return nil
}
