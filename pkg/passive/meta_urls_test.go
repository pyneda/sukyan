package passive

import (
	"sort"
	"testing"
)

func TestExtractURLsFromMetaTags(t *testing.T) {
	base := "http://example.com/section/page.html"
	tests := []struct {
		name    string
		body    string
		wantWeb []string
	}{
		{
			name:    "meta refresh with relative url and no quote",
			body:    `<meta http-equiv="refresh" content="10; url=/next/redirect.found">`,
			wantWeb: []string{"http://example.com/next/redirect.found"},
		},
		{
			name:    "meta refresh with absolute url",
			body:    `<meta http-equiv="refresh" content="0; url=https://other.example.org/dest">`,
			wantWeb: []string{"https://other.example.org/dest"},
		},
		{
			name:    "meta refresh case insensitive http-equiv",
			body:    `<meta http-equiv="REFRESH" content="5;URL='/relative.html'">`,
			wantWeb: []string{"http://example.com/relative.html"},
		},
		{
			name:    "csp report-uri directive",
			body:    `<meta http-equiv="Content-Security-Policy" content="script-src 'self'; report-uri /csp/report.found">`,
			wantWeb: []string{"http://example.com/csp/report.found"},
		},
		{
			name:    "csp report-to directive",
			body:    `<meta http-equiv="content-security-policy" content="default-src 'self'; report-to /csp/reportto.found">`,
			wantWeb: []string{"http://example.com/csp/reportto.found"},
		},
		{
			name:    "meta refresh without url is ignored",
			body:    `<meta http-equiv="refresh" content="5">`,
			wantWeb: []string{},
		},
		{
			name:    "non http-equiv meta not parsed for refresh/csp url= syntax",
			body:    `<meta name="generator" content="refresh url=/should/not/extract">`,
			wantWeb: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractAndAnalyzeURLS(tt.body, base)
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
