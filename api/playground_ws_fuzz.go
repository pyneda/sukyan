package api

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/playground/wsfuzz"
	"gorm.io/gorm"
)

// GetWsFuzzerConfig godoc
// @Summary Fetch the persisted WsFuzzerConfig for a session
// @Tags Playground
// @Param id path int true "Playground Session ID"
// @Success 200 {object} wsfuzz.WsFuzzerConfig
// @Success 204 "No Content (no config persisted yet)"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/sessions/{id}/ws-fuzzer-config [get]
func GetWsFuzzerConfig(c *fiber.Ctx) error {
	sessionID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "invalid session id"})
	}
	conn := db.Connection()
	ws, err := conn.GetPlaygroundWsSessionBySessionID(uint(sessionID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "session not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
	}
	if len(ws.Options) == 0 || string(ws.Options) == "null" {
		return c.SendStatus(fiber.StatusNoContent)
	}
	c.Set("Content-Type", "application/json")
	return c.Send(ws.Options)
}

// PutWsFuzzerConfig godoc
// @Summary Persist the WsFuzzerConfig for a session (autosaved by the UI)
// @Tags Playground
// @Accept json
// @Param id path int true "Playground Session ID"
// @Param input body wsfuzz.WsFuzzerConfig true "Fuzzer config"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/sessions/{id}/ws-fuzzer-config [put]
func PutWsFuzzerConfig(c *fiber.Ctx) error {
	sessionID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "invalid session id"})
	}
	var cfg wsfuzz.WsFuzzerConfig
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "invalid body: " + err.Error()})
	}
	conn := db.Connection()
	ws, err := conn.GetPlaygroundWsSessionBySessionID(uint(sessionID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "session not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
	}
	body, mErr := json.Marshal(cfg)
	if mErr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "marshal: " + mErr.Error()})
	}
	ws.Options = body
	if err := conn.UpdatePlaygroundWsSession(ws); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// previewWsFuzzResponse is the JSON returned by POST /playground/ws-fuzz/preview.
type previewWsFuzzResponse struct {
	IterationCount int      `json:"iteration_count"`
	PositionsCount int      `json:"positions_count"`
	Warnings       []string `json:"warnings"`
	Errors         []string `json:"errors"`
}

// PreviewWsFuzz computes the planned iteration count, position count, and
// surfaces validator warnings/errors without launching anything. Always
// returns 200 with a structured body — the caller inspects Errors to decide
// whether to display them.
func PreviewWsFuzz(c *fiber.Ctx) error {
	var cfg wsfuzz.WsFuzzerConfig
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "invalid body: " + err.Error()})
	}
	iters, pos, warns, errs := wsfuzz.Preview(cfg)
	if warns == nil {
		warns = []string{}
	}
	if errs == nil {
		errs = []string{}
	}
	return c.JSON(previewWsFuzzResponse{
		IterationCount: iters,
		PositionsCount: pos,
		Warnings:       warns,
		Errors:         errs,
	})
}
