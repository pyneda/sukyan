package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/pyneda/sukyan/db"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// FindInteractions gets interactions with pagination and filtering options
// @Summary Get interactions
// @Description Get interactions with optional pagination and filtering options
// @Tags Interactions
// @Produce json
// @Param workspace query int true "Workspace ID"
// @Param page_size query integer false "Size of each page" default(50)
// @Param page query integer false "Page number" default(1)
// @Param protocols query string false "Comma-separated list of protocols to filter by"
// @Param qtypes query string false "Comma-separated list of query types to filter by"
// @Param full_ids query string false "Comma-separated list of full IDs to filter by"
// @Param remote_addresses query string false "Comma-separated list of remote addresses to filter by"
// @Param oob_test_ids query string false "Comma-separated list of OOB test IDs to filter by"
// @Param issue_ids query string false "Comma-separated list of issue IDs to filter by"
// @Param scan_ids query string false "Comma-separated list of scan IDs to filter by (filters via related OOB test)"
// @Param scan_job_ids query string false "Comma-separated list of scan job IDs to filter by (filters via related OOB test)"
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/interactions [get]
func FindInteractions(c *fiber.Ctx) error {
	unparsedPageSize := c.Query("page_size", "50")
	unparsedPage := c.Query("page", "1")
	unparsedProtocols := c.Query("protocols")
	unparsedQTypes := c.Query("qtypes")
	unparsedFullIDs := c.Query("full_ids")
	unparsedRemoteAddresses := c.Query("remote_addresses")
	unparsedOOBTestIDs := c.Query("oob_test_ids")
	unparsedIssueIDs := c.Query("issue_ids")
	unparsedScanIDs := c.Query("scan_ids")
	unparsedScanJobIDs := c.Query("scan_job_ids")

	var protocols, qtypes, fullIDs, remoteAddresses []string
	var oobTestIDs, issueIDs, scanIDs, scanJobIDs []uint

	workspaceID, err := parseWorkspaceID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}

	pageSize, err := strconv.Atoi(unparsedPageSize)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing page size parameter query")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid page size parameter"})
	}

	page, err := strconv.Atoi(unparsedPage)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing page parameter query")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid page parameter"})
	}

	if unparsedProtocols != "" {
		protocols = strings.Split(unparsedProtocols, ",")
	}

	if unparsedQTypes != "" {
		qtypes = strings.Split(unparsedQTypes, ",")
	}

	if unparsedFullIDs != "" {
		fullIDs = strings.Split(unparsedFullIDs, ",")
	}

	if unparsedRemoteAddresses != "" {
		remoteAddresses = strings.Split(unparsedRemoteAddresses, ",")
	}

	if unparsedOOBTestIDs != "" {
		oobTestIDs, err = stringToUintSlice(unparsedOOBTestIDs, []uint{}, false)
		if err != nil {
			log.Error().Err(err).Str("unparsed", unparsedOOBTestIDs).Msg("Error parsing OOB test IDs parameter")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid OOB test IDs parameter"})
		}
	}

	if unparsedIssueIDs != "" {
		issueIDs, err = stringToUintSlice(unparsedIssueIDs, []uint{}, false)
		if err != nil {
			log.Error().Err(err).Str("unparsed", unparsedIssueIDs).Msg("Error parsing issue IDs parameter")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid issue IDs parameter"})
		}
	}

	if unparsedScanIDs != "" {
		scanIDs, err = stringToUintSlice(unparsedScanIDs, []uint{}, false)
		if err != nil {
			log.Error().Err(err).Str("unparsed", unparsedScanIDs).Msg("Error parsing scan IDs parameter")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid scan IDs parameter"})
		}
	}

	if unparsedScanJobIDs != "" {
		scanJobIDs, err = stringToUintSlice(unparsedScanJobIDs, []uint{}, false)
		if err != nil {
			log.Error().Err(err).Str("unparsed", unparsedScanJobIDs).Msg("Error parsing scan job IDs parameter")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid scan job IDs parameter"})
		}
	}

	issues, count, err := db.Connection().ListInteractions(db.InteractionsFilter{
		Pagination: db.Pagination{
			Page: page, PageSize: pageSize,
		},
		Protocols:       protocols,
		QTypes:          qtypes,
		FullIDs:         fullIDs,
		RemoteAddresses: remoteAddresses,
		OOBTestIDs:      oobTestIDs,
		IssueIDs:        issueIDs,
		ScanIDs:         scanIDs,
		ScanJobIDs:      scanJobIDs,
		WorkspaceID:     workspaceID,
	})

	if err != nil {
		// Should handle this better
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": issues, "count": count})
}

// GetInteractionDetail fetches the details of a specific OOB Interaction by its ID.
// @Summary Get interaction detail
// @Description Fetch the detail of an OOB Interaction by its ID
// @Tags Interactions
// @Produce json
// @Param id path int true "Interaction ID"
// @Success 200 {object} db.OOBInteraction
// @Failure 404 {object} ErrorResponse "Interaction not found"
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/interactions/{id} [get]
func GetInteractionDetail(c *fiber.Ctx) error {
	interactionID, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid interaction ID",
			"message": "The provided interaction ID does not seem valid",
		})
	}

	interaction, err := db.Connection().GetInteraction(uint(interactionID))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error":   "Interaction not found",
				"message": "The requested interaction does not exist",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
	}

	return c.Status(http.StatusOK).JSON(interaction)
}
