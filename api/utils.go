package api

import (
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"strconv"
)

func parseWorkspaceID(c *fiber.Ctx) (uint, error) {
	unparsedWorkspaceID := c.Query("workspace")
	workspaceID64, err := strconv.ParseUint(unparsedWorkspaceID, 10, strconv.IntSize)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing workspace parameter query")
		return 0, err

	}

	workspaceID := uint(workspaceID64)
	workspaceExists, _ := db.Connection.WorkspaceExists(workspaceID)
	if !workspaceExists {
		return 0, errors.New("Invalid workspace")
	}
	return workspaceID, nil
}
