package discovery

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Endpoints are inserted as one batch, so a single field the spec makes too long for
// its column used to fail the statement and discard every endpoint of the definition
// — leaving a definition reported as parsed that the scanner never sends a request for.
func TestPersistOpenAPIDefinitionKeepsEndpointsWithOverlongOperationID(t *testing.T) {
	workspace := setupTestWorkspace(t)

	spec := `{
		"openapi": "3.0.3",
		"info": {"title": "Overlong", "version": "1.0.0"},
		"servers": [{"url": "http://api.test"}],
		"paths": {
			"/ok": {"get": {"operationId": "ok", "responses": {"200": {"description": "ok"}}}},
			"/long": {"get": {"operationId": "` + strings.Repeat("over", 200) + `", "responses": {"200": {"description": "ok"}}}}
		}
	}`

	definition := persistSpecFromURL(t, workspace.ID, "http://api.test/"+uuid.NewString()+".json", spec)

	assert.Equal(t, 2, definition.EndpointCount)

	endpoints, err := db.Connection().GetAPIEndpointsByDefinitionID(definition.ID)
	require.NoError(t, err)
	require.Len(t, endpoints, 2)
	for _, endpoint := range endpoints {
		assert.LessOrEqual(t, len([]rune(endpoint.OperationID)), 255)
		assert.LessOrEqual(t, len([]rune(endpoint.Name)), 255)
	}
}
