package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestGetIssueDetail(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/issues/:id", GetIssueDetail)

	req := httptest.NewRequest("GET", "/api/v1/issues/1", nil)
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	req = httptest.NewRequest("GET", "/api/v1/issues/invalidID", nil)
	resp, _ = app.Test(req)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	req = httptest.NewRequest("GET", "/api/v1/issues/9999", nil)
	resp, _ = app.Test(req)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
