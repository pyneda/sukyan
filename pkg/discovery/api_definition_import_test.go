package discovery

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Two specs that share no operation at all, so a definition holding the wrong
// one's endpoints is impossible to miss.
const importSpecUsers = `{
	"openapi": "3.0.3",
	"info": {"title": "Users API", "version": "1.0.0"},
	"servers": [{"url": "http://users.test"}],
	"paths": {
		"/users": {"get": {"operationId": "listUsers", "responses": {"200": {"description": "ok"}}}},
		"/users/{id}": {"get": {"operationId": "getUser", "responses": {"200": {"description": "ok"}}}}
	}
}`

const importSpecOrders = `{
	"openapi": "3.0.3",
	"info": {"title": "Orders API", "version": "2.0.0"},
	"servers": [{"url": "http://orders.test"}],
	"paths": {
		"/orders": {"post": {"operationId": "createOrder", "responses": {"201": {"description": "ok"}}}}
	}
}`

const importSDL = `
type Query { order(id: ID!): Order }
type Order { id: ID! total: Float! }
`

func storedOperationIDs(t *testing.T, definitionID uuid.UUID) []string {
	t.Helper()
	endpoints, err := db.Connection().GetAPIEndpointsByDefinitionID(definitionID)
	require.NoError(t, err)

	ids := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		ids = append(ids, endpoint.OperationID)
	}
	return ids
}

// assertNameMatchesOperations is the invariant the import bug broke: a definition
// named after one document while holding another's operations.
func assertNameMatchesOperations(t *testing.T, definitionID uuid.UUID, expectedName string, expectedOperations []string) {
	t.Helper()
	stored := storedDefinition(t, definitionID)
	assert.Equal(t, expectedName, stored.Name)
	assert.ElementsMatch(t, expectedOperations, storedOperationIDs(t, definitionID))
	assert.Equal(t, len(expectedOperations), stored.EndpointCount)
}

// Pasted content carries no source URL. Storing it under the synthetic "/" the
// content was wrapped in made every paste in a workspace the same definition: the
// second import was answered with the first one's row, renamed to the second
// document's title but still holding the first document's operations.
func TestPastedImportsDoNotCollideWithEachOther(t *testing.T) {
	workspace := setupTestWorkspace(t)

	first, err := PersistAPIDefinitionFromContent([]byte(importSpecUsers), db.APIDefinitionTypeOpenAPI, APIPersistenceFromContentOptions{
		WorkspaceID: workspace.ID,
	})
	require.NoError(t, err)

	second, err := PersistAPIDefinitionFromContent([]byte(importSpecOrders), db.APIDefinitionTypeOpenAPI, APIPersistenceFromContentOptions{
		WorkspaceID: workspace.ID,
	})
	require.NoError(t, err)

	require.NotEqual(t, first.ID, second.ID, "a second pasted import must not be answered with the first one's definition")

	assertNameMatchesOperations(t, first.ID, "Users API", []string{"listUsers", "getUser"})
	assertNameMatchesOperations(t, second.ID, "Orders API", []string{"createOrder"})

	// No fake URL stands in for "pasted": an invented one would collide again as
	// soon as two workspaces or two pastes agreed on it.
	assert.Empty(t, storedDefinition(t, first.ID).SourceURL)
	assert.Empty(t, storedDefinition(t, second.ID).SourceURL)
}

// Re-importing a URL already in the library adds a definition. It used to rename
// the existing one — a QA run renamed a real, curated definition this way — while
// leaving its endpoints untouched, so the row ended up describing operations that
// had nothing to do with its name.
func TestReimportingAKnownURLLeavesTheExistingDefinitionAlone(t *testing.T) {
	workspace := setupTestWorkspace(t)
	sourceURL := "http://reimport.test/" + uuid.NewString() + "/openapi.json"

	first, err := PersistAPIDefinitionFromContent([]byte(importSpecUsers), db.APIDefinitionTypeOpenAPI, APIPersistenceFromContentOptions{
		WorkspaceID: workspace.ID,
		SourceURL:   sourceURL,
	})
	require.NoError(t, err)

	second, err := PersistAPIDefinitionFromContent([]byte(importSpecOrders), db.APIDefinitionTypeOpenAPI, APIPersistenceFromContentOptions{
		WorkspaceID: workspace.ID,
		SourceURL:   sourceURL,
		Name:        "Orders On Purpose",
	})
	require.NoError(t, err)

	require.NotEqual(t, first.ID, second.ID, "a re-import must not take over the definition already in the library")

	assertNameMatchesOperations(t, first.ID, "Users API", []string{"listUsers", "getUser"})
	assertNameMatchesOperations(t, second.ID, "Orders On Purpose", []string{"createOrder"})

	assert.Equal(t, sourceURL, storedDefinition(t, second.ID).SourceURL)
}

// The name override applies to the document that was just parsed, never to one
// that was parsed earlier: whatever a definition is called, its operations come
// from the document it was named for.
func TestImportNameNeverDisagreesWithStoredOperations(t *testing.T) {
	workspace := setupTestWorkspace(t)

	named, err := PersistAPIDefinitionFromContent([]byte(importSpecOrders), db.APIDefinitionTypeOpenAPI, APIPersistenceFromContentOptions{
		WorkspaceID: workspace.ID,
		Name:        "Chosen Name",
		BaseURL:     "http://override.test",
	})
	require.NoError(t, err)

	assertNameMatchesOperations(t, named.ID, "Chosen Name", []string{"createOrder"})
	assert.Equal(t, "http://override.test", storedDefinition(t, named.ID).BaseURL)
}

// A pasted GraphQL schema is SDL, not an introspection response. It used to fail
// with "invalid character 'y' in literal true" — the JSON decoder's reading of
// "type Query".
func TestPastedGraphQLSDLImportsItsOperations(t *testing.T) {
	workspace := setupTestWorkspace(t)

	definition, err := PersistAPIDefinitionFromContent([]byte(importSDL), db.APIDefinitionTypeGraphQL, APIPersistenceFromContentOptions{
		WorkspaceID: workspace.ID,
		Name:        "Pasted GraphQL",
	})
	require.NoError(t, err)

	assertNameMatchesOperations(t, definition.ID, "Pasted GraphQL", []string{"order"})

	stored := storedDefinition(t, definition.ID)
	assert.Equal(t, 1, stored.GraphQLQueryCount)
	assert.Empty(t, stored.SourceURL)
	// Pasted content has no origin to derive one from, and "://" is not a base URL.
	assert.Empty(t, stored.BaseURL)
}

func TestImportRejectsContentThatIsNotADefinition(t *testing.T) {
	workspace := setupTestWorkspace(t)

	_, err := PersistAPIDefinitionFromContent([]byte("not a schema at all"), db.APIDefinitionTypeGraphQL, APIPersistenceFromContentOptions{
		WorkspaceID: workspace.ID,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidDefinitionContent, "an unreadable document is the caller's input, not a server fault")
}

// Automatic discovery keeps reusing the definition it already stored for a source
// URL: it re-encounters the same spec many times in one scan, and a new row for
// each would bury the library.
func TestDiscoveryStillReusesTheDefinitionForAKnownSourceURL(t *testing.T) {
	workspace := setupTestWorkspace(t)
	sourceURL := "http://discovered.test/" + uuid.NewString() + "/openapi.json"

	opts := APIPersistenceOptions{WorkspaceID: workspace.ID}

	first, err := PersistOpenAPIDefinition(historyForSpec(t, workspace.ID, sourceURL, importSpecUsers), opts)
	require.NoError(t, err)

	second, err := PersistOpenAPIDefinition(historyForSpec(t, workspace.ID, sourceURL, importSpecUsers), opts)
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID)
	assertNameMatchesOperations(t, first.ID, "Users API", []string{"listUsers", "getUser"})
}

func historyForSpec(t *testing.T, workspaceID uint, sourceURL, spec string) *db.History {
	t.Helper()

	parsed, err := url.Parse(sourceURL)
	require.NoError(t, err)

	response := &http.Response{
		StatusCode: 200,
		Proto:      "HTTP/1.1",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(spec))),
		Request:    &http.Request{Method: "GET", URL: parsed},
	}

	history, err := http_utils.ReadHttpResponseAndCreateHistory(response, http_utils.HistoryCreationOptions{
		Source:      db.SourceScanner,
		WorkspaceID: workspaceID,
	})
	require.NoError(t, err)
	return history
}
