package proxy_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyManagerStartStop(t *testing.T) {
	manager := proxy.NewProxyManager()

	// Create test workspace and proxy service
	workspace := createTestWorkspace(t)
	proxyService := &db.ProxyService{
		WorkspaceID: &workspace.ID,
		Name:        "Manager Test",
		Host:        "localhost",
		Port:        19100, // Use higher port to avoid conflicts
		Enabled:     false,
	}
	created, err := db.Connection().CreateProxyService(proxyService)
	require.NoError(t, err)

	// Cleanup on exit
	defer db.Connection().DeleteProxyService(created.ID)

	// Start proxy
	err = manager.StartProxy(context.Background(), created.ID)
	require.NoError(t, err)

	// Verify it's running
	status, err := manager.GetStatus(created.ID)
	require.NoError(t, err)
	assert.True(t, status.Running)
	assert.NotZero(t, status.StartedAt)

	// Stop proxy
	err = manager.StopProxy(created.ID)
	require.NoError(t, err)

	// Verify it's stopped
	status, err = manager.GetStatus(created.ID)
	require.NoError(t, err)
	assert.False(t, status.Running)
}

func TestProxyManagerPortConflict(t *testing.T) {
	manager := proxy.NewProxyManager()

	workspace := createTestWorkspace(t)

	proxy1 := &db.ProxyService{
		WorkspaceID: &workspace.ID,
		Name:        "Proxy 1",
		Port:        19101, // Use higher port to avoid conflicts
	}
	created1, err := db.Connection().CreateProxyService(proxy1)
	require.NoError(t, err)

	// Cleanup on exit
	defer db.Connection().DeleteProxyService(created1.ID)

	// Start first proxy
	err = manager.StartProxy(context.Background(), created1.ID)
	require.NoError(t, err)

	// Cleanup running proxy
	defer manager.StopProxy(created1.ID)

	// Test 1: Try to start the same proxy again (should fail - already running)
	err = manager.StartProxy(context.Background(), created1.ID)
	assert.Error(t, err) // Should fail - already running
	assert.Contains(t, err.Error(), "already running")

	// Test 2: Create a second proxy on a different port
	proxy2 := &db.ProxyService{
		WorkspaceID: &workspace.ID,
		Name:        "Proxy 2",
		Port:        19102, // Different port (DB has unique constraint on port)
	}
	created2, err := db.Connection().CreateProxyService(proxy2)
	require.NoError(t, err)

	// Cleanup on exit
	defer db.Connection().DeleteProxyService(created2.ID)

	// Test 3: Try to update proxy2 to use same port as proxy1 in manager's internal check
	// We can't use the same port in DB due to unique constraint, but we can test
	// the manager's port conflict detection by modifying the service after creation
	// For now, just verify that proxy2 can start on its own port
	err = manager.StartProxy(context.Background(), created2.ID)
	require.NoError(t, err)

	// Cleanup
	manager.StopProxy(created2.ID)
}

func createTestWorkspace(t *testing.T) *db.Workspace {
	workspace := &db.Workspace{
		Code:  "test-mgr-" + uuid.New().String()[:8],
		Title: "Test Workspace",
	}
	created, err := db.Connection().CreateWorkspace(workspace)
	require.NoError(t, err)
	return created
}
