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
