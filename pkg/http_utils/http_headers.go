package http_utils

import (
	"encoding/json"
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

// IsCommonHTTPHeader checks if the given HTTP header key is common.
func IsCommonHTTPHeader(headerKey string) bool {
	commonHeaders := map[string]bool{
		"Accept":                      true,
		"Accept-Charset":              true,
		"Accept-Encoding":             true,
		"Accept-Language":             true,
		"Accept-Ranges":               true,
		"Access-Control-Allow-Origin": true,
		"Age":                         true,
		"Allow":                       true,
		"Authorization":               true,
		"Cache-Control":               true,
		"Connection":                  true,
		"Content-Encoding":            true,
		"Content-Language":            true,
		"Content-Length":              true,
		"Content-Location":            true,
		"Content-Range":               true,
		"Content-Type":                true,
		"Cookie":                      true,
		"Date":                        true,
		"ETag":                        true,
		"Expect":                      true,
		"Expires":                     true,
		"From":                        true,
		"Host":                        true,
		"If-Match":                    true,
		"If-Modified-Since":           true,
		"If-None-Match":               true,
		"If-Range":                    true,
		"If-Unmodified-Since":         true,
		"Last-Modified":               true,
		"Location":                    true,
		"Max-Forwards":                true,
		"Pragma":                      true,
		"Proxy-Authenticate":          true,
		"Proxy-Authorization":         true,
		"Range":                       true,
		"Referer":                     true,
		"Retry-After":                 true,
		"Server":                      true,
		"Set-Cookie":                  true,
		"TE":                          true,
		"Trailer":                     true,
		"Transfer-Encoding":           true,
		"Upgrade":                     true,
		"User-Agent":                  true,
		"Vary":                        true,
		"Via":                         true,
		"WWW-Authenticate":            true,
		"Warning":                     true,
	}

	// Normalize the header key to capitalize each word, similar to the canonical MIME header key format
	canonicalHeaderKey := strings.Title(headerKey)

	return commonHeaders[canonicalHeaderKey]
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
			"Server":      true,
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
