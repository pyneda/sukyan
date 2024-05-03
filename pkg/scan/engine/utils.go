package engine

import (
	"net/url"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

type UniqueHistoryidentifiers struct {
	URL              string
	Method           string
	RequestBodySize  int
	ResponseBodySize int
	StatusCode       int
}

func removeDuplicateHistoryItems(histories []*db.History) []*db.History {
	keys := make(map[UniqueHistoryidentifiers]bool)
	result := []*db.History{}

	for _, entry := range histories {
		key := UniqueHistoryidentifiers{
			URL:              entry.URL,
			Method:           entry.Method,
			ResponseBodySize: entry.ResponseBodySize,
			RequestBodySize:  entry.RequestBodySize,
			StatusCode:       entry.StatusCode,
		}

		if _, value := keys[key]; !value {
			keys[key] = true
			result = append(result, entry)
		}
	}

	return result
}

// SeparateHistoriesByBaseURL takes a slice of db.History and returns them separated by base URL in a map.
func separateHistoriesByBaseURL(histories []*db.History) map[string][]*db.History {
	baseURLMap := make(map[string][]*db.History)

	for _, history := range histories {
		parsedURL, err := url.Parse(history.URL)
		if err != nil {
			log.Error().Err(err).Msg("Invalid URL")
			continue
		}

		baseURL := parsedURL.Scheme + "://" + parsedURL.Host
		baseURLMap[baseURL] = append(baseURLMap[baseURL], history)
	}

	return baseURLMap
}
