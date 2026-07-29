package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/manual"
	"github.com/rs/zerolog/log"
)

type BrowserReplayActionsInput struct {
	PreRequestActionID  *uint `json:"pre_request_action_id" validate:"omitempty"`
	PostRequestActionID *uint `json:"post_request_action_id" validate:"omitempty"`
}

type PlaygroundReplayInput struct {
	Mode           string                    `json:"mode" validate:"required,oneof=raw browser"`
	Request        manual.Request            `json:"request" validate:"required"`
	Options        manual.RequestOptions     `json:"options"`
	BrowserActions BrowserReplayActionsInput `json:"browser_actions" validate:"omitempty"`
	SessionID      uint                      `json:"session_id" validate:"required"`
}

// ReplayErrorResponse is returned when a replay could not complete. It carries
// any browser action results collected before the failure so the client can
// show which step broke rather than only a message.
type ReplayErrorResponse struct {
	Error                 string                              `json:"error"`
	Message               string                              `json:"message"`
	BrowserActionsResults *manual.BrowserReplayActionsResults `json:"browser_actions_results,omitempty"`
}

// ReplayRequest godoc
// @Summary Sends a request to a target
// @Description Sends a request to a target and returns the response
// @Tags Playground
// @Accept  json
// @Produce  json
// @Param input body PlaygroundReplayInput true "Set the request configuration"
// @Success 200 {object} manual.ReplayResult
// @Failure 400 {object} ReplayErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/replay [post]
func ReplayRequest(c *fiber.Ctx) error {
	input := new(PlaygroundReplayInput)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Cannot parse JSON body",
		})
	}

	if err := validate.Struct(input); err != nil {
		log.Error().Err(err).Msg("Invalid playground replay input")
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid input",
			Message: "The provided input is not valid",
		})
	}

	session, err := db.Connection().GetPlaygroundSession(input.SessionID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid session",
			Message: "The provided session ID does not seem valid",
		})
	}

	browserActions := manual.BrowserReplayActions{}
	log.Debug().Interface("actions", input.BrowserActions).Msg("Replay request browser actions")

	if input.Mode == "browser" && input.BrowserActions.PreRequestActionID != nil {
		pre, err := db.Connection().GetStoredBrowserActionsByID(*input.BrowserActions.PreRequestActionID)
		log.Info().Interface("actions", pre).Uint("id", pre.ID).Msg("Pre replay request actions")
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Invalid pre-request action",
				Message: "The provided pre-request action ID does not seem valid",
			})
		}
		browserActions.PreRequestAction = pre
	}

	if input.Mode == "browser" && input.BrowserActions.PostRequestActionID != nil {
		post, err := db.Connection().GetStoredBrowserActionsByID(*input.BrowserActions.PostRequestActionID)
		log.Info().Interface("actions", post).Uint("id", post.ID).Msg("Post replay request actions")
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Invalid post-request action",
				Message: "The provided post-request action ID does not seem valid",
			})
		}
		browserActions.PostRequestAction = post
	}

	options := manual.RequestReplayOptions{
		Mode:           input.Mode,
		Request:        input.Request,
		Session:        *session,
		BrowserActions: browserActions,
		Options:        input.Options,
	}
	result, err := manual.Replay(options)
	if err != nil {
		log.Error().Err(err).Msg("Error replaying request")
		response := ReplayErrorResponse{
			Error:   "Request Replay Failed",
			Message: err.Error(),
		}
		if result.BrowserActionsResults.PreRequest != nil || result.BrowserActionsResults.PostRequest != nil {
			response.BrowserActionsResults = &result.BrowserActionsResults
		}
		return c.Status(fiber.StatusBadRequest).JSON(response)
	}

	return c.JSON(result)
}
