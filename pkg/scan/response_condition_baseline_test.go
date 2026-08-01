package scan

import (
	"context"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
)

func historyWithHeaders(headers, body string) *db.History {
	return &db.History{
		RawResponse: []byte("HTTP/1.1 200 OK\r\n" + headers + "\r\n\r\n" + body),
	}
}

func TestBaselineContainsMarker(t *testing.T) {
	tests := []struct {
		name     string
		original *db.History
		part     generation.ResponseContainsPart
		marker   string
		want     bool
	}{
		{
			// The graphql-api false positives: every response from a debug-mode
			// Python app carries the sqlite3 module name in its traceback, so the
			// sqli-error-sqlite marker fired on parameterised queries.
			name:     "fingerprint already in baseline body is suppressed",
			original: historyWithBody(`{"errors":[{"extensions":{"exception":{"traceback":["sqlite3.IntegrityError"]}}}]}`),
			part:     generation.Body,
			marker:   "sqlite3",
			want:     true,
		},
		{
			name:     "clean baseline keeps the finding",
			original: historyWithBody(`{"data":{"updateProfile":{"id":"1"}}}`),
			part:     generation.Body,
			marker:   "sqlite3",
			want:     false,
		},
		{
			// Markers rendered from the payload are unique per request, so the
			// guard must never touch reflection-style templates.
			name:     "payload-derived marker is not in the baseline",
			original: historyWithBody("<html>welcome</html>"),
			part:     generation.Body,
			marker:   "st4r7stest3nd991817",
			want:     false,
		},
		{
			name:     "nil baseline must not suppress",
			original: nil,
			part:     generation.Body,
			marker:   "sqlite3",
			want:     false,
		},
		{
			name:     "empty marker must not suppress",
			original: historyWithBody("sqlite3"),
			part:     generation.Body,
			marker:   "",
			want:     false,
		},
		{
			name:     "header marker compared against baseline headers",
			original: historyWithHeaders("X-Powered-By: Express", "body"),
			part:     generation.Headers,
			marker:   "Express",
			want:     true,
		},
		{
			// A body marker must never be satisfied by a header and vice versa,
			// or the guard would suppress findings it has no evidence about.
			name:     "header marker is not matched from the body",
			original: historyWithHeaders("Content-Type: text/html", "Express"),
			part:     generation.Headers,
			marker:   "Express",
			want:     false,
		},
		{
			name:     "raw part sees status line and headers",
			original: historyWithHeaders("Server: uvicorn", "body"),
			part:     generation.Raw,
			marker:   "uvicorn",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := baselineContainsMarker(tt.original, tt.part, tt.marker); got != tt.want {
				t.Errorf("baselineContainsMarker() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestResponseConditionIgnoresBaselineMarker covers the sqli-error-sqlite marker
// that fired at confidence 95 on parameterised queries: a debug traceback names
// the sqlite3 module whatever the payload was.
func TestResponseConditionIgnoresBaselineMarker(t *testing.T) {
	scanner := &TemplateScanner{}
	method := generation.DetectionMethod{
		ResponseCondition: &generation.ResponseConditionDetectionMethod{
			Contains:   "sqlite3",
			Part:       generation.Body,
			Confidence: 95,
		},
	}
	const traceback = `{"errors":[{"extensions":{"exception":{"traceback":["sqlite3.IntegrityError: FOREIGN KEY constraint failed"]}}}]}`

	matched, _, _, _, err := scanner.EvaluateDetectionMethod(context.Background(),
		resultWithBodies(traceback, traceback), method)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched {
		t.Error("marker present in the baseline must not raise a finding")
	}

	matched, _, confidence, _, err := scanner.EvaluateDetectionMethod(context.Background(),
		resultWithBodies(`{"data":{"updateProfile":{"id":"1"}}}`, traceback), method)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matched {
		t.Fatal("a marker absent from the baseline is still evidence")
	}
	if confidence != 95 {
		t.Errorf("confidence = %d, want 95", confidence)
	}
}
