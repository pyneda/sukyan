package discovery

import (
	"strings"
	"testing"

	"github.com/pyneda/sukyan/db"
)

func TestIsCICDBuildFileValidationFuncContentType(t *testing.T) {
	const body = "stages:\n  - build\n  - deploy\nbuild:\n  script:\n    - make all\n"

	tests := []struct {
		name               string
		contentType        string
		wantMatch          bool
		wantConfidence     int
		wantContentTypeHit bool
	}{
		{
			name:               "bare yaml",
			contentType:        "text/yaml",
			wantMatch:          true,
			wantConfidence:     80,
			wantContentTypeHit: true,
		},
		{
			name:               "yaml with charset parameter",
			contentType:        "text/yaml; charset=utf-8",
			wantMatch:          true,
			wantConfidence:     80,
			wantContentTypeHit: true,
		},
		{
			name:               "yaml uppercase",
			contentType:        "TEXT/YAML",
			wantMatch:          true,
			wantConfidence:     80,
			wantContentTypeHit: true,
		},
		{
			name:               "json with charset parameter",
			contentType:        "application/json; charset=utf-8",
			wantMatch:          true,
			wantConfidence:     60,
			wantContentTypeHit: true,
		},
		{
			name:               "plain text with charset parameter",
			contentType:        "text/plain;charset=UTF-8",
			wantMatch:          true,
			wantConfidence:     50,
			wantContentTypeHit: true,
		},
		{
			name:               "octet stream is not a known configuration type",
			contentType:        "application/octet-stream",
			wantMatch:          false,
			wantConfidence:     30,
			wantContentTypeHit: false,
		},
		{
			name:               "missing content type",
			contentType:        "",
			wantMatch:          false,
			wantConfidence:     30,
			wantContentTypeHit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history := &db.History{
				StatusCode:          200,
				ResponseContentType: tt.contentType,
				URL:                 "https://example.com/.gitlab-ci.yml",
				RawResponse:         []byte("HTTP/1.1 200 OK\r\nContent-Type: " + tt.contentType + "\r\n\r\n" + body),
			}

			matched, details, confidence := IsCICDBuildFileValidationFunc(history, nil)

			if matched != tt.wantMatch {
				t.Errorf("match = %v, want %v (details: %q)", matched, tt.wantMatch, details)
			}
			if confidence != tt.wantConfidence {
				t.Errorf("confidence = %d, want %d", confidence, tt.wantConfidence)
			}
			gotContentTypeHit := strings.Contains(details, "Content-Type indicates configuration format")
			if gotContentTypeHit != tt.wantContentTypeHit {
				t.Errorf("content-type bonus applied = %v, want %v (details: %q)", gotContentTypeHit, tt.wantContentTypeHit, details)
			}
		})
	}
}
