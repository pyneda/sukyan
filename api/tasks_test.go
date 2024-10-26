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

func TestFindTasks(t *testing.T) {
	app := fiber.New()

	app.Get("/tasks", FindTasks)
	workspace, err := db.Connection.GetOrCreateWorkspace(&db.Workspace{
		Title: "TestFindTasks",
		Code:  "TestFindTasks",
	})
	assert.Nil(t, err)
	req := httptest.NewRequest(
		"GET",
		fmt.Sprintf("/tasks?page=1&page_size=10&status=Completed&workspace=%d", workspace.ID),
		nil,
	)
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
