package web

import (
	"context"
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
)

// QueryParamDiscoveryHookScript is the JavaScript code that hooks URLSearchParams
// methods to track which query parameters the application accesses at runtime.
const QueryParamDiscoveryHookScript = `
(function() {
    window.__sukyanAccessedParams = new Set();

    // Hook URLSearchParams.prototype.get
    const origGet = URLSearchParams.prototype.get;
    URLSearchParams.prototype.get = function(name) {
        if (name) window.__sukyanAccessedParams.add(name);
        return origGet.call(this, name);
    };

    // Hook URLSearchParams.prototype.getAll
    const origGetAll = URLSearchParams.prototype.getAll;
    URLSearchParams.prototype.getAll = function(name) {
        if (name) window.__sukyanAccessedParams.add(name);
        return origGetAll.call(this, name);
    };

    // Hook URLSearchParams.prototype.has
    const origHas = URLSearchParams.prototype.has;
    URLSearchParams.prototype.has = function(name) {
        if (name) window.__sukyanAccessedParams.add(name);
        return origHas.call(this, name);
    };

    window.__sukyanHooksReady = true;
})();
`

// StorageDiscoveryHookScript is the JavaScript code that hooks localStorage and
// sessionStorage methods to track which keys the application accesses at runtime.
const StorageDiscoveryHookScript = `
(function() {
    window.__sukyanAccessedStorageKeys = {
        localStorage: new Set(),
        sessionStorage: new Set()
    };

    // Hook localStorage.getItem
    const origLSGetItem = localStorage.getItem.bind(localStorage);
    localStorage.getItem = function(key) {
        if (key) window.__sukyanAccessedStorageKeys.localStorage.add(key);
        return origLSGetItem(key);
    };

    // Hook sessionStorage.getItem
    const origSSGetItem = sessionStorage.getItem.bind(sessionStorage);
    sessionStorage.getItem = function(key) {
        if (key) window.__sukyanAccessedStorageKeys.sessionStorage.add(key);
        return origSSGetItem(key);
    };

    window.__sukyanStorageHooksReady = true;
})();
`

// BrowserDiscoveryOptions configures browser-based parameter discovery
type BrowserDiscoveryOptions struct {
	// WaitAfterLoad is the time to wait for async JavaScript after page load
	WaitAfterLoad time.Duration
	// PageTimeout is the timeout for page operations
	PageTimeout time.Duration
}

// DefaultBrowserDiscoveryOptions returns sensible defaults for browser discovery
func DefaultBrowserDiscoveryOptions() BrowserDiscoveryOptions {
	return BrowserDiscoveryOptions{
		WaitAfterLoad: 500 * time.Millisecond,
		PageTimeout:   10 * time.Second,
	}
}

// DiscoverQueryParamsInBrowser loads a page with JavaScript hooks to intercept
// URLSearchParams access and returns the parameter names the application reads.
// This is more accurate than static analysis as it captures runtime behavior.
func DiscoverQueryParamsInBrowser(ctx context.Context, rodBrowser *rod.Browser, targetURL string, opts *BrowserDiscoveryOptions) ([]string, error) {
	if rodBrowser == nil {
		return nil, fmt.Errorf("browser is nil")
	}

	if opts == nil {
		defaults := DefaultBrowserDiscoveryOptions()
		opts = &defaults
	}

	taskLog := log.With().
		Str("url", targetURL).
		Logger()

	page, err := rodBrowser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		return nil, fmt.Errorf("failed to create page: %w", err)
	}
	defer page.Close()

	IgnoreCertificateErrors(page)
	pageWithCtx := page.Context(ctx)

	// Add script to evaluate on new document (runs before page scripts)
	_, err = pageWithCtx.EvalOnNewDocument(QueryParamDiscoveryHookScript)
	if err != nil {
		return nil, fmt.Errorf("failed to inject hooks: %w", err)
	}

	// Navigate to the target URL
	err = pageWithCtx.Navigate(targetURL)
	if err != nil {
		return nil, fmt.Errorf("navigation failed: %w", err)
	}

	// Wait for page load
	err = pageWithCtx.WaitLoad()
	if err != nil {
		taskLog.Debug().Err(err).Msg("WaitLoad returned error during param discovery")
	}

	// Wait for async JavaScript to execute
	time.Sleep(opts.WaitAfterLoad)

	// Retrieve the accessed parameters
	result, err := pageWithCtx.Eval(`() => {
		if (window.__sukyanAccessedParams) {
			return Array.from(window.__sukyanAccessedParams);
		}
		return [];
	}`)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve accessed params: %w", err)
	}

	// Parse the result
	var accessedParams []string
	if result.Value.Arr() != nil {
		for _, v := range result.Value.Arr() {
			if s := v.Str(); s != "" {
				accessedParams = append(accessedParams, s)
			}
		}
	}

	taskLog.Debug().
		Int("discovered", len(accessedParams)).
		Strs("params", accessedParams).
		Msg("Discovered query parameters via browser hooks")

	return accessedParams, nil
}

// DiscoverStorageKeysInBrowser loads a page with JavaScript hooks to intercept
// storage access and returns the keys the application reads from localStorage or sessionStorage.
// storageType should be either "localStorage" or "sessionStorage".
func DiscoverStorageKeysInBrowser(ctx context.Context, rodBrowser *rod.Browser, targetURL string, storageType string, opts *BrowserDiscoveryOptions) ([]string, error) {
	if storageType != "localStorage" && storageType != "sessionStorage" {
		return nil, fmt.Errorf("invalid storage type: %s (must be 'localStorage' or 'sessionStorage')", storageType)
	}

	if rodBrowser == nil {
		return nil, fmt.Errorf("browser is nil")
	}

	if opts == nil {
		defaults := DefaultBrowserDiscoveryOptions()
		opts = &defaults
	}

	taskLog := log.With().
		Str("url", targetURL).
		Str("storageType", storageType).
		Logger()

	page, err := rodBrowser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		return nil, fmt.Errorf("failed to create page: %w", err)
	}
	defer page.Close()

	IgnoreCertificateErrors(page)
	pageWithCtx := page.Context(ctx)

	// Add script to evaluate on new document
	_, err = pageWithCtx.EvalOnNewDocument(StorageDiscoveryHookScript)
	if err != nil {
		return nil, fmt.Errorf("failed to inject hooks: %w", err)
	}

	// Navigate to the target URL
	err = pageWithCtx.Navigate(targetURL)
	if err != nil {
		return nil, fmt.Errorf("navigation failed: %w", err)
	}

	// Wait for page load
	err = pageWithCtx.WaitLoad()
	if err != nil {
		taskLog.Debug().Err(err).Msg("WaitLoad returned error during storage key discovery")
	}

	// Wait for async JavaScript
	time.Sleep(opts.WaitAfterLoad)

	// Retrieve the accessed keys for the specified storage type
	evalScript := fmt.Sprintf(`() => {
		if (window.__sukyanAccessedStorageKeys && window.__sukyanAccessedStorageKeys.%s) {
			return Array.from(window.__sukyanAccessedStorageKeys.%s);
		}
		return [];
	}`, storageType, storageType)

	result, err := pageWithCtx.Eval(evalScript)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve accessed storage keys: %w", err)
	}

	// Parse the result
	var accessedKeys []string
	if result.Value.Arr() != nil {
		for _, v := range result.Value.Arr() {
			if s := v.Str(); s != "" {
				accessedKeys = append(accessedKeys, s)
			}
		}
	}

	taskLog.Debug().
		Int("discovered", len(accessedKeys)).
		Strs("keys", accessedKeys).
		Msg("Discovered storage keys via browser hooks")

	return accessedKeys, nil
}
