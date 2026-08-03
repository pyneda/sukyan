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

func TestListWebSocketConnections_Query(t *testing.T) {
	require.NoError(t, Connection().DB().AutoMigrate(&WebSocketConnection{}))
	workspace := createTestWorkspace(t)

	mkConn := func(url string) uint {
		c := &WebSocketConnection{URL: url, Source: "Scanner", WorkspaceID: &workspace.ID}
		require.NoError(t, Connection().CreateWebSocketConnection(c))
		return c.ID
	}

	lichess := mkConn("wss://socket5.lichess.org/play/abc")
	mkConn("wss://push.services.mozilla.com/")
	literal := mkConn("wss://example.com/100%discount")

	t.Run("matches a url substring", func(t *testing.T) {
		out, total, err := Connection().ListWebSocketConnections(WebSocketConnectionFilter{
			WorkspaceID: workspace.ID,
			Query:       "lichess",
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Len(t, out, 1)
		require.Equal(t, lichess, out[0].ID)
	})

	t.Run("is case insensitive", func(t *testing.T) {
		_, total, err := Connection().ListWebSocketConnections(WebSocketConnectionFilter{
			WorkspaceID: workspace.ID,
			Query:       "LICHESS",
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
	})

	t.Run("treats wildcards as literal text", func(t *testing.T) {
		out, total, err := Connection().ListWebSocketConnections(WebSocketConnectionFilter{
			WorkspaceID: workspace.ID,
			Query:       "100%",
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, total, "a trailing %% must not match every row")
		require.Len(t, out, 1)
		require.Equal(t, literal, out[0].ID)
	})
}

func TestListWebSocketConnections_MinMessages(t *testing.T) {
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

	mkConn("wss://example.com/empty", 0)
	mkConn("wss://example.com/noise", 2)
	busy := mkConn("wss://example.com/busy", 5)

	out, total, err := Connection().ListWebSocketConnections(WebSocketConnectionFilter{
		WorkspaceID: workspace.ID,
		MinMessages: 5,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total, "the count must reflect the filter, not the whole workspace")
	require.Len(t, out, 1)
	require.Equal(t, busy, out[0].ID)

	_, total, err = Connection().ListWebSocketConnections(WebSocketConnectionFilter{
		WorkspaceID: workspace.ID,
		MinMessages: 1,
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)

	_, total, err = Connection().ListWebSocketConnections(WebSocketConnectionFilter{
		WorkspaceID: workspace.ID,
	})
	require.NoError(t, err)
	require.EqualValues(t, 3, total, "an unset MinMessages must not filter")
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

func TestWebSocketPrefixClauseMatchesBothSchemes(t *testing.T) {
	clause, args := webSocketPrefixClause([]string{"https://app.test/ws"})

	assert.Contains(t, clause, "url = ?")
	assert.Contains(t, args, "https://app.test/ws")
	assert.Contains(t, args, "wss://app.test/ws", "the tree normalises wss to https, the rows do not")
	assert.Contains(t, args, "wss://app.test/ws/%")
	assert.Contains(t, args, "wss://app.test/ws?%", "socket.io appends a query string with no trailing slash")
}

func TestWebSocketPrefixClauseEscapesWildcards(t *testing.T) {
	_, args := webSocketPrefixClause([]string{"https://app.test/a_b"})

	assert.Contains(t, args, `https://app.test/a\_b/%`)
}

func TestWebSocketPrefixClauseEmpty(t *testing.T) {
	clause, args := webSocketPrefixClause(nil)

	assert.Empty(t, clause)
	assert.Empty(t, args)
}
