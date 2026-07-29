package web

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// Hash-routed applications (AngularJS hashbang, Vue/Ember hash mode) express their
// entire route table as fragments on a single document. Stripping the fragment
// during extraction collapses the whole app to one URL.
func TestGetPageAnchorsResolvesHashRoutes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><head><base href="/app/"></head><body>
<a id="bang" href="index.html#!ng-href.found">hashbang route</a>
<a id="slash" href="#/users/5">hash route</a>
<a id="anchor" href="#section">in-page anchor</a>
<a id="plain" href="/plain.found">plain link</a>
</body></html>`))
	}))
	defer server.Close()

	controlURL := launcher.New().Headless(true).Set("no-sandbox", "true").MustLaunch()
	browser := rod.New().ControlURL(controlURL).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage("")
	page.MustNavigate(server.URL + "/app/").MustWaitLoad()

	anchors, err := GetPageAnchors(page)
	if err != nil {
		t.Fatalf("GetPageAnchors: %v", err)
	}

	want := []string{
		server.URL + "/app/ng-href.found",
		server.URL + "/app/users/5",
		server.URL + "/plain.found",
	}
	for _, w := range want {
		if !slices.Contains(anchors, w) {
			t.Errorf("expected anchor %q, got %v", w, anchors)
		}
	}

	// A bare in-page anchor is not a route; it must still collapse to the document.
	if slices.Contains(anchors, server.URL+"/app/#section") {
		t.Errorf("in-page anchor should not be emitted with its fragment, got %v", anchors)
	}
}
