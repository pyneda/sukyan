package db

import (
	"testing"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

func TestGetWorkspaceByID(t *testing.T) {

	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        "TestGetWorkspaceByID",
		Title:       "TestGetWorkspaceByID",
		Description: "TestGetWorkspaceByID",
	})
	assert.NotNil(t, workspace)
	assert.Nil(t, err)

	fetchedWorkspace, err := Connection().GetWorkspaceByID(workspace.ID)
	assert.NotNil(t, fetchedWorkspace)
	assert.Nil(t, err)

	_, err = Connection().GetWorkspaceByID(99999)
	assert.NotNil(t, err)
}

func TestGetWorkspaceByCode(t *testing.T) {

	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        "TestGetWorkspaceByCode",
		Title:       "TestGetWorkspaceByCode",
		Description: "TestGetWorkspaceByCode",
	})
	assert.NotNil(t, workspace)
	assert.Nil(t, err)

	fetchedWorkspace, err := Connection().GetWorkspaceByCode(workspace.Code)
	assert.NotNil(t, fetchedWorkspace)
	assert.Nil(t, err)

	_, err = Connection().GetWorkspaceByCode("invalidCode")
	assert.NotNil(t, err)
}

func TestWorkspaceExists(t *testing.T) {

	workspace, err := Connection().CreateDefaultWorkspace()
	assert.NotNil(t, workspace)
	assert.Nil(t, err)

	exists, err := Connection().WorkspaceExists(workspace.ID)
	assert.True(t, exists)
	assert.Nil(t, err)

	exists, err = Connection().WorkspaceExists(99999)
	assert.False(t, exists)
	assert.Nil(t, err)
}

func TestListWorkspaces(t *testing.T) {

	_, err := Connection().CreateDefaultWorkspace()
	assert.Nil(t, err)

	filters := WorkspaceFilters{Query: ""}
	items, count, err := Connection().ListWorkspaces(filters)
	assert.NotNil(t, items)
	assert.Greater(t, count, int64(0))
	assert.Nil(t, err)
}

func TestCreateWorkspace(t *testing.T) {

	newWorkspace := &Workspace{
		Code:        "testCode",
		Title:       "testTitle",
		Description: "testDescription",
	}

	createdWorkspace, err := Connection().CreateWorkspace(newWorkspace)
	assert.NotNil(t, createdWorkspace)
	assert.Nil(t, err)

	// Try creating a workspace with a duplicate code
	_, err = Connection().CreateWorkspace(newWorkspace)
	assert.NotNil(t, err)
}

func TestGetOrCreateWorkspace(t *testing.T) {

	newWorkspace := &Workspace{
		Code:        "testCode2",
		Title:       "testTitle2",
		Description: "testDescription2",
	}

	// Test with a new workspace
	workspace, err := Connection().GetOrCreateWorkspace(newWorkspace)
	assert.NotNil(t, workspace)
	assert.Nil(t, err)

	// Test with an existing workspace
	workspace, err = Connection().GetOrCreateWorkspace(newWorkspace)
	assert.NotNil(t, workspace)
	assert.Nil(t, err)
}

func TestDeleteWorkspace(t *testing.T) {
	newWorkspace := &Workspace{
		Code:        "to-delete",
		Title:       "To Delete",
		Description: "description",
	}

	workspace, err := Connection().GetOrCreateWorkspace(newWorkspace)
	assert.NotNil(t, workspace)
	assert.Nil(t, err)
	// Create various objects to validate CASCADE constraint

	history, err := Connection().CreateHistory(&History{URL: "http://example.com", StatusCode: 200, Method: "GET", WorkspaceID: &workspace.ID})
	assert.Nil(t, err)
	assert.NotNil(t, history)
	historyID := history.ID
	history, err = Connection().GetHistoryByID(historyID)
	assert.Nil(t, err)
	assert.NotNil(t, history)

	issue, err := CreateIssueFromHistoryAndTemplate(history, SqlInjectionCode, "details", 100, "High", &workspace.ID, nil, nil, nil, nil)
	assert.Nil(t, err)
	assert.NotNil(t, issue)
	issueID := issue.ID
	issue, err = Connection().GetIssue(int(issueID), true)
	assert.Nil(t, err)
	assert.NotNil(t, issue)
	// Delete the workspace
	err = Connection().DeleteWorkspace(workspace.ID)
	assert.Nil(t, err)

	// Try fetching the deleted workspace
	_, err = Connection().GetWorkspaceByID(workspace.ID)
	assert.NotNil(t, err)
	// Try fetching the deleted history to validate CASCADE constraint
	history, err = Connection().GetHistoryByID(historyID)
	assert.NotNil(t, err)
	assert.Nil(t, history)
	// Try fetching the deleted issue to validate CASCADE constraint
	issue, err = Connection().GetIssue(int(issueID), true)
	assert.NotNil(t, err)
	log.Warn().Interface("issue", issue).Msg("issue")
	assert.Equal(t, issue.ID, uint(0))
	assert.Equal(t, issue.Title, "")

}

func TestUpdateWorkspace(t *testing.T) {

	workspace, err := Connection().CreateDefaultWorkspace()
	assert.NotNil(t, workspace)
	assert.Nil(t, err)

	updatedWorkspace := &Workspace{
		Code:        "updatedCode",
		Title:       "updatedTitle",
		Description: "updatedDescription",
	}

	// Update the workspace
	err = Connection().UpdateWorkspace(workspace.ID, updatedWorkspace)
	assert.Nil(t, err)

	// Fetch and validate
	fetchedWorkspace, err := Connection().GetWorkspaceByID(workspace.ID)
	assert.NotNil(t, fetchedWorkspace)
	assert.Nil(t, err)
	assert.Equal(t, "updatedCode", fetchedWorkspace.Code)
	assert.Equal(t, "updatedTitle", fetchedWorkspace.Title)
	assert.Equal(t, "updatedDescription", fetchedWorkspace.Description)
}
