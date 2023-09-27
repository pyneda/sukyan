package http_utils

import (
	"encoding/json"
	"github.com/pyneda/sukyan/db"
	"net/http"
	"reflect"
	"testing"
)

func TestSetRequestHeadersFromHistoryItem(t *testing.T) {
	request, _ := http.NewRequest("GET", "http://example.com", nil)

	// Add some initial headers to the request
	initialHeaders := map[string]string{
		"Initial-Header-1": "Initial Value 1",
		"Initial-Header-2": "Initial Value 2",
	}
	for key, value := range initialHeaders {
		request.Header.Set(key, value)
	}

	headers := RequestHeaders{
		"Content-Type": []string{"application/json"},
		"User-Agent":   []string{"test-agent"},
		"X-Test":       []string{"AAAAAAAAAAAAAAA"},
	}
	headersBytes, _ := json.Marshal(headers)

	historyItem := &db.History{
		RequestHeaders: headersBytes,
	}

	err := SetRequestHeadersFromHistoryItem(request, historyItem)
	if err != nil {
		t.Errorf("Expected no error, but got %v", err)
	}

	// Now we check if the headers were correctly set
	for key, values := range headers {
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

	// Test the error scenario
	badHistoryItem := &db.History{
		RequestHeaders: []byte("{"),
	}
	err = SetRequestHeadersFromHistoryItem(request, badHistoryItem)
	if err == nil {
		t.Errorf("Expected error due to bad JSON, but got none")
	}
}
