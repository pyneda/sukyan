package db

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetWorkspaceByID(t *testing.T) {

	workspace, err := Connection.CreateDefaultWorkspace()
	assert.NotNil(t, workspace)
	assert.Nil(t, err)

	fetchedWorkspace, err := Connection.GetWorkspaceByID(workspace.ID)
	assert.NotNil(t, fetchedWorkspace)
	assert.Nil(t, err)

	_, err = Connection.GetWorkspaceByID(99999)
	assert.NotNil(t, err)
}

func TestGetWorkspaceByCode(t *testing.T) {

	workspace, err := Connection.CreateDefaultWorkspace()
	assert.NotNil(t, workspace)
	assert.Nil(t, err)

	fetchedWorkspace, err := Connection.GetWorkspaceByCode(workspace.Code)
	assert.NotNil(t, fetchedWorkspace)
	assert.Nil(t, err)

	_, err = Connection.GetWorkspaceByCode("invalidCode")
	assert.NotNil(t, err)
}

func TestWorkspaceExists(t *testing.T) {

	workspace, err := Connection.CreateDefaultWorkspace()
	assert.NotNil(t, workspace)
	assert.Nil(t, err)

	exists, err := Connection.WorkspaceExists(workspace.ID)
	assert.True(t, exists)
	assert.Nil(t, err)

	exists, err = Connection.WorkspaceExists(99999)
	assert.False(t, exists)
	assert.Nil(t, err)
}

func TestListWorkspaces(t *testing.T) {

	_, err := Connection.CreateDefaultWorkspace()
	assert.Nil(t, err)

	filters := WorkspaceFilters{Query: ""}
	items, count, err := Connection.ListWorkspaces(filters)
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

	createdWorkspace, err := Connection.CreateWorkspace(newWorkspace)
	assert.NotNil(t, createdWorkspace)
	assert.Nil(t, err)

	// Try creating a workspace with a duplicate code
	_, err = Connection.CreateWorkspace(newWorkspace)
	assert.NotNil(t, err)
}

func TestGetOrCreateWorkspace(t *testing.T) {

	newWorkspace := &Workspace{
		Code:        "testCode2",
		Title:       "testTitle2",
		Description: "testDescription2",
	}

	// Test with a new workspace
	workspace, err := Connection.GetOrCreateWorkspace(newWorkspace)
	assert.NotNil(t, workspace)
	assert.Nil(t, err)

	// Test with an existing workspace
	workspace, err = Connection.GetOrCreateWorkspace(newWorkspace)
	assert.NotNil(t, workspace)
	assert.Nil(t, err)
}

func TestDeleteWorkspace(t *testing.T) {
	newWorkspace := &Workspace{
		Code:        "to-delete",
		Title:       "To Delete",
		Description: "description",
	}

	workspace, err := Connection.GetOrCreateWorkspace(newWorkspace)
	assert.NotNil(t, workspace)
	assert.Nil(t, err)
	// Delete the workspace
	err = Connection.DeleteWorkspace(workspace.ID)
	assert.Nil(t, err)

	// // Try deleting the same workspace again
	// err = Connection.DeleteWorkspace(workspace.ID)
	// assert.NotNil(t, err)
}

func TestUpdateWorkspace(t *testing.T) {

	workspace, err := Connection.CreateDefaultWorkspace()
	assert.NotNil(t, workspace)
	assert.Nil(t, err)

	updatedWorkspace := &Workspace{
		Code:        "updatedCode",
		Title:       "updatedTitle",
		Description: "updatedDescription",
	}

	// Update the workspace
	err = Connection.UpdateWorkspace(workspace.ID, updatedWorkspace)
	assert.Nil(t, err)

	// Fetch and validate
	fetchedWorkspace, err := Connection.GetWorkspaceByID(workspace.ID)
	assert.NotNil(t, fetchedWorkspace)
	assert.Nil(t, err)
	assert.Equal(t, "updatedCode", fetchedWorkspace.Code)
	assert.Equal(t, "updatedTitle", fetchedWorkspace.Title)
	assert.Equal(t, "updatedDescription", fetchedWorkspace.Description)
}
