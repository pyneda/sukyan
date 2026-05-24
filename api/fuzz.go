package api

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/pyneda/sukyan/pkg/playground/stream"
	"github.com/rs/zerolog/log"
)

// PlaygroundFuzzInput is the launch payload for POST /api/v1/playground/fuzz.
// The shape must match the engine spec's data-model section.
type PlaygroundFuzzInput struct {
	URL            string                      `json:"url" validate:"required" example:"https://example.com/"`
	RawRequest     string                      `json:"raw_request" validate:"required"`
	SessionID      uint                        `json:"session_id" validate:"required,min=1"`
	Mode           fuzz.FuzzMode               `json:"mode" validate:"required,oneof=single all paired combinations"`
	Positions      []fuzz.FuzzerPosition       `json:"positions" validate:"required,min=1"`
	SharedPayloads *fuzz.FuzzerPayloadsGroup   `json:"shared_payloads,omitempty"`
	Options        fuzz.RequestOptions         `json:"options"`
	Execution      fuzz.FuzzerExecutionOptions `json:"execution"`
}

// PlaygroundFuzzResponse is the launch response.
type PlaygroundFuzzResponse struct {
	RunID         uint `json:"run_id"`
	RequestsCount int  `json:"requests_count"`
}

// PlaygroundFuzzPreviewInput is the preview payload. Same as launch input
// minus the URL/RawRequest (preview doesn't need them; only counts requests).
type PlaygroundFuzzPreviewInput struct {
	Mode           fuzz.FuzzMode             `json:"mode" validate:"required,oneof=single all paired combinations"`
	Positions      []fuzz.FuzzerPosition     `json:"positions" validate:"required,min=1"`
	SharedPayloads *fuzz.FuzzerPayloadsGroup `json:"shared_payloads,omitempty"`
}

// FuzzRequest godoc
// @Summary Launch a new fuzz run
// @Description Validates the input, snapshots the config onto a new PlaygroundFuzzRun row, and launches the engine asynchronously. Returns immediately with the run id.
// @Tags Playground
// @Accept json
// @Produce json
// @Param input body PlaygroundFuzzInput true "Fuzz launch input"
// @Success 200 {object} PlaygroundFuzzResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/fuzz [post]
func FuzzRequest(c *fiber.Ctx) error {
	input := new(PlaygroundFuzzInput)
	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Bad Request", Message: "Cannot parse JSON body"})
	}
	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Validation Failed", Message: err.Error()})
	}

	session, err := db.Connection().GetPlaygroundSession(input.SessionID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid Session", Message: "The provided session ID does not seem valid"})
	}

	// Engine-level validation (mode/payload consistency, overlap).
	if err := fuzz.Validate(input.Mode, input.Positions, input.SharedPayloads); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid fuzz config", Message: err.Error()})
	}

	resolved := fuzz.Resolve(input.Mode, input.Positions, input.SharedPayloads)
	strategy, err := fuzz.StrategyFor(input.Mode)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid mode", Message: err.Error()})
	}
	requestCount, err := strategy.RequestCount(input.Positions, resolved)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid fuzz config", Message: err.Error()})
	}

	// Snapshot the config to freeze what was actually launched.
	cfg := fuzz.FuzzerConfig{
		Mode:      input.Mode,
		Positions: input.Positions,
		Shared:    input.SharedPayloads,
		Request:   input.Options,
		Execution: input.Execution,
	}
	configRaw, _ := json.Marshal(cfg)

	// Persist the launched config onto the session as the new live config so
	// re-opening restores exactly what was last launched.
	if err := db.Connection().UpdatePlaygroundSessionFuzzerConfig(session.ID, configRaw); err != nil {
		log.Warn().Err(err).Uint("session_id", session.ID).Msg("api: persist fuzzer config on launch (continuing)")
	}

	run := &db.PlaygroundFuzzRun{
		PlaygroundSessionID: session.ID,
		WorkspaceID:         session.WorkspaceID,
		ConfigSnapshot:      configRaw,
		Status:              db.FuzzRunPending,
		PlannedRequestCount: requestCount,
	}
	if err := db.Connection().CreatePlaygroundFuzzRun(run); err != nil {
		log.Error().Err(err).Msg("api: create fuzz run")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Could not create run", Message: err.Error()})
	}

	// Per-run broadcaster + cancellation context; registered for cancel and
	// streaming endpoints to find.
	bcast := stream.NewBroadcaster(64, 1000)
	runCtx, cancel := context.WithCancel(context.Background())
	fuzz.Default().Register(run.ID, cancel, bcast)

	go runFuzzAsync(runCtx, run, input, resolved, strategy, bcast)

	return c.JSON(PlaygroundFuzzResponse{
		RunID:         run.ID,
		RequestsCount: requestCount,
	})
}

// runFuzzAsync is the background goroutine that drives one run to completion.
// It owns the run row's status transitions and the registry cleanup.
func runFuzzAsync(
	ctx context.Context,
	run *db.PlaygroundFuzzRun,
	input *PlaygroundFuzzInput,
	resolved fuzz.ResolvedPayloads,
	strategy fuzz.ModeStrategy,
	bcast *stream.Broadcaster,
) {
	// Keep the broadcaster registered after the run finishes for a grace
	// period (5 minutes) so a user who opens the just-finished run still
	// gets the buffered history of result events. Without this, fast runs
	// that complete before the UI has time to open the WS stream lose all
	// rich per-result data (payload values, word/line counts).
	const postRunGrace = 5 * time.Minute
	defer time.AfterFunc(postRunGrace, func() {
		fuzz.Default().Unregister(run.ID)
		bcast.Close()
	})

	// Calibration phase — runs before the main fuzz when AutoBaseline isn't off.
	var baseline *fuzz.RunBaseline
	if input.Execution.AutoBaseline != fuzz.AutoBaselineOff {
		now := time.Now()
		run.Status = db.FuzzRunCalibrating
		run.StartedAt = &now
		if err := db.Connection().UpdatePlaygroundFuzzRun(run); err != nil {
			log.Warn().Err(err).Uint("run_id", run.ID).Msg("api: update run to calibrating")
		}
		bcast.Publish(&fuzz.FuzzEvent{
			Type:   fuzz.FuzzEventStatus,
			RunID:  run.ID,
			At:     time.Now(),
			Status: &fuzz.FuzzStatusEv{From: string(db.FuzzRunPending), To: string(db.FuzzRunCalibrating)},
		})
		var err error
		baseline, err = fuzz.Calibrate(ctx, fuzz.CalibrateInput{
			TargetURL:             input.URL,
			RawRequest:            input.RawRequest,
			Mode:                  input.Mode,
			Positions:             input.Positions,
			ProbeCount:            input.Execution.BaselineProbeCount,
			Threshold:             input.Execution.BaselineSimhashThreshold,
			BaselineMode:          input.Execution.AutoBaseline,
			RequestTimeoutSeconds: input.Execution.RequestTimeoutSeconds,
		})
		if err != nil {
			log.Warn().Err(err).Uint("run_id", run.ID).Msg("api: calibration failed, continuing without baseline")
			baseline = nil
		} else {
			raw, _ := json.Marshal(baseline)
			run.Baseline = raw
			bcast.Publish(&fuzz.FuzzEvent{
				Type:  fuzz.FuzzEventBaseline,
				RunID: run.ID,
				At:    time.Now(),
				Baseline: &fuzz.FuzzBaselineEv{
					Fingerprints: baseline.Fingerprints,
					Warnings:     baseline.Warnings,
				},
			})
		}
	}

	// Transition pending/calibrating → running, unless a pause request
	// landed during calibration and flipped DB status to paused — in that
	// case, honor it (the engine's worker loop will block on the gate
	// regardless).
	now := time.Now()
	prevStatus := run.Status
	if fresh, err := db.Connection().GetPlaygroundFuzzRun(run.ID); err == nil && fresh.Status == db.FuzzRunPaused {
		run.Status = db.FuzzRunPaused
	} else {
		run.Status = db.FuzzRunRunning
	}
	if run.StartedAt == nil {
		run.StartedAt = &now
	}
	if err := db.Connection().UpdatePlaygroundFuzzRun(run); err != nil {
		log.Error().Err(err).Uint("run_id", run.ID).Msg("api: update run status after calibration")
	}
	bcast.Publish(&fuzz.FuzzEvent{
		Type:   fuzz.FuzzEventStatus,
		RunID:  run.ID,
		At:     time.Now(),
		Status: &fuzz.FuzzStatusEv{From: string(prevStatus), To: string(run.Status)},
	})

	hooks := fuzz.Hooks{
		UpdateProgress: func(sent, errs int) {
			run.SentRequestCount = sent
			run.ErrorCount = errs
			// Use a targeted update so a concurrent pause/resume that flipped
			// status in the DB is not overwritten by our in-memory run.Status.
			_ = db.Connection().UpdatePlaygroundFuzzRunProgress(run.ID, sent, errs)
			bcast.Publish(&fuzz.FuzzEvent{
				Type:  fuzz.FuzzEventProgress,
				RunID: run.ID,
				At:    time.Now(),
				Progress: &fuzz.FuzzProgress{
					Sent:           sent,
					Errors:         errs,
					Planned:        run.PlannedRequestCount,
					ElapsedSeconds: int(time.Since(now).Seconds()),
				},
			})
		},
	}

	outcome := fuzz.Run(ctx, fuzz.RunInput{
		RunID:               run.ID,
		WorkspaceID:         run.WorkspaceID,
		PlaygroundSessionID: run.PlaygroundSessionID,
		TargetURL:           input.URL,
		RawRequest:          input.RawRequest,
		Mode:                input.Mode,
		Positions:           input.Positions,
		Resolved:            resolved,
		Request:             input.Options,
		Execution:           input.Execution,
		Strategy:            strategy,
		Broadcaster:         bcast,
		Baseline:            baseline,
		PauseGate:           fuzz.Default().Gate(run.ID),
		Hooks:               hooks,
	})

	finishedAt := time.Now()
	// Re-read the row so a pause/resume that flipped run.Status mid-flight
	// (via the API endpoints) is not overwritten by our stale in-memory copy.
	// We only need the status — counters and timestamps are owned by us.
	if fresh, err := db.Connection().GetPlaygroundFuzzRun(run.ID); err == nil {
		run.Status = fresh.Status
	}
	preFinalStatus := run.Status
	run.Status = outcome.Status
	run.FinishedAt = &finishedAt
	run.SentRequestCount = outcome.SentCount
	run.ErrorCount = outcome.ErrorCount
	if outcome.FailureReason != "" {
		reason := outcome.FailureReason
		run.FailureReason = &reason
	}
	if err := db.Connection().UpdatePlaygroundFuzzRun(run); err != nil {
		log.Error().Err(err).Uint("run_id", run.ID).Msg("api: finalize run")
	}

	// Emit terminal events.
	bcast.Publish(&fuzz.FuzzEvent{
		Type:  fuzz.FuzzEventStatus,
		RunID: run.ID,
		At:    finishedAt,
		Status: &fuzz.FuzzStatusEv{
			From:   string(preFinalStatus),
			To:     string(outcome.Status),
			Reason: run.FailureReason,
		},
	})
	bcast.Publish(&fuzz.FuzzEvent{
		Type:  fuzz.FuzzEventDone,
		RunID: run.ID,
		At:    finishedAt,
		Done: &fuzz.FuzzDoneEv{
			FinalStatus:     string(outcome.Status),
			TotalSent:       outcome.SentCount,
			TotalErrors:     outcome.ErrorCount,
			DurationSeconds: int(outcome.Duration.Seconds()),
			Reason:          run.FailureReason,
		},
	})
}

// FuzzPreview godoc
// @Summary Compute request count + warnings for a fuzz config without launching
// @Tags Playground
// @Accept json
// @Produce json
// @Param input body PlaygroundFuzzPreviewInput true "Fuzz preview input"
// @Success 200 {object} fuzz.PreviewResult
// @Failure 400 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/fuzz/preview [post]
func FuzzPreview(c *fiber.Ctx) error {
	input := new(PlaygroundFuzzPreviewInput)
	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Bad Request", Message: "Cannot parse JSON body"})
	}
	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Validation Failed", Message: err.Error()})
	}
	res, err := fuzz.Preview(input.Mode, input.Positions, input.SharedPayloads)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid fuzz config", Message: err.Error()})
	}
	return c.JSON(res)
}

// CancelFuzzRun godoc
// @Summary Cancel an in-flight fuzz run
// @Tags Playground
// @Param run_id path int true "Fuzz Run ID"
// @Success 204 "No Content"
// @Success 200 {object} map[string]string "already_finished"
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/fuzz/runs/{run_id} [delete]
func CancelFuzzRun(c *fiber.Ctx) error {
	runID, err := c.ParamsInt("run_id")
	if err != nil || runID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid run id"})
	}
	run, err := db.Connection().GetPlaygroundFuzzRun(uint(runID))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Run not found"})
	}
	if run.Status.IsTerminal() {
		return c.JSON(fiber.Map{"status": "already_finished"})
	}
	if !fuzz.Default().Cancel(uint(runID)) {
		// Run is non-terminal but no cancel func — orphaned (process restart
		// during run; the recovery sweep should have caught it). Stamp it now
		// to avoid leaving the row in pending forever.
		now := time.Now()
		reason := "cancelled (no live engine context found)"
		run.Status = db.FuzzRunCancelled
		run.FinishedAt = &now
		run.FailureReason = &reason
		_ = db.Connection().UpdatePlaygroundFuzzRun(run)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// PauseFuzzRun godoc
// @Summary Pause an in-flight fuzz run
// @Description Stops scheduling new requests; in-flight workers complete naturally. Idempotent — calling on an already-paused run returns 200 with {"status":"already_paused"}.
// @Tags Playground
// @Param run_id path int true "Fuzz Run ID"
// @Success 204 "No Content"
// @Success 200 {object} map[string]string "already_paused"
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/fuzz/runs/{run_id}/pause [post]
func PauseFuzzRun(c *fiber.Ctx) error {
	runID, err := c.ParamsInt("run_id")
	if err != nil || runID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid run id"})
	}
	run, err := db.Connection().GetPlaygroundFuzzRun(uint(runID))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Run not found"})
	}
	if run.Status.IsTerminal() {
		return c.Status(fiber.StatusConflict).JSON(ErrorResponse{Error: "Invalid state", Message: "cannot pause a terminal run"})
	}
	if run.Status == db.FuzzRunPaused {
		return c.JSON(fiber.Map{"status": "already_paused"})
	}
	if !fuzz.Default().Pause(uint(runID)) {
		// Gate didn't flip — either run is unknown to the registry (orphan)
		// or somehow already paused at the gate. The DB check above already
		// covered the already-paused case, so treat this as 404.
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Run not found", Message: "no live engine context for this run"})
	}
	prev := run.Status
	run.Status = db.FuzzRunPaused
	if err := db.Connection().UpdatePlaygroundFuzzRun(run); err != nil {
		log.Error().Err(err).Uint("run_id", run.ID).Msg("api: persist paused status")
	}
	if bcast := fuzz.Default().Broadcaster(uint(runID)); bcast != nil {
		bcast.Publish(&fuzz.FuzzEvent{
			Type:   fuzz.FuzzEventStatus,
			RunID:  uint(runID),
			At:     time.Now(),
			Status: &fuzz.FuzzStatusEv{From: string(prev), To: string(db.FuzzRunPaused)},
		})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ResumeFuzzRun godoc
// @Summary Resume a paused fuzz run
// @Description Re-opens the pause gate so workers resume scheduling. Idempotent — calling on a non-paused run returns 200 with {"status":"not_paused"}.
// @Tags Playground
// @Param run_id path int true "Fuzz Run ID"
// @Success 204 "No Content"
// @Success 200 {object} map[string]string "not_paused"
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/fuzz/runs/{run_id}/resume [post]
func ResumeFuzzRun(c *fiber.Ctx) error {
	runID, err := c.ParamsInt("run_id")
	if err != nil || runID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid run id"})
	}
	run, err := db.Connection().GetPlaygroundFuzzRun(uint(runID))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Run not found"})
	}
	if run.Status.IsTerminal() {
		return c.Status(fiber.StatusConflict).JSON(ErrorResponse{Error: "Invalid state", Message: "cannot resume a terminal run"})
	}
	if run.Status != db.FuzzRunPaused {
		return c.JSON(fiber.Map{"status": "not_paused"})
	}
	if !fuzz.Default().Resume(uint(runID)) {
		// DB says paused but no live gate — orphan. Mark it cancelled so the
		// row doesn't linger; the user can launch a fresh run.
		now := time.Now()
		reason := "resume requested but no live engine context found"
		run.Status = db.FuzzRunCancelled
		run.FinishedAt = &now
		run.FailureReason = &reason
		_ = db.Connection().UpdatePlaygroundFuzzRun(run)
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Run not found", Message: "no live engine context for this run"})
	}
	run.Status = db.FuzzRunRunning
	if err := db.Connection().UpdatePlaygroundFuzzRun(run); err != nil {
		log.Error().Err(err).Uint("run_id", run.ID).Msg("api: persist running status on resume")
	}
	if bcast := fuzz.Default().Broadcaster(uint(runID)); bcast != nil {
		bcast.Publish(&fuzz.FuzzEvent{
			Type:   fuzz.FuzzEventStatus,
			RunID:  uint(runID),
			At:     time.Now(),
			Status: &fuzz.FuzzStatusEv{From: string(db.FuzzRunPaused), To: string(db.FuzzRunRunning)},
		})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// GetFuzzRun godoc
// @Summary Fetch a single fuzz run by ID
// @Tags Playground
// @Param run_id path int true "Fuzz Run ID"
// @Success 200 {object} db.PlaygroundFuzzRun
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/fuzz/runs/{run_id} [get]
func GetFuzzRun(c *fiber.Ctx) error {
	runID, err := c.ParamsInt("run_id")
	if err != nil || runID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid run id"})
	}
	run, err := db.Connection().GetPlaygroundFuzzRun(uint(runID))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Run not found"})
	}
	return c.JSON(run)
}

// GetFuzzerConfig godoc
// @Summary Fetch the persisted fuzzer config for a session
// @Tags Playground
// @Param id path int true "Playground Session ID"
// @Success 200 {object} fuzz.FuzzerConfig
// @Success 204 "No Content (no config persisted yet)"
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/sessions/{id}/fuzzer-config [get]
func GetFuzzerConfig(c *fiber.Ctx) error {
	sessID, err := c.ParamsInt("id")
	if err != nil || sessID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid session id"})
	}
	sess, err := db.Connection().GetPlaygroundSession(uint(sessID))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Session not found"})
	}
	if len(sess.FuzzerConfig) == 0 {
		return c.SendStatus(fiber.StatusNoContent)
	}
	c.Set("Content-Type", "application/json")
	return c.Send(sess.FuzzerConfig)
}

// PutFuzzerConfig godoc
// @Summary Persist the fuzzer config for a session (autosaved by the UI)
// @Tags Playground
// @Accept json
// @Param id path int true "Playground Session ID"
// @Param input body fuzz.FuzzerConfig true "Fuzzer config"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/sessions/{id}/fuzzer-config [put]
func PutFuzzerConfig(c *fiber.Ctx) error {
	sessID, err := c.ParamsInt("id")
	if err != nil || sessID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid session id"})
	}
	// Verify the config is valid JSON before persisting. We do NOT enforce
	// fuzz.Validate here because autosave happens before the config is
	// complete — a half-edited config (e.g. mode chosen but no payloads yet)
	// must still persist so the UI can restore it later.
	var probe map[string]any
	body := c.Body()
	if err := json.Unmarshal(body, &probe); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid JSON", Message: err.Error()})
	}
	if err := db.Connection().UpdatePlaygroundSessionFuzzerConfig(uint(sessID), json.RawMessage(body)); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Session not found", Message: err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ListFuzzRunsForSession godoc
// @Summary List fuzz runs for a session, newest first
// @Tags Playground
// @Param id path int true "Playground Session ID"
// @Param page query int false "Page (1-based)"
// @Param page_size query int false "Page size"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/sessions/{id}/fuzz-runs [get]
func ListFuzzRunsForSession(c *fiber.Ctx) error {
	sessID, err := c.ParamsInt("id")
	if err != nil || sessID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid session id"})
	}
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	runs, total, err := db.Connection().ListPlaygroundFuzzRuns(uint(sessID), page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Could not list runs", Message: err.Error()})
	}
	return c.JSON(fiber.Map{
		"runs":      runs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// FlushFuzzerConfig is the navigator.sendBeacon target for HTTP fuzz autosave.
// Identical semantics to PutFuzzerConfig but POST so the Beacon API can reach
// it (Beacon is POST-only).
// @Router /api/v1/playground/sessions/{id}/fuzzer-config/flush [post]
func FlushFuzzerConfig(c *fiber.Ctx) error {
	return PutFuzzerConfig(c)
}
