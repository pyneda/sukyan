package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// TestQueryParamDiscoveryHookScript verifies the JavaScript hook script for
// query parameter discovery is syntactically correct and contains expected hooks.
func TestQueryParamDiscoveryHookScript(t *testing.T) {
	script := QueryParamDiscoveryHookScript

	// Verify script is not empty
	if len(script) == 0 {
		t.Error("QueryParamDiscoveryHookScript should not be empty")
	}

	// Verify it's wrapped in an IIFE
	if !strings.Contains(script, "(function()") {
		t.Error("Script should be wrapped in an IIFE")
	}

	// Verify it creates the tracking set
	if !strings.Contains(script, "__sukyanAccessedParams") {
		t.Error("Script should create __sukyanAccessedParams Set")
	}

	// Verify it hooks URLSearchParams.prototype.get
	if !strings.Contains(script, "URLSearchParams.prototype.get") {
		t.Error("Script should hook URLSearchParams.prototype.get")
	}

	// Verify it hooks URLSearchParams.prototype.getAll
	if !strings.Contains(script, "URLSearchParams.prototype.getAll") {
		t.Error("Script should hook URLSearchParams.prototype.getAll")
	}

	// Verify it hooks URLSearchParams.prototype.has
	if !strings.Contains(script, "URLSearchParams.prototype.has") {
		t.Error("Script should hook URLSearchParams.prototype.has")
	}

	// Verify it sets a ready flag
	if !strings.Contains(script, "__sukyanHooksReady") {
		t.Error("Script should set __sukyanHooksReady flag")
	}
}

// TestStorageDiscoveryHookScript verifies the JavaScript hook script for
// storage key discovery is syntactically correct and contains expected hooks.
func TestStorageDiscoveryHookScript(t *testing.T) {
	script := StorageDiscoveryHookScript

	// Verify script is not empty
	if len(script) == 0 {
		t.Error("StorageDiscoveryHookScript should not be empty")
	}

	// Verify it's wrapped in an IIFE
	if !strings.Contains(script, "(function()") {
		t.Error("Script should be wrapped in an IIFE")
	}

	// Verify it creates the tracking object
	if !strings.Contains(script, "__sukyanAccessedStorageKeys") {
		t.Error("Script should create __sukyanAccessedStorageKeys object")
	}

	// Verify it tracks both localStorage and sessionStorage
	if !strings.Contains(script, "localStorage: new Set()") {
		t.Error("Script should track localStorage keys")
	}
	if !strings.Contains(script, "sessionStorage: new Set()") {
		t.Error("Script should track sessionStorage keys")
	}

	// Verify it hooks localStorage.getItem
	if !strings.Contains(script, "localStorage.getItem") {
		t.Error("Script should hook localStorage.getItem")
	}

	// Verify it hooks sessionStorage.getItem
	if !strings.Contains(script, "sessionStorage.getItem") {
		t.Error("Script should hook sessionStorage.getItem")
	}

	// Verify it sets a ready flag
	if !strings.Contains(script, "__sukyanStorageHooksReady") {
		t.Error("Script should set __sukyanStorageHooksReady flag")
	}
}

// TestQueryParamHookScriptPreservesOriginalBehavior verifies the hook script
// preserves the original URLSearchParams method behavior by calling the original.
func TestQueryParamHookScriptPreservesOriginalBehavior(t *testing.T) {
	script := QueryParamDiscoveryHookScript

	// Verify each hook stores and calls the original method
	hooks := []struct {
		name     string
		origVar  string
		callOrig string
	}{
		{"get", "origGet", "origGet.call"},
		{"getAll", "origGetAll", "origGetAll.call"},
		{"has", "origHas", "origHas.call"},
	}

	for _, hook := range hooks {
		t.Run(hook.name, func(t *testing.T) {
			if !strings.Contains(script, hook.origVar+" = URLSearchParams.prototype."+hook.name) &&
				!strings.Contains(script, hook.origVar+" =URLSearchParams.prototype."+hook.name) {
				t.Errorf("Script should store original %s method in %s", hook.name, hook.origVar)
			}
			if !strings.Contains(script, hook.callOrig) {
				t.Errorf("Script should call %s to preserve original behavior", hook.callOrig)
			}
		})
	}
}

// TestStorageHookScriptPreservesOriginalBehavior verifies the storage hook script
// preserves the original storage method behavior.
func TestStorageHookScriptPreservesOriginalBehavior(t *testing.T) {
	script := StorageDiscoveryHookScript

	// Verify hooks store and call original methods
	if !strings.Contains(script, "origLSGetItem") {
		t.Error("Script should store original localStorage.getItem")
	}
	if !strings.Contains(script, "origLSGetItem(key)") {
		t.Error("Script should call original localStorage.getItem with key")
	}

	if !strings.Contains(script, "origSSGetItem") {
		t.Error("Script should store original sessionStorage.getItem")
	}
	if !strings.Contains(script, "origSSGetItem(key)") {
		t.Error("Script should call original sessionStorage.getItem with key")
	}
}

// TestHookScriptsAreIIFE verifies both scripts use IIFE pattern to avoid
// polluting global scope with internal variables.
func TestHookScriptsAreIIFE(t *testing.T) {
	scripts := map[string]string{
		"QueryParamDiscoveryHookScript": QueryParamDiscoveryHookScript,
		"StorageDiscoveryHookScript":    StorageDiscoveryHookScript,
	}

	for name, script := range scripts {
		t.Run(name, func(t *testing.T) {
			trimmed := strings.TrimSpace(script)
			if !strings.HasPrefix(trimmed, "(function()") {
				t.Error("Script should start with (function()")
			}
			if !strings.HasSuffix(trimmed, "})();") && !strings.HasSuffix(trimmed, "})();\n") {
				t.Error("Script should end with })();")
			}
		})
	}
}

// TestHookScriptsOnlyAddToSet verifies hooks only add parameters/keys when they exist.
func TestHookScriptsOnlyAddToSet(t *testing.T) {
	// Query param script should check if name exists before adding
	if !strings.Contains(QueryParamDiscoveryHookScript, "if (name)") {
		t.Error("QueryParamDiscoveryHookScript should check if name exists before adding")
	}

	// Storage script should check if key exists before adding
	if !strings.Contains(StorageDiscoveryHookScript, "if (key)") {
		t.Error("StorageDiscoveryHookScript should check if key exists before adding")
	}
}

// TestDefaultBrowserDiscoveryOptions verifies default options are sensible.
func TestDefaultBrowserDiscoveryOptions(t *testing.T) {
	opts := DefaultBrowserDiscoveryOptions()

	if opts.WaitAfterLoad == 0 {
		t.Error("Default WaitAfterLoad should be non-zero")
	}

	if opts.PageTimeout == 0 {
		t.Error("Default PageTimeout should be non-zero")
	}

	// WaitAfterLoad should be less than PageTimeout
	if opts.WaitAfterLoad >= opts.PageTimeout {
		t.Error("WaitAfterLoad should be less than PageTimeout")
	}
}

// TestDiscoverQueryParamsInBrowserNilBrowser verifies error handling when browser is nil.
func TestDiscoverQueryParamsInBrowserNilBrowser(t *testing.T) {
	ctx := context.Background()

	// Passing nil browser should return an error
	params, err := DiscoverQueryParamsInBrowser(ctx, nil, "https://example.com", nil)

	if err == nil {
		t.Error("Expected error when browser is nil")
	}
	if params != nil {
		t.Error("Expected nil params when browser is nil")
	}
}

// TestDiscoverQueryParamsInBrowserCancelledContext verifies behavior with cancelled context.
func TestDiscoverQueryParamsInBrowserCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// With cancelled context, should fail gracefully
	params, err := DiscoverQueryParamsInBrowser(ctx, nil, "https://example.com", nil)

	// Should return error (either from nil browser or cancelled context)
	if err == nil {
		t.Error("Expected error with cancelled context")
	}
	if params != nil {
		t.Error("Expected nil params with cancelled context")
	}
}

// TestDiscoverStorageKeysInBrowserNilBrowser verifies error handling when browser is nil.
func TestDiscoverStorageKeysInBrowserNilBrowser(t *testing.T) {
	ctx := context.Background()

	// Passing nil browser should return an error
	keys, err := DiscoverStorageKeysInBrowser(ctx, nil, "https://example.com", "localStorage", nil)

	if err == nil {
		t.Error("Expected error when browser is nil")
	}
	if keys != nil {
		t.Error("Expected nil keys when browser is nil")
	}
}

// TestDiscoverStorageKeysInBrowserInvalidStorageType verifies error handling for invalid storage type.
func TestDiscoverStorageKeysInBrowserInvalidStorageType(t *testing.T) {
	ctx := context.Background()

	invalidTypes := []string{
		"invalidStorage",
		"LocalStorage", // case sensitive
		"session",
		"",
	}

	for _, storageType := range invalidTypes {
		t.Run(storageType, func(t *testing.T) {
			keys, err := DiscoverStorageKeysInBrowser(ctx, nil, "https://example.com", storageType, nil)

			if err == nil {
				t.Errorf("Expected error for invalid storage type: %q", storageType)
			}
			if keys != nil {
				t.Error("Expected nil keys for invalid storage type")
			}
			if err != nil && !strings.Contains(err.Error(), "invalid storage type") {
				t.Errorf("Error should mention 'invalid storage type', got: %v", err)
			}
		})
	}
}

// TestDiscoverStorageKeysInBrowserValidStorageTypes verifies valid storage types are accepted.
func TestDiscoverStorageKeysInBrowserValidStorageTypes(t *testing.T) {
	ctx := context.Background()

	validTypes := []string{
		"localStorage",
		"sessionStorage",
	}

	for _, storageType := range validTypes {
		t.Run(storageType, func(t *testing.T) {
			// Will fail due to nil browser, but should NOT fail due to invalid storage type
			_, err := DiscoverStorageKeysInBrowser(ctx, nil, "https://example.com", storageType, nil)

			if err != nil && strings.Contains(err.Error(), "invalid storage type") {
				t.Errorf("Storage type %q should be valid", storageType)
			}
		})
	}
}

// TestDiscoverQueryParamsInBrowserWithCustomOptions verifies custom options are accepted.
func TestDiscoverQueryParamsInBrowserWithCustomOptions(t *testing.T) {
	ctx := context.Background()
	opts := &BrowserDiscoveryOptions{
		WaitAfterLoad: 100 * time.Millisecond,
		PageTimeout:   5 * time.Second,
	}

	// Will fail due to nil browser, but should accept custom options
	_, err := DiscoverQueryParamsInBrowser(ctx, nil, "https://example.com", opts)

	// Error should be about browser, not options
	if err == nil {
		t.Error("Expected error when browser is nil")
	}
}

// TestDiscoverStorageKeysInBrowserWithCustomOptions verifies custom options are accepted.
func TestDiscoverStorageKeysInBrowserWithCustomOptions(t *testing.T) {
	ctx := context.Background()
	opts := &BrowserDiscoveryOptions{
		WaitAfterLoad: 200 * time.Millisecond,
		PageTimeout:   3 * time.Second,
	}

	// Will fail due to nil browser, but should accept custom options
	_, err := DiscoverStorageKeysInBrowser(ctx, nil, "https://example.com", "localStorage", opts)

	// Error should be about browser, not options
	if err == nil {
		t.Error("Expected error when browser is nil")
	}
}

// TestDiscoverQueryParamsInBrowserEmptyURL verifies handling of empty URL.
func TestDiscoverQueryParamsInBrowserEmptyURL(t *testing.T) {
	ctx := context.Background()

	// Empty URL with nil browser - should fail on browser first
	_, err := DiscoverQueryParamsInBrowser(ctx, nil, "", nil)

	if err == nil {
		t.Error("Expected error with empty URL and nil browser")
	}
}

// TestDiscoverStorageKeysInBrowserEmptyURL verifies handling of empty URL.
func TestDiscoverStorageKeysInBrowserEmptyURL(t *testing.T) {
	ctx := context.Background()

	// Empty URL with nil browser - should fail on browser first
	_, err := DiscoverStorageKeysInBrowser(ctx, nil, "", "localStorage", nil)

	if err == nil {
		t.Error("Expected error with empty URL and nil browser")
	}
}

// TestDiscoverQueryParamsInBrowserIntegration tests the actual browser-based
// query parameter discovery with a real browser and test HTTP server.
func TestDiscoverQueryParamsInBrowserIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser integration test in short mode")
	}

	// Test cases with different JavaScript patterns that access URL parameters
	testCases := []struct {
		name           string
		html           string
		expectedParams []string
	}{
		{
			name: "URLSearchParams.get single param",
			html: `<!DOCTYPE html>
<html><body>
<script>
const params = new URLSearchParams(window.location.search);
const value = params.get('foo');
console.log(value);
</script>
</body></html>`,
			expectedParams: []string{"foo"},
		},
		{
			name: "URLSearchParams.get multiple params",
			html: `<!DOCTYPE html>
<html><body>
<script>
const params = new URLSearchParams(window.location.search);
const a = params.get('alpha');
const b = params.get('beta');
const c = params.get('gamma');
</script>
</body></html>`,
			expectedParams: []string{"alpha", "beta", "gamma"},
		},
		{
			name: "URLSearchParams.has and getAll",
			html: `<!DOCTYPE html>
<html><body>
<script>
const params = new URLSearchParams(window.location.search);
// has() is always called regardless of result
const hasCheck = params.has('checkme');
// getAll is called unconditionally
const allItems = params.getAll('items');
</script>
</body></html>`,
			expectedParams: []string{"checkme", "items"},
		},
		{
			name: "no params accessed",
			html: `<!DOCTYPE html>
<html><body>
<script>
console.log('Hello world');
var x = 1 + 2;
</script>
</body></html>`,
			expectedParams: []string{},
		},
		{
			name: "params accessed in async function",
			html: `<!DOCTYPE html>
<html><body>
<script>
async function init() {
    const params = new URLSearchParams(location.search);
    const token = params.get('token');
    const redirect = params.get('redirect_url');
}
init();
</script>
</body></html>`,
			expectedParams: []string{"token", "redirect_url"},
		},
		{
			name: "params accessed via URL object",
			html: `<!DOCTYPE html>
<html><body>
<script>
const url = new URL(window.location.href);
const id = url.searchParams.get('id');
const action = url.searchParams.get('action');
</script>
</body></html>`,
			expectedParams: []string{"id", "action"},
		},
	}

	// Start browser once for all test cases
	path, _ := launcher.LookPath()
	u := launcher.New().Bin(path).Headless(true).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test server for this case
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, tc.html)
			}))
			defer server.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			opts := &BrowserDiscoveryOptions{
				WaitAfterLoad: 300 * time.Millisecond,
				PageTimeout:   5 * time.Second,
			}

			params, err := DiscoverQueryParamsInBrowser(ctx, browser, server.URL, opts)
			if err != nil {
				t.Fatalf("DiscoverQueryParamsInBrowser failed: %v", err)
			}

			// Sort both slices for comparison
			sort.Strings(params)
			expected := make([]string, len(tc.expectedParams))
			copy(expected, tc.expectedParams)
			sort.Strings(expected)

			if len(params) != len(expected) {
				t.Errorf("Expected %d params %v, got %d params %v", len(expected), expected, len(params), params)
				return
			}

			for i, p := range params {
				if p != expected[i] {
					t.Errorf("Param mismatch at index %d: expected %q, got %q", i, expected[i], p)
				}
			}
		})
	}
}
