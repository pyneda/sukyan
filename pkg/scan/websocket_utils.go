package scan

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
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
func replayPreviousMessages(client *websocket.Conn, newConnectionID uint, messages []db.WebSocketMessage, upToIndex int) ([]db.WebSocketMessage, error) {
	var replayedMessages []db.WebSocketMessage

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
			log.Error().Err(err).Uint("connection", newConnectionID).Str("payload", msg.PayloadData).Msg("Failed to send WebSocket message")
		}
		replayedMsg := db.WebSocketMessage{
			ConnectionID: newConnectionID,
			Opcode:       msg.Opcode,
			Mask:         msg.Mask,
			PayloadData:  msg.PayloadData,
			Timestamp:    time.Now(),
			Direction:    db.MessageSent,
			IsBinary:     msg.IsBinary,
		}

		err = db.Connection().CreateWebSocketMessage(&replayedMsg)
		if err != nil {
			log.Error().Err(err).Msg("Failed to save replayed WebSocket message")
		}

		replayedMessages = append(replayedMessages, replayedMsg)

		// Small delay to avoid overwhelming the server
		time.Sleep(100 * time.Millisecond)
	}
	return replayedMessages, nil
}
