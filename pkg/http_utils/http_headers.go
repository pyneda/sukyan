package http_utils

import (
	"encoding/json"
	"fmt"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"net/http"
	"strings"
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
			if key == "Content-Length" {
				continue
			}
			for _, value := range values {
				log.Debug().Str("key", key).Str("value", value).Msg("Setting header")
				request.Header.Set(key, value)
			}
		}
	}

	return nil
}

func HeadersToString(headersMap map[string][]string) string {
	headers := make([]string, 0, len(headersMap))
	for name, values := range headersMap {
		for _, value := range values {
			headers = append(headers, fmt.Sprintf("%s: %s", name, value))
		}
	}
	return strings.Join(headers, "\n")
}

// ClassifyHTTPResponseHeader classifies a given HTTP response header key by its purpose.
func ClassifyHTTPResponseHeader(headerKey string) string {
	headerCategories := map[string]map[string]bool{
		"Caching": {
			"Age":           true,
			"Cache-Control": true,
			"Expires":       true,
			"Pragma":        true,
			"Vary":          true,
			"Warning":       true,
		},
		"Security": {
			"Access-Control-Allow-Origin":      true,
			"Access-Control-Allow-Methods":     true,
			"Access-Control-Allow-Headers":     true,
			"Access-Control-Allow-Credentials": true,
			"Access-Control-Max-Age":           true,
			"Access-Control-Expose-Headers":    true,
			"Access-Control-Request-Method":    true,
			"Access-Control-Request-Headers":   true,
			"Strict-Transport-Security":        true,
			"Content-Security-Policy":          true,
			"X-Content-Type-Options":           true,
			"X-XSS-Protection":                 true,
			"X-Frame-Options":                  true,
		},
		"Transport": {
			"Transfer-Encoding": true,
			"Trailer":           true,
			"Connection":        true,
			"Keep-Alive":        true,
			"Upgrade":           true,
		},
		"Information": {
			"Allow":       true,
			"Date":        true,
			"Location":    true,
			"Retry-After": true,
			"Via":         true,
		},
		"Content": {
			"Accept-Ranges":    true,
			"Content-Encoding": true,
			"Content-Language": true,
			"Content-Length":   true,
			"Content-Location": true,
			"Content-MD5":      true,
			"Content-Range":    true,
			"Content-Type":     true,
			"ETag":             true,
			"Last-Modified":    true,
		},
		"Rate-Limiting": {
			"RateLimit-Limit":     true,
			"RateLimit-Remaining": true,
			"RateLimit-Reset":     true,
		},
		"Authentication": {
			"WWW-Authenticate": true,
			"Set-Cookie":       true,
		},
		"Fingerprint": {
			"Server":           true,
			"X-Powered-By":     true,
			"X-AspNet-Version": true,
			"X-Runtime":        true,
			"X-Version":        true,
			"X-Generator":      true,
			"X-Drupal-Cache":   true,
		},
	}

	// Normalize the header key to capitalize each word, similar to the canonical MIME header key format
	canonicalHeaderKey := strings.Title(headerKey)

	for category, headers := range headerCategories {
		if headers[canonicalHeaderKey] {
			return category
		}
	}
	return "Uncommon"
}
