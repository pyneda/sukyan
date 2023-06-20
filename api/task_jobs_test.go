package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestFindTaskJobs(t *testing.T) {
	app := fiber.New()

	app.Get("/taskjobs", FindTaskJobs)

	req := httptest.NewRequest("GET", "/taskjobs?page=1&page_size=10&status=Completed&title=JobTitle", nil)
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
