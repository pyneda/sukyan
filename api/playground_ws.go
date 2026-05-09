package api

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
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
