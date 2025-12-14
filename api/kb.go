package api

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
)

// ListIssueTemplates returns all issue templates from the knowledge base.
//
// @Summary List all issue templates from the knowledge base
// @Description Returns all defined issue types with their code, title, description, remediation, CWE, severity, and references.
// @Tags Knowledge Base
// @Produce json
// @Success 200 {array} db.IssueTemplate "List of issue templates"
// @Security ApiKeyAuth
// @Router /api/v1/kb/issues [get]
func ListIssueTemplates(c *fiber.Ctx) error {
	templates := db.GetAllIssueTemplates()
	return c.Status(http.StatusOK).JSON(templates)
}
