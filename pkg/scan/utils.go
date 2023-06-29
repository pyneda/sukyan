package scan

import (
	"github.com/pyneda/sukyan/db"
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
