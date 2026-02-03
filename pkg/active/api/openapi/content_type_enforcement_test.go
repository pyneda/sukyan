package openapi

import (
	"testing"
)

func TestIsContentTypeDeclared(t *testing.T) {
	audit := &ContentTypeEnforcementAudit{}

	tests := []struct {
		name        string
		contentType string
		declared    []string
		expected    bool
	}{
		{
			name:        "exact match",
			contentType: "application/xml",
			declared:    []string{"application/xml"},
			expected:    true,
		},
		{
			name:        "xml equivalence: application/xml declared, testing text/xml",
			contentType: "text/xml",
			declared:    []string{"application/xml"},
			expected:    true,
		},
		{
			name:        "xml equivalence: text/xml declared, testing application/xml",
			contentType: "application/xml",
			declared:    []string{"text/xml"},
			expected:    true,
		},
		{
			name:        "json equivalence: application/json declared, testing text/json",
			contentType: "text/json",
			declared:    []string{"application/json"},
			expected:    true,
		},
		{
			name:        "json equivalence: text/json declared, testing application/json",
			contentType: "application/json",
			declared:    []string{"text/json"},
			expected:    true,
		},
		{
			name:        "javascript equivalence: application/javascript declared, testing text/javascript",
			contentType: "text/javascript",
			declared:    []string{"application/javascript"},
			expected:    true,
		},
		{
			name:        "no match: json declared, testing xml",
			contentType: "text/xml",
			declared:    []string{"application/json"},
			expected:    false,
		},
		{
			name:        "no match: form-urlencoded has no equivalents",
			contentType: "application/x-www-form-urlencoded",
			declared:    []string{"application/json"},
			expected:    false,
		},
		{
			name:        "declared with charset parameter",
			contentType: "text/xml",
			declared:    []string{"application/xml; charset=utf-8"},
			expected:    true,
		},
		{
			name:        "test content type with charset parameter",
			contentType: "text/xml; charset=utf-8",
			declared:    []string{"application/xml"},
			expected:    true,
		},
		{
			name:        "case insensitive match",
			contentType: "TEXT/XML",
			declared:    []string{"Application/XML"},
			expected:    true,
		},
		{
			name:        "multiple declared types with equivalent match",
			contentType: "text/xml",
			declared:    []string{"application/json", "application/xml"},
			expected:    true,
		},
		{
			name:        "multiple declared types with no match",
			contentType: "text/xml",
			declared:    []string{"application/json", "multipart/form-data"},
			expected:    false,
		},
		{
			name:        "empty declared list",
			contentType: "application/xml",
			declared:    []string{},
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := audit.isContentTypeDeclared(tt.contentType, tt.declared)
			if result != tt.expected {
				t.Errorf("isContentTypeDeclared(%q, %v) = %v, want %v",
					tt.contentType, tt.declared, result, tt.expected)
			}
		})
	}
}
