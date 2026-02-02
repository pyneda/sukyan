package http_utils

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
)

// Test server types that we want to detect:
// 1. Standard server that returns 404s
// 2. Server that returns same response for everything (no 404s)
// 3. Server that returns custom error pages with 200
type testServer struct {
	server *httptest.Server
}

func createStandard404Server() *testServer {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "<html><body>Home page</body></html>")
			return
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, "<html><body>404 Not Found</body></html>")
	})
	return &testServer{httptest.NewServer(handler)}
}

func createSameResponseServer() *testServer {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "<html><body>Always the same response</body></html>")
	})
	return &testServer{httptest.NewServer(handler)}
}

func createCustomErrorServer() *testServer {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if r.URL.Path == "/" || r.URL.Path == "" {
			fmt.Fprintln(w, "<html><body style='background-color: gray;'><h1>Welcome page!!!!</h1></body></html>")
			return
		}
		fmt.Fprintln(w, "<html><body>Custom error page - Not found</body></html>")
	})
	return &testServer{httptest.NewServer(handler)}
}

func setupTestWorkspace(t *testing.T) uint {
	workspace := &db.Workspace{
		Code:        "behaviour-workspace",
		Title:       "Behaviour Test Workspace",
		Description: "Test workspace for behavior detection tests",
	}
	createdWorkspace, err := db.Connection().GetOrCreateWorkspace(workspace)
	if err != nil {
		t.Fatalf("Failed to create test workspace: %v", err)
	}
	return createdWorkspace.ID
}

func TestSiteBehaviorDetection(t *testing.T) {
	workspaceID := setupTestWorkspace(t)

	tests := []struct {
		name     string
		setup    func() *testServer
		validate func(*testing.T, *SiteBehavior)
	}{
		{
			name:  "Standard 404 Server",
			setup: createStandard404Server,
			validate: func(t *testing.T, behavior *SiteBehavior) {
				assert.True(t, behavior.NotFoundReturns404, "should detect standard 404 responses")
				assert.False(t, behavior.NotFoundChanges, "404 responses should be consistent")
				assert.NotEqual(t, behavior.NotFoundCommonHash, behavior.BaseURLSample.ResponseHash(), "error pages should differ from base URL")
			},
		},
		{
			name:  "Same Response Server",
			setup: createSameResponseServer,
			validate: func(t *testing.T, behavior *SiteBehavior) {
				assert.False(t, behavior.NotFoundReturns404, "should not detect 404s")
				assert.False(t, behavior.NotFoundChanges, "responses should not change")
				assert.Equal(t, behavior.NotFoundCommonHash, behavior.BaseURLSample.ResponseHash(), "all responses should match base URL")
			},
		},
		{
			name:  "Custom Error Pages",
			setup: createCustomErrorServer,
			validate: func(t *testing.T, behavior *SiteBehavior) {
				assert.False(t, behavior.NotFoundReturns404, "should not detect 404s")
				assert.False(t, behavior.NotFoundChanges, "error pages should be consistent")
				assert.NotEqual(t, behavior.NotFoundCommonHash, behavior.BaseURLSample.ResponseHash(), "error pages should differ from base URL")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setup()
			defer server.server.Close()

			behavior, err := CheckSiteBehavior(SiteBehaviourCheckOptions{
				BaseURL: server.server.URL,
				HistoryCreationOptions: HistoryCreationOptions{
					Source:      db.SourceScanner,
					WorkspaceID: workspaceID,
				},
			})

			assert.NoError(t, err)
			assert.NotNil(t, behavior)
			tt.validate(t, behavior)
		})
	}
}

func TestNewSiteBehaviorFromResult(t *testing.T) {
	t.Run("nil result returns nil", func(t *testing.T) {
		sb := NewSiteBehaviorFromResult(nil)
		assert.Nil(t, sb)
	})

	t.Run("reconstructs scalar fields", func(t *testing.T) {
		result := &db.SiteBehaviorResult{
			NotFoundReturns404: true,
			NotFoundChanges:    false,
			NotFoundCommonHash: "abc123",
			NotFoundStatusCode: 404,
		}
		sb := NewSiteBehaviorFromResult(result)
		assert.NotNil(t, sb)
		assert.True(t, sb.NotFoundReturns404)
		assert.False(t, sb.NotFoundChanges)
		assert.Equal(t, "abc123", sb.NotFoundCommonHash)
		assert.Equal(t, 404, sb.NotFoundStatusCode)
		assert.Nil(t, sb.BaseURLSample)
		assert.Empty(t, sb.NotFoundSamples)
	})

	t.Run("reconstructs base URL sample", func(t *testing.T) {
		baseHistory := &db.History{URL: "http://example.com/"}
		baseHistory.ID = 42
		result := &db.SiteBehaviorResult{
			NotFoundReturns404: false,
			BaseURLSample:      baseHistory,
		}
		sb := NewSiteBehaviorFromResult(result)
		assert.NotNil(t, sb.BaseURLSample)
		assert.Equal(t, uint(42), sb.BaseURLSample.ID)
		assert.Equal(t, "http://example.com/", sb.BaseURLSample.URL)
	})

	t.Run("reconstructs not found samples", func(t *testing.T) {
		h1 := db.History{URL: "http://example.com/nf1"}
		h1.ID = 10
		h2 := db.History{URL: "http://example.com/nf2"}
		h2.ID = 20
		result := &db.SiteBehaviorResult{
			NotFoundSamples: []db.SiteBehaviorNotFoundSample{
				{History: h1},
				{History: h2},
			},
		}
		sb := NewSiteBehaviorFromResult(result)
		assert.Len(t, sb.NotFoundSamples, 2)
		assert.Equal(t, uint(10), sb.NotFoundSamples[0].ID)
		assert.Equal(t, uint(20), sb.NotFoundSamples[1].ID)
	})

	t.Run("empty not found samples produces empty slice", func(t *testing.T) {
		result := &db.SiteBehaviorResult{
			NotFoundSamples: []db.SiteBehaviorNotFoundSample{},
		}
		sb := NewSiteBehaviorFromResult(result)
		assert.Empty(t, sb.NotFoundSamples)
	})
}

func TestIsNotFound(t *testing.T) {
	workspaceID := setupTestWorkspace(t)

	tests := []struct {
		name  string
		setup func() *testServer
		paths []struct {
			path     string
			expected bool
		}
	}{
		{
			name:  "Standard 404 Server",
			setup: createStandard404Server,
			paths: []struct {
				path     string
				expected bool
			}{
				{"/", false},
				{"/nonexistent", true},
				{"/another-missing", true},
			},
		},
		{
			name:  "Same Response Server",
			setup: createSameResponseServer,
			paths: []struct {
				path     string
				expected bool
			}{
				{"/", true}, // All paths return same response, so all are considered "not found"
				{"/nonexistent", true},
				{"/another-path", true},
			},
		},
		{
			name:  "Custom Error Pages",
			setup: createCustomErrorServer,
			paths: []struct {
				path     string
				expected bool
			}{
				// {"/", false}, // TODO: Review this test case
				{"/nonexistent", true},
				{"/another-missing", true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setup()
			defer server.server.Close()

			behavior, err := CheckSiteBehavior(SiteBehaviourCheckOptions{
				BaseURL: server.server.URL,
				HistoryCreationOptions: HistoryCreationOptions{
					Source:      db.SourceScanner,
					WorkspaceID: workspaceID,
				},
			})
			assert.NoError(t, err)

			client := CreateHttpClient()
			for _, tc := range tt.paths {
				req, err := http.NewRequest(http.MethodGet, server.server.URL+tc.path, nil)
				assert.NoError(t, err)

				executionResult := ExecuteRequest(req, RequestExecutionOptions{
					Client:        client,
					CreateHistory: true,
					HistoryCreationOptions: HistoryCreationOptions{
						Source:      db.SourceScanner,
						WorkspaceID: workspaceID,
					},
				})
				assert.NoError(t, executionResult.Err)

				history := executionResult.History
				assert.NoError(t, err)

				result := behavior.IsNotFound(history)
				assert.Equal(t, tc.expected, result, "Path: %s should return isNotFound=%v", tc.path, tc.expected)
			}
		})
	}
}
