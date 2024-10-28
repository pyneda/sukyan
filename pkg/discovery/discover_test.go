package discovery

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testPaths = []string{
	"/",
	"/admin",
	"/api/v1",
	"/api/v2",
	"/dashboard",
	"/secret",
	"/login",
	"/config",
	"/backup",
	"/hidden",
	"/api/docs",
	"/swagger",
}

var largeTestPaths = generateLargePaths()

func generateLargePaths() []string {
	paths := make([]string, 100)
	for i := 0; i < 100; i++ {
		paths[i] = fmt.Sprintf("/path-%d", i)
	}
	return paths
}

type serverConfig struct {
	validPaths       []string
	slowPaths        []string
	slowResponseTime time.Duration
	forbiddenPaths   []string
	redirectPaths    map[string]string
	customResponses  map[string]serverResponse
}

type serverResponse struct {
	status  int
	body    string
	headers map[string]string
	delay   time.Duration
}

func setupTestServer(config serverConfig) *httptest.Server {
	mux := http.NewServeMux()

	pathExists := func(path string, paths []string) bool {
		for _, p := range paths {
			if p == path {
				return true
			}
		}
		return false
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if response, exists := config.customResponses[path]; exists {
			if response.delay > 0 {
				time.Sleep(response.delay)
			}
			for k, v := range response.headers {
				w.Header().Set(k, v)
			}
			w.WriteHeader(response.status)
			fmt.Fprint(w, response.body)
			return
		}

		if newPath, shouldRedirect := config.redirectPaths[path]; shouldRedirect {
			http.Redirect(w, r, newPath, http.StatusFound)
			return
		}

		if pathExists(path, config.slowPaths) {
			time.Sleep(config.slowResponseTime)
		}

		switch {
		case pathExists(path, config.validPaths):
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"status": "success",
				"path":   path,
			})
		case pathExists(path, config.forbiddenPaths):
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, "Forbidden")
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, "Not Found")
		}
	})

	return httptest.NewServer(mux)
}

func setupTestWorkspace(t *testing.T) *db.Workspace {
	workspace, err := db.Connection.GetOrCreateWorkspace(&db.Workspace{
		Title: "TestDiscovery",
		Code:  "test-discovery",
	})
	require.NoError(t, err)
	require.NotNil(t, workspace)
	return workspace
}

func TestDiscoverPaths(t *testing.T) {
	workspace := setupTestWorkspace(t)

	config := serverConfig{
		validPaths:       []string{"/api/v1", "/dashboard", "/hidden"},
		slowPaths:        []string{"/backup", "/config"},
		slowResponseTime: 200 * time.Millisecond,
		forbiddenPaths:   []string{"/admin", "/secret"},
		redirectPaths:    map[string]string{"/login": "/auth/login"},
		customResponses: map[string]serverResponse{
			"/swagger": {
				status:  200,
				body:    "OpenAPI documentation",
				headers: map[string]string{"Content-Type": "text/plain"},
			},
		},
	}

	server := setupTestServer(config)
	defer server.Close()

	tests := []struct {
		name          string
		input         DiscoveryInput
		expectedCodes map[string]int
		shouldStop    bool
		maxResponses  int
		expectError   bool
	}{
		{
			name: "Basic discovery without stopping",
			input: DiscoveryInput{
				URL:         server.URL,
				Method:      "GET",
				Paths:       testPaths,
				Concurrency: 2,
				Timeout:     5,
				HistoryCreationOptions: http_utils.HistoryCreationOptions{
					Source:      "Scanner",
					WorkspaceID: workspace.ID,
				},
			},
			expectedCodes: map[string]int{
				"/api/v1":    200,
				"/dashboard": 200,
				"/hidden":    200,
				"/admin":     403,
				"/secret":    403,
				"/":          404,
			},
			shouldStop:   false,
			maxResponses: len(testPaths),
		},
		{
			name: "Stop after first valid path",
			input: DiscoveryInput{
				URL:            server.URL,
				Method:         "GET",
				Paths:          testPaths,
				Concurrency:    2,
				Timeout:        5,
				StopAfterValid: true,
				ValidationFunc: DefaultValidationFunc,
				HistoryCreationOptions: http_utils.HistoryCreationOptions{
					Source:      "Scanner",
					WorkspaceID: workspace.ID,
				},
			},
			expectedCodes: map[string]int{
				"/api/v1": 200,
			},
			shouldStop:   true,
			maxResponses: 5,
		},
		{
			name: "Custom validation function",
			input: DiscoveryInput{
				URL:            server.URL,
				Method:         "GET",
				Paths:          testPaths,
				Concurrency:    2,
				Timeout:        5,
				StopAfterValid: true,
				ValidationFunc: func(h *db.History) (bool, string, int) {
					return h.StatusCode == 403, "Found forbidden endpoint", 90
				},
				HistoryCreationOptions: http_utils.HistoryCreationOptions{
					Source:      "Scanner",
					WorkspaceID: workspace.ID,
				},
			},
			expectedCodes: map[string]int{
				"/admin": 403,
			},
			shouldStop:   true,
			maxResponses: 5,
		},
		{
			name: "High concurrency test",
			input: DiscoveryInput{
				URL:         server.URL,
				Method:      "GET",
				Paths:       largeTestPaths,
				Concurrency: 20,
				Timeout:     5,
				HistoryCreationOptions: http_utils.HistoryCreationOptions{
					Source:      "Scanner",
					WorkspaceID: workspace.ID,
				},
			},
			expectedCodes: map[string]int{},
			shouldStop:    false,
			maxResponses:  len(largeTestPaths),
		},
		{
			name: "Timeout handling",
			input: DiscoveryInput{
				URL:         server.URL,
				Method:      "GET",
				Paths:       []string{"/backup", "/config"},
				Concurrency: 1,
				Timeout:     1,
				HistoryCreationOptions: http_utils.HistoryCreationOptions{
					Source:      "Scanner",
					WorkspaceID: workspace.ID,
				},
			},
			expectedCodes: map[string]int{},
			shouldStop:    false,
			maxResponses:  2,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := DiscoverPaths(tt.input)

			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, tt.shouldStop, results.Stopped, "Unexpected stopped state")

			assert.LessOrEqual(t, len(results.Responses), tt.maxResponses,
				"Got more responses than expected (got %d, max %d)",
				len(results.Responses), tt.maxResponses)

			if len(tt.expectedCodes) > 0 {
				foundCodes := make(map[string]int)
				for _, resp := range results.Responses {
					path := strings.TrimPrefix(resp.URL, server.URL)
					foundCodes[path] = resp.StatusCode
				}

				for path, expectedCode := range tt.expectedCodes {
					if tt.shouldStop {
						found := false
						for _, resp := range results.Responses {
							respPath := strings.TrimPrefix(resp.URL, server.URL)
							if respPath == path && resp.StatusCode == expectedCode {
								found = true
								break
							}
						}
						if found {
							break
						}
					} else {
						code, exists := foundCodes[path]
						if expectedCode != 0 {
							assert.True(t, exists, "Expected path %s not found", path)
							assert.Equal(t, expectedCode, code,
								"Unexpected status code for path %s", path)
						}
					}
				}
			}
		})
	}
}

func TestInputValidation(t *testing.T) {
	workspace := setupTestWorkspace(t)

	tests := []struct {
		name        string
		input       DiscoveryInput
		expectError bool
	}{
		{
			name: "Empty URL",
			input: DiscoveryInput{
				HistoryCreationOptions: http_utils.HistoryCreationOptions{
					Source:      "Scanner",
					WorkspaceID: workspace.ID,
				},
			},
			expectError: true,
		},
		{
			name: "Valid minimal input",
			input: DiscoveryInput{
				URL: "http://example.com",
				HistoryCreationOptions: http_utils.HistoryCreationOptions{
					Source:      "Scanner",
					WorkspaceID: workspace.ID,
				},
			},
			expectError: false,
		},
		{
			name: "Default values test",
			input: DiscoveryInput{
				URL: "http://example.com",
				HistoryCreationOptions: http_utils.HistoryCreationOptions{
					Source:      "Scanner",
					WorkspaceID: workspace.ID,
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			if tt.input.Concurrency == 0 {
				assert.Equal(t, DefaultConcurrency, tt.input.Concurrency)
			}
			if tt.input.Timeout == 0 {
				assert.Equal(t, DefaultTimeout, tt.input.Timeout)
			}
			if tt.input.Method == "" {
				assert.Equal(t, DefaultMethod, tt.input.Method)
			}
			if tt.input.ValidationFunc == nil {
				assert.NotNil(t, tt.input.ValidationFunc)
			}
		})
	}
}

func TestJoinURLPath(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		urlPath  string
		expected string
	}{
		{
			name:     "Simple join",
			baseURL:  "http://example.com",
			urlPath:  "/test",
			expected: "http://example.com/test",
		},
		{
			name:     "Base URL with trailing slash",
			baseURL:  "http://example.com/",
			urlPath:  "/test",
			expected: "http://example.com/test",
		},
		{
			name:     "URL path without leading slash",
			baseURL:  "http://example.com",
			urlPath:  "test",
			expected: "http://example.com/test",
		},
		{
			name:     "Complex path",
			baseURL:  "http://example.com/api",
			urlPath:  "/v1/users",
			expected: "http://example.com/api/v1/users",
		},
		{
			name:     "Invalid base URL",
			baseURL:  "not-a-url",
			urlPath:  "/test",
			expected: "not-a-url/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := joinURLPath(tt.baseURL, tt.urlPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}
