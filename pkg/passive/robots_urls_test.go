package passive

import (
	"net/url"
	"sort"
	"testing"
)

func TestIsRobotsTxtURL(t *testing.T) {
	tests := map[string]bool{
		"http://example.com/robots.txt":        true,
		"https://example.com/robots.txt":       true,
		"http://example.com/robots.txt/":       true,
		"http://example.com/ROBOTS.TXT":        true,
		"http://example.com/path/robots.txt":   false,
		"http://example.com/robots.txt.backup": false,
		"http://example.com/sitemap.xml":       false,
		"http://example.com/":                  false,
	}
	for u, want := range tests {
		if got := isRobotsTxtURL(u); got != want {
			t.Errorf("isRobotsTxtURL(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestExtractURLsFromRobotsTxt(t *testing.T) {
	base, _ := url.Parse("http://example.com/robots.txt")
	tests := []struct {
		name    string
		body    string
		wantWeb []string
	}{
		{
			name:    "disallow directive",
			body:    "User-agent: *\nDisallow: /admin/secret.found",
			wantWeb: []string{"http://example.com/admin/secret.found"},
		},
		{
			name: "multiple disallow and allow",
			body: "User-agent: *\nDisallow: /admin/\nAllow: /admin/public\nDisallow: /backup",
			wantWeb: []string{
				"http://example.com/admin/",
				"http://example.com/admin/public",
				"http://example.com/backup",
			},
		},
		{
			name:    "sitemap absolute url",
			body:    "Sitemap: https://example.com/sitemap-index.xml",
			wantWeb: []string{"https://example.com/sitemap-index.xml"},
		},
		{
			name:    "root-only disallow is skipped",
			body:    "User-agent: *\nDisallow: /",
			wantWeb: []string{},
		},
		{
			name:    "empty disallow is skipped",
			body:    "User-agent: *\nDisallow:",
			wantWeb: []string{},
		},
		{
			name:    "wildcard paths are skipped",
			body:    "Disallow: /*.php$\nDisallow: /search?q=*",
			wantWeb: []string{},
		},
		{
			name:    "comments and blank lines ignored",
			body:    "# comment\n\nUser-agent: *\nDisallow: /private # inline comment\n",
			wantWeb: []string{"http://example.com/private"},
		},
		{
			name:    "crawl-delay and other directives ignored",
			body:    "User-agent: *\nCrawl-delay: 10\nHost: example.com",
			wantWeb: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractURLsFromRobotsTxt(tt.body, base)
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
