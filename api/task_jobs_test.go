package api

import (
	"fmt"
	"github.com/pyneda/sukyan/db"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestFindTaskJobs(t *testing.T) {
	app := fiber.New()

	app.Get("/taskjobs", FindTaskJobs)
	task, err := db.Connection.NewTask(1, nil, "Test task", "crawl")
	assert.Nil(t, err)
	path := fmt.Sprintf("/taskjobs?page=1&page_size=10&status=Completed&title=JobTitle&task=%d", task.ID)
	req := httptest.NewRequest("GET", path, nil)
	resp, _ := app.Test(req)

	db.Connection.DeleteTask(task.ID)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

}
