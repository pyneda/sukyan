package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
)

func setupTestWorkspace(t *testing.T) *db.Workspace {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Code:        "test-report",
		Title:       "Test Report",
		Description: "Test Report Workspace",
	})
	assert.NoError(t, err)
	assert.NotNil(t, workspace)
	return workspace
}

func createTestIssue(workspaceID uint) *db.Issue {
	taskID := uint(1)
	return &db.Issue{
		Code:          "test-code",
		Title:         "Test Issue",
		Description:   "Test Description",
		Details:       "Test Details",
		Remediation:   "Test Remediation",
		Cwe:           123,
		URL:           "https://example.com",
		StatusCode:    200,
		HTTPMethod:    "GET",
		Payload:       "test-payload",
		Request:       []byte("GET / HTTP/1.1\nHost: example.com"),
		Response:      []byte("HTTP/1.1 200 OK\nContent-Type: text/plain\n\nTest response"),
		FalsePositive: false,
		Confidence:    90,
		References:    db.StringSlice{"https://example.com/ref1"},
		Severity:      "High",
		CURLCommand:   "curl https://example.com",
		WorkspaceID:   &workspaceID,
		TaskID:        &taskID,
	}
}

func TestGenerateReport(t *testing.T) {
	workspace := setupTestWorkspace(t)
	issue := createTestIssue(workspace.ID)
	savedIssue, err := db.Connection().CreateIssue(*issue)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		options     ReportOptions
		wantErr     bool
		checkOutput func(t *testing.T, output []byte)
	}{
		{
			name: "HTML Report",
			options: ReportOptions{
				WorkspaceID: workspace.ID,
				Issues:      []*db.Issue{&savedIssue},
				Title:       "Test HTML Report",
				Format:      ReportFormatHTML,
			},
			wantErr: false,
			checkOutput: func(t *testing.T, output []byte) {
				content := string(output)
				assert.Contains(t, content, "Test HTML Report", "Report should contain title")
				assert.Contains(t, content, issue.Title, "Report should contain issue title")
				assert.Contains(t, content, issue.Description, "Report should contain issue description")
				assert.Contains(t, content, string(issue.Severity), "Report should contain severity")
				assert.Contains(t, content, issue.URL, "Report should contain URL")
			},
		},
		{
			name: "JSON Report",
			options: ReportOptions{
				WorkspaceID: workspace.ID,
				Issues:      []*db.Issue{&savedIssue},
				Title:       "Test JSON Report",
				Format:      ReportFormatJSON,
			},
			wantErr: false,
			checkOutput: func(t *testing.T, output []byte) {
				var report map[string]interface{}
				err := json.Unmarshal(output, &report)
				assert.NoError(t, err)

				assert.Equal(t, "Test JSON Report", report["title"])
				assert.Equal(t, float64(workspace.ID), report["workspaceID"])

				issues, ok := report["issues"].([]interface{})
				assert.True(t, ok)
				assert.Len(t, issues, 1)

				issueData := issues[0].(map[string]interface{})
				assert.Equal(t, issue.Title, issueData["title"])
				assert.Equal(t, issue.Code, issueData["code"])
				assert.Equal(t, string(issue.Severity), issueData["severity"])
			},
		},
		{
			name: "Invalid Format",
			options: ReportOptions{
				WorkspaceID: workspace.ID,
				Issues:      []*db.Issue{&savedIssue},
				Title:       "Invalid Format Report",
				Format:      "invalid",
			},
			wantErr: true,
			checkOutput: func(t *testing.T, output []byte) {
				assert.Empty(t, output)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := GenerateReport(tt.options, &buf)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			tt.checkOutput(t, buf.Bytes())
		})
	}

	err = db.Connection().DeleteWorkspace(workspace.ID)
	assert.NoError(t, err)
}

func TestHelperFunctions(t *testing.T) {
	t.Run("toString", func(t *testing.T) {
		tests := []struct {
			input    interface{}
			expected string
		}{
			{[]byte("test bytes"), "test bytes"},
			{"test string", "test string"},
			{123, "123"},
			{true, "true"},
			{nil, "<nil>"},
		}

		for _, tt := range tests {
			result := toString(tt.input)
			assert.Equal(t, tt.expected, result)
		}
	})

	t.Run("toJSON", func(t *testing.T) {
		tests := []struct {
			input    interface{}
			expected string
		}{
			{map[string]string{"key": "value"}, `{"key":"value"}`},
			{[]string{"a", "b"}, `["a","b"]`},
			{nil, `null`},
		}

		for _, tt := range tests {
			result := toJSON(tt.input)
			assert.Equal(t, tt.expected, strings.TrimSpace(string(result)))
		}
	})
}
