package db

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupDefinitionSearchTest(t *testing.T, names ...string) (uint, func()) {
	t.Helper()

	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Title:       "TestWorkspaceAPIDefinitionSearch",
		Code:        "test-workspace-apidef-search-" + uuid.New().String(),
		Description: "Test workspace for API definition search",
	})
	require.NoError(t, err)

	for _, name := range names {
		definition := &APIDefinition{
			WorkspaceID: workspace.ID,
			Name:        name,
			Type:        APIDefinitionTypeOpenAPI,
			Status:      APIDefinitionStatusParsed,
			SourceURL:   "http://search.test/" + uuid.NewString() + ".json",
			BaseURL:     "http://search.test",
		}
		_, err := Connection().CreateAPIDefinition(definition)
		require.NoError(t, err)
	}

	return workspace.ID, func() {
		Connection().DB().Where("workspace_id = ?", workspace.ID).Delete(&APIDefinition{})
		Connection().DeleteWorkspace(workspace.ID)
	}
}

func searchDefinitionNames(t *testing.T, workspaceID uint, query string) []string {
	t.Helper()

	items, _, err := Connection().ListAPIDefinitions(APIDefinitionFilter{
		WorkspaceID: workspaceID,
		Query:       query,
		Pagination:  Pagination{Page: 1, PageSize: 100},
	})
	require.NoError(t, err)

	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	return names
}

// A search term is text the user typed, not a pattern they authored. Interpolated raw,
// "50%" matched every definition because the trailing % became "anything".
func TestListAPIDefinitionsTreatsPercentInSearchTermAsLiteral(t *testing.T) {
	workspaceID, cleanup := setupDefinitionSearchTest(t, "Checkout 50% off", "Checkout 50 units", "Billing")
	defer cleanup()

	assert.ElementsMatch(t, []string{"Checkout 50% off"}, searchDefinitionNames(t, workspaceID, "50%"))
}

// "_" is LIKE's single-character wildcard, so "v1_beta" used to match "v1-beta" too.
func TestListAPIDefinitionsTreatsUnderscoreInSearchTermAsLiteral(t *testing.T) {
	workspaceID, cleanup := setupDefinitionSearchTest(t, "v1_beta", "v1-beta", "v1xbeta")
	defer cleanup()

	assert.ElementsMatch(t, []string{"v1_beta"}, searchDefinitionNames(t, workspaceID, "v1_beta"))
}

// A term that is nothing but wildcards used to return the entire workspace; it must
// now return only the rows that literally contain that character.
func TestListAPIDefinitionsDoesNotMatchEverythingOnWildcardOnlyTerm(t *testing.T) {
	workspaceID, cleanup := setupDefinitionSearchTest(t, "Alpha", "Beta", "100% Gamma")
	defer cleanup()

	assert.ElementsMatch(t, []string{"100% Gamma"}, searchDefinitionNames(t, workspaceID, "%"))
	assert.Empty(t, searchDefinitionNames(t, workspaceID, "_"))
}

// The escaper introduces backslashes, so a term already containing one must not have
// them re-interpreted as an escape of the following character.
func TestListAPIDefinitionsTreatsBackslashInSearchTermAsLiteral(t *testing.T) {
	workspaceID, cleanup := setupDefinitionSearchTest(t, `path\to`, "pathxto")
	defer cleanup()

	assert.ElementsMatch(t, []string{`path\to`}, searchDefinitionNames(t, workspaceID, `path\to`))
}

// Escaping must not break the ordinary case the filter box is used for.
func TestListAPIDefinitionsStillMatchesPlainSubstrings(t *testing.T) {
	workspaceID, cleanup := setupDefinitionSearchTest(t, "Petstore API", "petstore internal", "Billing")
	defer cleanup()

	assert.ElementsMatch(t,
		[]string{"Petstore API", "petstore internal"},
		searchDefinitionNames(t, workspaceID, "petstore"),
	)
}
