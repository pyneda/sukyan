package lib

import (
	"fmt"
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

// ParseBool parses a string into a boolean value.
func ParseBool(str string) (bool, error) {
	s := strings.ToLower(strings.TrimSpace(str))

	switch s {
	case "true", "t", "yes", "y", "1", "on", "enable", "enabled":
		return true, nil

	case "false", "f", "no", "n", "0", "off", "disable", "disabled":
		return false, nil
	}

	return false, fmt.Errorf("cannot parse %q as boolean", str)
}
