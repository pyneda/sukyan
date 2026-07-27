package cmd

import "testing"

// A definition loaded from a file inherits "" from the OpenAPI loader (which refuses
// non-HTTP schemes) or "file://" from the GraphQL and WSDL ones (which take the source
// URL's origin verbatim). Both produce a scan that is accepted and sends nothing.
func TestIsRequestableBaseURL(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{"http://127.0.0.1:8080", true},
		{"https://api.example.com/v1", true},
		{"", false},
		{"file://", false},
		{"file:///tmp/spec.json", false},
		{"ftp://files.example.com", false},
		{"ws://api.example.com", false},
		{"/api/v3", false},
		{"api.example.com", false},
		{"://bad", false},
	}

	for _, tt := range tests {
		if got := isRequestableBaseURL(tt.raw); got != tt.want {
			t.Errorf("isRequestableBaseURL(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}
