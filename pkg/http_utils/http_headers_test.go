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
	headers := RequestHeaders{
		"Content-Type": []string{"application/json"},
		"User-Agent":   []string{"test-agent"},
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

	// Test the error scenario
	badHistoryItem := &db.History{
		RequestHeaders: []byte("{"),
	}
	err = SetRequestHeadersFromHistoryItem(request, badHistoryItem)
	if err == nil {
		t.Errorf("Expected error due to bad JSON, but got none")
	}
}
