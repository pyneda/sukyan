package lib

import (
	"strings"
)

// ParseHeadersStringToMap parses a string containing key-value pairs separated by commas into a map[string][]string
func ParseHeadersStringToMap(headersStr string) map[string][]string {
	headers := make(map[string][]string)
	pairs := strings.Split(headersStr, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, ":", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])
			headers[key] = append(headers[key], value)
		}
	}
	return headers
}
