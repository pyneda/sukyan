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
	"github.com/stretchr/testify/assert"
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

	config := HijackConfig{AnalyzeJs: false, AnalyzeHTML: false}
	router := HijackWithContext(config, browser, server.URL, resultsChannel, ctx, workspace.ID, 0, 0, 0)
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

	config := HijackConfig{AnalyzeJs: false, AnalyzeHTML: false}
	Hijack(config, browser, "test", resultsChannel, workspace.ID, 0, 0, 0)

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
