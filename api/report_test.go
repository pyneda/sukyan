package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/report"
	"github.com/stretchr/testify/assert"

	"github.com/gofiber/fiber/v2"
)

func TestReportHandler(t *testing.T) {
	// Initialize Fiber app
	workspace, _ := db.Connection().CreateDefaultWorkspace()

	// Create a task
	task, err := db.Connection().NewTask(workspace.ID, nil, "Test Task", "completed", db.TaskTypeScan)
	assert.Nil(t, err)

	app := fiber.New()
	app.Post("/report", ReportHandler)

	// Create some issues
	issues := []db.Issue{
		{
			Code:        "ISSUE1",
			Title:       "Test Issue 1",
			Description: "Description 1",
			WorkspaceID: &workspace.ID,
			TaskID:      &task.ID,
		},
		{
			Code:        "ISSUE2",
			Title:       "Test Issue 2",
			Description: "Description 2",
			WorkspaceID: &workspace.ID,
			TaskID:      &task.ID,
		},
	}

	for _, issue := range issues {
		_, err := db.Connection().CreateIssue(issue)
		assert.Nil(t, err)
	}

	// Valid Request Payload for HTML
	validHTMLPayload := ReportRequest{
		WorkspaceID: workspace.ID,
		TaskID:      task.ID,
		Title:       "Test HTML Report",
		Format:      report.ReportFormatHTML,
	}
	jsonHTMLPayload, _ := json.Marshal(validHTMLPayload)

	// Test with valid HTML request
	req := httptest.NewRequest("POST", "/report", bytes.NewReader(jsonHTMLPayload))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	// Check status code
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Check response headers for HTML
	assert.Equal(t, "text/html", resp.Header.Get("Content-Type"))
	assert.Equal(t, "attachment; filename=report.html", resp.Header.Get("Content-Disposition"))

	// Check that one of the issues is included in the HTML report
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	assert.Contains(t, buf.String(), "Test Issue 1")
	assert.Contains(t, buf.String(), "Description 1")

	// Valid Request Payload for JSON
	validJSONPayload := ReportRequest{
		WorkspaceID: workspace.ID,
		TaskID:      task.ID,
		Title:       "Test JSON Report",
		Format:      report.ReportFormatJSON,
	}
	jsonJSONPayload, _ := json.Marshal(validJSONPayload)

	// Test with valid JSON request
	req = httptest.NewRequest("POST", "/report", bytes.NewReader(jsonJSONPayload))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)

	// Check status code
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Check response headers for JSON
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	assert.Equal(t, "attachment; filename=report.json", resp.Header.Get("Content-Disposition"))

	// Check that one of the issues is included in the JSON report
	buf.Reset()
	buf.ReadFrom(resp.Body)
	assert.Contains(t, buf.String(), "Test Issue 1")
	assert.Contains(t, buf.String(), "Description 1")
}
