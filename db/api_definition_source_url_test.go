package db

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSourceURLTestWorkspace(t *testing.T) uint {
	t.Helper()

	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Title:       "TestWorkspaceAPIDefinitionSourceURL",
		Code:        "test-workspace-apidef-source-url-" + uuid.NewString(),
		Description: "Test workspace for API definition source URL lookups",
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		Connection().DB().Where("workspace_id = ?", workspace.ID).Delete(&APIDefinition{})
		Connection().DeleteWorkspace(workspace.ID)
	})

	return workspace.ID
}

func createSourceURLDefinition(t *testing.T, workspaceID uint, name, sourceURL string) *APIDefinition {
	t.Helper()

	definition, err := Connection().CreateAPIDefinition(&APIDefinition{
		WorkspaceID: workspaceID,
		Name:        name,
		Type:        APIDefinitionTypeOpenAPI,
		Status:      APIDefinitionStatusParsed,
		SourceURL:   sourceURL,
	})
	require.NoError(t, err)
	return definition
}

// Pasted definitions are stored with no source URL, so several of them share the
// empty string. Treating that as an identity is what let one pasted import be
// answered with an unrelated one.
func TestAPIDefinitionLookupIgnoresDefinitionsWithoutASourceURL(t *testing.T) {
	workspaceID := setupSourceURLTestWorkspace(t)

	first := createSourceURLDefinition(t, workspaceID, "Pasted One", "")
	second := createSourceURLDefinition(t, workspaceID, "Pasted Two", "")
	require.NotEqual(t, first.ID, second.ID)

	exists, err := Connection().APIDefinitionExistsBySourceURL(workspaceID, "")
	require.NoError(t, err)
	assert.False(t, exists, "an empty source URL must never match a stored definition")

	_, err = Connection().GetAPIDefinitionBySourceURL(workspaceID, "")
	assert.Error(t, err, "an empty source URL identifies nothing and must not resolve to a definition")
}

func TestAPIDefinitionLookupFindsAStoredSourceURL(t *testing.T) {
	workspaceID := setupSourceURLTestWorkspace(t)

	sourceURL := "http://lookup.test/" + uuid.NewString() + "/openapi.json"
	created := createSourceURLDefinition(t, workspaceID, "From URL", sourceURL)

	exists, err := Connection().APIDefinitionExistsBySourceURL(workspaceID, sourceURL)
	require.NoError(t, err)
	assert.True(t, exists)

	found, err := Connection().GetAPIDefinitionBySourceURL(workspaceID, sourceURL)
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)

	// The lookup is scoped to the workspace: another workspace's import of the
	// same URL is a different definition.
	otherWorkspaceID := setupSourceURLTestWorkspace(t)
	exists, err = Connection().APIDefinitionExistsBySourceURL(otherWorkspaceID, sourceURL)
	require.NoError(t, err)
	assert.False(t, exists)
}
