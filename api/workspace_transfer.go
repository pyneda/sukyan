package api

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/workspace"
	"github.com/rs/zerolog/log"
)

// ExportWorkspace godoc
// @Summary Export a workspace
// @Description Streams a compressed archive holding every row that belongs to the workspace
// @Tags Workspaces
// @Produce  application/zstd
// @Param id path string true "Workspace ID"
// @Success 200 {file} binary "Workspace archive"
// @Failure 404 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/workspaces/{id}/export [get]
func ExportWorkspace(c fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"message": "Invalid workspace ID", "error": "Invalid workspace ID"})
	}

	source, err := db.Connection().GetWorkspaceByID(uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Workspace not found", "error": "Workspace not found"})
	}

	filename := fmt.Sprintf("%s-%s.sukyan.zst", source.Code, time.Now().UTC().Format("20060102-150405"))
	c.Set(fiber.HeaderContentType, "application/zstd")
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf("attachment; filename=%q", filename))

	// The response is streamed: a workspace archive can run to gigabytes, so it
	// must never be assembled in memory before the first byte goes out.
	//
	// The writer runs on its own goroutine after this handler returns, by which
	// point fiber has recycled the Ctx. Nothing from c may be touched inside it,
	// so the workspace id is captured by value and the export gets its own
	// context; a client that disconnects surfaces as a write error instead.
	workspaceID := uint(id)
	return c.SendStreamWriter(func(w *bufio.Writer) {
		result, err := workspace.Export(context.Background(), db.Connection(), workspaceID, w, workspace.ExportOptions{})
		if err != nil {
			log.Error().Err(err).Uint("workspace", workspaceID).Msg("Workspace export failed mid-stream")
			return
		}
		if err := w.Flush(); err != nil {
			log.Error().Err(err).Uint("workspace", workspaceID).Msg("Failed to flush workspace export")
			return
		}
		log.Info().Uint("workspace", workspaceID).Int64("rows", result.TotalRows).Msg("Workspace exported")
	})
}

// ImportWorkspace godoc
// @Summary Import a workspace archive
// @Description Recreates an exported workspace as a new workspace in this deployment
// @Tags Workspaces
// @Accept  application/zstd
// @Produce  json
// @Param code query string false "Code for the imported workspace"
// @Param title query string false "Title for the imported workspace"
// @Success 201 {object} workspace.ImportResult
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/workspaces/import [post]
func ImportWorkspace(c fiber.Ctx) error {
	// Read the buffered body rather than a stream: request streaming disables
	// fasthttp's body limit entirely, and an unbounded upload on a shared server
	// is the worse trade. Archives too large for api.body_limit are imported
	// with the CLI instead.
	raw := c.Body()
	if len(raw) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "An archive body is required", "error": "Empty request body"})
	}
	body := bytes.NewReader(raw)

	result, err := workspace.Import(c.Context(), db.Connection(), body, workspace.ImportOptions{
		Code:  c.Query("code"),
		Title: c.Query("title"),
	})
	if err != nil {
		log.Error().Err(err).Msg("Workspace import failed")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Failed to import workspace", "error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(result)
}
