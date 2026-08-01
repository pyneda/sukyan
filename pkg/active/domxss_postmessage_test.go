package active

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// The audit sends its payload by evaluating a script in the page. rod wraps
// non-function JS as `function() { return <js> }`, so a bare statement block is
// a SyntaxError and nothing is ever posted - silently, because the audit
// discards the Eval error.
func TestPostMessageScriptsActuallyDeliverAMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><html><body><script>
			window.__received = [];
			window.addEventListener('message', function (e) {
				window.__received.push(typeof e.data === 'string' ? e.data : JSON.stringify(e.data));
			});
		</script></body></html>`)
	}))
	defer server.Close()

	page, err := browser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	defer page.Close()

	if err := page.Timeout(10 * time.Second).Navigate(server.URL); err != nil {
		t.Fatalf("navigate failed: %v", err)
	}
	if err := page.Timeout(10 * time.Second).WaitLoad(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// A payload carrying single quotes is the normal case, not an edge case.
	const marker = "mk4213x"
	payload := fmt.Sprintf("alert('%s')", marker)

	scripts := buildPostMessageScripts(payload)
	if len(scripts) == 0 {
		t.Fatal("no postMessage scripts built")
	}

	for i, script := range scripts {
		if _, err := page.Timeout(10 * time.Second).Eval(script); err != nil {
			t.Errorf("script %d failed to evaluate: %v", i, err)
		}
	}

	time.Sleep(500 * time.Millisecond)

	got, err := page.Timeout(10 * time.Second).Eval(`() => JSON.stringify(window.__received)`)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}
	received := got.Value.Str()

	if !strings.Contains(received, marker) {
		t.Errorf("page received %s, want a message containing %q", received, marker)
	}
	if count := strings.Count(received, marker); count < len(scripts) {
		t.Errorf("page received the marker %d times, want at least %d (one per script shape)", count, len(scripts))
	}
}
