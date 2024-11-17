package active

import (
	"fmt"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/stretchr/testify/assert"
)

func TestIsJsonpResponse(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     bool
	}{
		{
			name:     "valid JSONP with function name",
			response: `callback({"data": "test"})`,
			want:     true,
		},
		{
			name:     "valid JSONP with array",
			response: `myFunction([1,2,3])`,
			want:     true,
		},
		{
			name:     "valid JSONP with trailing semicolon",
			response: `jsonp({"key": "value"});`,
			want:     true,
		},
		{
			name:     "invalid JSONP - no function",
			response: `{"data": "test"}`,
			want:     false,
		},
		{
			name:     "invalid JSONP - invalid JSON",
			response: `callback({data: test})`,
			want:     false,
		},
		{
			name:     "invalid JSONP - function name contains parentheses",
			response: `my(function)({"data": "test"})`,
			want:     false,
		},
		{
			name:     "invalid JSONP - empty function name",
			response: `({"data": "test"})`,
			want:     false,
		},
		{
			name:     "invalid JSONP - malformed",
			response: `callback{"data": "test"}`,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isJsonpResponse(tt.response)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHasJsonpParameter(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{
			name:     "exact match callback parameter",
			url:      "https://example.com/api?callback=test",
			expected: true,
		},
		{
			name:     "exact match jsonp parameter",
			url:      "https://example.com/api?jsonp=func",
			expected: true,
		},
		{
			name:     "substring match",
			url:      "https://example.com/api?mycallback=test",
			expected: true,
		},
		{
			name:     "case insensitive match",
			url:      "https://example.com/api?JSONPCALLBACK=test",
			expected: true,
		},
		{
			name:     "no JSONP parameter",
			url:      "https://example.com/api?param=value",
			expected: false,
		},
		{
			name:     "empty parameter",
			url:      "https://example.com/api?callback=",
			expected: true,
		},
		{
			name:     "multiple parameters with JSONP",
			url:      "https://example.com/api?param=value&callback=test&other=value",
			expected: true,
		},
		{
			name:     "invalid URL",
			url:      "not-a-url",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history := &db.History{
				URL: tt.url,
			}
			got := hasJsonpParameter(history)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestGetCallbacksForMode(t *testing.T) {
	tests := []struct {
		name           string
		mode           options.ScanMode
		hasJsonParam   bool
		expectedCount  int
		shouldContain  []string
		shouldNotMatch func([]string) bool
	}{
		{
			name:          "fuzz mode always returns all",
			mode:          options.ScanModeFuzz,
			hasJsonParam:  false,
			expectedCount: len(jsonpCallbackParameters),
			shouldContain: []string{"callback", "jsonp", "jquery"},
		},
		{
			name:          "fuzz mode with param returns all",
			mode:          options.ScanModeFuzz,
			hasJsonParam:  true,
			expectedCount: len(jsonpCallbackParameters),
			shouldContain: []string{"callback", "jsonp", "jquery"},
		},
		{
			name:          "smart mode without param returns top 5",
			mode:          options.ScanModeSmart,
			hasJsonParam:  false,
			expectedCount: 5,
			shouldContain: []string{"callback", "jsonp"},
		},
		{
			name:          "smart mode with param returns all",
			mode:          options.ScanModeSmart,
			hasJsonParam:  true,
			expectedCount: len(jsonpCallbackParameters),
			shouldContain: []string{"callback", "jsonp", "jquery"},
		},
		{
			name:          "fast mode without param returns top 2",
			mode:          options.ScanModeFast,
			hasJsonParam:  false,
			expectedCount: 2,
			shouldContain: []string{"callback"},
			shouldNotMatch: func(result []string) bool {
				return len(result) > 2
			},
		},
		{
			name:          "fast mode with param returns all",
			mode:          options.ScanModeFast,
			hasJsonParam:  true,
			expectedCount: len(jsonpCallbackParameters),
			shouldContain: []string{"callback", "jsonp", "jquery"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCallbacksForMode(tt.mode, tt.hasJsonParam)

			assert.Equal(t, tt.expectedCount, len(result), "unexpected number of callbacks")

			for _, expected := range tt.shouldContain {
				assert.Contains(t, result, expected, fmt.Sprintf("should contain %s", expected))
			}

			if tt.shouldNotMatch != nil {
				assert.False(t, tt.shouldNotMatch(result), "result matches unwanted condition")
			}
		})
	}
}
