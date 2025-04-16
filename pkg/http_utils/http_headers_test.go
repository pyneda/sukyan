package http_utils

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/pyneda/sukyan/db"
)

func TestSetRequestHeadersFromHistoryItem(t *testing.T) {
	request, _ := http.NewRequest("GET", "http://example.com", nil)

	initialHeaders := map[string]string{
		"Initial-Header-1": "Initial Value 1",
		"Initial-Header-2": "Initial Value 2",
	}
	for key, value := range initialHeaders {
		request.Header.Set(key, value)
	}

	// Create a raw HTTP request with headers
	rawRequest := []byte("GET /path HTTP/1.1\r\nContent-Type: application/json\r\nUser-Agent: test-agent\r\nX-Test: AAAAAAAAAAAAAAA\r\n\r\nSome body content")

	historyItem := &db.History{
		RawRequest: rawRequest,
	}

	SetRequestHeadersFromHistoryItem(request, historyItem)

	// Expected headers from the raw request
	expectedHeaders := map[string][]string{
		"Content-Type": {"application/json"},
		"User-Agent":   {"test-agent"},
		"X-Test":       {"AAAAAAAAAAAAAAA"},
	}

	// Now we check if the headers were correctly set
	for key, values := range expectedHeaders {
		for _, value := range values {
			if !reflect.DeepEqual(request.Header.Values(key), []string{value}) {
				t.Errorf("Expected header %s to be set to %s, but got %v", key, value, request.Header.Values(key))
			}
		}
	}

	// Check if the initial headers are still present
	for key, value := range initialHeaders {
		if !reflect.DeepEqual(request.Header.Values(key), []string{value}) {
			t.Errorf("Expected initial header %s to still be set to %s, but got %v", key, value, request.Header.Values(key))
		}
	}

	// Test scenario with malformed raw request, where headers cannot be parsed and should stay intact
	badRequest, _ := http.NewRequest("GET", "http://example.com", nil)
	badHistoryItem := &db.History{
		RawRequest: []byte("This is not a valid HTTP request"),
	}

	SetRequestHeadersFromHistoryItem(badRequest, badHistoryItem)

	for key, value := range initialHeaders {
		badRequest.Header.Set(key, value)
	}

	SetRequestHeadersFromHistoryItem(badRequest, badHistoryItem)

	for key, value := range initialHeaders {
		if badRequest.Header.Get(key) != value {
			t.Errorf("Headers should remain intact when parsing fails. Expected %s: %s, got: %s",
				key, value, badRequest.Header.Get(key))
		}
	}
}
