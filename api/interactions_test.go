package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
)

func TestFindInteractions(t *testing.T) {
	app := fiber.New()

	app.Get("/interactions", FindInteractions)
	workspace, err := db.Connection.GetOrCreateWorkspace(&db.Workspace{
		Title: "Interactions Workspace",
		Code:  "interactions-workspace",
	})
	assert.Nil(t, err)

	req := httptest.NewRequest(
		"GET",
		fmt.Sprintf("/interactions?page=1&page_size=10&protocols=HTTP,FTP&workspace=%d", workspace.ID),
		nil,
	)
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetInteractionDetail(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/interactions/:id", GetInteractionDetail)

	workspace := db.Workspace{
		Title: "Interactions Workspace",
		Code:  "interactions-workspace",
	}
	_, err := db.Connection.CreateWorkspace(&workspace)
	assert.Nil(t, err)

	// Create OOBTest
	test := db.OOBTest{
		Code:              "test-code",
		TestName:          "TestName",
		Target:            "http://example.com",
		InteractionDomain: "example.com",
		InteractionFullID: "12345",
		Payload:           "test-payload",
		InsertionPoint:    "query",
		WorkspaceID:       &workspace.ID,
	}
	test, err = db.Connection.CreateOOBTest(test)
	assert.Nil(t, err)

	// Create OOBInteraction
	interaction := db.OOBInteraction{
		OOBTestID:     &test.ID,
		Protocol:      "HTTP",
		FullID:        "12345",
		UniqueID:      "unique-123",
		QType:         "A",
		RawRequest:    "GET / HTTP/1.1",
		RawResponse:   "HTTP/1.1 200 OK",
		RemoteAddress: "127.0.0.1",
		Timestamp:     time.Now(),
		WorkspaceID:   &workspace.ID,
	}
	createdInteraction, err := db.Connection.CreateInteraction(&interaction)
	assert.Nil(t, err)

	req := httptest.NewRequest("GET", "/api/v1/interactions/"+strconv.Itoa(int(createdInteraction.ID)), nil)
	resp, _ := app.Test(req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

}
