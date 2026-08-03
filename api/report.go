package api

import (
	"bytes"
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/report"
	"github.com/rs/zerolog/log"
)

// ReportRequest represents the structure of the JSON payload for generating a report.
type ReportRequest struct {
	WorkspaceID    uint                `json:"workspace_id" validate:"required"`
	TaskID         uint                `json:"task_id"`
	ScanID         uint                `json:"scan_id"`
	Title          string              `json:"title" validate:"required"`
	Format         report.ReportFormat `json:"format" validate:"required,oneof=html json pdf"`
	MinConfidence  int                 `json:"min_confidence" validate:"omitempty"`
	MaxRequestSize int                 `json:"max_request_size" validate:"omitempty"`
	// Omitting this highlights, matching the UI. See report.ReportOptions.
	DisableSyntaxHighlight bool `json:"disable_syntax_highlight" validate:"omitempty"`
}

// reportContentType maps a format onto how the browser should receive it.
func reportContentType(format report.ReportFormat) (contentType, extension string) {
	switch format {
	case report.ReportFormatJSON:
		return "application/json", "json"
	case report.ReportFormatPDF:
		return "application/pdf", "pdf"
	default:
		return "text/html", "html"
	}
}

// ReportHandler godoc
// @Summary Generate a report
// @Description Generates a report for a given workspace
// @Tags Reports
// @Accept  json
// @Produce  json
// @Param report body ReportRequest true "Report request"
// @Success 200 {object} string
// @Failure 400 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/report [post]
func ReportHandler(c *fiber.Ctx) error {
	input := new(ReportRequest)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	validate := validator.New()
	if err := validate.Struct(input); err != nil {
		errors := make(map[string]string)
		for _, err := range err.(validator.ValidationErrors) {
			errors[err.Field()] = fmt.Sprintf("Invalid value for %s", err.Field())
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"message": errors,
		})
	}

	workspaceExists, _ := db.Connection().WorkspaceExists(input.WorkspaceID)
	if !workspaceExists {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}

	issues, _, err := db.Connection().ListIssues(db.IssueFilter{
		WorkspaceID:         input.WorkspaceID,
		MinConfidence:       input.MinConfidence,
		TaskID:              input.TaskID,
		ScanID:              input.ScanID,
		IncludeInteractions: true,
	})

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Error fetching issues",
			"message": "There has been an error fetching issues to generate report",
		})
	}

	options := report.ReportOptions{
		WorkspaceID:    input.WorkspaceID,
		Issues:         issues,
		Title:          input.Title,
		Format:         input.Format,
		MaxRequestSize: input.MaxRequestSize,
		TaskID:         input.TaskID,
		ScanID:         input.ScanID,
		GeneratedAt:    time.Now(),

		DisableSyntaxHighlight: input.DisableSyntaxHighlight,
	}

	// Create a buffer to temporarily hold the generated report
	var buf bytes.Buffer

	// Generate the report
	if err := report.GenerateReport(options, &buf); err != nil {
		log.Error().Err(err).Msg("Failed to generate report")
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate report")
	}

	contentType, fileExtension := reportContentType(input.Format)
	c.Response().Header.Set(fiber.HeaderContentType, contentType)

	// Make the file downloadable
	filename := "report." + fileExtension
	c.Response().Header.Set("Content-Disposition", "attachment; filename="+filename)

	// Buffered rather than streamed so a mid-render failure cannot emit a 200
	// with a truncated report. SetBodyRaw avoids re-copying it.
	c.Response().SetBodyRaw(buf.Bytes())
	return nil
}
