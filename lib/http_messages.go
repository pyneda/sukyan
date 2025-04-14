package lib

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
)

// SplitHTTPMessage splits an HTTP message into headers and body parts
func SplitHTTPMessage(message []byte) ([]byte, []byte, error) {
	parts := bytes.SplitN(message, []byte("\r\n\r\n"), 2)
	if len(parts) != 2 {
		// Try with just \n\n in case the message uses LF instead of CRLF
		parts = bytes.SplitN(message, []byte("\n\n"), 2)
		if len(parts) != 2 {
			return nil, nil, errors.New("invalid HTTP message format")
		}
	}

	return parts[0], parts[1], nil
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
	var result strings.Builder
	for name, values := range headers {
		for _, value := range values {
			result.WriteString(fmt.Sprintf("%s: %s\n", name, value))
		}
	}
	return result.String()
}
