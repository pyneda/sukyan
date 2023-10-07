package web

import (
	"encoding/json"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
	"time"
)

func ListenForWebSocketEvents(page *rod.Page, workspaceID, taskID uint, source string) {
	wsConnections := make(map[proto.NetworkRequestID]*db.WebSocketConnection)

	go page.EachEvent(func(e *proto.NetworkWebSocketCreated) {
		headers, _ := json.Marshal(e.Initiator)
		connection := &db.WebSocketConnection{
			URL:            e.URL,
			RequestHeaders: datatypes.JSON(headers),
			WorkspaceID:    &workspaceID,
			TaskID:         &taskID,
			Source:         source,
		}
		err := db.Connection.CreateWebSocketConnection(connection)
		if err != nil {
			log.Error().Uint("workspace", workspaceID).Err(err).Str("url", e.URL).Msg("Failed to create WebSocket connection")
			return
		}
		log.Info().Uint("workspace", workspaceID).Str("url", e.URL).Msg("Created WebSocket connection")
		wsConnections[e.RequestID] = connection
	}, func(e *proto.NetworkWebSocketHandshakeResponseReceived) {
		connection, ok := wsConnections[e.RequestID]
		if !ok {
			log.Warn().Uint("workspace", workspaceID).Str("request_id", string(e.RequestID)).Msg("Unknown connection")
			return
		}
		headers, err := json.Marshal(e.Response.Headers)
		if err == nil {
			connection.ResponseHeaders = datatypes.JSON(headers)
		}
		requestHeaders, err := json.Marshal(e.Response.RequestHeaders)
		if err == nil {
			connection.RequestHeaders = datatypes.JSON(requestHeaders)
		}
		connection.StatusCode = e.Response.Status
		connection.StatusText = e.Response.StatusText
		err = db.Connection.UpdateWebSocketConnection(connection)
		if err != nil {
			log.Error().Uint("workspace", workspaceID).Err(err).Str("url", connection.URL).Msg("Failed to update WebSocket connection")
		}

	}, func(e *proto.NetworkWebSocketFrameSent) {
		connection, ok := wsConnections[e.RequestID]
		if !ok {
			log.Warn().Uint("workspace", workspaceID).Str("request_id", string(e.RequestID)).Msg("Unknown connection")
			return
		}
		message := &db.WebSocketMessage{
			ConnectionID: connection.ID,
			Opcode:       e.Response.Opcode,
			Mask:         e.Response.Mask,
			PayloadData:  e.Response.PayloadData,
			Timestamp:    time.Now(),
			Direction:    db.MessageSent,
		}
		err := db.Connection.CreateWebSocketMessage(message)
		if err != nil {
			log.Error().Uint("workspace", workspaceID).Err(err).Str("data", e.Response.PayloadData).Msg("Failed to create WebSocket message")
		}
	}, func(e *proto.NetworkWebSocketFrameReceived) {
		connection, ok := wsConnections[e.RequestID]
		if !ok {
			log.Warn().Uint("workspace", workspaceID).Str("request_id", string(e.RequestID)).Msg("Unknown connection")
			return
		}
		message := &db.WebSocketMessage{
			ConnectionID: connection.ID,
			Opcode:       e.Response.Opcode,
			Mask:         e.Response.Mask,
			PayloadData:  e.Response.PayloadData,
			Timestamp:    time.Now(),
			Direction:    db.MessageReceived,
		}
		err := db.Connection.CreateWebSocketMessage(message)
		if err != nil {
			log.Error().Uint("workspace", workspaceID).Err(err).Str("data", e.Response.PayloadData).Msg("Failed to create WebSocket message")
		}
	}, func(e *proto.NetworkWebSocketClosed) {
		connection, ok := wsConnections[e.RequestID]
		if !ok {
			log.Warn().Uint("workspace", workspaceID).Str("request_id", string(e.RequestID)).Msg("Unknown connection")
			return
		}
		now := time.Now()
		connection.ClosedAt = now
		err := db.Connection.UpdateWebSocketConnection(connection)
		if err != nil {
			log.Error().Uint("workspace", workspaceID).Err(err).Str("url", connection.URL).Msg("Failed to update WebSocket connection closed at")
		}
		delete(wsConnections, e.RequestID)
	})()
}
