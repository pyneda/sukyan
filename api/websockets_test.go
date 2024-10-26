package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
)

func TestFindWebSocketConnections(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/wsconnections", FindWebSocketConnections)

	workspace, err := db.Connection.GetOrCreateWorkspace(&db.Workspace{
		Title: "TestFindWebSocketConnections",
		Code:  "TestFindWebSocketConnections",
	})
	assert.Nil(t, err)

	// Test with valid parameters
	req := httptest.NewRequest(
		"GET",
		fmt.Sprintf("/api/v1/wsconnections?page_size=2&page=1&workspace=%d", workspace.ID),
		nil,
	)
	resp, _ := app.Test(req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test with invalid page parameter
	req = httptest.NewRequest(
		"GET",
		fmt.Sprintf("/api/v1/wsconnections?page_size=2&page=abc&workspace=%d", workspace.ID),
		nil,
	)
	resp, _ = app.Test(req)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	// Test with invalid page_size parameter
	req = httptest.NewRequest(
		"GET",
		fmt.Sprintf("/api/v1/wsconnections?page_size=xyz&page=1&workspace=%d", workspace.ID),
		nil,
	)
	resp, _ = app.Test(req)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestFindWebSocketMessages(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/wsmessages", FindWebSocketMessages)

	// Test with valid parameters
	req := httptest.NewRequest("GET", "/api/v1/wsmessages?page_size=2&page=1", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test with invalid page parameter
	req = httptest.NewRequest("GET", "/api/v1/wsmessages?page_size=2&page=abc", nil)
	resp, _ = app.Test(req)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	// Test with invalid page_size parameter
	req = httptest.NewRequest("GET", "/api/v1/wsmessages?page_size=xyz&page=1", nil)
	resp, _ = app.Test(req)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}
