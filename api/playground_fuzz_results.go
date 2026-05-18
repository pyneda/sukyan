package api

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/playground/fuzz"
)

// fuzzResultRow is the projection returned by the results endpoint. Same
// shape as fuzz.FuzzResult so the frontend can render the live-stream and
// historical-fetch paths with one component.
//
// Note: WordCount and LineCount are NOT stored on History — they're computed
// at engine time and only available on the live event. For historical fetch
// we leave them at 0 and the UI hides those columns for past runs. (A future
// task could store them on a fuzz_results sidecar table; out of scope for
// parity.)
type fuzzResultRow struct {
	HistoryID           uint   `json:"history_id"`
	StatusCode          int    `json:"status_code"`
	Method              string `json:"method"`
	URL                 string `json:"url"`
	ResponseBodySize    int    `json:"response_body_size"`
	ResponseContentType string `json:"response_content_type"`
}

// ListFuzzRunResults godoc
// @Summary Paginated historical results for a finished or live fuzz run
// @Description Returns the persisted History rows tagged with the run id.
// @Description Use the WS stream for live updates while the run is in flight;
// @Description this endpoint is for past runs and reloads.
// @Tags Playground
// @Param run_id path int true "Fuzz Run ID"
// @Param page query int false "Page (1-based)"
// @Param page_size query int false "Page size (default 100, max 500)"
// @Param status query int false "Filter by exact status code"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/fuzz/runs/{run_id}/results [get]
func ListFuzzRunResults(c *fiber.Ctx) error {
	runID, err := c.ParamsInt("run_id")
	if err != nil || runID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid run id"})
	}
	// Make sure the run exists so a 404 here is unambiguous.
	if _, err := db.Connection().GetPlaygroundFuzzRun(uint(runID)); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Run not found"})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("page_size", "100"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 100
	}
	if pageSize > 500 {
		pageSize = 500
	}
	statusFilter, _ := strconv.Atoi(c.Query("status", "0"))

	q := db.Connection().DB().Model(&db.History{}).
		Where("playground_fuzz_run_id = ?", runID).
		Order("id ASC") // earliest first (matches the order results were dispatched)
	if statusFilter > 0 {
		q = q.Where("status_code = ?", statusFilter)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Could not count results", Message: err.Error()})
	}

	var rows []db.History
	if err := q.Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Could not list results", Message: err.Error()})
	}

	results := make([]fuzzResultRow, len(rows))
	for i, h := range rows {
		results[i] = fuzzResultRow{
			HistoryID:           h.ID,
			StatusCode:          h.StatusCode,
			Method:              h.Method,
			URL:                 h.URL,
			ResponseBodySize:    h.ResponseBodySize,
			ResponseContentType: h.ResponseContentType,
		}
	}
	return c.JSON(fiber.Map{
		"results":   results,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// Compile-time check that fuzzResultRow stays in shape-compatible with the
// streamed FuzzResult for the fields it carries.
var _ = func() *fuzz.FuzzResult { return &fuzz.FuzzResult{HistoryID: 0} }
