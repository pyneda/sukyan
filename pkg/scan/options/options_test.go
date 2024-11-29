package options

import (
	"fmt"
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestIsHigherOrEqual(t *testing.T) {
	testCases := []struct {
		a        ScanMode
		b        ScanMode
		expected bool
	}{
		{ScanModeFast, ScanModeFast, true},
		{ScanModeFast, ScanModeSmart, false},
		{ScanModeFast, ScanModeFuzz, false},
		{ScanModeSmart, ScanModeFast, true},
		{ScanModeSmart, ScanModeSmart, true},
		{ScanModeSmart, ScanModeFuzz, false},
		{ScanModeFuzz, ScanModeFast, true},
		{ScanModeFuzz, ScanModeSmart, true},
		{ScanModeFuzz, ScanModeFuzz, true},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			actual := tc.a.IsHigherOrEqual(tc.b)
			if actual != tc.expected {
				t.Errorf("Test failed for a=%s, b=%s. Expected %v but got %v", tc.a, tc.b, tc.expected, actual)
			}
		})
	}
}

func TestFullScanOptionsValidation(t *testing.T) {
	validate := validator.New()
	tests := []struct {
		name    string
		options FullScanOptions
		valid   bool
	}{
		{
			name: "Valid All Fields",
			options: FullScanOptions{
				Title:           "Valid Title",
				StartURLs:       []string{"https://valid.url"},
				MaxDepth:        1,
				MaxPagesToCrawl: 5,
				WorkspaceID:     1,
				PagesPoolSize:   1,
				Headers:         map[string][]string{"key": {"value"}},
				InsertionPoints: []string{"urlpath", "body", "headers", "parameters", "xml", "cookies"},
				Mode:            "fast",
			},
			valid: true,
		},
		{
			name: "No mode / insertion points",
			options: FullScanOptions{
				Title:           "Valid Title",
				StartURLs:       []string{"https://valid.url"},
				MaxDepth:        1,
				MaxPagesToCrawl: 5,
				WorkspaceID:     1,
				PagesPoolSize:   1,
				Headers:         map[string][]string{"key": {"value"}},
			},
			valid: true,
		},
		{
			name: "Invalid Insertion Points",
			options: FullScanOptions{
				Title:           "",
				StartURLs:       []string{"https://valid.url"},
				MaxDepth:        1,
				MaxPagesToCrawl: 5,
				WorkspaceID:     1,
				PagesPoolSize:   1,
				Headers:         map[string][]string{"key": {"value"}},
				InsertionPoints: []string{"url", "body", "header", "asdfasdf"},
				Mode:            "fast",
			},
			valid: false,
		},
		{
			name: "Invalid URL",
			options: FullScanOptions{
				StartURLs: []string{"invalid_url"},
			},
			valid: false,
		},
		{
			name: "Invalid MaxDepth",
			options: FullScanOptions{
				MaxDepth: -1,
			},
			valid: false,
		},
		{
			name: "Invalid MaxPagesToCrawl",
			options: FullScanOptions{
				MaxPagesToCrawl: -1,
			},
			valid: false,
		},
		{
			name: "Invalid WorkspaceID",
			options: FullScanOptions{
				WorkspaceID: 0,
			},
			valid: false,
		},
		{
			name: "Invalid PagesPoolSize",
			options: FullScanOptions{
				PagesPoolSize: 0,
			},
			valid: false,
		},
		{
			name: "Invalid InsertionPoints",
			options: FullScanOptions{
				InsertionPoints: []string{"invalid"},
			},
			valid: false,
		},
		{
			name: "Valid Mode Fast",
			options: FullScanOptions{
				Title:           "Valid Title",
				StartURLs:       []string{"https://valid.url"},
				MaxDepth:        1,
				MaxPagesToCrawl: 5,
				WorkspaceID:     1,
				PagesPoolSize:   1,
				Mode:            "fast",
			},
			valid: true,
		},
		{
			name: "Invalid Mode",
			options: FullScanOptions{
				Mode: "invalid",
			},
			valid: false,
		},
		{
			name: "Valid Mixed AuditCategories",
			options: FullScanOptions{
				Title:       "Valid Title",
				StartURLs:   []string{"https://valid.url"},
				WorkspaceID: 1,
				AuditCategories: AuditCategories{
					ServerSide: false,
					ClientSide: true,
					Passive:    true,
				},
			},
			valid: true,
		},
		{
			name: "Valid All Fields",
			options: FullScanOptions{
				Title:           "Valid Title",
				StartURLs:       []string{"https://valid.url"},
				MaxDepth:        1,
				MaxPagesToCrawl: 5,
				WorkspaceID:     1,
				PagesPoolSize:   1,
				Headers:         map[string][]string{"key": {"value"}},
				InsertionPoints: []string{"urlpath", "body", "headers", "parameters", "xml", "cookies"},
				Mode:            "fast",
				AuditCategories: AuditCategories{
					ServerSide: true,
					ClientSide: true,
					Passive:    true,
				},
			},
			valid: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validate.Struct(test.options)
			if test.valid && err != nil {
				t.Errorf("expected valid but got error: %v", err)
			} else if !test.valid && err == nil {
				t.Errorf("expected error but got valid")
			}
		})
	}
}
