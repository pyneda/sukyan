package db

import (
	"testing"

	"github.com/pyneda/sukyan/pkg/browser/actions"
	"github.com/stretchr/testify/assert"
)

func TestStoredBrowserActionsCreateAndFetch(t *testing.T) {
	testActions := []actions.Action{
		{Type: actions.ActionClick, Selector: "#button"},
		{Type: actions.ActionFill, Selector: "#input", Value: "test"},
		{Type: actions.ActionNavigate, URL: "https://example.com"},
		{Type: actions.ActionWait, Duration: 5000},
		{Type: actions.ActionScreenshot, File: "screenshot.png"},
	}

	sba := &StoredBrowserActions{
		Title:   "Test Actions",
		Actions: testActions,
		Scope:   BrowserActionScopeGlobal,
	}

	// Create
	created, err := Connection().CreateStoredBrowserActions(sba)
	assert.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.Equal(t, sba.Title, created.Title)
	assert.Equal(t, sba.Scope, created.Scope)

	// Fetch
	fetched, err := Connection().GetStoredBrowserActionsByID(created.ID)
	assert.NoError(t, err)
	assert.Equal(t, created.ID, fetched.ID)
	assert.Equal(t, created.Title, fetched.Title)
	assert.Equal(t, created.Scope, fetched.Scope)
	assert.Len(t, fetched.Actions, len(testActions))

	// Check each action
	for i, action := range fetched.Actions {
		assert.Equal(t, testActions[i].Type, action.Type)
		assert.Equal(t, testActions[i].Selector, action.Selector)
		assert.Equal(t, testActions[i].Value, action.Value)
		assert.Equal(t, testActions[i].URL, action.URL)
		assert.Equal(t, testActions[i].Duration, action.Duration)
		assert.Equal(t, testActions[i].File, action.File)
	}
}

func TestStoredBrowserActionsUpdate(t *testing.T) {
	// First, create a StoredBrowserActions
	initialActions := []actions.Action{
		{Type: actions.ActionClick, Selector: "#button"},
	}

	sba := &StoredBrowserActions{
		Title:   "Initial Actions",
		Actions: initialActions,
		Scope:   BrowserActionScopeGlobal,
	}

	created, err := Connection().CreateStoredBrowserActions(sba)
	assert.NoError(t, err)

	// Update with new actions
	updatedActions := []actions.Action{
		{Type: actions.ActionNavigate, URL: "https://example.com"},
		{Type: actions.ActionFill, Selector: "#input", Value: "updated"},
	}

	created.Title = "Updated Actions"
	created.Actions = updatedActions
	created.Scope = BrowserActionScopeWorkspace

	updated, err := Connection().UpdateStoredBrowserActions(created.ID, created)
	assert.NoError(t, err)

	// Fetch and verify the update
	fetched, err := Connection().GetStoredBrowserActionsByID(updated.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Actions", fetched.Title)
	assert.Equal(t, BrowserActionScopeWorkspace, fetched.Scope)
	assert.Len(t, fetched.Actions, len(updatedActions))

	// Check each updated action
	for i, action := range fetched.Actions {
		assert.Equal(t, updatedActions[i].Type, action.Type)
		assert.Equal(t, updatedActions[i].Selector, action.Selector)
		assert.Equal(t, updatedActions[i].Value, action.Value)
		assert.Equal(t, updatedActions[i].URL, action.URL)
	}
}
