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

func TestExtractURLsFromHeadersIgnoresNonURLValues(t *testing.T) {
	base := "http://example.com/account/history"
	tests := []struct {
		name    string
		headers map[string][]string
		wantWeb []string
	}{
		{
			name:    "framework banner is not a relative URL",
			headers: map[string][]string{"X-Powered-By": {"Next.js"}},
			wantWeb: []string{},
		},
		{
			name:    "attachment filename is not a relative URL",
			headers: map[string][]string{"Content-Disposition": {`attachment; filename="report.json"`}},
			wantWeb: []string{},
		},
		{
			name:    "server banner is not a relative URL",
			headers: map[string][]string{"Server": {"Werkzeug/2.0.3 Python/3.6.9"}},
			wantWeb: []string{},
		},
		{
			name:    "absolute URL in an arbitrary header is still extracted",
			headers: map[string][]string{"X-Backend": {"https://internal.example.org/api/v1.json"}},
			wantWeb: []string{"https://internal.example.org/api/v1.json"},
		},
		{
			name:    "root relative value in an arbitrary header is still extracted",
			headers: map[string][]string{"X-Backend": {"/internal/api.json"}},
			wantWeb: []string{"http://example.com/internal/api.json"},
		},
		{
			// resolveRelative treats an extension-less base as a directory; RFC 3986
			// would give /account/next-step.html. Pinned as current behaviour, not as intent.
			name:    "relative Location is still extracted",
			headers: map[string][]string{"Location": {"next-step.html"}},
			wantWeb: []string{"http://example.com/account/history/next-step.html"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractURLsFromHeaders(tt.headers, base)
			if len(got.Web) != len(tt.wantWeb) {
				t.Fatalf("web URLs got %v, want %v", got.Web, tt.wantWeb)
			}
			for i := range tt.wantWeb {
				if got.Web[i] != tt.wantWeb[i] {
					t.Errorf("web URLs got %v, want %v", got.Web, tt.wantWeb)
				}
			}
		})
	}
}

func TestExtractURLsFromHeadersPathLikeAndOpaqueValues(t *testing.T) {
	base := "http://example.com/app/p.html"
	tests := []struct {
		name    string
		headers map[string][]string
		wantWeb []string
	}{
		{
			name:    "path relative value in an arbitrary header is extracted",
			headers: map[string][]string{"X-Template": {"views/profile.html"}},
			wantWeb: []string{"http://example.com/app/views/profile.html"},
		},
		{
			name:    "single segment value in an arbitrary header is not a URL",
			headers: map[string][]string{"X-Powered-By": {"Next.js"}},
			wantWeb: []string{},
		},
		{
			name:    "authority-less Link target is normalized",
			headers: map[string][]string{"Link": {`<http:example.com/next.js>; rel="preload"`}},
			wantWeb: []string{"http://example.com/next.js"},
		},
		{
			name:    "authority-less Refresh target is normalized",
			headers: map[string][]string{"Refresh": {`0;url=http:example.com/next`}},
			wantWeb: []string{"http://example.com/next"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractURLsFromHeaders(tt.headers, base)
			if len(got.Web) != len(tt.wantWeb) {
				t.Fatalf("web URLs got %v, want %v", got.Web, tt.wantWeb)
			}
			for i := range tt.wantWeb {
				if got.Web[i] != tt.wantWeb[i] {
					t.Errorf("web URLs got %v, want %v", got.Web, tt.wantWeb)
				}
			}
		})
	}
}
