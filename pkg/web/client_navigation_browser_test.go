package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

func TestClientNavigationCaptureInBrowser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body>
<a id="hashlink" href="#/hashroute-abc.found">hash route</a>
<button id="go">navigate</button>
<script>
  var seg = ['push','state','xyz'];
  history.pushState({}, '', '/' + seg.join('-') + '.' + 'found');
  document.getElementById('go').addEventListener('click', function(){
    history.pushState({}, '', '/' + ['click','nav','qqq'].join('-') + '.' + 'found');
  });
</script>
</body></html>`))
	}))
	defer server.Close()

	url := launcher.New().Headless(true).Set("no-sandbox", "true").MustLaunch()
	browser := rod.New().ControlURL(url).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage("")
	if _, err := page.EvalOnNewDocument(ClientNavigationHookScript); err != nil {
		t.Fatalf("inject hook: %v", err)
	}
	page.MustNavigate(server.URL + "/").MustWaitLoad()
	time.Sleep(300 * time.Millisecond)

	afterLoad := DrainClientNavigations(page)
	if !containsSuffix(afterLoad, "/push-state-xyz.found") {
		t.Fatalf("expected on-load pushState route captured, got %v", afterLoad)
	}

	page.MustElement("#hashlink").MustClick()
	time.Sleep(200 * time.Millisecond)
	page.MustElement("#go").MustClick()
	time.Sleep(200 * time.Millisecond)

	afterClick := DrainClientNavigations(page)
	if !containsSuffix(afterClick, "/hashroute-abc.found") {
		t.Errorf("expected hash route captured, got %v", afterClick)
	}
	if !containsSuffix(afterClick, "/click-nav-qqq.found") {
		t.Errorf("expected click pushState route captured, got %v", afterClick)
	}
}

func containsSuffix(urls []string, suffix string) bool {
	for _, u := range urls {
		if len(u) >= len(suffix) && u[len(u)-len(suffix):] == suffix {
			return true
		}
	}
	return false
}
