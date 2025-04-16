package lib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitHTTPMessage(t *testing.T) {
	tests := []struct {
		name          string
		message       []byte
		expectedHead  []byte
		expectedBody  []byte
		expectedError bool
	}{
		{
			name:         "CRLF Standard HTTP Message",
			message:      []byte("GET / HTTP/1.1\r\nHost: example.com\r\nAccept: */*\r\n\r\nBody content"),
			expectedHead: []byte("GET / HTTP/1.1\r\nHost: example.com\r\nAccept: */*"),
			expectedBody: []byte("Body content"),
		},
		{
			name:         "LF Only HTTP Message",
			message:      []byte("GET / HTTP/1.1\nHost: example.com\nAccept: */*\n\nBody content"),
			expectedHead: []byte("GET / HTTP/1.1\nHost: example.com\nAccept: */*"),
			expectedBody: []byte("Body content"),
		},
		{
			name:          "Invalid HTTP Message",
			message:       []byte("This is not a valid HTTP message"),
			expectedError: true,
		},
		{
			name:         "Empty Body",
			message:      []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"),
			expectedHead: []byte("GET / HTTP/1.1\r\nHost: example.com"),
			expectedBody: []byte(""),
		},
		{
			name:         "Response Message",
			message:      []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<html>Content</html>"),
			expectedHead: []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html"),
			expectedBody: []byte("<html>Content</html>"),
		},
		{
			name:         "Message with JSON Body",
			message:      []byte("POST /api HTTP/1.1\r\nContent-Type: application/json\r\n\r\n{\"key\":\"value\"}"),
			expectedHead: []byte("POST /api HTTP/1.1\r\nContent-Type: application/json"),
			expectedBody: []byte("{\"key\":\"value\"}"),
		},
		{
			name:         "Mixed CRLF and LF",
			message:      []byte("GET / HTTP/1.1\r\nHost: example.com\nAccept: */*\r\n\r\nBody content"),
			expectedHead: []byte("GET / HTTP/1.1\r\nHost: example.com\nAccept: */*"),
			expectedBody: []byte("Body content"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers, body, err := SplitHTTPMessage(tt.message)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedHead, headers)
				assert.Equal(t, tt.expectedBody, body)
			}
		})
	}
}

func TestParseHTTPHeaders(t *testing.T) {
	tests := []struct {
		name           string
		headerBytes    []byte
		expectedOutput map[string][]string
		expectedError  bool
	}{
		{
			name:        "Simple HTTP Headers with CRLF",
			headerBytes: []byte("GET / HTTP/1.1\r\nHost: example.com\r\nAccept: */*\r\nAccept-Encoding: gzip"),
			expectedOutput: map[string][]string{
				"Host":            {"example.com"},
				"Accept":          {"*/*"},
				"Accept-Encoding": {"gzip"},
			},
		},
		{
			name:        "Simple HTTP Headers with LF",
			headerBytes: []byte("GET / HTTP/1.1\nHost: example.com\nAccept: */*\nAccept-Encoding: gzip"),
			expectedOutput: map[string][]string{
				"Host":            {"example.com"},
				"Accept":          {"*/*"},
				"Accept-Encoding": {"gzip"},
			},
		},
		{
			name:        "HTTP Headers with Multiple Values",
			headerBytes: []byte("GET / HTTP/1.1\r\nAccept: text/html\r\nAccept: application/json\r\nHost: example.com"),
			expectedOutput: map[string][]string{
				"Host":   {"example.com"},
				"Accept": {"text/html", "application/json"},
			},
		},
		{
			name:           "Empty Headers",
			headerBytes:    []byte(""),
			expectedOutput: map[string][]string{},
		},
		{
			name:        "Malformed Header Line",
			headerBytes: []byte("GET / HTTP/1.1\r\nInvalid-Header\r\nHost: example.com"),
			expectedOutput: map[string][]string{
				"Host": {"example.com"},
			},
		},
		{
			name:        "Headers with Whitespace",
			headerBytes: []byte("GET / HTTP/1.1\r\n  Spaced-Header : value with spaces  \r\nHost: example.com"),
			expectedOutput: map[string][]string{
				"Spaced-Header": {"value with spaces"},
				"Host":          {"example.com"},
			},
		},
		{
			name:        "Headers with Empty Lines",
			headerBytes: []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\nContent-Type: text/html"),
			expectedOutput: map[string][]string{
				"Host":         {"example.com"},
				"Content-Type": {"text/html"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers, err := ParseHTTPHeaders(tt.headerBytes)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedOutput, headers)
			}
		})
	}
}

func TestFormatHeadersAsString(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string][]string
		expectedOutput string
	}{
		{
			name: "Simple Headers",
			headers: map[string][]string{
				"Host":            {"example.com"},
				"Accept":          {"*/*"},
				"Accept-Encoding": {"gzip"},
			},
			expectedOutput: "Accept: */*\nAccept-Encoding: gzip\nHost: example.com\n",
		},
		{
			name: "Headers with Multiple Values",
			headers: map[string][]string{
				"Host":   {"example.com"},
				"Accept": {"text/html", "application/json"},
			},
			expectedOutput: "Accept: text/html\nAccept: application/json\nHost: example.com\n",
		},
		{
			name:           "Empty Headers",
			headers:        map[string][]string{},
			expectedOutput: "",
		},
		{
			name: "Headers with Special Characters",
			headers: map[string][]string{
				"X-Custom-Header": {"value;with:special@chars"},
				"Content-Type":    {"application/json; charset=utf-8"},
			},
			expectedOutput: "Content-Type: application/json; charset=utf-8\nX-Custom-Header: value;with:special@chars\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatHeadersAsString(tt.headers)
			assert.Equal(t, tt.expectedOutput, result)
		})
	}
}
