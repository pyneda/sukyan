package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"go.uber.org/goleak"
)

// jsSubmitFormHTML serves a form whose submit is intercepted by JS:
// preventDefault + fetch POST to /api/invoice. The POST only fires on a
// real `submit` event, which is exactly what SubmitForm must trigger.
const jsSubmitFormHTML = `<!DOCTYPE html>
<html><body>
<form id="f">
  <textarea id="x" name="payload">default</textarea>
  <button>Parse &amp; preview</button>
</form>
<script>
document.getElementById('f').addEventListener('submit', function(e) {
  e.preventDefault();
  fetch('/api/invoice', {
    method: 'POST',
    headers: {'content-type': 'application/xml'},
    body: document.getElementById('x').value
  });
});
</script>
</body></html>`

func TestSubmitFormFiresJSSubmitHandler(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	var posts int32
	gotContentType := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/invoice" {
			atomic.AddInt32(&posts, 1)
			select {
			case gotContentType <- r.Header.Get("Content-Type"):
			default:
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(jsSubmitFormHTML))
	}))
	defer server.Close()

	path, _ := launcher.LookPath()
	u := launcher.New().Bin(path).Headless(true).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(server.URL)
	page.MustWaitLoad()

	form := page.MustElement("form")
	AutoFillForm(form, page)
	if ok := SubmitForm(form, page); !ok {
		t.Fatal("SubmitForm returned false")
	}

	select {
	case ct := <-gotContentType:
		if ct != "application/xml" {
			t.Errorf("expected content-type application/xml, got %q", ct)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no POST /api/invoice observed within 5s (posts=%d)", atomic.LoadInt32(&posts))
	}
}

func TestSubmitFormCancelledContextDoesNotPOST(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var posts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/invoice" {
			atomic.AddInt32(&posts, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(jsSubmitFormHTML))
	}))
	defer server.Close()

	path, _ := launcher.LookPath()
	u := launcher.New().Bin(path).Headless(true).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(server.URL)
	page.MustWaitLoad()

	// Fetch the form from the UNCANCELLED page (the DOM is already loaded), then
	// drive interaction through a cancelled page clone. SubmitForm's own child
	// lookups (form.Element / form.Timeout().Eval) and the submit-control click
	// run under the cancelled context via the page arg, so they must no-op.
	form := page.MustElement("form")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before interaction
	pageWithCtx := page.Context(ctx)

	done := make(chan struct{})
	go func() {
		defer close(done)
		AutoFillForm(form, pageWithCtx)
		SubmitForm(form, pageWithCtx)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("interaction did not return promptly under a cancelled context")
	}

	if n := atomic.LoadInt32(&posts); n != 0 {
		t.Errorf("expected zero POSTs under cancelled context, got %d", n)
	}
}
