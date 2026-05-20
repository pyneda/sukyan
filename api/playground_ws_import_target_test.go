package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/require"
)

// makeFakeWebSocketConnection inserts a WebSocketConnection + 2 sent messages
// so the import-connection endpoint has source data.
func makeFakeWebSocketConnection(t *testing.T, workspaceID uint) uint {
	t.Helper()
	conn := db.Connection()
	wsConn := &db.WebSocketConnection{
		URL:         "wss://example.com/ws",
		Source:      "scanner",
		WorkspaceID: &workspaceID,
	}
	require.NoError(t, conn.CreateWebSocketConnection(wsConn))
	for i := 0; i < 2; i++ {
		m := &db.WebSocketMessage{
			ConnectionID: wsConn.ID,
			Opcode:       1,
			PayloadData:  fmt.Sprintf("hello-%d", i),
			Direction:    db.MessageSent,
		}
		require.NoError(t, conn.CreateWebSocketMessage(m))
	}
	return wsConn.ID
}

func TestImportConnection_TargetsWsFuzzSession(t *testing.T) {
	conn := db.Connection()
	ws, err := conn.GetOrCreateWorkspace(&db.Workspace{Title: "import-target-" + t.Name(), Code: "imptg_" + sanitizeForCode(t.Name())})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.DeleteWorkspace(ws.ID) })

	connID := makeFakeWebSocketConnection(t, ws.ID)

	app := fiber.New()
	app.Post("/api/v1/playground/ws/sessions/import-connection", ImportConnectionToPlaygroundWs)

	body := map[string]any{
		"workspace_id":        ws.ID,
		"name":                "imported-fuzz",
		"connection_id":       connID,
		"target_session_type": "ws_fuzz",
	}
	resp := doJSON(t, app, "POST", "/api/v1/playground/ws/sessions/import-connection", body)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	sessionMap := out["session"].(map[string]any)
	sessionID := uint(sessionMap["id"].(float64))
	parent, err := conn.GetPlaygroundSession(sessionID)
	require.NoError(t, err)
	require.Equal(t, db.WsFuzzType, parent.Type)
}

func TestImportConnection_DefaultsToWsManual(t *testing.T) {
	conn := db.Connection()
	ws, err := conn.GetOrCreateWorkspace(&db.Workspace{Title: "import-default-" + t.Name(), Code: "impdef_" + sanitizeForCode(t.Name())})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.DeleteWorkspace(ws.ID) })

	connID := makeFakeWebSocketConnection(t, ws.ID)

	app := fiber.New()
	app.Post("/api/v1/playground/ws/sessions/import-connection", ImportConnectionToPlaygroundWs)

	body := map[string]any{
		"workspace_id":  ws.ID,
		"name":          "imported-manual",
		"connection_id": connID,
		// target_session_type omitted
	}
	resp := doJSON(t, app, "POST", "/api/v1/playground/ws/sessions/import-connection", body)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	sessionMap := out["session"].(map[string]any)
	sessionID := uint(sessionMap["id"].(float64))
	parent, err := conn.GetPlaygroundSession(sessionID)
	require.NoError(t, err)
	require.Equal(t, db.WsManualType, parent.Type)
}
