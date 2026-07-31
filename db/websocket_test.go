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

func TestListWebSocketConnections_ExcludeSources(t *testing.T) {
	err := Connection().DB().AutoMigrate(&WebSocketConnection{})
	require.NoError(t, err)
	workspace := createTestWorkspace(t)

	mkConn := func(source string) uint {
		c := &WebSocketConnection{
			URL:         "wss://example.com/ws",
			Source:      source,
			WorkspaceID: &workspace.ID,
		}
		require.NoError(t, Connection().CreateWebSocketConnection(c))
		return c.ID
	}

	mkConn("Scanner")
	mkConn(SourcePlayground)
	mkConn(SourceWsFuzz)
	mkConn("Proxy")

	out, total, err := Connection().ListWebSocketConnections(WebSocketConnectionFilter{
		WorkspaceID:    workspace.ID,
		ExcludeSources: []string{SourcePlayground, SourceWsFuzz},
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	for _, c := range out {
		require.NotEqual(t, SourcePlayground, c.Source)
		require.NotEqual(t, SourceWsFuzz, c.Source)
	}
}

func TestListWebSocketConnections_SortByMessageCount(t *testing.T) {
	require.NoError(t, Connection().DB().AutoMigrate(&WebSocketConnection{}, &WebSocketMessage{}))
	workspace := createTestWorkspace(t)

	mkConn := func(url string, messages int) uint {
		c := &WebSocketConnection{URL: url, Source: "Scanner", WorkspaceID: &workspace.ID}
		require.NoError(t, Connection().CreateWebSocketConnection(c))
		for i := 0; i < messages; i++ {
			require.NoError(t, Connection().CreateWebSocketMessage(&WebSocketMessage{
				ConnectionID: c.ID,
				Opcode:       1,
				PayloadData:  "hello",
				Direction:    MessageSent,
			}))
		}
		return c.ID
	}

	quiet := mkConn("wss://example.com/quiet", 1)
	busy := mkConn("wss://example.com/busy", 3)

	out, _, err := Connection().ListWebSocketConnections(WebSocketConnectionFilter{
		WorkspaceID: workspace.ID,
		SortBy:      "message_count",
		SortOrder:   "desc",
	})
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.Equal(t, busy, out[0].ID)
	require.EqualValues(t, 3, out[0].MessageCount)
	require.Equal(t, quiet, out[1].ID)
	require.EqualValues(t, 1, out[1].MessageCount)
}

func TestListWebSocketConnections_SortByRejectsUnknownColumn(t *testing.T) {
	require.NoError(t, Connection().DB().AutoMigrate(&WebSocketConnection{}))
	workspace := createTestWorkspace(t)

	for _, url := range []string{"wss://example.com/a", "wss://example.com/b"} {
		c := &WebSocketConnection{URL: url, Source: "Scanner", WorkspaceID: &workspace.ID}
		require.NoError(t, Connection().CreateWebSocketConnection(c))
	}

	out, _, err := Connection().ListWebSocketConnections(WebSocketConnectionFilter{
		WorkspaceID: workspace.ID,
		SortBy:      "url; DROP TABLE web_socket_connections",
		SortOrder:   "asc",
	})
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.Greater(t, out[0].ID, out[1].ID)
}
