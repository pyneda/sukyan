package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const importUsersSpec = `{
	"openapi": "3.0.3",
	"info": {"title": "Import Users API", "version": "1.0.0"},
	"servers": [{"url": "http://users.import.test"}],
	"paths": {
		"/users": {"get": {"operationId": "listUsers", "responses": {"200": {"description": "ok"}}}},
		"/users/{id}": {"get": {"operationId": "getUser", "responses": {"200": {"description": "ok"}}}}
	}
}`

const importOrdersSpec = `{
	"openapi": "3.0.3",
	"info": {"title": "Import Orders API", "version": "1.0.0"},
	"servers": [{"url": "http://orders.import.test"}],
	"paths": {
		"/orders": {"post": {"operationId": "createOrder", "responses": {"201": {"description": "ok"}}}}
	}
}`

const importGraphQLSDL = `
type Query {
  order(id: ID!): Order
}

type Mutation {
  placeOrder(total: Float!): Order!
}

type Order {
  id: ID!
  total: Float!
}
`

type createdDefinition struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	SourceURL string    `json:"source_url"`
	Created   bool      `json:"created"`
}

func importTestWorkspace(t *testing.T) *db.Workspace {
	t.Helper()

	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Title:       "TestAPIDefinitionImport",
		Code:        "test-apidef-import-" + uuid.NewString(),
		Description: "Workspace for API definition import tests",
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Connection().DB().Where("workspace_id = ?", workspace.ID).Delete(&db.APIDefinition{})
		db.Connection().DeleteWorkspace(workspace.ID)
	})

	return workspace
}

func postDefinition(t *testing.T, body map[string]interface{}) (int, []byte) {
	t.Helper()

	app := fiber.New()
	app.Post("/api/v1/api-definitions", CreateAPIDefinition)

	payload, err := json.Marshal(body)
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/api-definitions", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")

	response, err := app.Test(request, 30000)
	require.NoError(t, err)

	buffer := new(bytes.Buffer)
	_, err = buffer.ReadFrom(response.Body)
	require.NoError(t, err)

	return response.StatusCode, buffer.Bytes()
}

func importDefinition(t *testing.T, body map[string]interface{}) createdDefinition {
	t.Helper()

	status, raw := postDefinition(t, body)
	require.Equal(t, fiber.StatusCreated, status, "import failed: %s", string(raw))

	var created createdDefinition
	require.NoError(t, json.Unmarshal(raw, &created))
	require.NotEqual(t, uuid.Nil, created.ID)
	return created
}

func definitionOperationIDs(t *testing.T, definitionID uuid.UUID) []string {
	t.Helper()

	endpoints, err := db.Connection().GetAPIEndpointsByDefinitionID(definitionID)
	require.NoError(t, err)

	ids := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		ids = append(ids, endpoint.OperationID)
	}
	return ids
}

func assertDefinitionMatches(t *testing.T, definitionID uuid.UUID, name string, operations []string) {
	t.Helper()

	stored, err := db.Connection().GetAPIDefinitionByID(definitionID)
	require.NoError(t, err)
	assert.Equal(t, name, stored.Name)
	assert.ElementsMatch(t, operations, definitionOperationIDs(t, definitionID))
	assert.Equal(t, len(operations), stored.EndpointCount)
}

// Two pastes into one workspace are two definitions. They used to be one: pasted
// content was stored under the synthetic path "/", so the second import found the
// first by source URL, was answered with it, and renamed it — leaving a
// definition whose name came from one document and whose operations came from
// another.
func TestCreateAPIDefinitionKeepsPastedImportsApart(t *testing.T) {
	workspace := importTestWorkspace(t)

	first := importDefinition(t, map[string]interface{}{
		"workspace_id": workspace.ID,
		"content":      importUsersSpec,
		"type":         "openapi",
	})
	second := importDefinition(t, map[string]interface{}{
		"workspace_id": workspace.ID,
		"content":      importOrdersSpec,
		"type":         "openapi",
	})

	require.NotEqual(t, first.ID, second.ID)
	assert.True(t, first.Created)
	assert.True(t, second.Created)

	assertDefinitionMatches(t, first.ID, "Import Users API", []string{"listUsers", "getUser"})
	assertDefinitionMatches(t, second.ID, "Import Orders API", []string{"createOrder"})

	assert.Empty(t, first.SourceURL, "pasted content has no source URL and must not be given a stand-in")
	assert.Empty(t, second.SourceURL)
}

// Importing a URL already in the library adds a definition instead of rewriting
// the one that is there — the row already stored may carry endpoints the user
// enabled, issues found against them and a name they chose.
func TestCreateAPIDefinitionDoesNotOverwriteAKnownSourceURL(t *testing.T) {
	workspace := importTestWorkspace(t)

	spec := importUsersSpec
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(spec))
	}))
	defer server.Close()

	specURL := server.URL + "/openapi.json"

	first := importDefinition(t, map[string]interface{}{
		"workspace_id": workspace.ID,
		"url":          specURL,
	})
	assertDefinitionMatches(t, first.ID, "Import Users API", []string{"listUsers", "getUser"})

	// The same URL now serves a different API, and the caller names the import.
	spec = importOrdersSpec
	second := importDefinition(t, map[string]interface{}{
		"workspace_id": workspace.ID,
		"url":          specURL,
		"name":         "Renamed On Import",
	})

	require.NotEqual(t, first.ID, second.ID, "re-importing a known URL must not take over the stored definition")
	assertDefinitionMatches(t, first.ID, "Import Users API", []string{"listUsers", "getUser"})
	assertDefinitionMatches(t, second.ID, "Renamed On Import", []string{"createOrder"})
	assert.Equal(t, specURL, second.SourceURL)
}

// The UI offers to paste "a GraphQL schema", which is SDL. Only an introspection
// JSON response was accepted, so a pasted schema answered 500 with "failed to
// parse introspection response: invalid character 'y' in literal true".
func TestCreateAPIDefinitionAcceptsAPastedGraphQLSchema(t *testing.T) {
	workspace := importTestWorkspace(t)

	created := importDefinition(t, map[string]interface{}{
		"workspace_id": workspace.ID,
		"content":      importGraphQLSDL,
		"type":         "graphql",
		"name":         "Pasted Schema",
		"base_url":     "http://graphql.import.test/graphql",
	})

	assertDefinitionMatches(t, created.ID, "Pasted Schema", []string{"order", "placeOrder"})

	stored, err := db.Connection().GetAPIDefinitionByID(created.ID)
	require.NoError(t, err)
	assert.Equal(t, db.APIDefinitionTypeGraphQL, stored.Type)
	assert.Equal(t, 1, stored.GraphQLQueryCount)
	assert.Equal(t, 1, stored.GraphQLMutationCount)
}

// An introspection response still imports: SDL support is an addition, not a
// replacement.
func TestCreateAPIDefinitionStillAcceptsGraphQLIntrospection(t *testing.T) {
	workspace := importTestWorkspace(t)

	introspection := `{"data":{"__schema":{
		"queryType":{"name":"Query"},
		"types":[{"kind":"OBJECT","name":"Query","fields":[
			{"name":"order","type":{"kind":"SCALAR","name":"String"},"args":[]}
		]}]
	}}}`

	created := importDefinition(t, map[string]interface{}{
		"workspace_id": workspace.ID,
		"content":      introspection,
		"type":         "graphql",
		"name":         "Introspected Schema",
	})

	assertDefinitionMatches(t, created.ID, "Introspected Schema", []string{"order"})
}

// Content that is neither shape is the caller's mistake, and the answer has to
// say so rather than report a server fault.
func TestCreateAPIDefinitionRejectsUnparseableContent(t *testing.T) {
	workspace := importTestWorkspace(t)

	status, raw := postDefinition(t, map[string]interface{}{
		"workspace_id": workspace.ID,
		"content":      "this is not a schema",
		"type":         "graphql",
	})

	assert.Equal(t, fiber.StatusBadRequest, status, "body: %s", string(raw))

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &body))
	assert.Contains(t, body["error"], "parse")
}
