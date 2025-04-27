package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pyneda/sukyan/db"

	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestGetIssueDetail(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/issues/:id", GetIssueDetail)

	issue1Template := db.GetIssueTemplateByCode(db.NosqlInjectionCode)
	issue2Template := db.GetIssueTemplateByCode(db.OsCmdInjectionCode)

	// Create the issues in the database
	createdIssue1, err := db.Connection().CreateIssue(*issue1Template)
	if err != nil {
		t.Fatalf("Error creating mock issue: %s", err)
	}

	createdIssue2, err := db.Connection().CreateIssue(*issue2Template)
	if err != nil {
		t.Fatalf("Error creating mock issue: %s", err)
	}

	// Test without details for issue1
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/issues/%d", createdIssue1.ID), nil)
	resp, _ := app.Test(req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test without details for issue2
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/issues/%d", createdIssue2.ID), nil)
	resp, _ = app.Test(req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test with invalid ID
	req = httptest.NewRequest("GET", "/api/v1/issues/invalidID", nil)
	resp, _ = app.Test(req)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Test with non-existing ID
	req = httptest.NewRequest("GET", "/api/v1/issues/999999", nil)
	resp, _ = app.Test(req)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

}
