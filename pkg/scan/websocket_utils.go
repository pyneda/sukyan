package scan

import (
	"net/http"
	"strings"
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

// autoManagedWebSocketHeaders are set by the WebSocket client during the
// handshake and must not be copied verbatim from a captured connection.
// Sec-WebSocket-Protocol is intentionally NOT here: it is re-negotiated through
// dialer.Subprotocols so subprotocol-gated servers (e.g. graphql-transport-ws)
// still accept the replayed handshake.
func isAutoManagedWebSocketHeader(key string) bool {
	switch {
	case strings.EqualFold(key, "Connection"),
		strings.EqualFold(key, "Upgrade"),
		strings.EqualFold(key, "Sec-WebSocket-Key"),
		strings.EqualFold(key, "Sec-WebSocket-Version"),
		strings.EqualFold(key, "Sec-WebSocket-Extensions"):
		return true
	}
	return false
}

// dialerAndHeadersForConnection builds a dialer + request headers that reproduce
// the captured handshake as closely as possible: the Origin and any custom
// headers are preserved, and the captured Sec-WebSocket-Protocol is negotiated
// via dialer.Subprotocols rather than dropped (previously the scanner stripped
// the subprotocol entirely, so subprotocol-gated endpoints rejected the replay).
func dialerAndHeadersForConnection(conn *db.WebSocketConnection) (*websocket.Dialer, http.Header, error) {
	dialer, err := createWebSocketDialer(conn)
	if err != nil {
		return nil, nil, err
	}

	headers, err := conn.GetRequestHeadersAsMap()
	if err != nil {
		return nil, nil, err
	}

	httpHeaders := http.Header{}
	for key, values := range headers {
		if strings.EqualFold(key, "Sec-WebSocket-Protocol") {
			for _, value := range values {
				for _, proto := range strings.Split(value, ",") {
					if trimmed := strings.TrimSpace(proto); trimmed != "" {
						dialer.Subprotocols = append(dialer.Subprotocols, trimmed)
					}
				}
			}
			continue
		}
		if isAutoManagedWebSocketHeader(key) {
			continue
		}
		for _, value := range values {
			httpHeaders.Add(key, value)
		}
	}

	return dialer, httpHeaders, nil
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
