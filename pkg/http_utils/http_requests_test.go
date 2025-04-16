package http_utils

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/url"
	"testing"

	"github.com/pyneda/sukyan/db"
)

func TestBuildRequestFromHistoryItem(t *testing.T) {
	testCases := []struct {
		name        string
		historyItem *db.History
		expected    *http.Request
		expectError bool
	}{
		{
			name: "Simple GET request",
			historyItem: &db.History{
				Method:     "GET",
				URL:        "https://example.com",
				RawRequest: []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"),
			},
			expected:    &http.Request{Method: "GET", URL: mustParseURL("https://example.com"), Body: io.NopCloser(bytes.NewReader([]byte{}))},
			expectError: false,
		},
		{
			name: "POST request with body",
			historyItem: &db.History{
				Method:     "POST",
				URL:        "https://example.com",
				RawRequest: []byte("POST / HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/json\r\n\r\ntest body"),
			},
			expected:    &http.Request{Method: "POST", URL: mustParseURL("https://example.com"), Body: io.NopCloser(bytes.NewReader([]byte("test body")))},
			expectError: false,
		},
		{
			name: "GET request with empty body",
			historyItem: &db.History{
				Method:     "GET",
				URL:        "https://example.com",
				RawRequest: []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"),
			},
			expected:    &http.Request{Method: "GET", URL: mustParseURL("https://example.com"), Body: io.NopCloser(bytes.NewReader([]byte{}))},
			expectError: false,
		},
		{
			name: "Another GET request example",
			historyItem: &db.History{
				Method:     "GET",
				URL:        "https://example.com",
				RawRequest: []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"),
			},
			expected:    &http.Request{Method: "GET", URL: mustParseURL("https://example.com"), Body: io.NopCloser(bytes.NewReader([]byte{}))},
			expectError: false,
		},
		{
			name: "POST request with query parameters",
			historyItem: &db.History{
				Method:     "POST",
				URL:        "https://example.com/path?query=value",
				RawRequest: []byte("POST /path?query=value HTTP/1.1\r\nHost: example.com\r\n\r\ntest body"),
			},
			expected:    &http.Request{Method: "POST", URL: mustParseURL("https://example.com/path?query=value"), Body: io.NopCloser(bytes.NewReader([]byte("test body")))},
			expectError: false,
		},
		{
			name: "PUT request",
			historyItem: &db.History{
				Method:     "PUT",
				URL:        "https://example.com",
				RawRequest: []byte("PUT / HTTP/1.1\r\nHost: example.com\r\n\r\ntest body"),
			},
			expected:    &http.Request{Method: "PUT", URL: mustParseURL("https://example.com"), Body: io.NopCloser(bytes.NewReader([]byte("test body")))},
			expectError: false,
		},
		{
			name: "DELETE request",
			historyItem: &db.History{
				Method:     "DELETE",
				URL:        "https://example.com",
				RawRequest: []byte("DELETE / HTTP/1.1\r\nHost: example.com\r\n\r\ntest body"),
			},
			expected:    &http.Request{Method: "DELETE", URL: mustParseURL("https://example.com"), Body: io.NopCloser(bytes.NewReader([]byte("test body")))},
			expectError: false,
		},
		{
			name: "HEAD request",
			historyItem: &db.History{
				Method:     "HEAD",
				URL:        "https://example.com",
				RawRequest: []byte("HEAD / HTTP/1.1\r\nHost: example.com\r\n\r\ntest body"),
			},
			expected:    &http.Request{Method: "HEAD", URL: mustParseURL("https://example.com"), Body: io.NopCloser(bytes.NewReader([]byte("test body")))},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildRequestFromHistoryItem(tc.historyItem)
			if (err != nil) != tc.expectError {
				t.Errorf("BuildRequestFromHistoryItem(%v) error: got %v, expectError %v", tc.historyItem, err, tc.expectError)
			}

			if err == nil {
				if got.Method != tc.expected.Method {
					t.Errorf("expected method %s, got %s", tc.expected.Method, got.Method)
				}

				if got.URL.String() != tc.expected.URL.String() {
					t.Errorf("expected URL %s, got %s", tc.expected.URL.String(), got.URL.String())
				}

				// Check body
				if tc.expected.Body == nil {
					if got.Body != nil {
						t.Errorf("expected nil body, got non-nil body")
					}
				} else {
					if got.Body == nil {
						t.Errorf("expected non-nil body, got nil body")
					} else {
						gotBody, _ := io.ReadAll(got.Body)
						got.Body.Close()

						expectedBody, _ := io.ReadAll(tc.expected.Body)
						tc.expected.Body.Close()

						if string(gotBody) != string(expectedBody) {
							t.Errorf("expected body %q, got %q", string(expectedBody), string(gotBody))
						}
					}
				}
			}
		})
	}
}

func mustParseURL(u string) *url.URL {
	parsed, err := url.Parse(u)
	if err != nil {
		log.Fatal(err)
	}
	return parsed
}
