package api

import (
	"strings"
	"testing"

	"github.com/pyneda/sukyan/db"
)

func TestAdHocScanTitle(t *testing.T) {
	tests := []struct {
		name  string
		items []db.History
		want  string
	}{
		{
			name:  "no items",
			items: nil,
			want:  "Ad-hoc scan",
		},
		{
			name:  "single item uses method, path and host",
			items: []db.History{{Method: "GET", URL: "http://127.0.0.1:41733/protected"}},
			want:  "GET /protected — 127.0.0.1:41733",
		},
		{
			name:  "single item without a path falls back to root",
			items: []db.History{{Method: "POST", URL: "https://example.com"}},
			want:  "POST / — example.com",
		},
		{
			name:  "single item drops the query string",
			items: []db.History{{Method: "GET", URL: "https://example.com/search?q=1&page=2"}},
			want:  "GET /search — example.com",
		},
		{
			name:  "unparseable url falls back to the raw value",
			items: []db.History{{Method: "GET", URL: "not a url"}},
			want:  "GET not a url",
		},
		{
			name: "many items on one host",
			items: []db.History{
				{Method: "GET", URL: "https://example.com/a"},
				{Method: "GET", URL: "https://example.com/b"},
				{Method: "POST", URL: "https://example.com/c"},
			},
			want: "3 requests — example.com",
		},
		{
			name: "many items across hosts",
			items: []db.History{
				{Method: "GET", URL: "https://example.com/a"},
				{Method: "GET", URL: "https://other.com/b"},
				{Method: "GET", URL: "https://third.com/c"},
			},
			want: "3 requests — 3 hosts",
		},
		{
			name: "ports make hosts distinct",
			items: []db.History{
				{Method: "GET", URL: "http://127.0.0.1:8080/a"},
				{Method: "GET", URL: "http://127.0.0.1:9090/b"},
			},
			want: "2 requests — 2 hosts",
		},
		{
			name: "many items with no parseable host",
			items: []db.History{
				{Method: "GET", URL: "not a url"},
				{Method: "GET", URL: "also not a url"},
			},
			want: "2 requests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := adHocScanTitle(tt.items); got != tt.want {
				t.Errorf("adHocScanTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAdHocScanTitleTruncatesToColumnSize(t *testing.T) {
	items := []db.History{{Method: "GET", URL: "https://example.com/" + strings.Repeat("a", 400)}}

	got := adHocScanTitle(items)

	if runes := []rune(got); len(runes) != maxScanTitleLength {
		t.Fatalf("adHocScanTitle() length = %d, want %d", len(runes), maxScanTitleLength)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("adHocScanTitle() = %q, want it to end with an ellipsis", got)
	}
}
