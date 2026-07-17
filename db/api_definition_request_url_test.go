package db

import "testing"

func TestAPIDefinitionRequestURL(t *testing.T) {
	tests := []struct {
		name      string
		sourceURL string
		baseURL   string
		expected  string
	}{
		{
			name:      "prefers SourceURL which holds the endpoint path",
			sourceURL: "http://127.0.0.1:18000/graphql",
			baseURL:   "http://127.0.0.1:18000",
			expected:  "http://127.0.0.1:18000/graphql",
		},
		{
			name:      "falls back to BaseURL when SourceURL is empty",
			sourceURL: "",
			baseURL:   "http://127.0.0.1:18000",
			expected:  "http://127.0.0.1:18000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := APIDefinition{SourceURL: tt.sourceURL, BaseURL: tt.baseURL}
			if got := d.RequestURL(); got != tt.expected {
				t.Errorf("RequestURL() = %q, want %q", got, tt.expected)
			}
		})
	}
}
