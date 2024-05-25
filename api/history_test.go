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

func TestFindHistory(t *testing.T) {
	workspace, err := db.Connection.CreateDefaultWorkspace()
	assert.Nil(t, err)
	app := fiber.New()

	app.Get("/history", FindHistory)
	url := fmt.Sprintf("/history?page=1&page_size=10&status=200,404&workspace=%d&methods=GET,POST&sources=scan", workspace.ID)
	req := httptest.NewRequest("GET", url, nil)
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
