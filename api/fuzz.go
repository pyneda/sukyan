package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/manual"
	"github.com/rs/zerolog/log"
)

type PlaygroundFuzzInput struct {
	URL             string                        `json:"url" validate:"required" example:"https://example.com/"`
	RawRequest      string                        `json:"raw_request" validate:"required" example:"GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"`
	InsertionPoints []manual.FuzzerInsertionPoint `json:"insertion_points" validate:"required"`
	SessionID       uint                          `json:"session_id" validate:"required"`
	Options         manual.RequestOptions         `json:"options"`
}

type PlaygroundFuzzResponse struct {
	Message string `json:"message"`
	TaskID  uint   `json:"task_id"`
}

// FuzzRequest godoc
// @Summary Schedules a new task to fuzz the provided request
// @Description Schedules a new task to fuzz the provided request with the provided insertion points, payloads, etc and returns the task ID to filter the results
// @Tags Playground
// @Accept  json
// @Produce  json
// @Param input body PlaygroundFuzzInput true "Set the fuzzing request configuration"
// @Success 200 {string} PlaygroundFuzzResponse
// @Failure 400 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/fuzz [post]
func FuzzRequest(c *fiber.Ctx) error {
	input := new(PlaygroundFuzzInput)

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

	fuzzOptions := manual.RequestFuzzOptions{
		URL:             input.URL,
		Raw:             input.RawRequest,
		InsertionPoints: input.InsertionPoints,
		Session:         *session,
		Options:         input.Options,
	}
	task, err := db.Connection.NewTask(session.WorkspaceID, "Fuzzing", db.TaskStatusPending)
	if err != nil {
		log.Error().Err(err).Interface("task", task).Msg("Task creation failed")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "There was an error initiating fuzzing",
			"message": "Cannot create a new task",
		})
	}
	err = manual.Fuzz(fuzzOptions, task.ID)
	if err != nil {
		log.Error().Err(err).Interface("options", fuzzOptions).Msg("Failed to initiate playground fuzzing")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "There was an error initiating fuzzing",
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Fuzzing initiated successfully",
		"task_id": task.ID,
	})

}
