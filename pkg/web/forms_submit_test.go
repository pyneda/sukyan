package web

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// TestSubmitFormFiresJSFetchHandler verifies that SubmitForm exercises modern
// JS-driven forms (addEventListener('submit') + preventDefault + fetch) rather than
// silently no-opping them. The native form.submit() does NOT dispatch the submit
// event, so this guards the requestSubmit()/button-click behavior.
func TestSubmitFormFiresJSFetchHandler(t *testing.T) {
	var fetched int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/submit":
			atomic.StoreInt32(&fetched, 1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			w.Header().Set("Content-Type", "text/html")
			// A bare <button> (no type=submit) inside a form whose submit handler
			// preventDefaults and fetches — the exact modern-SPA pattern the crawler
			// must exercise.
			_, _ = w.Write([]byte(`<!doctype html><html><body>
<form id="f">
  <input name="q" value="x">
  <button>Go</button>
</form>
<script>
document.getElementById('f').addEventListener('submit', function(e){
  e.preventDefault();
  fetch('/api/submit', {method:'POST', headers:{'content-type':'application/json'}, body:'{}'});
});
</script>
</body></html>`))
		}
	}))
	defer server.Close()

	url := launcher.New().Headless(true).Set("no-sandbox", "true").MustLaunch()
	browser := rod.New().ControlURL(url).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(server.URL + "/").MustWaitLoad()
	form := page.MustElement("form#f")

	if !SubmitForm(form, page) {
		t.Fatal("SubmitForm returned false")
	}

	// The fetch is async; poll briefly for it to land server-side.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&fetched) == 1 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("submit handler's fetch never reached the server — SubmitForm did not dispatch the submit event")
}
