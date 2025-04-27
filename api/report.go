package api

import (
	"bytes"
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/report"
	"github.com/rs/zerolog/log"
)

// ReportRequest represents the structure of the JSON payload for generating a report.
type ReportRequest struct {
	WorkspaceID   uint                `json:"workspace_id" validate:"required"`
	Title         string              `json:"title" validate:"required"`
	Format        report.ReportFormat `json:"format" validate:"required,oneof=html json"`
	MinConfidence int                 `json:"min_confidence" validate:"omitempty"`
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
		WorkspaceID:   input.WorkspaceID,
		MinConfidence: input.MinConfidence,
	})

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Error fetching issues",
			"message": "There has been an error fetching issues to generate report",
		})
	}

	options := report.ReportOptions{
		WorkspaceID: input.WorkspaceID,
		Issues:      issues,
		Title:       input.Title,
		Format:      input.Format,
	}

	// Create a buffer to temporarily hold the generated report
	var buf bytes.Buffer

	// Generate the report
	if err := report.GenerateReport(options, &buf); err != nil {
		log.Error().Err(err).Msg("Failed to generate report")
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate report")
	}

	// Set the content type based on the report format
	contentType := "text/html"
	fileExtension := "html"
	if input.Format == report.ReportFormatJSON {
		contentType = "application/json"
		fileExtension = "json"
	}
	c.Response().Header.Set(fiber.HeaderContentType, contentType)

	// Make the file downloadable
	filename := "report." + fileExtension
	c.Response().Header.Set("Content-Disposition", "attachment; filename="+filename)

	return c.Send(buf.Bytes())
}
