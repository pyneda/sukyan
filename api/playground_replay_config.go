package api

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/manual"
)

// ReplayConfig is the shape of the replay config blob stored in
// PlaygroundSession.ReplayConfig and exchanged over the HTTP API.
type ReplayConfig struct {
	URL          string                `json:"url"`
	RawRequest   string                `json:"raw_request"`
	Options      manual.RequestOptions `json:"options"`
	ReplayMode   string                `json:"replay_mode"`
	PreActionID  *uint                 `json:"pre_action_id,omitempty"`
	PostActionID *uint                 `json:"post_action_id,omitempty"`
}

// GetReplayConfig godoc
// @Summary Fetch the persisted replay config for a session
// @Tags Playground
// @Param id path int true "Playground Session ID"
// @Success 200 {object} ReplayConfig
// @Success 204 "No Content (no config persisted yet)"
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/sessions/{id}/replay-config [get]
func GetReplayConfig(c *fiber.Ctx) error {
	sessID, err := c.ParamsInt("id")
	if err != nil || sessID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid session id"})
	}
	sess, err := db.Connection().GetPlaygroundSession(uint(sessID))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Session not found"})
	}
	if len(sess.ReplayConfig) == 0 {
		return c.SendStatus(fiber.StatusNoContent)
	}
	c.Set("Content-Type", "application/json")
	return c.Send(sess.ReplayConfig)
}

// PutReplayConfig godoc
// @Summary Persist the replay config for a session (autosaved by the UI)
// @Tags Playground
// @Accept json
// @Param id path int true "Playground Session ID"
// @Param input body ReplayConfig true "Replay config"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/sessions/{id}/replay-config [put]
func PutReplayConfig(c *fiber.Ctx) error {
	return upsertReplayConfig(c)
}

// FlushReplayConfig godoc
// @Summary Flush (force-save) the replay config for a session
// @Tags Playground
// @Accept json
// @Param id path int true "Playground Session ID"
// @Param input body ReplayConfig true "Replay config"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/sessions/{id}/replay-config/flush [post]
func FlushReplayConfig(c *fiber.Ctx) error {
	return upsertReplayConfig(c)
}

func upsertReplayConfig(c *fiber.Ctx) error {
	sessID, err := c.ParamsInt("id")
	if err != nil || sessID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid session id"})
	}
	// Verify the config is valid JSON before persisting. We do NOT enforce a
	// strict schema here because autosave happens before the draft is complete
	// — a half-edited replay (e.g. URL typed but raw body still empty) must
	// still persist so the UI can restore it later.
	var probe map[string]any
	body := c.Body()
	if err := json.Unmarshal(body, &probe); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid JSON", Message: err.Error()})
	}
	if err := db.Connection().UpdatePlaygroundSessionReplayConfig(uint(sessID), json.RawMessage(body)); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Session not found", Message: err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
