package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/discovery"
	"github.com/stretchr/testify/require"
)

const detailSpec = `{
  "openapi": "3.0.0",
  "info": {"title": "t", "version": "1"},
  "servers": [{"url": "https://api.example.com"}],
  "paths": {
    "/pets": {
      "get": {
        "operationId": "listPets",
        "parameters": [{"name": "limit", "in": "query", "required": true, "schema": {"type": "integer"}}],
        "responses": {"200": {"description": "ok"}}
      }
    }
  }
}`

func detailFixture(t *testing.T) (*db.APIDefinition, *db.APIEndpoint) {
	t.Helper()
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Code:        fmt.Sprintf("test-detail-%d", time.Now().UnixNano()),
		Title:       "Test operation detail",
		Description: "Temporary workspace",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Connection().DB().Where("workspace_id = ?", workspace.ID).Delete(&db.APIDefinition{})
		db.Connection().DeleteWorkspace(workspace.ID)
	})

	definition, err := discovery.PersistAPIDefinitionFromContent(
		[]byte(detailSpec),
		db.APIDefinitionTypeOpenAPI,
		discovery.APIPersistenceFromContentOptions{
			WorkspaceID: workspace.ID,
			SourceURL:   fmt.Sprintf("https://api.example.com/openapi-%d.json", time.Now().UnixNano()),
		},
	)
	require.NoError(t, err)

	endpoints, err := db.Connection().GetAPIEndpointsByDefinitionID(definition.ID)
	require.NoError(t, err)
	require.Len(t, endpoints, 1)
	return definition, endpoints[0]
}

func getDetail(t *testing.T, definitionID, endpointID, query string) (int, endpointDetailResponse) {
	t.Helper()
	app := fiber.New()
	app.Get("/api-definitions/:id/endpoints/:endpoint_id/detail", GetAPIEndpointDetail)

	url := fmt.Sprintf("/api-definitions/%s/endpoints/%s/detail?%s", definitionID, endpointID, query)
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, url, nil), fiber.TestConfig{Timeout: 10 * time.Second, FailOnTimeout: true})
	require.NoError(t, err)

	var body endpointDetailResponse
	if resp.StatusCode == http.StatusOK {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	}
	return resp.StatusCode, body
}

func TestGetAPIEndpointDetailReturnsOperationAndExample(t *testing.T) {
	definition, endpoint := detailFixture(t)

	status, body := getDetail(t, definition.ID.String(), endpoint.ID.String(), "")
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "/pets", body.Operation.Path)
	require.Len(t, body.Operation.Parameters, 1)
	require.Equal(t, "limit", body.Operation.Parameters[0].Name)
	require.NotNil(t, body.ExampleRequest)
	require.Contains(t, body.ExampleRequest.Raw, "GET /pets")
	require.Contains(t, body.ExampleRequest.URL, "api.example.com")
}

func TestGetAPIEndpointDetailUsesCurrentBaseURL(t *testing.T) {
	definition, endpoint := detailFixture(t)

	require.NoError(t, db.Connection().UpdateAPIDefinitionFields(definition.ID, map[string]interface{}{
		"base_url": "https://staging.example.com",
	}))

	_, body := getDetail(t, definition.ID.String(), endpoint.ID.String(), "")
	require.NotNil(t, body.ExampleRequest)
	require.Contains(t, body.ExampleRequest.URL, "staging.example.com")
	require.Contains(t, body.ExampleRequest.Raw, "Host: staging.example.com")
}

func TestGetAPIEndpointDetailBackfillsWhenOperationJSONMissing(t *testing.T) {
	definition, endpoint := detailFixture(t)

	require.NoError(t, db.Connection().DB().Model(&db.APIEndpoint{}).
		Where("definition_id = ?", definition.ID).
		Update("operation_json", nil).Error)

	status, body := getDetail(t, definition.ID.String(), endpoint.ID.String(), "")
	require.Equal(t, http.StatusOK, status)
	require.True(t, body.Backfilled)
	require.Equal(t, "/pets", body.Operation.Path)

	reloaded, err := db.Connection().GetAPIEndpointByID(endpoint.ID)
	require.NoError(t, err)
	require.NotEmpty(t, reloaded.OperationJSON)

	_, second := getDetail(t, definition.ID.String(), endpoint.ID.String(), "")
	require.False(t, second.Backfilled)
}

func TestGetAPIEndpointDetailRejectsUnknownEndpoint(t *testing.T) {
	definition, _ := detailFixture(t)
	status, _ := getDetail(t, definition.ID.String(), "00000000-0000-0000-0000-000000000000", "")
	require.Equal(t, http.StatusNotFound, status)
}

func TestGetAPIEndpointDetailRejectsEndpointFromAnotherDefinition(t *testing.T) {
	_, endpoint := detailFixture(t)
	otherDefinition, _ := detailFixture(t)

	status, _ := getDetail(t, otherDefinition.ID.String(), endpoint.ID.String(), "")
	require.Equal(t, http.StatusNotFound, status)
}
