package openapi

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func contentOf(types ...string) openapi3.Content {
	content := openapi3.Content{}
	for _, mediaType := range types {
		content[mediaType] = openapi3.NewMediaType()
	}
	return content
}

func TestSelectBodyContentType(t *testing.T) {
	tests := []struct {
		name   string
		offers []string
		want   string
	}{
		{"exact json beats everything", []string{"application/xml", "application/json"}, "application/json"},
		{"vendor json beats xml", []string{"application/xml", "application/vnd.api+json"}, "application/vnd.api+json"},
		{"form beats an opaque blob", []string{"application/x-www-form-urlencoded", "application/octet-stream"}, "application/x-www-form-urlencoded"},
		{"multipart beats xml", []string{"multipart/form-data", "application/xml"}, "multipart/form-data"},
		{"xml beats an unknown type", []string{"application/xml", "application/cbor"}, "application/xml"},
		{"xml family resolves smallest", []string{"text/xml", "application/xml"}, "application/xml"},
		{"text beats an unknown type", []string{"text/plain", "application/cbor"}, "text/plain"},
		{"unknown types resolve smallest", []string{"application/cbor", "application/protobuf"}, "application/cbor"},
		{"no content selects nothing", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := contentOf(tt.offers...)
			// Repeated because Go randomizes map iteration order: an unstable choice
			// makes both the body parameters and the Content-Type header vary per run.
			for i := 0; i < 50; i++ {
				if got := SelectBodyContentType(content); got != tt.want {
					t.Fatalf("iteration %d: got %q, want %q", i, got, tt.want)
				}
			}
		})
	}
}
