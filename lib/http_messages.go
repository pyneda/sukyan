package lib

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// SplitHTTPMessage splits an HTTP message into headers and body parts
// It tries to handle various line ending formats and edge cases
func SplitHTTPMessage(message []byte) ([]byte, []byte, error) {
	// Try standard HTTP delimiter CRLF+CRLF
	parts := bytes.SplitN(message, []byte("\r\n\r\n"), 2)
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}

	// Try LF+LF format
	parts = bytes.SplitN(message, []byte("\n\n"), 2)
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}

	// Try mixed format CRLF+LF (sometimes seen in hand-crafted requests)
	parts = bytes.SplitN(message, []byte("\r\n\n"), 2)
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}

	// Check if we have a request line but no headers
	// REQUEST_METHOD URI HTTP/VERSION\r\n\r\n
	// or
	// REQUEST_METHOD URI HTTP/VERSION\n\n
	if bytes.Contains(message, []byte(" HTTP/")) {
		reqLineEnd := bytes.Index(message, []byte("\r\n"))
		if reqLineEnd > 0 {
			if len(message) > reqLineEnd+2 && bytes.Equal(message[reqLineEnd:reqLineEnd+4], []byte("\r\n\r\n")) {
				return message[:reqLineEnd+2], message[reqLineEnd+4:], nil
			}
		}

		reqLineEnd = bytes.Index(message, []byte("\n"))
		if reqLineEnd > 0 {
			if len(message) > reqLineEnd+1 && bytes.Equal(message[reqLineEnd:reqLineEnd+2], []byte("\n\n")) {
				return message[:reqLineEnd+1], message[reqLineEnd+2:], nil
			}
		}
	}

	// If the message doesn't have a body
	// but has valid headers, return an empty body
	if bytes.Contains(message, []byte(" HTTP/")) ||
		bytes.Contains(message, []byte("HTTP/")) ||
		bytes.Contains(message, []byte(": ")) {
		return message, []byte{}, nil
	}

	return nil, nil, errors.New("invalid HTTP message format")
}

// ParseHTTPHeaders parses HTTP headers from a byte array
func ParseHTTPHeaders(headerBytes []byte) (map[string][]string, error) {
	headers := make(map[string][]string)
	lines := bytes.Split(headerBytes, []byte("\r\n"))
	if len(lines) == 1 {
		// Try with just \n in case the message uses LF instead of CRLF
		lines = bytes.Split(headerBytes, []byte("\n"))
	}

	// Skip first line as it's the request/status line
	for i := 1; i < len(lines); i++ {
		line := string(lines[i])
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		headers[key] = append(headers[key], value)
	}

	return headers, nil
}

// FormatHeadersAsString formats headers as a string
func FormatHeadersAsString(headers map[string][]string) string {
	var keys []string
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result strings.Builder
	for _, k := range keys {
		for _, v := range headers[k] {
			result.WriteString(fmt.Sprintf("%s: %s\n", k, v))
		}
	}
	return result.String()
}

type HeaderCompareOptions struct {
	IgnoreOrder bool
	IgnoreCase  bool
}

func CompareHeaders(expected, actual map[string][]string, options HeaderCompareOptions) (bool, string) {
	if options.IgnoreCase {
		expected = normalizeHeaderCase(expected)
		actual = normalizeHeaderCase(actual)
	}

	if len(expected) != len(actual) {
		return false, fmt.Sprintf("Different header count: expected %d, got %d", len(expected), len(actual))
	}

	if options.IgnoreOrder {
		for k, expectedValues := range expected {
			actualValues, exists := actual[k]
			if !exists {
				return false, fmt.Sprintf("Header '%s' not found", k)
			}

			if len(expectedValues) != len(actualValues) {
				return false, fmt.Sprintf("Different value count for '%s': expected %d, got %d", k, len(expectedValues), len(actualValues))
			}

			expectedCopy := make([]string, len(expectedValues))
			copy(expectedCopy, expectedValues)
			sort.Strings(expectedCopy)

			actualCopy := make([]string, len(actualValues))
			copy(actualCopy, actualValues)
			sort.Strings(actualCopy)

			for i := range expectedCopy {
				if expectedCopy[i] != actualCopy[i] {
					return false, fmt.Sprintf("Value mismatch for header '%s'", k)
				}
			}
		}
		return true, ""
	} else {
		expectedStr := FormatHeadersAsString(expected)
		actualStr := FormatHeadersAsString(actual)
		if expectedStr != actualStr {
			return false, fmt.Sprintf("Headers differ when order matters:\nExpected: %s\nActual: %s", expectedStr, actualStr)
		}
		return true, ""
	}
}

func normalizeHeaderCase(headers map[string][]string) map[string][]string {
	result := make(map[string][]string)
	for k, v := range headers {
		result[strings.ToLower(k)] = v
	}
	return result
}
