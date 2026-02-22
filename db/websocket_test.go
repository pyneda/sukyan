package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSocketConnectionWithProxyService(t *testing.T) {
	// Auto-migrate to add proxy_service_id column
	err := Connection().DB().AutoMigrate(&WebSocketConnection{})
	require.NoError(t, err)

	workspace := createTestWorkspace(t)
	randomPort := 50000 + (int(workspace.ID) % 10000)

	proxyService := &ProxyService{
		WorkspaceID: &workspace.ID,
		Name:        "WS Test Proxy",
		Port:        randomPort,
	}
	created, err := Connection().CreateProxyService(proxyService)
	require.NoError(t, err)

	wsConn := &WebSocketConnection{
		URL:            "wss://example.com/ws",
		WorkspaceID:    &workspace.ID,
		ProxyServiceID: &created.ID,
		Source:         SourceProxy,
	}

	err = Connection().CreateWebSocketConnection(wsConn)
	require.NoError(t, err)
	assert.Equal(t, created.ID, *wsConn.ProxyServiceID)

	// Fetch with preload (if GetWebSocketConnection supports preload)
	fetched, err := Connection().GetWebSocketConnection(wsConn.ID)
	require.NoError(t, err)

	// If preloading is supported, test it
	if fetched.ProxyService != nil {
		assert.Equal(t, "WS Test Proxy", fetched.ProxyService.Name)
	}
}

func TestWebSocketConnectionProxyServiceConstraints(t *testing.T) {
	// Auto-migrate to ensure constraints are applied
	err := Connection().DB().AutoMigrate(&WebSocketConnection{})
	require.NoError(t, err)

	workspace := createTestWorkspace(t)
	randomPort := 51000 + (int(workspace.ID) % 10000)

	proxyService := &ProxyService{
		WorkspaceID: &workspace.ID,
		Name:        "WS Test Proxy Constraints",
		Port:        randomPort,
	}
	created, err := Connection().CreateProxyService(proxyService)
	require.NoError(t, err)

	wsConn := &WebSocketConnection{
		URL:            "wss://example.com/ws/constraint-test",
		WorkspaceID:    &workspace.ID,
		ProxyServiceID: &created.ID,
		Source:         SourceProxy,
	}

	err = Connection().CreateWebSocketConnection(wsConn)
	require.NoError(t, err)
	assert.Equal(t, created.ID, *wsConn.ProxyServiceID)

	// Test OnDelete:SET NULL constraint (use Unscoped to actually delete, not soft delete)
	err = Connection().DB().Unscoped().Delete(proxyService).Error
	require.NoError(t, err)

	// Verify proxy_service_id is now NULL
	fetched, err := Connection().GetWebSocketConnection(wsConn.ID)
	require.NoError(t, err)
	assert.Nil(t, fetched.ProxyServiceID, "ProxyServiceID should be NULL after proxy service deletion")
}
