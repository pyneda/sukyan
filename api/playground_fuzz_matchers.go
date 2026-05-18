package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/playground/fuzz"
)

// GetFuzzRunMatchers godoc
// @Summary Fetch persisted matchers for a fuzz run
// @Tags Playground
// @Param run_id path int true "Fuzz Run ID"
// @Success 200 {object} fuzz.MatcherSet
// @Success 204 "No matchers persisted"
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/fuzz/runs/{run_id}/matchers [get]
func GetFuzzRunMatchers(c *fiber.Ctx) error {
	runID, err := c.ParamsInt("run_id")
	if err != nil || runID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid run id"})
	}
	run, err := db.Connection().GetPlaygroundFuzzRun(uint(runID))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Run not found"})
	}
	if len(run.Matchers) == 0 {
		return c.SendStatus(fiber.StatusNoContent)
	}
	c.Set("Content-Type", "application/json")
	return c.Send(run.Matchers)
}

// PutFuzzRunMatchers godoc
// @Summary Persist matcher set on a fuzz run
// @Tags Playground
// @Accept json
// @Param run_id path int true "Fuzz Run ID"
// @Param input body fuzz.MatcherSet true "Matcher set"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/fuzz/runs/{run_id}/matchers [put]
func PutFuzzRunMatchers(c *fiber.Ctx) error {
	runID, err := c.ParamsInt("run_id")
	if err != nil || runID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid run id"})
	}
	var set fuzz.MatcherSet
	if err := json.Unmarshal(c.Body(), &set); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid JSON", Message: err.Error()})
	}
	if err := fuzz.ValidateSet(set); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid matcher set", Message: err.Error()})
	}
	run, err := db.Connection().GetPlaygroundFuzzRun(uint(runID))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Run not found"})
	}
	run.Matchers = json.RawMessage(c.Body())
	if err := db.Connection().UpdatePlaygroundFuzzRun(run); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Could not save matchers", Message: err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// MatchFuzzRunInput is the body for the server-side matcher evaluation
// endpoint. Caller passes a matcher set + optional history-id filter (so the
// UI can ask "of these visible rows, which pass these body matchers?"
// without re-fetching the whole run).
type MatchFuzzRunInput struct {
	Matchers       fuzz.MatcherSet `json:"matchers"`
	HistoryIDs     []uint          `json:"history_ids,omitempty"`
	IncludeBodies  bool            `json:"include_bodies,omitempty"`
}

// MatchFuzzRunResponse is the body of the response: just the history ids
// that pass the server-side rules.
type MatchFuzzRunResponse struct {
	MatchingHistoryIDs []uint `json:"matching_history_ids"`
	Total              int    `json:"total"`
}

// MatchFuzzRun godoc
// @Summary Evaluate body/header matchers against a fuzz run's persisted results
// @Description Server-side companion to the client's fast-matcher evaluation.
// @Description Returns the history IDs of rows that pass all body/header rules
// @Description in the supplied matcher set. Client-side rules (status, size,
// @Description etc.) are ignored here — the UI applies them locally.
// @Tags Playground
// @Accept json
// @Produce json
// @Param run_id path int true "Fuzz Run ID"
// @Param input body MatchFuzzRunInput true "Matcher input"
// @Success 200 {object} MatchFuzzRunResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/fuzz/runs/{run_id}/match [post]
func MatchFuzzRun(c *fiber.Ctx) error {
	runID, err := c.ParamsInt("run_id")
	if err != nil || runID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid run id"})
	}
	if _, err := db.Connection().GetPlaygroundFuzzRun(uint(runID)); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Run not found"})
	}
	var input MatchFuzzRunInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid JSON", Message: err.Error()})
	}
	if err := fuzz.ValidateSet(input.Matchers); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid matcher set", Message: err.Error()})
	}

	// Fast path: if there are no server-side rules, return immediately.
	anyServerSide := false
	for _, r := range input.Matchers.Rules {
		if r.IsServerSide() {
			anyServerSide = true
			break
		}
	}
	if !anyServerSide {
		// Caller asked us to filter, but supplied no server-side rules. Just
		// return the input ids (or nothing if no ids passed).
		return c.JSON(MatchFuzzRunResponse{MatchingHistoryIDs: input.HistoryIDs, Total: len(input.HistoryIDs)})
	}

	q := db.Connection().DB().Model(&db.History{}).
		Where("playground_fuzz_run_id = ?", runID)
	if len(input.HistoryIDs) > 0 {
		q = q.Where("id IN ?", input.HistoryIDs)
	}

	var rows []db.History
	if err := q.Find(&rows).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Could not load history", Message: err.Error()})
	}

	matching := make([]uint, 0, len(rows))
	for _, row := range rows {
		body := extractBody(row.RawResponse)
		headers := extractHeaders(row.RawResponse)
		ok, err := input.Matchers.EvalServerSide(body, headers)
		if err != nil {
			// Bad regex etc — surface to caller so the UI can flag it.
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Matcher evaluation failed", Message: err.Error()})
		}
		if ok {
			matching = append(matching, row.ID)
		}
	}
	return c.JSON(MatchFuzzRunResponse{MatchingHistoryIDs: matching, Total: len(matching)})
}

// extractBody splits the response body from a raw HTTP response blob.
// Tolerates both CRLF and LF separators. Returns nil if there's no body.
func extractBody(raw []byte) []byte {
	if idx := strings.Index(string(raw), "\r\n\r\n"); idx >= 0 {
		return raw[idx+4:]
	}
	if idx := strings.Index(string(raw), "\n\n"); idx >= 0 {
		return raw[idx+2:]
	}
	return nil
}

// extractHeaders returns the header block as a "Key: Value\r\n..." string.
func extractHeaders(raw []byte) string {
	if idx := strings.Index(string(raw), "\r\n\r\n"); idx >= 0 {
		return string(raw[:idx])
	}
	if idx := strings.Index(string(raw), "\n\n"); idx >= 0 {
		return string(raw[:idx])
	}
	return string(raw)
}

// silence linter when imports are otherwise tree-shaken in some builds.
var _ = fmt.Sprintf
var _ = http.StatusOK
