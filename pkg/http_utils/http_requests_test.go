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
		historyItem *db.History
		expected    *http.Request
		expectError bool
	}{
		{
			historyItem: &db.History{Method: "GET", URL: "https://example.com", RequestBody: []byte("")},
			expected:    &http.Request{Method: "GET", URL: mustParseURL("https://example.com"), Body: io.NopCloser(bytes.NewReader([]byte("")))},
			expectError: false,
		},
		{
			historyItem: &db.History{Method: "POST", URL: "https://example.com", RequestBody: []byte("test body")},
			expected:    &http.Request{Method: "POST", URL: mustParseURL("https://example.com"), Body: io.NopCloser(bytes.NewReader([]byte("test body")))},
			expectError: false,
		},
		{
			historyItem: &db.History{Method: "GET", URL: "https://example.com", RequestBody: nil},
			expected:    &http.Request{Method: "GET", URL: mustParseURL("https://example.com"), Body: nil},
			expectError: false,
		},
		{
			historyItem: &db.History{Method: "GET", URL: "https://example.com", RequestBody: []byte{}},
			expected:    &http.Request{Method: "GET", URL: mustParseURL("https://example.com"), Body: io.NopCloser(bytes.NewReader([]byte{}))},
			expectError: false,
		},
		{
			historyItem: &db.History{Method: "POST", URL: "https://example.com/path?query=value", RequestBody: []byte("test body")},
			expected:    &http.Request{Method: "POST", URL: mustParseURL("https://example.com/path?query=value"), Body: io.NopCloser(bytes.NewReader([]byte("test body")))},
			expectError: false,
		},
		{
			historyItem: &db.History{Method: "PUT", URL: "https://example.com", RequestBody: []byte("test body")},
			expected:    &http.Request{Method: "PUT", URL: mustParseURL("https://example.com"), Body: io.NopCloser(bytes.NewReader([]byte("test body")))},
			expectError: false,
		},
		{
			historyItem: &db.History{Method: "DELETE", URL: "https://example.com", RequestBody: []byte("test body")},
			expected:    &http.Request{Method: "DELETE", URL: mustParseURL("https://example.com"), Body: io.NopCloser(bytes.NewReader([]byte("test body")))},
			expectError: false,
		},
		{
			historyItem: &db.History{Method: "HEAD", URL: "https://example.com", RequestBody: []byte("test body")},
			expected:    &http.Request{Method: "HEAD", URL: mustParseURL("https://example.com"), Body: io.NopCloser(bytes.NewReader([]byte("test body")))},
			expectError: false,
		}}

	for _, tc := range testCases {
		got, err := BuildRequestFromHistoryItem(tc.historyItem)
		if (err != nil) != tc.expectError {
			t.Errorf("BuildRequestFromHistoryItem(%v) error: got %v, expectError %v", tc.historyItem, err, tc.expectError)
		}

		if got.Method != tc.expected.Method {
			t.Errorf("expected method %s, got %s", tc.expected.Method, got.Method)
		}

		if got.URL.String() != tc.expected.URL.String() {
			t.Errorf("expected URL %s, got %s", tc.expected.URL.String(), got.URL.String())
		}
		if tc.historyItem.RequestBody != nil {
			tc.expected.Body = io.NopCloser(bytes.NewReader(tc.historyItem.RequestBody))
		} else {
			tc.expected.Body = nil
		}
		if got.Body != nil && tc.expected.Body != nil {
			gotBody, _ := io.ReadAll(got.Body)
			got.Body.Close()

			expectedBody, _ := io.ReadAll(tc.expected.Body)
			tc.expected.Body.Close()

			if string(gotBody) != string(expectedBody) {
				t.Errorf("expected body %s, got %s", string(expectedBody), string(gotBody))
			}
		} else if (got.Body != nil && tc.expected.Body == nil) || (got.Body == nil && tc.expected.Body != nil) {
			t.Errorf("Body mismatch, got: %v, expected: %v", got.Body, tc.expected.Body)
		}

	}
}

func mustParseURL(u string) *url.URL {
	parsed, err := url.Parse(u)
	if err != nil {
		log.Fatal(err)
	}
	return parsed
}
