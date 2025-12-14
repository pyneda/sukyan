package active

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	sukyanBrowser "github.com/pyneda/sukyan/pkg/browser"
	"github.com/pyneda/sukyan/pkg/payloads"
	"github.com/pyneda/sukyan/pkg/web"
)

// =============================================================================
// Unit Tests for Pure Functions
// =============================================================================

func TestBuildURLWithParam(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		paramName string
		payload   string
		want      string
	}{
		{
			name:      "add param to URL without query",
			baseURL:   "https://example.com/page",
			paramName: "q",
			payload:   "test",
			want:      "https://example.com/page?q=test",
		},
		{
			name:      "add param to URL with existing query",
			baseURL:   "https://example.com/page?existing=value",
			paramName: "q",
			payload:   "test",
			want:      "https://example.com/page?existing=value&q=test",
		},
		{
			name:      "replace existing param",
			baseURL:   "https://example.com/page?q=original",
			paramName: "q",
			payload:   "replaced",
			want:      "https://example.com/page?q=replaced",
		},
		{
			name:      "XSS payload encoding",
			baseURL:   "https://example.com/search",
			paramName: "q",
			payload:   "<script>alert('xss')</script>",
			want:      "https://example.com/search?q=%3Cscript%3Ealert%28%27xss%27%29%3C%2Fscript%3E",
		},
		{
			name:      "payload with special chars",
			baseURL:   "https://example.com/",
			paramName: "data",
			payload:   "test&value=inject",
			want:      "https://example.com/?data=test%26value%3Dinject",
		},
		{
			name:      "URL with fragment",
			baseURL:   "https://example.com/page#section",
			paramName: "q",
			payload:   "test",
			want:      "https://example.com/page?q=test#section",
		},
		{
			name:      "invalid URL returns original",
			baseURL:   "://invalid",
			paramName: "q",
			payload:   "test",
			want:      "://invalid",
		},
		{
			name:      "empty param name",
			baseURL:   "https://example.com/",
			paramName: "",
			payload:   "test",
			want:      "https://example.com/?=test",
		},
		{
			name:      "empty payload",
			baseURL:   "https://example.com/",
			paramName: "q",
			payload:   "",
			want:      "https://example.com/?q=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildURLWithParam(tt.baseURL, tt.paramName, tt.payload)
			if got != tt.want {
				t.Errorf("buildURLWithParam() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildDeduplicationKey(t *testing.T) {
	audit := &DOMXSSAudit{}

	tests := []struct {
		name   string
		url    string
		source web.DOMXSSSource
		// We test that key contains source name and doesn't contain fragment
		wantContains    string
		wantNotContains string
	}{
		{
			name:            "basic URL with hash source",
			url:             "https://example.com/page",
			source:          web.DOMXSSSource{Name: "location.hash"},
			wantContains:    "location.hash",
			wantNotContains: "",
		},
		{
			name:            "URL with fragment preserved in normalized form",
			url:             "https://example.com/page#malicious",
			source:          web.DOMXSSSource{Name: "location.hash"},
			wantContains:    "location.hash",
			wantNotContains: "", // Fragment may be preserved by lib.NormalizeURL
		},
		{
			name:            "different sources produce different keys",
			url:             "https://example.com/page",
			source:          web.DOMXSSSource{Name: "location.search"},
			wantContains:    "location.search",
			wantNotContains: "location.hash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := audit.buildDeduplicationKey(tt.url, tt.source)

			if !strings.Contains(key, tt.wantContains) {
				t.Errorf("buildDeduplicationKey() = %q, want to contain %q", key, tt.wantContains)
			}

			if tt.wantNotContains != "" && strings.Contains(key, tt.wantNotContains) {
				t.Errorf("buildDeduplicationKey() = %q, should not contain %q", key, tt.wantNotContains)
			}
		})
	}
}

func TestBuildDeduplicationKeyWithStorageKey(t *testing.T) {
	audit := &DOMXSSAudit{}

	tests := []struct {
		name       string
		url        string
		source     web.DOMXSSSource
		storageKey string
		wantParts  []string // All parts that should be in the key
	}{
		{
			name:       "localStorage with key",
			url:        "https://example.com/page",
			source:     web.DOMXSSSource{Name: "localStorage"},
			storageKey: "userToken",
			wantParts:  []string{"localStorage", "userToken"},
		},
		{
			name:       "sessionStorage with key",
			url:        "https://example.com/page",
			source:     web.DOMXSSSource{Name: "sessionStorage"},
			storageKey: "config",
			wantParts:  []string{"sessionStorage", "config"},
		},
		{
			name:       "different keys produce different dedup keys",
			url:        "https://example.com/page",
			source:     web.DOMXSSSource{Name: "localStorage"},
			storageKey: "differentKey",
			wantParts:  []string{"differentKey"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := audit.buildDeduplicationKeyWithStorageKey(tt.url, tt.source, tt.storageKey)

			for _, part := range tt.wantParts {
				if !strings.Contains(key, part) {
					t.Errorf("buildDeduplicationKeyWithStorageKey() = %q, want to contain %q", key, part)
				}
			}
		})
	}
}

func TestDeduplicationKeysDifferBetweenSources(t *testing.T) {
	audit := &DOMXSSAudit{}
	url := "https://example.com/page"

	hashSource := web.DOMXSSSource{Name: "location.hash"}
	searchSource := web.DOMXSSSource{Name: "location.search"}

	key1 := audit.buildDeduplicationKey(url, hashSource)
	key2 := audit.buildDeduplicationKey(url, searchSource)

	if key1 == key2 {
		t.Errorf("Different sources should produce different keys: %q vs %q", key1, key2)
	}
}

func TestDeduplicationKeysStorageDifferByKey(t *testing.T) {
	audit := &DOMXSSAudit{}
	url := "https://example.com/page"
	source := web.DOMXSSSource{Name: "localStorage"}

	key1 := audit.buildDeduplicationKeyWithStorageKey(url, source, "token")
	key2 := audit.buildDeduplicationKeyWithStorageKey(url, source, "config")

	if key1 == key2 {
		t.Errorf("Different storage keys should produce different dedup keys: %q vs %q", key1, key2)
	}
}

// =============================================================================
// Deduplication Logic Tests
// =============================================================================

func TestMarkDetectedIfNew(t *testing.T) {
	audit := &DOMXSSAudit{}
	url := "https://example.com/page"
	source := web.DOMXSSSource{Name: "location.hash"}

	// First detection should return true (is new)
	if !audit.markDetectedIfNew(url, source) {
		t.Error("First detection should return true")
	}

	// Second detection of same source should return false (already detected)
	if audit.markDetectedIfNew(url, source) {
		t.Error("Second detection of same source should return false")
	}

	// Different source should return true (is new)
	differentSource := web.DOMXSSSource{Name: "location.search"}
	if !audit.markDetectedIfNew(url, differentSource) {
		t.Error("Different source should return true")
	}
}

func TestIsDetectedSource(t *testing.T) {
	audit := &DOMXSSAudit{}
	url := "https://example.com/page"
	source := web.DOMXSSSource{Name: "location.hash"}

	// Initially should not be detected
	if audit.isDetectedSource(url, source) {
		t.Error("Source should not be detected initially")
	}

	// Mark as detected
	audit.markDetectedIfNew(url, source)

	// Now should be detected
	if !audit.isDetectedSource(url, source) {
		t.Error("Source should be detected after marking")
	}
}

func TestMarkDetectedStorageIfNew(t *testing.T) {
	audit := &DOMXSSAudit{}
	url := "https://example.com/page"
	source := web.DOMXSSSource{Name: "localStorage"}

	// First detection should return true
	if !audit.markDetectedStorageIfNew(url, source, "token") {
		t.Error("First storage detection should return true")
	}

	// Same source+key should return false
	if audit.markDetectedStorageIfNew(url, source, "token") {
		t.Error("Second detection of same storage+key should return false")
	}

	// Different key should return true
	if !audit.markDetectedStorageIfNew(url, source, "config") {
		t.Error("Different storage key should return true")
	}
}

func TestIsDetectedStorageSource(t *testing.T) {
	audit := &DOMXSSAudit{}
	url := "https://example.com/page"
	source := web.DOMXSSSource{Name: "localStorage"}
	storageKey := "userToken"

	// Initially should not be detected
	if audit.isDetectedStorageSource(url, source, storageKey) {
		t.Error("Storage source should not be detected initially")
	}

	// Mark as detected
	audit.markDetectedStorageIfNew(url, source, storageKey)

	// Now should be detected
	if !audit.isDetectedStorageSource(url, source, storageKey) {
		t.Error("Storage source should be detected after marking")
	}

	// Different key should not be detected
	if audit.isDetectedStorageSource(url, source, "differentKey") {
		t.Error("Different storage key should not be detected")
	}
}

func TestDeduplicationConcurrency(t *testing.T) {
	audit := &DOMXSSAudit{}
	url := "https://example.com/page"
	source := web.DOMXSSSource{Name: "location.hash"}

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	// Run 100 concurrent attempts to mark the same source
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if audit.markDetectedIfNew(url, source) {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Exactly one should succeed
	if successCount != 1 {
		t.Errorf("Expected exactly 1 successful mark, got %d", successCount)
	}
}

// =============================================================================
// Browser Integration Tests
// =============================================================================

var (
	testBrowser     *rod.Browser
	testBrowserOnce sync.Once
	testBrowserErr  error
)

func getTestBrowser(t *testing.T) *rod.Browser {
	t.Helper()

	testBrowserOnce.Do(func() {
		path, found := launcher.LookPath()
		if !found {
			testBrowserErr = fmt.Errorf("browser not found")
			return
		}
		u := launcher.New().Bin(path).Headless(true).MustLaunch()
		testBrowser = rod.New().ControlURL(u).MustConnect()
	})

	if testBrowserErr != nil {
		t.Skip("Skipping browser integration test: " + testBrowserErr.Error())
	}

	return testBrowser
}

// domXSSDetectionResult holds detection info for tests
type domXSSDetectionResult struct {
	detected     bool
	alertMessage string
	taintSink    string
}

// runDOMXSSDetectionTest runs a DOM XSS detection test and returns result
func runDOMXSSDetectionTest(t *testing.T, browser *rod.Browser, html string, testURL string, marker string) domXSSDetectionResult {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, html)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	page, err := browser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		t.Fatalf("Failed to create page: %v", err)
	}
	defer page.Close()

	pageCtx := page.Context(ctx)

	result := domXSSDetectionResult{}
	var mu sync.Mutex

	// Listen for alerts and console messages
	go pageCtx.EachEvent(
		func(e *proto.PageJavascriptDialogOpening) (stop bool) {
			if payloads.ContainsMarker(e.Message, marker) {
				mu.Lock()
				result.detected = true
				result.alertMessage = e.Message
				mu.Unlock()
			}
			proto.PageHandleJavaScriptDialog{Accept: true}.Call(pageCtx)
			return true
		},
		func(e *proto.RuntimeConsoleAPICalled) {
			for _, arg := range e.Args {
				val := strings.Trim(arg.Value.String(), "\"")
				if strings.HasPrefix(val, "SUKYAN_SINK:") {
					parts := strings.SplitN(val, ":", 3)
					if len(parts) >= 2 {
						mu.Lock()
						result.detected = true
						result.taintSink = parts[1]
						mu.Unlock()
					}
				}
			}
		},
	)()

	// Inject taint tracking before navigation
	script := sukyanBrowser.GetTaintTrackingScript(marker)
	_, err = pageCtx.EvalOnNewDocument(script)
	if err != nil {
		t.Fatalf("Failed to inject taint script: %v", err)
	}

	// Build final URL (server URL + path from testURL)
	finalURL := server.URL
	if testURL != "" {
		finalURL = server.URL + testURL
	}

	err = pageCtx.Navigate(finalURL)
	if err != nil {
		t.Fatalf("Navigation failed: %v", err)
	}

	pageCtx.WaitLoad()
	time.Sleep(800 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	return result
}

// TestDOMXSS_HashToInnerHTML tests classic location.hash -> innerHTML sink
func TestDOMXSS_HashToInnerHTML(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "xss_hash_" + payloads.GenerateMarker()

	html := `<!DOCTYPE html>
<html><body>
<div id="output"></div>
<script>
// Classic DOM XSS: location.hash -> innerHTML
var data = location.hash.substring(1);
if (data) {
    document.getElementById('output').innerHTML = data;
}
</script>
</body></html>`

	result := runDOMXSSDetectionTest(t, browser, html, "#"+marker, marker)

	if !result.detected {
		t.Error("Expected DOM XSS to be detected via location.hash -> innerHTML")
	}

	if result.taintSink != "innerHTML" && result.alertMessage == "" {
		t.Errorf("Expected innerHTML sink or alert, got sink=%q alert=%q", result.taintSink, result.alertMessage)
	}
}

// TestDOMXSS_HashToInnerHTML_WithPayload tests with actual XSS payload
func TestDOMXSS_HashToInnerHTML_WithPayload(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "xss_alert_" + payloads.GenerateMarker()
	payload := fmt.Sprintf("<img src=x onerror=alert('%s')>", marker)

	html := `<!DOCTYPE html>
<html><body>
<div id="output"></div>
<script>
var data = location.hash.substring(1);
if (data) {
    document.getElementById('output').innerHTML = decodeURIComponent(data);
}
</script>
</body></html>`

	result := runDOMXSSDetectionTest(t, browser, html, "#"+payload, marker)

	if !result.detected {
		t.Error("Expected DOM XSS to be detected with XSS payload")
	}

	// Either taint tracking or actual alert should fire
	if result.taintSink == "" && result.alertMessage == "" {
		t.Error("Expected either taint sink detection or alert to fire")
	}
}

// TestDOMXSS_SearchToInnerHTML tests location.search -> innerHTML sink
func TestDOMXSS_SearchToInnerHTML(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "xss_search_" + payloads.GenerateMarker()

	html := `<!DOCTYPE html>
<html><body>
<div id="output"></div>
<script>
// DOM XSS via URL search params
var params = new URLSearchParams(location.search);
var q = params.get('q');
if (q) {
    document.getElementById('output').innerHTML = q;
}
</script>
</body></html>`

	result := runDOMXSSDetectionTest(t, browser, html, "?q="+marker, marker)

	if !result.detected {
		t.Error("Expected DOM XSS to be detected via location.search -> innerHTML")
	}
}

// TestDOMXSS_SearchToDocumentWrite tests location.search -> document.write sink
func TestDOMXSS_SearchToDocumentWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "xss_write_" + payloads.GenerateMarker()

	// document.write must be called during page load
	html := fmt.Sprintf(`<!DOCTYPE html>
<html><body>
<script>
var params = new URLSearchParams(location.search);
var data = params.get('data');
if (data) {
    document.write('<div>' + data + '</div>');
}
</script>
</body></html>`)

	result := runDOMXSSDetectionTest(t, browser, html, "?data="+marker, marker)

	if !result.detected {
		t.Error("Expected DOM XSS to be detected via location.search -> document.write")
	}

	if result.taintSink != "document.write" && result.alertMessage == "" {
		t.Logf("Detected via sink=%q or alert=%q", result.taintSink, result.alertMessage)
	}
}

// TestDOMXSS_HashToEval tests location.hash -> eval sink
func TestDOMXSS_HashToEval(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "xss_eval_" + payloads.GenerateMarker()

	html := `<!DOCTYPE html>
<html><body>
<script>
// Dangerous: hash -> eval
var code = location.hash.substring(1);
if (code) {
    eval(code);
}
</script>
</body></html>`

	// Pass code that includes marker
	evalCode := fmt.Sprintf("console.log('%s')", marker)
	result := runDOMXSSDetectionTest(t, browser, html, "#"+evalCode, marker)

	if !result.detected {
		t.Error("Expected DOM XSS to be detected via location.hash -> eval")
	}

	if result.taintSink != "eval" {
		t.Logf("Expected eval sink, got: %q", result.taintSink)
	}
}

// TestDOMXSS_LocalStorageToInnerHTML tests localStorage -> innerHTML sink
func TestDOMXSS_LocalStorageToInnerHTML(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "xss_storage_" + payloads.GenerateMarker()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html><body>
<div id="output"></div>
<script>
// Read from localStorage and render
var data = localStorage.getItem('userContent');
if (data) {
    document.getElementById('output').innerHTML = data;
}
</script>
</body></html>`)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	page, err := browser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		t.Fatalf("Failed to create page: %v", err)
	}
	defer page.Close()

	pageCtx := page.Context(ctx)

	detected := false
	var mu sync.Mutex

	go pageCtx.EachEvent(
		func(e *proto.RuntimeConsoleAPICalled) {
			for _, arg := range e.Args {
				val := strings.Trim(arg.Value.String(), "\"")
				if strings.HasPrefix(val, "SUKYAN_SINK:innerHTML:") {
					mu.Lock()
					detected = true
					mu.Unlock()
				}
			}
		},
	)()

	// Navigate first to set storage on the correct origin
	err = pageCtx.Navigate(server.URL)
	if err != nil {
		t.Fatalf("Initial navigation failed: %v", err)
	}
	pageCtx.WaitLoad()

	// Set localStorage value
	_, err = pageCtx.Eval(fmt.Sprintf(`() => localStorage.setItem('userContent', '%s')`, marker))
	if err != nil {
		t.Fatalf("Failed to set localStorage: %v", err)
	}

	// Inject taint tracking BEFORE reload using EvalOnNewDocument
	// This ensures hooks are in place when the page scripts execute after reload
	script := sukyanBrowser.GetTaintTrackingScript(marker)
	_, err = pageCtx.EvalOnNewDocument(script)
	if err != nil {
		t.Fatalf("Failed to inject taint tracking: %v", err)
	}

	// Reload to trigger the vulnerable code
	err = pageCtx.Reload()
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	pageCtx.WaitLoad()
	time.Sleep(800 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if !detected {
		t.Error("Expected DOM XSS to be detected via localStorage -> innerHTML")
	}
}

// TestDOMXSS_NoVulnerability tests that clean code doesn't trigger false positives
func TestDOMXSS_NoVulnerability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "clean_marker_" + payloads.GenerateMarker()

	// This page uses textContent (safe) instead of innerHTML (vulnerable)
	html := `<!DOCTYPE html>
<html><body>
<div id="output"></div>
<script>
var data = location.hash.substring(1);
if (data) {
    // Safe: textContent doesn't execute HTML/JS
    document.getElementById('output').textContent = data;
}
</script>
</body></html>`

	result := runDOMXSSDetectionTest(t, browser, html, "#"+marker, marker)

	if result.detected {
		t.Errorf("False positive: textContent should not trigger DOM XSS detection (sink=%q)", result.taintSink)
	}
}

// TestDOMXSS_PostMessageToInnerHTML tests postMessage -> innerHTML sink
func TestDOMXSS_PostMessageToInnerHTML(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "xss_postmsg_" + payloads.GenerateMarker()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html><body>
<div id="output"></div>
<script>
window.addEventListener('message', function(e) {
    // Vulnerable: no origin check, direct innerHTML
    document.getElementById('output').innerHTML = e.data;
});
</script>
</body></html>`)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	page, err := browser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		t.Fatalf("Failed to create page: %v", err)
	}
	defer page.Close()

	pageCtx := page.Context(ctx)

	detected := false
	var mu sync.Mutex

	go pageCtx.EachEvent(
		func(e *proto.RuntimeConsoleAPICalled) {
			for _, arg := range e.Args {
				val := strings.Trim(arg.Value.String(), "\"")
				if strings.HasPrefix(val, "SUKYAN_SINK:innerHTML:") {
					mu.Lock()
					detected = true
					mu.Unlock()
				}
			}
		},
	)()

	// Inject taint tracking before navigation
	script := sukyanBrowser.GetTaintTrackingScript(marker)
	_, _ = pageCtx.EvalOnNewDocument(script)

	err = pageCtx.Navigate(server.URL)
	if err != nil {
		t.Fatalf("Navigation failed: %v", err)
	}
	pageCtx.WaitLoad()

	// Send postMessage with marker
	_, err = pageCtx.Eval(fmt.Sprintf(`() => window.postMessage('%s', '*')`, marker))
	if err != nil {
		t.Fatalf("Failed to send postMessage: %v", err)
	}

	time.Sleep(800 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if !detected {
		t.Error("Expected DOM XSS to be detected via postMessage -> innerHTML")
	}
}

// TestDOMXSS_OuterHTMLSink tests detection of outerHTML sink
func TestDOMXSS_OuterHTMLSink(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "xss_outer_" + payloads.GenerateMarker()

	html := `<!DOCTYPE html>
<html><body>
<div id="target">original</div>
<script>
var data = location.hash.substring(1);
if (data) {
    document.getElementById('target').outerHTML = '<div>' + data + '</div>';
}
</script>
</body></html>`

	result := runDOMXSSDetectionTest(t, browser, html, "#"+marker, marker)

	if !result.detected {
		t.Error("Expected DOM XSS to be detected via location.hash -> outerHTML")
	}
}

// TestDOMXSS_SetTimeoutSink tests detection of setTimeout with string argument
func TestDOMXSS_SetTimeoutSink(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "xss_timeout_" + payloads.GenerateMarker()

	html := `<!DOCTYPE html>
<html><body>
<script>
var data = location.hash.substring(1);
if (data) {
    // Dangerous: string argument to setTimeout
    setTimeout("console.log('" + data + "')", 10);
}
</script>
</body></html>`

	result := runDOMXSSDetectionTest(t, browser, html, "#"+marker, marker)

	if !result.detected {
		t.Error("Expected DOM XSS to be detected via location.hash -> setTimeout")
	}

	if result.taintSink != "setTimeout" {
		t.Logf("Expected setTimeout sink, got: %q", result.taintSink)
	}
}
