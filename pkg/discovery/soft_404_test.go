package discovery

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const catchAllBody = `<!doctype html><html><head><title>App</title></head><body><div id="root">admin dashboard</div></body></html>`

// setupCatchAllServer answers 200 with an identical body for every path, like an
// SPA that serves index.html as a fallback route.
func setupCatchAllServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, catchAllBody)
	}))
}

// catchAllSiteBehavior mirrors what the site_behavior phase records for such a
// host: no 404s, and every sampled path hashing to the base URL response.
func catchAllSiteBehavior(baseURL string) *http_utils.SiteBehavior {
	sample := &db.History{
		URL:                 baseURL,
		StatusCode:          200,
		ResponseContentType: "text/html",
		RawResponse:         []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n" + catchAllBody),
	}
	return &http_utils.SiteBehavior{
		NotFoundReturns404: false,
		NotFoundStatusCode: 200,
		NotFoundCommonHash: sample.ResponseHash(),
		BaseURLSample:      sample,
		NotFoundSamples:    []*db.History{sample},
	}
}

// A response the site behavior classifies as a soft 404 must not reach
// results.Responses, otherwise every discovery module's ValidationFunc runs over
// it and files an issue for what is really the catch-all page.
func TestDiscoverPathsExcludesSoft404Responses(t *testing.T) {
	workspace := setupTestWorkspace(t)
	server := setupCatchAllServer()
	defer server.Close()

	behavior := catchAllSiteBehavior(server.URL)

	results, err := DiscoverPaths(DiscoveryInput{
		URL:          server.URL,
		Method:       "GET",
		Paths:        AdminPaths,
		Concurrency:  4,
		Timeout:      30,
		SiteBehavior: behavior,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			WorkspaceID: workspace.ID,
			Source:      db.SourceScanner,
		},
	})
	require.NoError(t, err)

	for _, history := range results.Responses {
		assert.Falsef(t, behavior.IsNotFound(history),
			"soft 404 response for %s was collected into results.Responses", history.URL)
	}
	assert.Empty(t, results.Responses, "every path on a catch-all host is a soft 404")
}

// End-to-end guard for the same bug through the issue-creating entrypoint that
// every discovery module uses.
func TestDiscoverAndCreateIssueSkipsSoft404Host(t *testing.T) {
	workspace := setupTestWorkspace(t)
	server := setupCatchAllServer()
	defer server.Close()

	output, err := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:          server.URL,
			Method:       "GET",
			Paths:        AdminPaths,
			Concurrency:  4,
			Timeout:      30,
			SiteBehavior: catchAllSiteBehavior(server.URL),
			HistoryCreationOptions: http_utils.HistoryCreationOptions{
				WorkspaceID: workspace.ID,
				Source:      db.SourceScanner,
			},
		},
		ValidationFunc: IsAdminInterfaceValidationFunc,
		IssueCode:      db.AdminInterfaceDetectedCode,
	})
	require.NoError(t, err)
	assert.Empty(t, output.Issues, "a catch-all host must not yield admin interface issues")
}

// The filter must stay inert on a host with real 404s, so genuine 200s still
// reach validation.
func TestDiscoverPathsKeepsResponsesOnNormal404Host(t *testing.T) {
	workspace := setupTestWorkspace(t)

	server := setupTestServer(serverConfig{validPaths: []string{"/admin", "/dashboard"}})
	defer server.Close()

	behavior := &http_utils.SiteBehavior{
		NotFoundReturns404: true,
		NotFoundStatusCode: 404,
		BaseURLSample: &db.History{
			URL:         server.URL,
			StatusCode:  404,
			RawResponse: []byte("HTTP/1.1 404 Not Found\r\n\r\nNot Found"),
		},
	}

	results, err := DiscoverPaths(DiscoveryInput{
		URL:          server.URL,
		Method:       "GET",
		Paths:        []string{"admin", "dashboard", "definitely-not-here"},
		Concurrency:  3,
		Timeout:      30,
		SiteBehavior: behavior,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			WorkspaceID: workspace.ID,
			Source:      db.SourceScanner,
		},
	})
	require.NoError(t, err)

	got := make(map[string]int, len(results.Responses))
	for _, history := range results.Responses {
		got[history.URL] = history.StatusCode
	}
	assert.Equal(t, 200, got[server.URL+"/admin"])
	assert.Equal(t, 200, got[server.URL+"/dashboard"])
	assert.NotContains(t, got, server.URL+"/definitely-not-here", "real 404s are still filtered")
}
