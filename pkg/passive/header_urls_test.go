package passive

import (
	"sort"
	"testing"
)

func TestExtractURLsFromHeadersKnownFields(t *testing.T) {
	base := "http://example.com/page/"
	tests := []struct {
		name    string
		headers map[string][]string
		wantWeb []string
	}{
		{
			name:    "Link header with angle brackets and rel",
			headers: map[string][]string{"Link": {`</resource/preload.js>; rel="preload"; as="script"`}},
			wantWeb: []string{"http://example.com/resource/preload.js"},
		},
		{
			name:    "Link header with multiple links",
			headers: map[string][]string{"Link": {`</a.css>; rel="preload", </b.js>; rel="preload"`}},
			wantWeb: []string{"http://example.com/a.css", "http://example.com/b.js"},
		},
		{
			name:    "Link header with absolute URL",
			headers: map[string][]string{"Link": {`<https://cdn.example.org/x.js>; rel="preload"`}},
			wantWeb: []string{"https://cdn.example.org/x.js"},
		},
		{
			name:    "Refresh header with url= and no quotes",
			headers: map[string][]string{"Refresh": {`999; url=/next/page.found`}},
			wantWeb: []string{"http://example.com/next/page.found"},
		},
		{
			name:    "Refresh header with quoted url",
			headers: map[string][]string{"Refresh": {`5;URL='/quoted.html'`}},
			wantWeb: []string{"http://example.com/quoted.html"},
		},
		{
			name:    "Refresh header with absolute url",
			headers: map[string][]string{"Refresh": {`0; url=https://other.example.com/dest`}},
			wantWeb: []string{"https://other.example.com/dest"},
		},
		{
			name:    "Location header still extracted via generic path",
			headers: map[string][]string{"Location": {`/redirect/target.found`}},
			wantWeb: []string{"http://example.com/redirect/target.found"},
		},
		{
			name:    "Content-Location header",
			headers: map[string][]string{"Content-Location": {`/content/here.found`}},
			wantWeb: []string{"http://example.com/content/here.found"},
		},
		{
			name:    "unrelated header does not add URLs",
			headers: map[string][]string{"Server": {`nginx/1.2.3`}, "Content-Type": {`text/html`}},
			wantWeb: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractURLsFromHeaders(tt.headers, base)
			gotWeb := append([]string{}, got.Web...)
			sort.Strings(gotWeb)
			want := append([]string{}, tt.wantWeb...)
			sort.Strings(want)
			if len(gotWeb) != len(want) {
				t.Fatalf("web URLs got %v, want %v", gotWeb, want)
			}
			for i := range want {
				if gotWeb[i] != want[i] {
					t.Errorf("web URLs got %v, want %v", gotWeb, want)
				}
			}
		})
	}
}
