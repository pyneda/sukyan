package http_utils

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExecuteRequest(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/timeout") {
			time.Sleep(2 * time.Second)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "success"}`))
	}))
	defer server.Close()

	t.Run("successful request", func(t *testing.T) {
		req, err := http.NewRequest("GET", server.URL+"/test", nil)
		assert.NoError(t, err)

		result := ExecuteRequestSimple(req, HistoryCreationOptions{
			Source:      "test",
			WorkspaceID: 1,
			TaskID:      1,
		})

		assert.NoError(t, result.Err)
		assert.False(t, result.TimedOut)
		assert.NotNil(t, result.Response)
		assert.NotNil(t, result.History)
		assert.Equal(t, http.StatusOK, result.Response.StatusCode)
		assert.Equal(t, "test", result.History.Source)
		assert.Contains(t, string(result.ResponseData.Body), "success")
	})

	t.Run("request with timeout", func(t *testing.T) {
		req, err := http.NewRequest("GET", server.URL+"/timeout", nil)
		assert.NoError(t, err)

		result := ExecuteRequestWithTimeout(req, 500*time.Millisecond, HistoryCreationOptions{
			Source:      "test",
			WorkspaceID: 1,
			TaskID:      1,
		})

		assert.Error(t, result.Err)
		assert.True(t, result.TimedOut)
		assert.Nil(t, result.Response)
		assert.NotNil(t, result.History) // Should have timeout history
		assert.Equal(t, "test", result.History.Source)
		assert.Contains(t, result.History.Note, "timed out")
	})

	t.Run("request with custom options", func(t *testing.T) {
		req, err := http.NewRequest("POST", server.URL+"/test", bytes.NewReader([]byte("test body")))
		assert.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}

		result := ExecuteRequest(req, RequestExecutionOptions{
			Client:        client,
			Timeout:       10 * time.Second,
			CreateHistory: true,
			HistoryCreationOptions: HistoryCreationOptions{
				Source:              "custom_test",
				WorkspaceID:         2,
				TaskID:              3,
				CreateNewBodyStream: true,
			},
		})

		assert.NoError(t, result.Err)
		assert.False(t, result.TimedOut)
		assert.NotNil(t, result.Response)
		assert.NotNil(t, result.History)
		assert.Equal(t, "POST", result.History.Method)
		assert.Equal(t, "custom_test", result.History.Source)
		assert.Equal(t, uint(2), *result.History.WorkspaceID)
		assert.Equal(t, uint(3), *result.History.TaskID)
	})

	t.Run("request without creating history", func(t *testing.T) {
		req, err := http.NewRequest("GET", server.URL+"/test", nil)
		assert.NoError(t, err)

		result := ExecuteRequest(req, RequestExecutionOptions{
			CreateHistory: false,
		})

		assert.NoError(t, result.Err)
		assert.False(t, result.TimedOut)
		assert.NotNil(t, result.Response)
		assert.Nil(t, result.History) // Should not create history
	})
}

func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: true,
		},
		{
			name:     "timeout error",
			err:      &timeoutError{},
			expected: true,
		},
		{
			name:     "regular error",
			err:      assert.AnError,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTimeoutError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateTimeoutForPayload(t *testing.T) {
	tests := []struct {
		name                  string
		expectedSleepDuration time.Duration
		expectedTimeout       time.Duration
	}{
		{
			name:                  "no sleep duration",
			expectedSleepDuration: 0,
			expectedTimeout:       2 * time.Minute,
		},
		{
			name:                  "short sleep duration",
			expectedSleepDuration: 5 * time.Second,
			expectedTimeout:       30 * time.Second, // minimum threshold
		},
		{
			name:                  "medium sleep duration",
			expectedSleepDuration: 45 * time.Second,
			expectedTimeout:       90 * time.Second, // 2x the sleep duration
		},
		{
			name:                  "long sleep duration",
			expectedSleepDuration: 10 * time.Minute,
			expectedTimeout:       5 * time.Minute, // maximum threshold
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateTimeoutForPayload(tt.expectedSleepDuration)
			assert.Equal(t, tt.expectedTimeout, result)
		})
	}
}

// timeoutError is a helper for testing timeout error detection
type timeoutError struct{}

func (e *timeoutError) Error() string {
	return "request timeout"
}

func (e *timeoutError) Timeout() bool {
	return true
}
