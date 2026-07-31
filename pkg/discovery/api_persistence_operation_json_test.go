package discovery

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
	"github.com/stretchr/testify/require"
)

const operationJSONSpec = `{
  "openapi": "3.0.0",
  "info": {"title": "t", "version": "1"},
  "servers": [{"url": "https://api.example.com"}],
  "paths": {
    "/pets": {
      "get": {
        "operationId": "listPets",
        "parameters": [{"name": "limit", "in": "query", "schema": {"type": "integer"}}],
        "responses": {"200": {"description": "ok"}}
      }
    }
  }
}`

func operationJSONWorkspace(t *testing.T) *db.Workspace {
	t.Helper()
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Code:        fmt.Sprintf("test-op-json-%d", time.Now().UnixNano()),
		Title:       "Test operation_json persistence",
		Description: "Temporary workspace",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Connection().DB().Where("workspace_id = ?", workspace.ID).Delete(&db.APIDefinition{})
		db.Connection().DeleteWorkspace(workspace.ID)
	})
	return workspace
}

func TestPersistOpenAPIStoresOperationJSON(t *testing.T) {
	workspace := operationJSONWorkspace(t)

	definition, err := PersistAPIDefinitionFromContent(
		[]byte(operationJSONSpec),
		db.APIDefinitionTypeOpenAPI,
		APIPersistenceFromContentOptions{
			WorkspaceID: workspace.ID,
			SourceURL:   "https://api.example.com/openapi.json",
		},
	)
	require.NoError(t, err)

	endpoints, err := db.Connection().GetAPIEndpointsByDefinitionID(definition.ID)
	require.NoError(t, err)
	require.Len(t, endpoints, 1)
	require.NotEmpty(t, endpoints[0].OperationJSON)

	var op core.Operation
	require.NoError(t, json.Unmarshal(endpoints[0].OperationJSON, &op))
	require.Equal(t, "/pets", op.Path)
	require.Equal(t, "listPets", op.OperationID)
	require.Len(t, op.Parameters, 1)
	require.Equal(t, "limit", op.Parameters[0].Name)
	require.Len(t, op.Responses, 1)
	require.Equal(t, "200", op.Responses[0].StatusCode)
}

const graphqlOperationJSONSchema = `
type Query {
  pets(limit: Int): [String!]!
}

type Mutation {
  addPet(name: String!): String
}
`

func TestPersistGraphQLStoresOperationJSON(t *testing.T) {
	workspace := operationJSONWorkspace(t)

	definition, err := PersistAPIDefinitionFromContent(
		[]byte(graphqlOperationJSONSchema),
		db.APIDefinitionTypeGraphQL,
		APIPersistenceFromContentOptions{
			WorkspaceID: workspace.ID,
			SourceURL:   "https://api.example.com/graphql",
			BaseURL:     "https://api.example.com/graphql",
		},
	)
	require.NoError(t, err)

	endpoints, err := db.Connection().GetAPIEndpointsByDefinitionID(definition.ID)
	require.NoError(t, err)
	require.NotEmpty(t, endpoints)

	withOperation := 0
	for _, endpoint := range endpoints {
		if len(endpoint.OperationJSON) > 0 {
			withOperation++
		}
	}
	require.Equal(t, len(endpoints), withOperation, "every GraphQL endpoint should carry its operation")
}
