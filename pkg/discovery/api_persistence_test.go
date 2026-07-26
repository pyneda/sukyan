package discovery

import (
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const noServersSpec = `{
	"openapi": "3.1.0",
	"info": {"title": "No Servers API", "version": "1.0.0"},
	"paths": {
		"/users": {"get": {"operationId": "listUsers", "responses": {"200": {"description": "ok"}}}},
		"/users/{id}": {"get": {"operationId": "getUser", "responses": {"200": {"description": "ok"}}}}
	}
}`

func persistSpecFromURL(t *testing.T, workspaceID uint, sourceURL, spec string) *db.APIDefinition {
	t.Helper()

	definition, err := PersistAPIDefinitionFromContent([]byte(spec), db.APIDefinitionTypeOpenAPI, APIPersistenceFromContentOptions{
		WorkspaceID: workspaceID,
		SourceURL:   sourceURL,
	})
	require.NoError(t, err)
	require.NotNil(t, definition)
	return definition
}

// A spec read from disk declares no servers and has no origin to borrow, so the
// only honest base URL is none at all. Storing "file://" instead made every
// request build fail with "no usable base URL" and the scan sent nothing.
func TestPersistOpenAPIDefinitionRejectsNonRequestableBaseURL(t *testing.T) {
	workspace := setupTestWorkspace(t)

	definition := persistSpecFromURL(t, workspace.ID, "file:///tmp/no-servers-"+uuid.NewString()+".json", noServersSpec)

	assert.Empty(t, definition.BaseURL, "a file-sourced spec without servers must not persist a base URL")
	assert.Equal(t, 2, definition.EndpointCount)
}

func TestPersistOpenAPIDefinitionUsesSourceOriginWhenServersAbsent(t *testing.T) {
	workspace := setupTestWorkspace(t)

	definition := persistSpecFromURL(t, workspace.ID, "http://api.example.test:8080/v1/"+uuid.NewString()+".json", noServersSpec)

	assert.Equal(t, "http://api.example.test:8080", definition.BaseURL)

	parsed, err := url.Parse(definition.BaseURL)
	require.NoError(t, err)
	assert.True(t, parsed.IsAbs())
	assert.NotEmpty(t, parsed.Host)
}
