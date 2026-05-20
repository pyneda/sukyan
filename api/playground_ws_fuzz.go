package api

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/playground/wsfuzz"
	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
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

// scheduleWsFuzzResponse is the JSON returned by POST .../sessions/:id/runs.
type scheduleWsFuzzResponse struct {
	RunID          uint `json:"run_id"`
	IterationCount int  `json:"iteration_count"`
}

// ScheduleWsFuzzRun snapshots the config, creates a pending run row, and kicks
// off the engine in a background goroutine. Returns the new run ID + planned
// iteration count immediately; clients subscribe to the live stream for
// progress.
func ScheduleWsFuzzRun(c *fiber.Ctx) error {
	sessionID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "invalid session id"})
	}
	var cfg wsfuzz.WsFuzzerConfig
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "invalid body: " + err.Error()})
	}

	// Block obvious misconfigs at launch time; preview validation already
	// surfaces warnings — only hard errors should prevent launch.
	if _, errs := wsfuzz.Validate(cfg); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"errors": errs})
	}

	iters, _, _, _ := wsfuzz.Preview(cfg)
	conn := db.Connection()
	snapshot, _ := json.Marshal(cfg)
	run := &db.PlaygroundWsFuzzRun{
		SessionID:      uint(sessionID),
		ConfigSnapshot: datatypes.JSON(snapshot),
		Status:         "pending",
		IterationCount: iters,
	}
	if err := conn.CreatePlaygroundWsFuzzRun(run); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
	}

	startWsFuzzRun(run.ID, cfg, conn)

	return c.JSON(scheduleWsFuzzResponse{RunID: run.ID, IterationCount: iters})
}

// startWsFuzzRun launches the engine in a background goroutine. Extracted so
// tests can mock it if needed; in production it's invoked from
// ScheduleWsFuzzRun.
func startWsFuzzRun(runID uint, cfg wsfuzz.WsFuzzerConfig, conn *db.DatabaseConnection) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Interface("panic", r).Uint("run_id", runID).Msg("wsfuzz engine panicked")
			}
		}()
		bcast := wsFuzzBroadcastersDefault.Acquire(runID)
		defer wsFuzzBroadcastersDefault.Release(runID)

		dial := func(ctx context.Context, dc wsreplay.SessionConfig) (wsfuzz.SessionHandle, error) {
			dc.Persister = wsreplay.NewDBPersister(conn)
			s, err := wsreplay.DialSession(ctx, dc)
			if err != nil {
				return nil, err
			}
			return wsfuzz.WrapSession(s), nil
		}

		if err := wsfuzz.Run(context.Background(), runID, cfg, wsfuzz.EngineDeps{
			Persister:   newDBRunPersister(conn),
			Broadcaster: bcast,
			Dial:        dial,
		}); err != nil {
			log.Warn().Err(err).Uint("run_id", runID).Msg("wsfuzz run terminated with error")
		}
	}()
}
