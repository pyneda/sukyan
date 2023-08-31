package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestFindTasks(t *testing.T) {
	app := fiber.New()

	app.Get("/tasks", FindTasks)

	req := httptest.NewRequest("GET", "/tasks?page=1&page_size=10&status=Completed&workspace=1", nil)
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
