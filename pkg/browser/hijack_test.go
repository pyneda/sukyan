package browser

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/assert"
	"github.com/ysmood/gson"
)

// setupHijackMockServer sets up a mock server with various endpoints
func setupHijackMockServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"message": "JSON response"})
		case "/text":
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, "Text response")
		case "/redirect":
			http.Redirect(w, r, "/final", http.StatusFound)
		case "/final":
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, "Final destination")
		case "/html-with-link":
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, `<html><body><a href="http://example.com/discovered-link">Link</a></body></html>`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// TestHijackWithContext tests the HijackWithContext function for different HTTP scenarios
func TestHijackWithContext(t *testing.T) {

	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Code:        "test-hijack",
		Title:       "test-hijack",
		Description: "test-hijack",
	})
	assert.NoError(t, err)

	server := setupHijackMockServer()
	defer server.Close()
	browser := setupRodBrowser(t, true)
	defer browser.MustClose()

	resultsChannel := make(chan HijackResult)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config := HijackConfig{}
	router := HijackWithContext(config, browser, nil, server.URL, resultsChannel, ctx, workspace.ID, 0, 0, 0)
	defer router.Stop()

	wg := sync.WaitGroup{}
	wg.Add(3)

	// Collecting and validating results
	go func() {
		processed := 0
		for res := range resultsChannel {
			wg.Done()
			// t.Log("Received hijack result:", res)
			assert.NotNil(t, res.History)
			assert.NotEmpty(t, res.History.URL)
			assert.Greater(t, res.History.StatusCode, 0)
			assert.Contains(t, string(res.History.Method), "GET")

			// Specific assertions based on the request
			switch res.History.URL {
			case server.URL + "/json":
				assert.Contains(t, string(res.History.RawResponse), "JSON response")
				assert.Contains(t, string(res.History.RawResponse), "Content-Type: application/json")
				body, err := res.History.ResponseBody()
				assert.NoError(t, err)
				assert.Contains(t, string(body), "JSON response")
			case server.URL + "/text":
				assert.Contains(t, string(res.History.RawResponse), "Text response")
				assert.Contains(t, string(res.History.RawResponse), "Content-Type: text/plain")
				body, err := res.History.ResponseBody()
				assert.NoError(t, err)
				assert.Contains(t, string(body), "Text response")
			case server.URL + "/final":
				assert.Contains(t, string(res.History.RawResponse), "Final destination")
				assert.Contains(t, string(res.History.RawResponse), "Content-Type: text/plain")
				body, err := res.History.ResponseBody()
				assert.NoError(t, err)
				assert.Contains(t, string(body), "Final destination")
			}

			processed++
			if processed >= 3 {
				close(resultsChannel)
			}

		}
	}()

	t.Log("server.URL", server.URL+"/final")

	page := browser.MustPage(server.URL + "/final")

	// Making requests to different endpoints
	page.MustNavigate(server.URL + "/json")
	page.MustNavigate(server.URL + "/text")
	page.MustNavigate(server.URL + "/redirect")
	wg.Wait()
}

func TestHijack(t *testing.T) {
	server := setupHijackMockServer()
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Code:        "test-hijack",
		Title:       "test-hijack",
		Description: "test-hijack",
	})

	assert.NoError(t, err)

	defer server.Close()

	browser := setupRodBrowser(t, true)
	defer browser.MustClose()

	resultsChannel := make(chan HijackResult)

	config := HijackConfig{}
	Hijack(config, browser, nil, "test", resultsChannel, workspace.ID, 0, 0, 0)

	wg := sync.WaitGroup{}
	wg.Add(4)

	// Collecting and validating results
	go func() {
		processed := 0
		for res := range resultsChannel {
			wg.Done()
			// t.Log("Received hijack result:", res)
			assert.NotNil(t, res.History)
			assert.NotEmpty(t, res.History.URL)
			assert.Greater(t, res.History.StatusCode, 0)
			assert.Contains(t, string(res.History.Method), "GET")

			// Specific assertions based on the request
			switch res.History.URL {
			case server.URL + "/json":
				assert.Contains(t, string(res.History.RawResponse), "JSON response")
				assert.Contains(t, string(res.History.RawResponse), "Content-Type: application/json")
				body, err := res.History.ResponseBody()
				assert.NoError(t, err)
				assert.Contains(t, string(body), "JSON response")
			case server.URL + "/text":
				assert.Contains(t, string(res.History.RawResponse), "Text response")
				assert.Contains(t, string(res.History.RawResponse), "Content-Type: text/plain")
				body, err := res.History.ResponseBody()
				assert.NoError(t, err)
				assert.Contains(t, string(body), "Text response")
			case server.URL + "/final":
				assert.Contains(t, string(res.History.RawResponse), "Final destination")
				assert.Contains(t, string(res.History.RawResponse), "Content-Type: text/plain")
				body, err := res.History.ResponseBody()
				assert.NoError(t, err)
				assert.Contains(t, string(body), "Final destination")
			}

			processed++
			if processed >= 4 {
				close(resultsChannel)
			}

		}
	}()
	page := browser.MustPage(server.URL + "/final")
	// Making requests to different endpoints
	page.MustNavigate(server.URL + "/json")
	page.MustNavigate(server.URL + "/text")
	page.MustNavigate(server.URL + "/redirect")
	wg.Wait()
}

// hijackExtractGatingCase is the shared table for pinning the ShouldExtract contract. It is run
// against both HijackWithContext (TestHijackShouldExtractGating) and Hijack
// (TestHijackShouldExtractGatingNonContext), since the two functions carry independent copies
// of the same gating conditional.
type hijackExtractGatingCase struct {
	name          string
	shouldExtract func(history *db.History) bool
	expectExtract bool
}

func hijackExtractGatingCases() []hijackExtractGatingCase {
	return []hijackExtractGatingCase{
		{"nil predicate skips extraction", nil, false},
		{"predicate returning true extracts", func(*db.History) bool { return true }, true},
		{"predicate returning false skips extraction", func(*db.History) bool { return false }, false},
	}
}

// TestHijackShouldExtractGating pins the ShouldExtract contract on HijackWithContext: a nil
// predicate (the zero value used by every browser-audit caller) must never run URL extraction,
// while a supplied predicate gates it per response based on its return value.
func TestHijackShouldExtractGating(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Code:        "test-hijack-extract-gating",
		Title:       "test-hijack-extract-gating",
		Description: "test-hijack-extract-gating",
	})
	assert.NoError(t, err)

	server := setupHijackMockServer()
	defer server.Close()

	targetURL := server.URL + "/html-with-link"

	for _, tt := range hijackExtractGatingCases() {
		t.Run(tt.name, func(t *testing.T) {
			browser := setupRodBrowser(t, true)
			defer browser.MustClose()

			resultsChannel := make(chan HijackResult)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			config := HijackConfig{ShouldExtract: tt.shouldExtract}
			router := HijackWithContext(config, browser, nil, server.URL, resultsChannel, ctx, workspace.ID, 0, 0, 0)
			defer router.Stop()

			browser.MustPage(targetURL)

			for {
				select {
				case res, ok := <-resultsChannel:
					if !ok {
						t.Fatal("results channel closed before the target response was observed")
					}
					if res.History.URL != targetURL {
						continue
					}
					if tt.expectExtract {
						assert.NotEmpty(t, res.DiscoveredURLs, "expected DiscoveredURLs to be populated")
					} else {
						assert.Empty(t, res.DiscoveredURLs, "expected DiscoveredURLs to stay empty")
					}
					return
				case <-ctx.Done():
					t.Fatal("timed out waiting for hijack result for " + targetURL)
				}
			}
		})
	}
}

// TestHijackShouldExtractGatingNonContext pins the same ShouldExtract contract on Hijack — the
// router pages_pool.go actually drives (via browser_pool.go:111) and the one the crawler's
// extraction depends on in production. Hijack carries its own copy of the gating conditional
// independent of HijackWithContext, so it needs its own coverage rather than inheriting
// TestHijackShouldExtractGating's.
func TestHijackShouldExtractGatingNonContext(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Code:        "test-hijack-extract-gating-nonctx",
		Title:       "test-hijack-extract-gating-nonctx",
		Description: "test-hijack-extract-gating-nonctx",
	})
	assert.NoError(t, err)

	server := setupHijackMockServer()
	defer server.Close()

	targetURL := server.URL + "/html-with-link"

	for _, tt := range hijackExtractGatingCases() {
		t.Run(tt.name, func(t *testing.T) {
			browser := setupRodBrowser(t, true)
			defer browser.MustClose()

			// Buffered so a stray hijacked request (e.g. a browser-initiated favicon
			// fetch) that arrives after the target response doesn't block its sending
			// goroutine forever — Hijack's send has no context/select escape hatch.
			resultsChannel := make(chan HijackResult, 8)

			config := HijackConfig{ShouldExtract: tt.shouldExtract}
			Hijack(config, browser, nil, server.URL, resultsChannel, workspace.ID, 0, 0, 0)

			browser.MustPage(targetURL)

			deadline := time.After(10 * time.Second)
			for {
				select {
				case res := <-resultsChannel:
					if res.History.URL != targetURL {
						continue
					}
					if tt.expectExtract {
						assert.NotEmpty(t, res.DiscoveredURLs, "expected DiscoveredURLs to be populated")
					} else {
						assert.Empty(t, res.DiscoveredURLs, "expected DiscoveredURLs to stay empty")
					}
					return
				case <-deadline:
					t.Fatal("timed out waiting for hijack result for " + targetURL)
				}
			}
		})
	}
}

func TestContentTypeFromNetworkHeaders(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected string
	}{
		{"canonical key", map[string]string{"Content-Type": "application/json"}, "application/json"},
		{"lowercase key (browser fetch over CDP)", map[string]string{"content-type": "application/xml"}, "application/xml"},
		{"mixed case key", map[string]string{"CONTENT-TYPE": "text/xml"}, "text/xml"},
		{"absent", map[string]string{"Accept": "*/*"}, ""},
		{"empty map", map[string]string{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nh := proto.NetworkHeaders{}
			for k, v := range tt.headers {
				nh[k] = gson.New(v)
			}
			if got := contentTypeFromNetworkHeaders(nh); got != tt.expected {
				t.Errorf("contentTypeFromNetworkHeaders(%v) = %q, want %q", tt.headers, got, tt.expected)
			}
		})
	}
}
