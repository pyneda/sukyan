package report

import (
	"bytes"
	"encoding/json"
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

func TestHTMLReportIsSelfContained(t *testing.T) {
	workspace := setupTestWorkspace(t)
	defer func() {
		assert.NoError(t, db.Connection().DeleteWorkspace(workspace.ID))
	}()

	issue := createTestIssue(workspace.ID)
	saved, err := db.Connection().CreateIssue(*issue)
	assert.NoError(t, err)

	var buf bytes.Buffer
	err = GenerateReport(ReportOptions{
		WorkspaceID: workspace.ID,
		Issues:      []*db.Issue{&saved},
		Title:       "Self Contained",
		Format:      ReportFormatHTML,
	}, &buf)
	assert.NoError(t, err)

	content := buf.String()

	// A report is a deliverable. It must render in an air-gapped room and must
	// not phone home when a client opens it.
	for _, forbidden := range []string{
		"cdn.tailwindcss.com",
		"cdn.jsdelivr.net",
		"fonts.googleapis.com",
		"fonts.gstatic.com",
		"<script src=",
		"<link rel=\"stylesheet\"",
		"@import",
	} {
		assert.NotContains(t, content, forbidden, "report must not reference external resources")
	}
}

func TestHTMLReportEscapesIssueDataInPayload(t *testing.T) {
	workspace := setupTestWorkspace(t)
	defer func() {
		assert.NoError(t, db.Connection().DeleteWorkspace(workspace.ID))
	}()

	issue := createTestIssue(workspace.ID)
	issue.Title = `</script><img src=x onerror=alert(1)>`
	issue.URL = `https://example.com/?q=</script><svg onload=alert(2)>`
	saved, err := db.Connection().CreateIssue(*issue)
	assert.NoError(t, err)

	var buf bytes.Buffer
	err = GenerateReport(ReportOptions{
		WorkspaceID: workspace.ID,
		Issues:      []*db.Issue{&saved},
		Title:       "Escaping",
		Format:      ReportFormatHTML,
	}, &buf)
	assert.NoError(t, err)

	content := buf.String()

	// The payload is embedded in a JS context and must not be able to close the
	// script element. json.Marshal alone does not escape "<", which is why the
	// payload is handed to html/template rather than pre-marshalled.
	assert.NotContains(t, content, "</script><img")
	assert.NotContains(t, content, "</script><svg")

	// Matching the tail of the unicode escape rather than the whole sequence,
	// because a literal backslash-u in this assertion is easy to mangle. The
	// fixture data above contains no such substring of its own.
	assert.Contains(t, content, "u003c", "angle brackets in issue data must be escaped")
}

func TestReportScriptNeverUsesInnerHTML(t *testing.T) {
	script := mustAsset("report.js")

	// Every value the report renders originates from a scanned target. Building
	// nodes from text avoids the whole class of injection, so the asset is not
	// allowed to reintroduce innerHTML.
	assert.NotContains(t, script, "innerHTML")
	assert.NotContains(t, script, "outerHTML")
	assert.NotContains(t, script, "insertAdjacentHTML")
	assert.NotContains(t, script, "document.write")
}
