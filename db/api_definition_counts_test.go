package db

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCountedDefinition(t *testing.T) (*APIDefinition, func()) {
	t.Helper()

	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Title:       "TestWorkspaceAPIDefinitionCounters",
		Code:        "test-workspace-apidef-counters-" + uuid.New().String(),
		Description: "Test workspace for API definition counter preservation",
	})
	require.NoError(t, err)

	definition, err := Connection().CreateAPIDefinition(&APIDefinition{
		WorkspaceID:              workspace.ID,
		Name:                     "Counted",
		Type:                     APIDefinitionTypeGraphQL,
		Status:                   APIDefinitionStatusParsed,
		SourceURL:                "http://counters.test/" + uuid.NewString(),
		BaseURL:                  "http://counters.test",
		EndpointCount:            7,
		GraphQLQueryCount:        4,
		GraphQLMutationCount:     2,
		GraphQLSubscriptionCount: 1,
	})
	require.NoError(t, err)

	return definition, func() {
		Connection().DB().Where("workspace_id = ?", workspace.ID).Delete(&APIDefinition{})
		Connection().DeleteWorkspace(workspace.ID)
	}
}

// The import paths apply a user-supplied name or base URL on top of a definition they
// just parsed. Doing that with a full-struct save rewrites every column from memory, so
// any caller holding a struct whose counters are stale silently resets them — which is
// how definitions ended up reporting 0 endpoints with their endpoint rows intact.
func TestUpdateAPIDefinitionFieldsLeavesUnnamedColumnsAlone(t *testing.T) {
	definition, cleanup := setupCountedDefinition(t)
	defer cleanup()

	require.NoError(t, Connection().UpdateAPIDefinitionFields(definition.ID, map[string]interface{}{
		"name":     "Renamed",
		"base_url": "http://renamed.test",
	}))

	stored, err := Connection().GetAPIDefinitionByID(definition.ID)
	require.NoError(t, err)

	assert.Equal(t, "Renamed", stored.Name)
	assert.Equal(t, "http://renamed.test", stored.BaseURL)
	assert.Equal(t, 7, stored.EndpointCount, "endpoint_count must survive a rename")
	assert.Equal(t, 4, stored.GraphQLQueryCount)
	assert.Equal(t, 2, stored.GraphQLMutationCount)
	assert.Equal(t, 1, stored.GraphQLSubscriptionCount)
}

// The hazard the field update exists to avoid, stated as a test so the difference is
// not theoretical: a full-struct save writes the caller's stale zero over the row.
func TestUpdateAPIDefinitionSavesTheWholeStructIncludingStaleCounters(t *testing.T) {
	definition, cleanup := setupCountedDefinition(t)
	defer cleanup()

	stale, err := Connection().GetAPIDefinitionByID(definition.ID)
	require.NoError(t, err)
	stale.EndpointCount = 0
	stale.Name = "Renamed"

	_, err = Connection().UpdateAPIDefinition(stale)
	require.NoError(t, err)

	stored, err := Connection().GetAPIDefinitionByID(definition.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, stored.EndpointCount,
		"UpdateAPIDefinition persists the struct verbatim; callers holding partial state must use UpdateAPIDefinitionFields")
}

func TestUpdateAPIDefinitionFieldsIsANoOpWithoutUpdates(t *testing.T) {
	definition, cleanup := setupCountedDefinition(t)
	defer cleanup()

	require.NoError(t, Connection().UpdateAPIDefinitionFields(definition.ID, nil))

	stored, err := Connection().GetAPIDefinitionByID(definition.ID)
	require.NoError(t, err)
	assert.Equal(t, "Counted", stored.Name)
	assert.Equal(t, 7, stored.EndpointCount)
}
