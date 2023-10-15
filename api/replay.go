package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/manual"
)

type PlaygroundReplayInput struct {
	Request   manual.Request        `json:"request" validate:"required"`
	Options   manual.RequestOptions `json:"options"`
	SessionID uint                  `json:"session_id" validate:"required"`
}

// ReplayRequest godoc
// @Summary Sends a request to a target
// @Description Sends a request to a target and returns the response
// @Tags Playground
// @Accept  json
// @Produce  json
// @Param input body PlaygroundReplayInput true "Set the request configuration"
// @Success 200 {object} db.History
// @Failure 400 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/replay [post]
func ReplayRequest(c *fiber.Ctx) error {
	input := new(PlaygroundReplayInput)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"message": err.Error(),
		})
	}

	session, err := db.Connection.GetPlaygroundSession(input.SessionID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid session",
			"message": "The provided session ID does not seem valid",
		})
	}

	options := manual.RequestReplayOptions{
		Request: input.Request,
		Session: *session,
		Options: input.Options,
	}
	result, err := manual.Replay(options)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "There was an error replaying the request",
			"message": err.Error(),
		})
	}

	return c.JSON(result)
}
