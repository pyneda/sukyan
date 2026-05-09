package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
	"github.com/rs/zerolog/log"
)

// CreateWsSessionInput represents the input for creating a Playground WS Session.
type CreateWsSessionInput struct {
	CollectionID   uint            `json:"collection_id" validate:"required,min=1"`
	WorkspaceID    uint            `json:"workspace_id" validate:"required,min=1"`
	Name           string          `json:"name" validate:"required"`
	TargetURL      string          `json:"target_url"`
	RequestHeaders json.RawMessage `json:"request_headers"`
	Script         json.RawMessage `json:"script"`
	Options        json.RawMessage `json:"options"`
}

// orEmpty returns the provided raw JSON, or fallback when empty.
func orEmpty(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(fallback)
	}
	return raw
}

// CreatePlaygroundWsSession godoc
// @Summary Create a new playground WebSocket session
// @Description Create a new playground WebSocket session and its associated WS payload
// @Tags Playground
// @Accept json
// @Produce json
// @Param input body CreateWsSessionInput true "Create Playground WS Session Input"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/ws/sessions [post]
func CreatePlaygroundWsSession(c *fiber.Ctx) error {
	input := new(CreateWsSessionInput)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Cannot parse JSON"})
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Validation failed", Message: err.Error()})
	}

	workspaceExists, err := db.Connection().WorkspaceExists(input.WorkspaceID)
	if !workspaceExists || err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid workspace", Message: "The provided workspace ID does not seem valid"})
	}

	collection, err := db.Connection().GetPlaygroundCollection(input.CollectionID)
	if err != nil {
		log.Error().Err(err).Interface("input", input).Msg("Failed to retrieve Playground Collection")
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid collection", Message: "The provided collection ID does not seem valid"})
	}

	if collection.WorkspaceID != input.WorkspaceID {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid collection", Message: "The collection does not belong to the provided workspace"})
	}

	sess := &db.PlaygroundSession{
		Name:         input.Name,
		Type:         db.WsManualType,
		WorkspaceID:  input.WorkspaceID,
		CollectionID: input.CollectionID,
	}
	if err := db.Connection().CreatePlaygroundSession(sess); err != nil {
		log.Error().Err(err).Interface("input", input).Msg("Failed to create playground ws session")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Could not create session", Message: err.Error()})
	}

	wsSess := &db.PlaygroundWsSession{
		PlaygroundSessionID: sess.ID,
		TargetURL:           input.TargetURL,
		RequestHeaders:      orEmpty(input.RequestHeaders, "[]"),
		Script:              orEmpty(input.Script, "[]"),
		Options:             orEmpty(input.Options, "{}"),
	}
	if err := db.Connection().CreatePlaygroundWsSession(wsSess); err != nil {
		log.Error().Err(err).Uint("session_id", sess.ID).Msg("Failed to create playground ws payload")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Could not create ws payload", Message: err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"session": sess, "ws": wsSess})
}

// GetPlaygroundWsSession godoc
// @Summary Get a playground WebSocket session by parent session ID
// @Description Get the playground WebSocket session payload along with its parent session and recent runs
// @Tags Playground
// @Accept json
// @Produce json
// @Param id path int true "Playground Session ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/ws/sessions/{id} [get]
func GetPlaygroundWsSession(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid id", Message: "The provided ID is not valid"})
	}

	sess, err := db.Connection().GetPlaygroundSession(uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Session not found"})
	}

	wsSess, err := db.Connection().GetPlaygroundWsSessionBySessionID(sess.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "WS payload missing"})
	}

	runs, _, _ := db.Connection().ListPlaygroundWsRuns(wsSess.ID, 1, 20)

	return c.JSON(fiber.Map{"session": sess, "ws": wsSess, "recent_runs": runs})
}

// UpdateWsSessionInput represents the input for updating a Playground WS Session.
type UpdateWsSessionInput struct {
	Name           *string         `json:"name"`
	TargetURL      *string         `json:"target_url"`
	RequestHeaders json.RawMessage `json:"request_headers"`
	Script         json.RawMessage `json:"script"`
	Options        json.RawMessage `json:"options"`
}

// UpdatePlaygroundWsSession godoc
// @Summary Update a playground WebSocket session
// @Description Update the playground WebSocket session payload (and optionally the parent session name)
// @Tags Playground
// @Accept json
// @Produce json
// @Param id path int true "Playground Session ID"
// @Param input body UpdateWsSessionInput true "Update Playground WS Session Input"
// @Success 200 {object} db.PlaygroundWsSession
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/ws/sessions/{id} [put]
func UpdatePlaygroundWsSession(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid id", Message: "The provided ID is not valid"})
	}

	input := new(UpdateWsSessionInput)
	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Cannot parse JSON"})
	}

	wsSess, err := db.Connection().GetPlaygroundWsSessionBySessionID(uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Not found", Message: "Playground ws session not found"})
	}

	if input.TargetURL != nil {
		wsSess.TargetURL = *input.TargetURL
	}
	if len(input.RequestHeaders) > 0 {
		wsSess.RequestHeaders = input.RequestHeaders
	}
	if len(input.Script) > 0 {
		wsSess.Script = input.Script
	}
	if len(input.Options) > 0 {
		wsSess.Options = input.Options
	}

	if err := db.Connection().UpdatePlaygroundWsSession(wsSess); err != nil {
		log.Error().Err(err).Uint("id", uint(id)).Msg("Failed to update playground ws session")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Failed to update playground ws session", Message: err.Error()})
	}

	if input.Name != nil {
		if err := db.Connection().UpdatePlaygroundSession(uint(id), &db.PlaygroundSession{Name: *input.Name}); err != nil {
			log.Error().Err(err).Uint("id", uint(id)).Msg("Failed to update playground session name")
		}
	}

	return c.JSON(wsSess)
}

// DeletePlaygroundWsSession godoc
// @Summary Delete a playground WebSocket session
// @Description Delete the playground WebSocket session and cascade-remove the associated WS payload and runs
// @Tags Playground
// @Accept json
// @Produce json
// @Param id path int true "Playground Session ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/ws/sessions/{id} [delete]
func DeletePlaygroundWsSession(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid id", Message: "The provided ID is not valid"})
	}

	// Hard-deletes the parent playground_sessions row so DB-level FK CASCADE removes
	// the playground_ws_sessions row and its playground_ws_runs.
	if err := db.Connection().DeletePlaygroundSession(uint(id)); err != nil {
		log.Error().Err(err).Uint("id", uint(id)).Msg("Failed to delete playground ws session")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Failed to delete playground ws session", Message: err.Error()})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// ImportConnectionInput represents the input for importing an existing WebSocket connection
// as a new playground WS replay session.
type ImportConnectionInput struct {
	ConnectionID uint   `json:"connection_id" validate:"required"`
	CollectionID *uint  `json:"collection_id"`
	WorkspaceID  uint   `json:"workspace_id" validate:"required"`
	Name         string `json:"name"`
}

// importMessageCap is the maximum number of WebSocket messages copied into a derived script.
const importMessageCap = 500

// ImportConnectionToPlaygroundWs godoc
// @Summary Import a WebSocket connection as a playground WS session
// @Description Derive a scripted WS replay session from a captured WebSocket connection. Each
// @Description sent text frame becomes a step; received frames between two sent frames attach a
// @Description wait_for(any) hint to the preceding step. Binary frames are skipped. Long
// @Description histories are capped at importMessageCap messages.
// @Tags Playground
// @Accept json
// @Produce json
// @Param input body ImportConnectionInput true "Import Connection Input"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/ws/sessions/import-connection [post]
func ImportConnectionToPlaygroundWs(c *fiber.Ctx) error {
	input := new(ImportConnectionInput)
	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Cannot parse JSON"})
	}
	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Validation failed", Message: err.Error()})
	}

	workspaceExists, err := db.Connection().WorkspaceExists(input.WorkspaceID)
	if err != nil {
		log.Error().Err(err).Uint("workspace_id", input.WorkspaceID).Msg("Failed to check workspace existence")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Failed to check workspace", Message: err.Error()})
	}
	if !workspaceExists {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid workspace", Message: "The provided workspace ID does not seem valid"})
	}

	conn, err := db.Connection().GetWebSocketConnection(input.ConnectionID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Connection not found"})
	}

	if conn.WorkspaceID == nil || *conn.WorkspaceID != input.WorkspaceID {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Connection does not belong to workspace"})
	}

	// Resolve or auto-create the destination collection.
	var collectionID uint
	if input.CollectionID != nil {
		coll, err := db.Connection().GetPlaygroundCollection(*input.CollectionID)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Collection not found"})
		}
		if coll.WorkspaceID != input.WorkspaceID {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid collection", Message: "The collection does not belong to the provided workspace"})
		}
		collectionID = coll.ID
	} else {
		coll, err := db.Connection().FindOrCreatePlaygroundCollection(input.WorkspaceID, "WebSocket Replays")
		if err != nil {
			log.Error().Err(err).Uint("workspace_id", input.WorkspaceID).Msg("Failed to find-or-create WebSocket Replays collection")
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
		}
		collectionID = coll.ID
	}

	// Fetch up to importMessageCap+1 messages so we can detect overflow.
	msgs, _, err := db.Connection().ListWebSocketMessages(db.WebSocketMessageFilter{
		ConnectionID: conn.ID,
		Pagination:   db.Pagination{Page: 1, PageSize: importMessageCap + 1},
	})
	if err != nil {
		log.Error().Err(err).Uint("connection_id", conn.ID).Msg("Failed to list WebSocket messages for import")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
	}

	totalMessages := len(msgs)
	skippedBinary := 0
	if totalMessages > importMessageCap {
		msgs = msgs[:importMessageCap]
	}

	// Derive script: each sent frame becomes a step. Received frames sandwiched between
	// two sent frames attach a wait_for(any) hint to the preceding step and downgrade
	// its on_timeout from "abort" to "continue".
	script := []map[string]any{}
	for _, m := range msgs {
		if m.Opcode == 2 { // binary frame
			skippedBinary++
			continue
		}
		if m.Direction == "sent" {
			script = append(script, map[string]any{
				"id":          generateID(),
				"name":        "",
				"content":     m.PayloadData,
				"opcode":      1,
				"delay_ms":    0,
				"on_timeout":  "abort",
				"on_no_match": "abort",
			})
		} else if m.Direction == "received" && len(script) > 0 {
			last := script[len(script)-1]
			if _, ok := last["wait_for"]; !ok {
				last["wait_for"] = map[string]any{
					"match_type": "any",
					"pattern":    "",
					"timeout_ms": 5000,
				}
				last["on_timeout"] = "continue"
			}
		}
	}

	scriptJSON, _ := json.Marshal(script)

	// Convert connection headers (map[string][]string JSON) to the playground
	// WS session's expected []HeaderSpec JSON shape ({key, value, enabled}).
	var headersJSON json.RawMessage = json.RawMessage("[]")
	if len(conn.RequestHeaders) > 0 {
		var raw map[string][]string
		if err := json.Unmarshal(conn.RequestHeaders, &raw); err == nil {
			type headerSpec struct {
				Key     string `json:"key"`
				Value   string `json:"value"`
				Enabled bool   `json:"enabled"`
			}
			specs := make([]headerSpec, 0, len(raw))
			for k, vs := range raw {
				for _, v := range vs {
					specs = append(specs, headerSpec{Key: k, Value: v, Enabled: true})
				}
			}
			if b, err := json.Marshal(specs); err == nil {
				headersJSON = b
			}
		} else {
			log.Warn().Err(err).Uint("connection_id", conn.ID).
				Msg("could not parse connection headers; importing with empty headers")
		}
	}

	name := input.Name
	if name == "" {
		name = deriveSessionName(conn.URL)
	}

	parent := &db.PlaygroundSession{
		Name:         name,
		Type:         db.WsManualType,
		WorkspaceID:  input.WorkspaceID,
		CollectionID: collectionID,
	}
	if err := db.Connection().CreatePlaygroundSession(parent); err != nil {
		log.Error().Err(err).Interface("input", input).Msg("Failed to create parent playground session for import")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
	}

	imported := conn.ID
	wsSess := &db.PlaygroundWsSession{
		PlaygroundSessionID:      parent.ID,
		TargetURL:                conn.URL,
		RequestHeaders:           headersJSON,
		Script:                   scriptJSON,
		Options:                  json.RawMessage(`{"connection_timeout_ms":10000,"send_timeout_ms":5000,"inter_step_delay_ms":0}`),
		ImportedFromConnectionID: &imported,
	}
	if err := db.Connection().CreatePlaygroundWsSession(wsSess); err != nil {
		log.Error().Err(err).Uint("session_id", parent.ID).Msg("Failed to create playground ws payload for import")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"session":        parent,
		"ws":             wsSess,
		"total_messages": totalMessages,
		"skipped_binary": skippedBinary,
		"capped":         totalMessages > importMessageCap,
		"cap":            importMessageCap,
	})
}

// deriveSessionName builds a default session name from a target URL plus the current time.
func deriveSessionName(url string) string {
	t := time.Now().Format("15:04")
	if len(url) > 60 {
		url = url[:60] + "…"
	}
	return url + " — " + t
}

// generateID returns a hex-encoded nanosecond timestamp suitable for tagging script steps.
func generateID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

// AppendMessagesInput represents the input for appending captured WebSocket messages
// onto an existing playground WS session script.
type AppendMessagesInput struct {
	MessageIDs []uint `json:"message_ids" validate:"required,min=1"`
}

// AppendMessagesToWsSession godoc
// @Summary Append captured WebSocket messages to a playground WS session script
// @Description Look up the given WebSocket message IDs and append each text frame as a new
// @Description step at the end of the session's script. Binary frames are silently skipped.
// @Tags Playground
// @Accept json
// @Produce json
// @Param id path int true "Playground Session ID"
// @Param input body AppendMessagesInput true "Append Messages Input"
// @Success 200 {object} db.PlaygroundWsSession
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/ws/sessions/{id}/messages-import [post]
func AppendMessagesToWsSession(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid id"})
	}

	input := new(AppendMessagesInput)
	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Cannot parse JSON"})
	}
	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Validation failed", Message: err.Error()})
	}

	wsSess, err := db.Connection().GetPlaygroundWsSessionBySessionID(uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Not found"})
	}

	parentSess, err := db.Connection().GetPlaygroundSession(uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Parent session not found"})
	}

	var script []map[string]any
	if err := json.Unmarshal(wsSess.Script, &script); err != nil {
		log.Error().Err(err).Uint("session_id", uint(id)).
			Msg("existing script is malformed; refusing to append")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Existing script is malformed",
			Message: err.Error(),
		})
	}

	crossTenantSkips := 0
	for _, mid := range input.MessageIDs {
		m, err := db.Connection().GetWebSocketMessage(mid)
		if err != nil {
			continue
		}
		if m.Opcode == 2 {
			continue
		}
		conn, err := db.Connection().GetWebSocketConnection(m.ConnectionID)
		if err != nil {
			continue
		}
		if conn.WorkspaceID == nil || *conn.WorkspaceID != parentSess.WorkspaceID {
			crossTenantSkips++
			continue
		}
		script = append(script, map[string]any{
			"id":          generateID(),
			"name":        "",
			"content":     m.PayloadData,
			"opcode":      1,
			"delay_ms":    0,
			"on_timeout":  "abort",
			"on_no_match": "abort",
		})
	}
	if crossTenantSkips > 0 {
		log.Warn().Int("count", crossTenantSkips).Uint("session_id", uint(id)).
			Msg("skipped cross-tenant messages during append")
	}

	scriptJSON, _ := json.Marshal(script)
	wsSess.Script = scriptJSON
	if err := db.Connection().UpdatePlaygroundWsSession(wsSess); err != nil {
		log.Error().Err(err).Uint("id", uint(id)).Msg("Failed to update playground ws session on messages-import")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
	}

	return c.JSON(wsSess)
}

// connectInput is a placeholder for future expansion of the connect endpoint body.
// The current connect handler uses the session's stored target URL and headers.
type connectInput struct{}

// ConnectPlaygroundWs godoc
// @Summary Open the interactive WebSocket for a playground WS session
// @Description Dial the upstream WebSocket using the session's stored target URL and headers,
// @Description and register the resulting interactive session with the in-process manager.
// @Tags Playground
// @Accept json
// @Produce json
// @Param id path int true "Playground Session ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/ws/sessions/{id}/connect [post]
func ConnectPlaygroundWs(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid id"})
	}
	wsSess, err := db.Connection().GetPlaygroundWsSessionBySessionID(uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Not found"})
	}
	mgr := wsreplay.Default()
	if existing := mgr.GetInteractive(wsSess.ID); existing != nil && existing.State() == wsreplay.StateConnected {
		return c.Status(fiber.StatusConflict).JSON(ErrorResponse{Error: "Already connected"})
	}
	var headers []wsreplay.HeaderSpec
	_ = json.Unmarshal(wsSess.RequestHeaders, &headers)
	pid := wsSess.PlaygroundSessionID
	cfg := wsreplay.SessionConfig{
		TargetURL:           wsSess.TargetURL,
		Headers:             headers,
		PlaygroundSessionID: &pid,
		Instance:            wsreplay.InteractiveInstance(),
		Persister:           wsreplay.NewDBPersister(db.Connection()),
		Events:              mgr.BroadcasterFor(wsSess.ID),
		ConnectTimeout:      10 * time.Second,
		SendTimeout:         5 * time.Second,
	}
	sess, err := mgr.OpenInteractive(context.Background(), wsSess.ID, cfg)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(ErrorResponse{Error: "Could not connect", Message: err.Error()})
	}
	return c.JSON(fiber.Map{"state": sess.State(), "websocket_connection_id": sess.ConnectionID()})
}

// DisconnectPlaygroundWs godoc
// @Summary Close the interactive WebSocket for a playground WS session
// @Description Close and unregister the interactive WebSocket session if one is currently open.
// @Description Safe to call when no interactive session exists; the manager treats it as a no-op.
// @Tags Playground
// @Accept json
// @Produce json
// @Param id path int true "Playground Session ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/ws/sessions/{id}/disconnect [post]
func DisconnectPlaygroundWs(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid id"})
	}
	wsSess, err := db.Connection().GetPlaygroundWsSessionBySessionID(uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Not found"})
	}
	wsreplay.Default().CloseInteractive(wsSess.ID)
	return c.SendStatus(fiber.StatusNoContent)
}

// SendFrameInput represents the input for sending a single frame on the interactive WebSocket.
type SendFrameInput struct {
	Opcode  int    `json:"opcode" validate:"required,oneof=1 2"`
	Content string `json:"content"`
}

// SendInteractiveFrame godoc
// @Summary Send a frame on the interactive WebSocket of a playground WS session
// @Description Queue a single text (opcode 1) or binary (opcode 2) frame on the currently open
// @Description interactive WebSocket. Returns 409 if the session is not connected.
// @Tags Playground
// @Accept json
// @Produce json
// @Param id path int true "Playground Session ID"
// @Param input body SendFrameInput true "Send Frame Input"
// @Success 202 "Accepted"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/ws/sessions/{id}/frames [post]
func SendInteractiveFrame(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid id"})
	}
	input := new(SendFrameInput)
	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Cannot parse JSON"})
	}
	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Validation failed", Message: err.Error()})
	}
	wsSess, err := db.Connection().GetPlaygroundWsSessionBySessionID(uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Not found"})
	}
	sess := wsreplay.Default().GetInteractive(wsSess.ID)
	if sess == nil {
		return c.Status(fiber.StatusConflict).JSON(ErrorResponse{Error: "Not connected"})
	}
	if err := sess.Send(input.Opcode, input.Content); err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(ErrorResponse{Error: err.Error()})
	}
	return c.SendStatus(fiber.StatusAccepted)
}
