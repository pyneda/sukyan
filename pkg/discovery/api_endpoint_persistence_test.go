package discovery

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Endpoints are inserted as one batch, so an endpoint the database rejects used to
// fail the statement and discard every endpoint of the definition — leaving a
// definition reported as parsed that the scanner never sends a request for. The
// batch is now retried one at a time, so only the unstorable endpoint is lost.
func TestPersistOpenAPIDefinitionDropsOnlyTheUnstorableEndpoint(t *testing.T) {
	workspace := setupTestWorkspace(t)

	spec := `{
		"openapi": "3.0.3",
		"info": {"title": "Overlong", "version": "1.0.0"},
		"servers": [{"url": "http://api.test"}],
		"paths": {
			"/ok": {"get": {"operationId": "ok", "responses": {"200": {"description": "ok"}}}},
			"/also-ok": {"get": {"operationId": "alsoOk", "responses": {"200": {"description": "ok"}}}},
			"/long": {"get": {"operationId": "` + strings.Repeat("over", 200) + `", "responses": {"200": {"description": "ok"}}}}
		}
	}`

	definition := persistSpecFromURL(t, workspace.ID, "http://api.test/"+uuid.NewString()+".json", spec)

	assert.Equal(t, 2, definition.EndpointCount, "the storable endpoints must survive their unstorable sibling")

	endpoints, err := db.Connection().GetAPIEndpointsByDefinitionID(definition.ID)
	require.NoError(t, err)
	require.Len(t, endpoints, 2)

	paths := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		paths = append(paths, endpoint.Path)
	}
	assert.ElementsMatch(t, []string{"/ok", "/also-ok"}, paths)
}

// The scan executor matches an endpoint to its parsed operation by OperationID, and
// for GraphQL and SOAP every operation shares Path "" and method POST — so a shortened
// identifier does not merely miss, it falls through to a positional match and binds to
// the wrong operation. Storing a truncated identifier is therefore never acceptable.
func TestPersistOpenAPIDefinitionNeverStoresATruncatedOperationID(t *testing.T) {
	workspace := setupTestWorkspace(t)

	operationID := strings.Repeat("a", 200)
	spec := `{
		"openapi": "3.0.3",
		"info": {"title": "Exact", "version": "1.0.0"},
		"servers": [{"url": "http://api.test"}],
		"paths": {
			"/exact": {"get": {"operationId": "` + operationID + `", "responses": {"200": {"description": "ok"}}}}
		}
	}`

	definition := persistSpecFromURL(t, workspace.ID, "http://api.test/"+uuid.NewString()+".json", spec)

	endpoints, err := db.Connection().GetAPIEndpointsByDefinitionID(definition.ID)
	require.NoError(t, err)
	require.Len(t, endpoints, 1)
	assert.Equal(t, operationID, endpoints[0].OperationID, "a stored OperationID must match the spec exactly or not be stored at all")
}

// An over-long info.title used to abort CreateAPIDefinition, discarding the whole
// definition rather than one field.
func TestPersistOpenAPIDefinitionSurvivesOverlongDefinitionFields(t *testing.T) {
	workspace := setupTestWorkspace(t)

	spec := `{
		"openapi": "3.0.3",
		"info": {"title": "` + strings.Repeat("T", 400) + `", "version": "1.0.0"},
		"servers": [{"url": "http://api.test"}],
		"paths": {"/ok": {"get": {"operationId": "ok", "responses": {"200": {"description": "ok"}}}}}
	}`

	definition := persistSpecFromURL(t, workspace.ID, "http://api.test/"+uuid.NewString()+".json", spec)

	assert.LessOrEqual(t, len([]rune(definition.Name)), 255)
	assert.Equal(t, 1, definition.EndpointCount)
}

// A definition whose child records cannot be stored must not be left looking parsed:
// the definition row is created before the transaction, so without this it survives
// with zero endpoints, indistinguishable from an API that genuinely has none, and the
// scan built from it runs to completion having sent nothing.
func TestFailDefinitionMarksTheDefinitionFailed(t *testing.T) {
	workspace := setupTestWorkspace(t)

	definition, err := db.Connection().CreateAPIDefinition(&db.APIDefinition{
		WorkspaceID:   workspace.ID,
		Name:          "Doomed",
		Type:          db.APIDefinitionTypeOpenAPI,
		Status:        db.APIDefinitionStatusParsed,
		SourceURL:     "http://api.test/" + uuid.NewString() + ".json",
		BaseURL:       "http://api.test",
		EndpointCount: 7,
	})
	require.NoError(t, err)

	returned := failDefinition(definition, errors.New("endpoint insert exploded"), "OpenAPI")

	require.Error(t, returned, "the caller must learn the definition is unusable")
	assert.Contains(t, returned.Error(), "endpoint insert exploded", "the cause must survive wrapping")
	assert.Equal(t, 0, definition.EndpointCount)

	stored, err := db.Connection().GetAPIDefinitionByID(definition.ID)
	require.NoError(t, err)
	assert.Equal(t, db.APIDefinitionStatusFailed, stored.Status)
	assert.Equal(t, 0, stored.EndpointCount)
}
