package api

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"gorm.io/gorm"
)

// ExportWsFuzzRunCSV streams the iteration rows for a run as CSV.
// Optional query params:
//   - findings_only=true → only iterations with status="check_failed"
func ExportWsFuzzRunCSV(c *fiber.Ctx) error {
	runID, err := c.ParamsInt("run_id")
	if err != nil || runID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "invalid run id"})
	}
	conn := db.Connection()
	if _, err := conn.GetPlaygroundWsFuzzRun(uint(runID)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "run not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
	}

	f := db.PlaygroundWsFuzzIterationFilter{RunID: uint(runID), Page: 1, PageSize: 100000}
	if c.Query("findings_only") == "true" {
		f.Statuses = []string{"check_failed"}
	}
	rows, _, err := conn.ListPlaygroundWsFuzzIterations(f)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
	}

	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", "attachment; filename=ws-fuzz-run-"+strconv.Itoa(runID)+".csv")

	w := csv.NewWriter(c.Response().BodyWriter())
	if err := w.Write([]string{
		"iteration", "status", "baseline_match", "duration_ms", "handshake_status",
		"peer_close_code", "payload_values_json", "failed_step_index", "failure_reason",
	}); err != nil {
		return err
	}
	for _, it := range rows {
		peerCode := ""
		if it.PeerCloseCode != nil {
			peerCode = strconv.Itoa(*it.PeerCloseCode)
		}
		failedStep := ""
		if it.FailedStepIndex != nil {
			failedStep = strconv.Itoa(*it.FailedStepIndex)
		}
		_ = w.Write([]string{
			strconv.Itoa(it.IterationIndex),
			it.Status,
			strconv.FormatBool(it.BaselineMatch),
			strconv.Itoa(it.DurationMs),
			strconv.Itoa(it.HandshakeStatusCode),
			peerCode,
			string(it.PayloadValues),
			failedStep,
			it.FailureReason,
		})
	}
	w.Flush()
	return nil
}

// ExportWsFuzzRunJSON streams the run + iterations (and optionally frames) as JSON.
// Optional query params:
//   - findings_only=true → only iterations with status="check_failed"
//   - include_frames=true → also include the persisted frame messages per iteration
func ExportWsFuzzRunJSON(c *fiber.Ctx) error {
	runID, err := c.ParamsInt("run_id")
	if err != nil || runID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "invalid run id"})
	}
	conn := db.Connection()
	run, err := conn.GetPlaygroundWsFuzzRun(uint(runID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "run not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
	}

	f := db.PlaygroundWsFuzzIterationFilter{RunID: uint(runID), Page: 1, PageSize: 100000}
	if c.Query("findings_only") == "true" {
		f.Statuses = []string{"check_failed"}
	}
	rows, _, err := conn.ListPlaygroundWsFuzzIterations(f)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
	}

	out := fiber.Map{"run": run, "iterations": rows}
	if c.Query("include_frames") == "true" {
		frameMap := map[int][]db.WebSocketMessage{}
		for _, it := range rows {
			if it.WebSocketConnectionID == nil {
				continue
			}
			var msgs []db.WebSocketMessage
			if err := conn.DB().Where("connection_id = ?", *it.WebSocketConnectionID).Order("created_at asc").Find(&msgs).Error; err != nil {
				continue
			}
			frameMap[it.IterationIndex] = msgs
		}
		out["frames"] = frameMap
	}

	c.Set("Content-Type", "application/json")
	c.Set("Content-Disposition", "attachment; filename=ws-fuzz-run-"+strconv.Itoa(runID)+".json")
	body, _ := json.Marshal(out)
	return c.Send(body)
}
