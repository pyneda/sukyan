package db

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// issueEndpointFixture creates a workspace holding one definition with two
// endpoints. issues.api_endpoint_id is a real foreign key, so the filter cannot be
// exercised with a synthetic UUID.
func issueEndpointFixture(t *testing.T, label string) (*Workspace, *APIEndpoint, *APIEndpoint) {
	t.Helper()
	unique := time.Now().UnixNano()

	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        fmt.Sprintf("test-%s-%d", label, unique),
		Title:       "Test issue endpoint filter",
		Description: "Temporary workspace",
	})
	require.NoError(t, err)

	definition, err := Connection().CreateAPIDefinition(&APIDefinition{
		WorkspaceID: workspace.ID,
		Name:        "filter fixture",
		Type:        APIDefinitionTypeOpenAPI,
		Status:      APIDefinitionStatusParsed,
		SourceURL:   fmt.Sprintf("https://api.example.com/spec-%d.json", unique),
	})
	require.NoError(t, err)

	first, err := Connection().CreateAPIEndpoint(&APIEndpoint{
		DefinitionID: definition.ID, Method: "GET", Path: "/first", Enabled: true,
	})
	require.NoError(t, err)
	second, err := Connection().CreateAPIEndpoint(&APIEndpoint{
		DefinitionID: definition.ID, Method: "GET", Path: "/second", Enabled: true,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		Connection().DB().Where("workspace_id = ?", workspace.ID).Delete(&Issue{})
		Connection().DB().Where("workspace_id = ?", workspace.ID).Delete(&APIDefinition{})
		Connection().DeleteWorkspace(workspace.ID)
	})

	return workspace, first, second
}

func TestListIssuesFiltersByAPIEndpointID(t *testing.T) {
	workspace, matchingEndpoint, otherEndpoint := issueEndpointFixture(t, "issue-endpoint")

	matchedID := matchingEndpoint.ID
	otherID := otherEndpoint.ID
	require.NoError(t, Connection().DB().Create(&Issue{
		Title: "matching", Code: "test-endpoint-filter", WorkspaceID: &workspace.ID, APIEndpointID: &matchedID,
	}).Error)
	require.NoError(t, Connection().DB().Create(&Issue{
		Title: "other", Code: "test-endpoint-filter", WorkspaceID: &workspace.ID, APIEndpointID: &otherID,
	}).Error)
	require.NoError(t, Connection().DB().Create(&Issue{
		Title: "unattached", Code: "test-endpoint-filter", WorkspaceID: &workspace.ID,
	}).Error)

	issues, count, err := Connection().ListIssues(IssueFilter{
		WorkspaceID:   workspace.ID,
		APIEndpointID: &matchedID,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	require.Len(t, issues, 1)
	require.Equal(t, "matching", issues[0].Title)
}

func TestListIssuesWithoutEndpointFilterReturnsAll(t *testing.T) {
	workspace, endpoint, _ := issueEndpointFixture(t, "issue-endpoint-all")

	endpointID := endpoint.ID
	require.NoError(t, Connection().DB().Create(&Issue{
		Title: "with", Code: "test-endpoint-filter-all", WorkspaceID: &workspace.ID, APIEndpointID: &endpointID,
	}).Error)
	require.NoError(t, Connection().DB().Create(&Issue{
		Title: "without", Code: "test-endpoint-filter-all", WorkspaceID: &workspace.ID,
	}).Error)

	_, count, err := Connection().ListIssues(IssueFilter{WorkspaceID: workspace.ID})
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
}

func TestListIssuesEndpointFilterExcludesUnrelatedUUID(t *testing.T) {
	workspace, endpoint, _ := issueEndpointFixture(t, "issue-endpoint-miss")

	endpointID := endpoint.ID
	require.NoError(t, Connection().DB().Create(&Issue{
		Title: "with", Code: "test-endpoint-filter-miss", WorkspaceID: &workspace.ID, APIEndpointID: &endpointID,
	}).Error)

	absent := uuid.New()
	_, count, err := Connection().ListIssues(IssueFilter{
		WorkspaceID:   workspace.ID,
		APIEndpointID: &absent,
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
}
