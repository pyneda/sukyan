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

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before interaction
	pageWithCtx := page.Context(ctx)

	// Fetch the form FROM the cancelled clone so the *element* carries the
	// cancelled context. SubmitForm/AutoFillForm operate on the element
	// (form.Element / form.Timeout().Eval), which snapshot the element's own
	// context at fetch time — so the element must be born from the cancelled
	// clone for cancellation to take effect. Fetching from the uncancelled page
	// would leave the element cancellation-immune (the actual bug being tested).
	form, ferr := pageWithCtx.Element("form")
	if ferr != nil {
		// Fetching under an already-cancelled context may itself fail fast; that
		// is an acceptable outcome — it also means no interaction/POST occurs.
		t.Logf("form fetch under cancelled context returned: %v", ferr)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if form == nil {
			return
		}
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

// animatedFormHTML serves a form whose text input is perpetually animated, so its
// bounding box never stabilizes. rod's Element.Input() calls Focus->ScrollIntoView->
// WaitStableRAF, whose requestAnimationFrame loop runs on the deadline-less root page
// and therefore never returns on such a page — hanging the whole crawl. AutoFillForm
// must remain bounded regardless.
const animatedFormHTML = `<!DOCTYPE html>
<html><head><style>
@keyframes drift { from { transform: translateX(0); } to { transform: translateX(40px); } }
#q { position: relative; animation: drift 0.3s linear infinite alternate; }
</style></head><body>
<form id="f">
  <input id="q" name="q" type="text">
  <button type="submit">go</button>
</form>
</body></html>`

// AutoFillForm must not hang on a page whose input is continuously animating (rod's
// WaitStableRAF would otherwise loop forever ignoring the element timeout).
func TestAutoFillFormDoesNotHangOnAnimatedInput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(animatedFormHTML))
	}))
	defer server.Close()

	path, _ := launcher.LookPath()
	u := launcher.New().Bin(path).Headless(true).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(server.URL)
	page.MustWaitLoad()
	form := page.MustElement("form")

	done := make(chan struct{})
	go func() {
		defer close(done)
		AutoFillForm(form, page)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("AutoFillForm hung on an animated input (WaitStableRAF never returned)")
	}

	value, err := page.MustElement("#q").Property("value")
	if err != nil {
		t.Fatalf("could not read input value: %v", err)
	}
	if value.Str() == "" {
		t.Errorf("expected the animated input to be filled, got empty value")
	}
}

// animatedSubmitFormHTML: the submit button is perpetually animated. rod's Click()
// calls Hover->ScrollIntoView->WaitStableRAF, which hangs on such a button. SubmitForm
// must stay bounded AND still fire the JS submit handler (which POSTs).
const animatedSubmitFormHTML = `<!DOCTYPE html>
<html><head><style>
@keyframes drift { from { transform: translateX(0); } to { transform: translateX(30px); } }
button { position: relative; animation: drift 0.3s linear infinite alternate; }
</style></head><body>
<form id="f"><button type="submit">go</button></form>
<script>
document.getElementById('f').addEventListener('submit', function(e){
  e.preventDefault();
  fetch('/submitted', {method:'POST'});
});
</script>
</body></html>`

// SubmitForm must not hang when the submit control is continuously animating, and
// must still trigger the form's submit handler.
func TestSubmitFormDoesNotHangOnAnimatedButton(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	var posts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/submitted" {
			atomic.AddInt32(&posts, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(animatedSubmitFormHTML))
	}))
	defer server.Close()

	path, _ := launcher.LookPath()
	u := launcher.New().Bin(path).Headless(true).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(server.URL)
	page.MustWaitLoad()
	form := page.MustElement("form")

	done := make(chan struct{})
	go func() {
		defer close(done)
		SubmitForm(form, page)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("SubmitForm hung on an animated submit button (Click->WaitStableRAF never returned)")
	}

	time.Sleep(3 * time.Second)
	if atomic.LoadInt32(&posts) == 0 {
		t.Errorf("expected the submit handler to POST, but it did not fire")
	}
}

// reactControlledInputHTML mimics React's controlled-input value tracking: an
// instance-level `value` setter is installed that records instance assignments
// separately, while the framework only trusts values written through the native
// prototype setter (which it captured before overriding). #state is updated on the
// input event by reading the NATIVE value. A naive `this.value = v` writes only the
// shadowed instance property, leaving the native value — and thus #state — empty.
const reactControlledInputHTML = `<!DOCTYPE html>
<html><body>
<form id="f"><input id="q" name="q" type="text"><span id="state"></span></form>
<script>
var el = document.getElementById('q');
var proto = Object.getPrototypeOf(el);
var nativeGet = Object.getOwnPropertyDescriptor(proto, 'value').get;
var nativeSet = Object.getOwnPropertyDescriptor(proto, 'value').set;
var shadow = '';
Object.defineProperty(el, 'value', {
  get: function () { return nativeGet.call(this); },
  set: function (v) { shadow = v; /* instance write is ignored by the "framework" */ }
});
el.addEventListener('input', function () {
  document.getElementById('state').textContent = nativeGet.call(el);
});
</script>
</body></html>`

// setElementValue must update framework-controlled inputs by using the native
// prototype value setter, so React-style onChange/state tracking fires.
func TestSetElementValueUsesNativeSetterForControlledInput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(reactControlledInputHTML))
	}))
	defer server.Close()

	path, _ := launcher.LookPath()
	u := launcher.New().Bin(path).Headless(true).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(server.URL)
	page.MustWaitLoad()

	if err := setElementValue(page.MustElement("#q"), "filled"); err != nil {
		t.Fatalf("setElementValue error: %v", err)
	}

	state := page.MustElement("#state").MustText()
	if state != "filled" {
		t.Errorf("expected framework state to reflect the native-setter fill, got %q", state)
	}
}

// disabledSubmitFormHTML has a disabled submit button whose form would POST on submit.
const disabledSubmitFormHTML = `<!DOCTYPE html>
<html><body>
<form id="f"><button type="submit" disabled>go</button></form>
<script>
document.getElementById('f').addEventListener('submit', function(e){ e.preventDefault(); fetch('/submitted', {method:'POST'}); });
</script>
</body></html>`

// SubmitForm must still submit a form whose submit button is disabled: SafeClick on a
// disabled button is a no-op, so SubmitForm must fall through to the requestSubmit()
// path rather than falsely reporting success.
func TestSubmitFormFallsBackWhenSubmitButtonDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	var posts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/submitted" {
			atomic.AddInt32(&posts, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(disabledSubmitFormHTML))
	}))
	defer server.Close()

	path, _ := launcher.LookPath()
	u := launcher.New().Bin(path).Headless(true).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(server.URL)
	page.MustWaitLoad()

	if ok := SubmitForm(page.MustElement("form"), page); !ok {
		t.Fatal("SubmitForm returned false on a form with a disabled submit button")
	}

	time.Sleep(2 * time.Second)
	if atomic.LoadInt32(&posts) == 0 {
		t.Errorf("expected the form to be submitted via requestSubmit fallback, but no POST fired")
	}
}
