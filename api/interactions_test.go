package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestFindInteractions(t *testing.T) {
	app := fiber.New()

	app.Get("/interactions", FindInteractions)

	req := httptest.NewRequest("GET", "/interactions?page=1&page_size=10&protocols=HTTP,FTP", nil)
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
