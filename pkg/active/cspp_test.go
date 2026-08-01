package active

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// The fragment is never sent to the server, so a hash-sourced pollution is
// invisible unless it is tested. It used to be skipped entirely whenever the
// crawled URL already carried a query string.
func TestCSPPSeparatorsAlwaysIncludeFragment(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want []string
	}{
		{
			name: "bare url",
			url:  "http://example.com/page",
			want: []string{"?", "#"},
		},
		{
			name: "url with query",
			url:  "http://example.com/page?id=1",
			want: []string{"&", "#"},
		},
		{
			name: "url with query and fragment",
			url:  "http://example.com/page?id=1#/route",
			want: []string{"&", "#"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := csppPayloadSeparators(tt.url)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestBuildCSPPTestURL(t *testing.T) {
	const payload = "__proto__[sukyan]=reserved"

	tests := []struct {
		name      string
		base      string
		separator string
		want      string
	}{
		{
			name:      "first query param",
			base:      "http://example.com/page",
			separator: "?",
			want:      "http://example.com/page?" + payload,
		},
		{
			name:      "appended query param",
			base:      "http://example.com/page?id=1",
			separator: "&",
			want:      "http://example.com/page?id=1&" + payload,
		},
		{
			name:      "fragment on a bare url",
			base:      "http://example.com/page",
			separator: "#",
			want:      "http://example.com/page#" + payload,
		},
		{
			name:      "fragment replaces an existing fragment",
			base:      "http://example.com/page#/settings",
			separator: "#",
			want:      "http://example.com/page#" + payload,
		},
		{
			name:      "fragment keeps the query",
			base:      "http://example.com/page?id=1#/settings",
			separator: "#",
			want:      "http://example.com/page?id=1#" + payload,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildCSPPTestURL(tt.base, tt.separator, payload); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// The hijack records the request URL, which never carries the fragment, so a
// lookup keyed on the fragment URL misses and the issue is filed with no URL,
// request or response at all.
func TestStripURLFragment(t *testing.T) {
	tests := []struct{ in, want string }{
		{"http://example.com/page#__proto__[x]=1", "http://example.com/page"},
		{"http://example.com/page?id=1#__proto__[x]=1", "http://example.com/page?id=1"},
		{"http://example.com/page?id=1", "http://example.com/page?id=1"},
		{"http://example.com/page", "http://example.com/page"},
	}

	for _, tt := range tests {
		if got := stripURLFragment(tt.in); got != tt.want {
			t.Errorf("stripURLFragment(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// Navigating between two URLs that differ only in the fragment is a
// same-document navigation: the document is not re-fetched and never re-parses
// its payload, so every fragment payload after the first silently tested
// nothing.
func TestNavigateForPayloadForcesFullLoadBetweenFragments(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)

	var loads int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/c/case" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		atomic.AddInt64(&loads, 1)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><html><body><script>window.parsed = location.hash;</script></body></html>`)
	}))
	defer server.Close()

	page, err := browser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	defer page.Close()

	// Each navigation has to be allowed to commit before the next one, exactly
	// as the audit does it. Back-to-back Navigate calls make Chrome restart the
	// first one, which hides the same-document behaviour being tested here.
	target := server.URL + "/c/case"
	if err := navigateForPayload(page, target+"#first=1", 10*time.Second); err != nil {
		t.Fatalf("first navigation failed: %v", err)
	}
	if err := page.Timeout(10 * time.Second).WaitLoad(); err != nil {
		t.Fatalf("first load failed: %v", err)
	}
	if err := navigateForPayload(page, target+"#second=2", 10*time.Second); err != nil {
		t.Fatalf("second navigation failed: %v", err)
	}
	if err := page.Timeout(10 * time.Second).WaitLoad(); err != nil {
		t.Fatalf("second load failed: %v", err)
	}

	if got := atomic.LoadInt64(&loads); got != 2 {
		t.Errorf("server saw %d document loads, want 2 - the fragment-only navigation did not re-parse the page", got)
	}

	parsed, err := page.Timeout(10 * time.Second).Eval(`() => window.parsed`)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}
	if parsed.Value.Str() != "#second=2" {
		t.Errorf("page parsed %q, want %q", parsed.Value.Str(), "#second=2")
	}
}
