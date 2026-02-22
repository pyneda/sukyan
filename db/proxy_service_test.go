package db_test

import (
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateProxyService(t *testing.T) {
	workspace := createTestWorkspace(t)

	proxyService := &db.ProxyService{
		WorkspaceID:           &workspace.ID,
		Name:                  "Test Proxy",
		Host:                  "localhost",
		Port:                  9001,
		Verbose:               true,
		LogOutOfScopeRequests: true,
		Enabled:               false,
	}

	created, err := db.Connection().CreateProxyService(proxyService)
	require.NoError(t, err)
	assert.NotNil(t, created.ID)
	assert.Equal(t, "Test Proxy", created.Name)
	assert.Equal(t, 9001, created.Port)
}

func TestProxyServicePortUniqueness(t *testing.T) {
	workspace := createTestWorkspace(t)

	proxy1 := &db.ProxyService{
		WorkspaceID: &workspace.ID,
		Name:        "Proxy 1",
		Port:        9002,
	}
	_, err := db.Connection().CreateProxyService(proxy1)
	require.NoError(t, err)

	// Try to create another proxy with same port
	proxy2 := &db.ProxyService{
		WorkspaceID: &workspace.ID,
		Name:        "Proxy 2",
		Port:        9002,
	}
	_, err = db.Connection().CreateProxyService(proxy2)
	assert.Error(t, err) // Should fail due to unique constraint
}

func createTestWorkspace(t *testing.T) *db.Workspace {
	workspace := &db.Workspace{
		Code:  "test-proxy-" + lib.GenerateRandomLowercaseString(8),
		Title: "Test Workspace",
	}
	created, err := db.Connection().CreateWorkspace(workspace)
	require.NoError(t, err)
	return created
}
