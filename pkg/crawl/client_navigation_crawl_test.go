package crawl

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/pyneda/sukyan/pkg/web"
)

func TestMergeClientNavURLs(t *testing.T) {
	existing := []string{"http://a/x", "http://a/y"}
	captured := []string{"http://a/y", "http://a/z", "http://a/z"}
	got := mergeClientNavURLs(existing, captured)
	want := map[string]bool{"http://a/x": true, "http://a/y": true, "http://a/z": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %d unique", got, len(want))
	}
	for _, u := range got {
		if !want[u] {
			t.Fatalf("unexpected url %q in %v", u, got)
		}
	}
}

func TestClientNavigationEndToEndResolution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><head><base href="/app/"></head><body>
<a id="hashlink" href="#!route-one.found">hash route</a>
<button id="go">navigate</button>
<script>
  document.getElementById('go').addEventListener('click', function(){
    history.pushState({}, '', '/app/click-route.found');
  });
</script>
</body></html>`))
	}))
	defer server.Close()

	url := launcher.New().Headless(true).Set("no-sandbox", "true").MustLaunch()
	browser := rod.New().ControlURL(url).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage("")
	if _, err := page.EvalOnNewDocument(web.ClientNavigationHookScript); err != nil {
		t.Fatalf("inject: %v", err)
	}
	// Serve at a path matching <base href="/app/"> so a hash-link click is a
	// same-document change that fires hashchange, mirroring how the maze pages
	// (base href == serving path) behave.
	page.MustNavigate(server.URL + "/app/").MustWaitLoad()
	time.Sleep(200 * time.Millisecond)

	all := web.DrainClientNavigations(page)

	page.MustElement("#hashlink").MustClick()
	time.Sleep(150 * time.Millisecond)
	page.MustElement("#go").MustClick()
	time.Sleep(150 * time.Millisecond)
	all = mergeClientNavURLs(all, web.DrainClientNavigations(page))

	if !hasURLSuffix(all, "/app/route-one.found") {
		t.Errorf("hash route not resolved against base; got %v", all)
	}
	if !hasURLSuffix(all, "/app/click-route.found") {
		t.Errorf("click pushState route not captured; got %v", all)
	}
}

func hasURLSuffix(urls []string, suffix string) bool {
	for _, u := range urls {
		if len(u) >= len(suffix) && u[len(u)-len(suffix):] == suffix {
			return true
		}
	}
	return false
}
