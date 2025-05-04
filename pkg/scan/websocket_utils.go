package scan

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pyneda/sukyan/db"
)

// createWebSocketDialer creates a WebSocket dialer configured based on the original connection
func createWebSocketDialer(conn *db.WebSocketConnection) (*websocket.Dialer, error) {
	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 45 * time.Second,
	}
	return dialer, nil
}

// replayPreviousMessages sends all original messages up to the target index
func replayPreviousMessages(client *websocket.Conn, messages []db.WebSocketMessage, upToIndex int) error {
	for i := 0; i < upToIndex; i++ {
		msg := messages[i]
		// Only send messages that were originally sent, not received
		if msg.Direction != db.MessageSent {
			continue
		}

		var messageType int
		if msg.Opcode == 1 {
			messageType = websocket.TextMessage
		} else {
			messageType = websocket.BinaryMessage
		}

		err := client.WriteMessage(messageType, []byte(msg.PayloadData))
		if err != nil {
			return err
		}

		// Small delay to avoid overwhelming the server
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}
