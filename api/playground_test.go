package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
)

func TestCreatePlaygroundCollection(t *testing.T) {
	workspace, err := db.Connection.GetOrCreateWorkspace(&db.Workspace{
		Code:        "test-playground",
		Title:       "test-playground",
		Description: "test-playground",
	})

	assert.NoError(t, err)
	app := fiber.New()
	app.Post("/api/v1/playground/collections", CreatePlaygroundCollection)

	t.Run("ValidInput", func(t *testing.T) {
		input := CreatePlaygroundCollectionInput{
			Name:        "Test Collection",
			Description: "Test description",
			WorkspaceID: workspace.ID,
		}

		inputJSON, _ := json.Marshal(input)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/playground/collections", strings.NewReader(string(inputJSON)))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		assert.NoError(t, err)

		assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

		var response db.PlaygroundCollection
		err = json.NewDecoder(resp.Body).Decode(&response)
		assert.NoError(t, err)

		assert.NotZero(t, response.ID)
		assert.Equal(t, input.Name, response.Name)
		assert.Equal(t, input.Description, response.Description)
		assert.Equal(t, input.WorkspaceID, response.WorkspaceID)
	})

	t.Run("InvalidInput", func(t *testing.T) {
		// Create an invalid input payload (missing Name)
		input := CreatePlaygroundCollectionInput{
			Description: "Test description",
			WorkspaceID: 1,
		}

		inputJSON, _ := json.Marshal(input)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/playground/collections", strings.NewReader(string(inputJSON)))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		assert.NoError(t, err)

		assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
	})
}

func TestListPlaygroundCollections(t *testing.T) {
	workspace, err := db.Connection.GetOrCreateWorkspace(&db.Workspace{
		Code:        "test-playground",
		Title:       "test-playground",
		Description: "test-playground",
	})
	assert.NoError(t, err)
	defer db.Connection.DeleteWorkspace(workspace.ID)

	app := fiber.New()
	app.Get("/api/v1/playground/collections", ListPlaygroundCollections)

	t.Run("ValidInput", func(t *testing.T) {
		// Send a GET request to list Playground Collections with the created workspace's ID
		url := fmt.Sprintf("/api/v1/playground/collections?workspace=%d", workspace.ID)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		q := req.URL.Query()
		q.Add("workspace", fmt.Sprintf("%d", workspace.ID))
		req.URL.RawQuery = q.Encode()

		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)

		var response struct {
			Data  []db.PlaygroundCollection `json:"data"`
			Count int                       `json:"count"`
		}
		err = json.NewDecoder(resp.Body).Decode(&response)
		assert.NoError(t, err)
		assert.Equal(t, len(response.Data), response.Count)
	})
}

func TestListPlaygroundSessions(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/playground/sessions", ListPlaygroundSessions)

	t.Run("ValidInput", func(t *testing.T) {
		query := "?workspace=1"

		req := httptest.NewRequest(http.MethodGet, "/api/v1/playground/sessions"+query, nil)

		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)

		var response struct {
			Data  []db.PlaygroundSession `json:"data"`
			Count int                    `json:"count"`
		}
		err = json.NewDecoder(resp.Body).Decode(&response)
		assert.NoError(t, err)
	})

	t.Run("InvalidInput", func(t *testing.T) {
		query := ""

		req := httptest.NewRequest(http.MethodGet, "/api/v1/playground/sessions"+query, nil)

		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
	})
}

func TestCreatePlaygroundCollectionAndSession(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/playground/collections", CreatePlaygroundCollection)
	app.Post("/api/v1/playground/sessions", CreatePlaygroundSession)
	workspace, err := db.Connection.GetOrCreateWorkspace(&db.Workspace{
		Code:        "test-playground",
		Title:       "test-playground",
		Description: "test-playground",
	})

	assert.NoError(t, err)
	t.Run("ValidInput", func(t *testing.T) {
		collectionInput := CreatePlaygroundCollectionInput{
			Name:        "Test Collection",
			Description: "Test Description",
			WorkspaceID: 1,
		}

		collectionReq := httptest.NewRequest(http.MethodPost, "/api/v1/playground/collections", toJSON(collectionInput))
		collectionReq.Header.Set("Content-Type", "application/json")

		collectionResp, err := app.Test(collectionReq)
		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusCreated, collectionResp.StatusCode)

		var collectionResponse db.PlaygroundCollection
		err = json.NewDecoder(collectionResp.Body).Decode(&collectionResponse)
		assert.NoError(t, err)

		// Create a Playground Session using the collection ID
		sessionInput := CreatePlaygroundSessionInput{
			Name:              "Test Session",
			Type:              "manual",
			OriginalRequestID: workspace.ID,
			CollectionID:      collectionResponse.ID,
		}

		sessionReq := httptest.NewRequest(http.MethodPost, "/api/v1/playground/sessions", toJSON(sessionInput))
		sessionReq.Header.Set("Content-Type", "application/json")

		sessionResp, err := app.Test(sessionReq)
		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusCreated, sessionResp.StatusCode)

		var sessionResponse db.PlaygroundSession
		err = json.NewDecoder(sessionResp.Body).Decode(&sessionResponse)
		assert.NoError(t, err)
	})
}

func toJSON(v interface{}) *bytes.Buffer {
	buf := new(bytes.Buffer)
	err := json.NewEncoder(buf).Encode(v)
	if err != nil {
		panic(err)
	}
	return buf
}
