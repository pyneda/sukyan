package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestFindWorkspaces(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/workspaces", FindWorkspaces)

	req := httptest.NewRequest("GET", "/api/v1/workspaces", nil)
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
