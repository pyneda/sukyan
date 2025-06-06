package api

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"gorm.io/gorm"

	"net/http"

	"github.com/rs/zerolog/log"
)

// FindIssues godoc
// @Summary List all issues
// @Description Retrieves all issues with a count
// @Tags Issues
// @Accept  json
// @Produce  json
// @Param workspace query int true "Workspace ID"
// @Param task query int false "Task ID"
// @Param taskjob query int false "Task Job ID"
// @Param codes query string false "Comma-separated list of issue codes to filter by"
// @Success 200 {array} db.Issue
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/issues [get]
func FindIssues(c *fiber.Ctx) error {
	workspaceID, err := parseWorkspaceID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}

	taskID, err := parseTaskID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid task",
			"message": "The provided task ID does not seem valid",
		})
	}

	taskJobID, err := parseTaskJobID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid task job",
			"message": "The provided task job ID does not seem valid",
		})
	}

	unparsedIssueCodes := c.Query("codes")
	var issueCodes []string
	if unparsedIssueCodes != "" {
		// TODO: Validate issue codes
		issueCodes = strings.Split(unparsedIssueCodes, ",")
	}

	issues, count, err := db.Connection().ListIssues(db.IssueFilter{
		WorkspaceID: workspaceID,
		TaskID:      taskID,
		TaskJobID:   taskJobID,
		Codes:       issueCodes,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get issues"})
	}
	log.Info().Int64("count", count).Uint("task", taskID).Msg("Found issues")
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": issues, "count": count})
}

// FindIssuesGrouped godoc
// @Summary List all issues grouped
// @Description Retrieves all issues grouped
// @Tags Issues
// @Accept  json
// @Produce  json
// @Param workspace query int true "Workspace ID"
// @Param task query int false "Task ID"
// @Param taskjob query int false "Task Job ID"
// @Success 200 {array} db.GroupedIssue
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/issues/grouped [get]
func FindIssuesGrouped(c *fiber.Ctx) error {
	workspaceID, err := parseWorkspaceID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}

	taskID, err := parseTaskID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid task",
			"message": "The provided task ID does not seem valid",
		})
	}

	taskJobID, err := parseTaskJobID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid task job",
			"message": "The provided task job ID does not seem valid",
		})
	}

	issues, err := db.Connection().ListIssuesGrouped(db.IssueFilter{
		WorkspaceID: workspaceID,
		TaskID:      taskID,
		TaskJobID:   taskJobID,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to get issues grouped")

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get issues grouped"})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": issues})
}

// GetIssueDetail godoc
// @Summary Get details of an issue
// @Description Retrieves details of a specific issue by its ID
// @Tags Issues
// @Accept  json
// @Produce  json
// @Param id path int true "Issue ID"
// @Success 200 {object} db.Issue
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/issues/{id} [get]
func GetIssueDetail(c *fiber.Ctx) error {
	issueID, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid issue ID",
			"message": "The provided issue ID is not valid",
		})
	}

	issue, err := db.Connection().GetIssue(issueID, true)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error":   "Issue not found",
				"message": "The requested issue does not exist",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get issue"})
	}
	return c.Status(http.StatusOK).JSON(issue)
}

type IssueUpdateResponse struct {
	Message string   `json:"message"`
	Issue   db.Issue `json:"issue"`
}

// SetFalsePositive godoc
// @Summary Set an issue as a false positive
// @Description Updates the FalsePositive attribute of a specific issue
// @Tags Issues
// @Accept  json
// @Produce  json
// @Param id path int true "Issue ID"
// @Param value body bool true "Boolean value for FalsePositive"
// @Success 200 {object} IssueUpdateResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/issues/{id}/set-false-positive [post]
func SetFalsePositive(c *fiber.Ctx) error {
	issueID, err := c.ParamsInt("id")
	if err != nil {
		log.Error().Int("id", issueID).Err(err).Msg("Failed to parse issue ID")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid issue ID",
			"message": "The provided issue ID is not valid",
		})
	}

	var body struct {
		Value bool `json:"value"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Parsing error",
			"message": "Unable to parse body",
		})
	}

	issue, err := db.Connection().GetIssue(issueID, false)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error":   "Issue not found",
				"message": "The requested issue does not exist",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get issue"})
	}

	err = issue.UpdateFalsePositive(body.Value)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update issue"})
	}

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message": "Issue false positive statepdated successfully",
		"issue":   issue,
	})
}
