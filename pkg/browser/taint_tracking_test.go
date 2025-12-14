package browser

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
	"github.com/pyneda/sukyan/pkg/payloads"
)

// TestGetTaintTrackingScript_ScriptStructure verifies the script is properly formatted
// and contains all required components.
func TestGetTaintTrackingScript_ScriptStructure(t *testing.T) {
	marker := "testmarker123"
	script := GetTaintTrackingScript(marker)

	// Verify script is not empty
	if len(script) == 0 {
		t.Fatal("GetTaintTrackingScript should not return empty string")
	}

	// Verify it's wrapped in an IIFE
	if !strings.Contains(script, "(function()") {
		t.Error("Script should be wrapped in an IIFE")
	}
	if !strings.HasSuffix(strings.TrimSpace(script), "})();") {
		t.Error("Script should end with })();")
	}

	// Verify marker is injected
	if !strings.Contains(script, marker) {
		t.Error("Script should contain the marker")
	}

	// Verify taint prefix is injected
	if !strings.Contains(script, payloads.TaintMarkerPrefix) {
		t.Error("Script should contain the taint marker prefix")
	}

	// Verify double injection prevention
	if !strings.Contains(script, "__sukyanTaintReady") {
		t.Error("Script should set __sukyanTaintReady flag")
	}
}

// TestGetTaintTrackingScript_HooksAllSinks verifies all DOM XSS sinks are hooked.
func TestGetTaintTrackingScript_HooksAllSinks(t *testing.T) {
	script := GetTaintTrackingScript("marker")

	sinks := []struct {
		name        string
		hookPattern string
	}{
		{"innerHTML", "Element.prototype, 'innerHTML'"},
		{"outerHTML", "Element.prototype, 'outerHTML'"},
		{"document.write", "document.write = function"},
		{"document.writeln", "document.writeln = function"},
		{"eval", "window.eval = function"},
		{"setTimeout", "window.setTimeout = function"},
		{"setInterval", "window.setInterval = function"},
		{"location.assign", "location.assign = function"},
		{"location.replace", "location.replace = function"},
	}

	for _, sink := range sinks {
		t.Run(sink.name, func(t *testing.T) {
			if !strings.Contains(script, sink.hookPattern) {
				t.Errorf("Script should hook %s with pattern %q", sink.name, sink.hookPattern)
			}
		})
	}
}

// TestGetTaintTrackingScript_ConsoleLogFormat verifies console log messages follow expected format.
func TestGetTaintTrackingScript_ConsoleLogFormat(t *testing.T) {
	marker := "uniquemarker456"
	script := GetTaintTrackingScript(marker)

	// All sink detections should log with SUKYAN_SINK prefix
	expectedLogs := []string{
		"'SUKYAN_SINK:innerHTML:' + MARKER",
		"'SUKYAN_SINK:outerHTML:' + MARKER",
		"'SUKYAN_SINK:document.write:' + MARKER",
		"'SUKYAN_SINK:document.writeln:' + MARKER",
		"'SUKYAN_SINK:eval:' + MARKER",
		"'SUKYAN_SINK:setTimeout:' + MARKER",
		"'SUKYAN_SINK:setInterval:' + MARKER",
		"'SUKYAN_SINK:location.assign:' + MARKER",
		"'SUKYAN_SINK:location.replace:' + MARKER",
	}

	for _, expectedLog := range expectedLogs {
		if !strings.Contains(script, expectedLog) {
			t.Errorf("Script should contain console.log with %q", expectedLog)
		}
	}
}

// TestGetTaintTrackingScript_PreservesOriginalBehavior verifies hooks store and call originals.
func TestGetTaintTrackingScript_PreservesOriginalBehavior(t *testing.T) {
	script := GetTaintTrackingScript("marker")

	// Verify original methods are stored and called
	preservedMethods := []struct {
		original string
		callSite string
	}{
		{"origInnerHTMLSet", "origInnerHTMLSet.call(this, value)"},
		{"origOuterHTMLSet", "origOuterHTMLSet.call(this, value)"},
		{"origWrite", "origWrite.apply(this, args)"},
		{"origWriteln", "origWriteln.apply(this, args)"},
		{"origEval", "origEval.call(this, code)"},
		{"origSetTimeout", "origSetTimeout.call(this, fn, delay, ...args)"},
		{"origSetInterval", "origSetInterval.call(this, fn, delay, ...args)"},
		{"origAssign", "origAssign.call(this, url)"},
		{"origReplace", "origReplace.call(this, url)"},
	}

	for _, m := range preservedMethods {
		t.Run(m.original, func(t *testing.T) {
			if !strings.Contains(script, m.original) {
				t.Errorf("Script should store original method in %s", m.original)
			}
			if !strings.Contains(script, m.callSite) {
				t.Errorf("Script should call original via %s", m.callSite)
			}
		})
	}
}

// TestGetTaintTrackingScript_TypeChecks verifies type checking before detection.
func TestGetTaintTrackingScript_TypeChecks(t *testing.T) {
	script := GetTaintTrackingScript("marker")

	// All hooks should check typeof === 'string' before checking for marker
	typeChecks := []string{
		"typeof value === 'string'",
		"typeof arg === 'string'",
		"typeof code === 'string'",
		"typeof fn === 'string'",
		"typeof url === 'string'",
	}

	for _, check := range typeChecks {
		if !strings.Contains(script, check) {
			t.Errorf("Script should include type check: %s", check)
		}
	}
}

// TestGetTaintTrackingScript_DifferentMarkers verifies unique markers produce unique scripts.
func TestGetTaintTrackingScript_DifferentMarkers(t *testing.T) {
	marker1 := "marker_alpha"
	marker2 := "marker_beta"

	script1 := GetTaintTrackingScript(marker1)
	script2 := GetTaintTrackingScript(marker2)

	if script1 == script2 {
		t.Error("Scripts with different markers should be different")
	}

	if !strings.Contains(script1, marker1) || strings.Contains(script1, marker2) {
		t.Error("Script1 should contain only marker1")
	}

	if !strings.Contains(script2, marker2) || strings.Contains(script2, marker1) {
		t.Error("Script2 should contain only marker2")
	}
}

// =============================================================================
// Browser Integration Tests
// =============================================================================

// testBrowser provides a shared browser instance for integration tests
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

// taintDetection holds info about a detected taint flow
type taintDetection struct {
	sink   string
	marker string
}

// parseTaintLog parses a SUKYAN_SINK console log message
func parseTaintLog(msg string) *taintDetection {
	if !strings.HasPrefix(msg, "SUKYAN_SINK:") {
		return nil
	}
	parts := strings.SplitN(msg, ":", 3)
	if len(parts) != 3 {
		return nil
	}
	return &taintDetection{
		sink:   parts[1],
		marker: parts[2],
	}
}

// runTaintTest executes a taint tracking test with the given HTML and returns detected sinks
func runTaintTest(t *testing.T, browser *rod.Browser, html string, marker string, setupFn func(*rod.Page) error) []taintDetection {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, html)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	page, err := browser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		t.Fatalf("Failed to create page: %v", err)
	}
	defer page.Close()

	pageCtx := page.Context(ctx)

	var detections []taintDetection
	var mu sync.Mutex

	// Listen for console messages
	go pageCtx.EachEvent(
		func(e *proto.RuntimeConsoleAPICalled) {
			for _, arg := range e.Args {
				val := arg.Value.String()
				// Remove quotes from JSON string value
				val = strings.Trim(val, "\"")
				if d := parseTaintLog(val); d != nil {
					mu.Lock()
					detections = append(detections, *d)
					mu.Unlock()
				}
			}
		},
	)()

	// Inject taint tracking script before navigation
	script := GetTaintTrackingScript(marker)
	_, err = pageCtx.EvalOnNewDocument(script)
	if err != nil {
		t.Fatalf("Failed to inject taint script: %v", err)
	}

	// Run optional setup (e.g., set window.name before navigation)
	if setupFn != nil {
		if err := setupFn(pageCtx); err != nil {
			t.Fatalf("Setup function failed: %v", err)
		}
	}

	// Navigate to test page
	err = pageCtx.Navigate(server.URL)
	if err != nil {
		t.Fatalf("Navigation failed: %v", err)
	}

	err = pageCtx.WaitLoad()
	if err != nil {
		t.Logf("WaitLoad warning: %v", err)
	}

	// Wait for any async scripts
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	result := make([]taintDetection, len(detections))
	copy(result, detections)
	mu.Unlock()

	return result
}

// TestTaintTracking_InnerHTMLSink tests detection when tainted data flows to innerHTML
func TestTaintTracking_InnerHTMLSink(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "taint_inner_" + payloads.GenerateMarker()

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><body>
<div id="target"></div>
<script>
document.getElementById('target').innerHTML = '%s';
</script>
</body></html>`, marker)

	detections := runTaintTest(t, browser, html, marker, nil)

	if len(detections) == 0 {
		t.Fatal("Expected innerHTML sink to be detected")
	}

	found := false
	for _, d := range detections {
		if d.sink == "innerHTML" && d.marker == marker {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected innerHTML sink with marker %s, got: %+v", marker, detections)
	}
}

// TestTaintTracking_OuterHTMLSink tests detection when tainted data flows to outerHTML
func TestTaintTracking_OuterHTMLSink(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "taint_outer_" + payloads.GenerateMarker()

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><body>
<div id="target">old</div>
<script>
document.getElementById('target').outerHTML = '<div>%s</div>';
</script>
</body></html>`, marker)

	detections := runTaintTest(t, browser, html, marker, nil)

	found := false
	for _, d := range detections {
		if d.sink == "outerHTML" && d.marker == marker {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected outerHTML sink detection, got: %+v", detections)
	}
}

// TestTaintTracking_DocumentWriteSink tests detection when tainted data flows to document.write
func TestTaintTracking_DocumentWriteSink(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "taint_write_" + payloads.GenerateMarker()

	// document.write must be called during page load
	html := fmt.Sprintf(`<!DOCTYPE html>
<html><body>
<script>
document.write('<p>%s</p>');
</script>
</body></html>`, marker)

	detections := runTaintTest(t, browser, html, marker, nil)

	found := false
	for _, d := range detections {
		if d.sink == "document.write" && d.marker == marker {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected document.write sink detection, got: %+v", detections)
	}
}

// TestTaintTracking_EvalSink tests detection when tainted data flows to eval
func TestTaintTracking_EvalSink(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "taint_eval_" + payloads.GenerateMarker()

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><body>
<script>
var code = "console.log('%s')";
eval(code);
</script>
</body></html>`, marker)

	detections := runTaintTest(t, browser, html, marker, nil)

	found := false
	for _, d := range detections {
		if d.sink == "eval" && d.marker == marker {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected eval sink detection, got: %+v", detections)
	}
}

// TestTaintTracking_SetTimeoutStringSink tests detection when tainted string flows to setTimeout
func TestTaintTracking_SetTimeoutStringSink(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "taint_timeout_" + payloads.GenerateMarker()

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><body>
<script>
// setTimeout with string argument (legacy but still works)
setTimeout("console.log('%s')", 1);
</script>
</body></html>`, marker)

	detections := runTaintTest(t, browser, html, marker, nil)

	found := false
	for _, d := range detections {
		if d.sink == "setTimeout" && d.marker == marker {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected setTimeout sink detection, got: %+v", detections)
	}
}

// TestTaintTracking_SetIntervalStringSink tests detection when tainted string flows to setInterval
func TestTaintTracking_SetIntervalStringSink(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "taint_interval_" + payloads.GenerateMarker()

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><body>
<script>
// setInterval with string argument
var id = setInterval("console.log('%s')", 100);
clearInterval(id);
</script>
</body></html>`, marker)

	detections := runTaintTest(t, browser, html, marker, nil)

	found := false
	for _, d := range detections {
		if d.sink == "setInterval" && d.marker == marker {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected setInterval sink detection, got: %+v", detections)
	}
}

// TestTaintTracking_SetTimeoutFunctionNotDetected verifies functions are not flagged
func TestTaintTracking_SetTimeoutFunctionNotDetected(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "taint_func_" + payloads.GenerateMarker()

	// setTimeout with function should NOT be detected (only string arguments are dangerous)
	html := fmt.Sprintf(`<!DOCTYPE html>
<html><body>
<script>
setTimeout(function() { console.log('%s'); }, 1);
</script>
</body></html>`, marker)

	detections := runTaintTest(t, browser, html, marker, nil)

	for _, d := range detections {
		if d.sink == "setTimeout" {
			t.Errorf("setTimeout with function should NOT be detected as taint sink, got: %+v", d)
		}
	}
}

// TestTaintTracking_TaintPrefixDetection tests detection using TaintMarkerPrefix
func TestTaintTracking_TaintPrefixDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "regular_marker"
	taintValue := payloads.TaintMarkerPrefix + "unique123"

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><body>
<div id="target"></div>
<script>
document.getElementById('target').innerHTML = '%s';
</script>
</body></html>`, taintValue)

	detections := runTaintTest(t, browser, html, marker, nil)

	found := false
	for _, d := range detections {
		if d.sink == "innerHTML" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected taint prefix to trigger innerHTML detection")
	}
}

// TestTaintTracking_NoFalsePositives verifies clean data doesn't trigger detection
func TestTaintTracking_NoFalsePositives(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "unique_marker_xyz789"

	// This HTML uses innerHTML but with clean data (no marker)
	html := `<!DOCTYPE html>
<html><body>
<div id="target"></div>
<script>
document.getElementById('target').innerHTML = '<p>Hello World</p>';
document.write('<span>Safe content</span>');
eval('var x = 1 + 1');
setTimeout('console.log("clean")', 1);
</script>
</body></html>`

	detections := runTaintTest(t, browser, html, marker, nil)

	if len(detections) > 0 {
		t.Errorf("Expected no detections for clean data, got: %+v", detections)
	}
}

// TestTaintTracking_DoubleInjectionPrevention verifies script only injects once
func TestTaintTracking_DoubleInjectionPrevention(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "double_test_" + payloads.GenerateMarker()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html><body>
<div id="target"></div>
<script>
document.getElementById('target').innerHTML = '%s';
</script>
</body></html>`, marker)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	page, err := browser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		t.Fatalf("Failed to create page: %v", err)
	}
	defer page.Close()

	pageCtx := page.Context(ctx)

	var detectionCount int
	var mu sync.Mutex

	go pageCtx.EachEvent(
		func(e *proto.RuntimeConsoleAPICalled) {
			for _, arg := range e.Args {
				val := strings.Trim(arg.Value.String(), "\"")
				if strings.HasPrefix(val, "SUKYAN_SINK:innerHTML:") {
					mu.Lock()
					detectionCount++
					mu.Unlock()
				}
			}
		},
	)()

	// Inject taint tracking script TWICE
	script := GetTaintTrackingScript(marker)
	_, _ = pageCtx.EvalOnNewDocument(script)
	_, _ = pageCtx.EvalOnNewDocument(script) // Second injection should be no-op

	err = pageCtx.Navigate(server.URL)
	if err != nil {
		t.Fatalf("Navigation failed: %v", err)
	}
	pageCtx.WaitLoad()
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	count := detectionCount
	mu.Unlock()

	// Should only detect once, not twice
	if count != 1 {
		t.Errorf("Expected exactly 1 detection (double injection prevention), got %d", count)
	}
}

// TestTaintTracking_OriginalFunctionalityPreserved verifies original methods still work
func TestTaintTracking_OriginalFunctionalityPreserved(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "preserve_test_" + payloads.GenerateMarker()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html><body>
<div id="target"></div>
<script>
// Test that innerHTML still works and actually sets content
document.getElementById('target').innerHTML = '<span id="inner">content</span>';
</script>
</body></html>`)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	page, err := browser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		t.Fatalf("Failed to create page: %v", err)
	}
	defer page.Close()

	pageCtx := page.Context(ctx)

	script := GetTaintTrackingScript(marker)
	_, _ = pageCtx.EvalOnNewDocument(script)

	err = pageCtx.Navigate(server.URL)
	if err != nil {
		t.Fatalf("Navigation failed: %v", err)
	}
	pageCtx.WaitLoad()
	time.Sleep(300 * time.Millisecond)

	// Verify the innerHTML actually worked
	result, err := pageCtx.Eval(`() => document.getElementById('inner') !== null`)
	if err != nil {
		t.Fatalf("Failed to check element: %v", err)
	}

	if !result.Value.Bool() {
		t.Error("innerHTML should still create elements - original functionality broken")
	}
}

// TestTaintTracking_MultipleSinksInSamePage tests detection of multiple different sinks
func TestTaintTracking_MultipleSinksInSamePage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "multi_sink_" + payloads.GenerateMarker()

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><body>
<div id="t1"></div>
<div id="t2"></div>
<script>
document.getElementById('t1').innerHTML = '%s';
document.getElementById('t2').outerHTML = '<div>%s</div>';
eval("var x = '%s'");
</script>
</body></html>`, marker, marker, marker)

	detections := runTaintTest(t, browser, html, marker, nil)

	sinks := make(map[string]bool)
	for _, d := range detections {
		sinks[d.sink] = true
	}

	expectedSinks := []string{"innerHTML", "outerHTML", "eval"}
	for _, expected := range expectedSinks {
		if !sinks[expected] {
			t.Errorf("Expected %s sink to be detected, found sinks: %v", expected, sinks)
		}
	}
}

// TestTaintTracking_URLFromHash tests common DOM XSS pattern with location.hash
func TestTaintTracking_URLFromHash(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "hash_test_" + payloads.GenerateMarker()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html><body>
<div id="output"></div>
<script>
// Common DOM XSS pattern: location.hash -> innerHTML
var data = location.hash.substring(1);
document.getElementById('output').innerHTML = data;
</script>
</body></html>`)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	page, err := browser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		t.Fatalf("Failed to create page: %v", err)
	}
	defer page.Close()

	pageCtx := page.Context(ctx)

	var detections []taintDetection
	var mu sync.Mutex

	go pageCtx.EachEvent(
		func(e *proto.RuntimeConsoleAPICalled) {
			for _, arg := range e.Args {
				val := strings.Trim(arg.Value.String(), "\"")
				if d := parseTaintLog(val); d != nil {
					mu.Lock()
					detections = append(detections, *d)
					mu.Unlock()
				}
			}
		},
	)()

	script := GetTaintTrackingScript(marker)
	_, _ = pageCtx.EvalOnNewDocument(script)

	// Navigate with marker in hash
	err = pageCtx.Navigate(server.URL + "#" + marker)
	if err != nil {
		t.Fatalf("Navigation failed: %v", err)
	}
	pageCtx.WaitLoad()
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	found := false
	for _, d := range detections {
		if d.sink == "innerHTML" && d.marker == marker {
			found = true
			break
		}
	}
	mu.Unlock()

	if !found {
		t.Error("Expected innerHTML sink to be detected when hash flows to innerHTML")
	}
}

// TestTaintTracking_DocumentWriteln tests document.writeln sink
func TestTaintTracking_DocumentWriteln(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	browser := getTestBrowser(t)
	marker := "writeln_" + payloads.GenerateMarker()

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><body>
<script>
document.writeln('<p>%s</p>');
</script>
</body></html>`, marker)

	detections := runTaintTest(t, browser, html, marker, nil)

	found := false
	for _, d := range detections {
		if d.sink == "document.writeln" && d.marker == marker {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected document.writeln sink detection, got: %+v", detections)
	}
}
