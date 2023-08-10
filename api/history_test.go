package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/pyneda/sukyan/db"
)

func TestFindHistory(t *testing.T) {
	db.Connection.CreateDefaultWorkspace()
	app := fiber.New()

	app.Get("/history", FindHistory)


	req := httptest.NewRequest("GET", "/history?page=1&page_size=10&status=200,404&workspace=1&methods=GET,POST&sources=scan", nil)
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
