package api

import (
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/scan"
)

type PassiveScanInput struct {
	Items       []uint `json:"items" validate:"required,dive,min=0"`
	WorkspaceID uint   `json:"workspace_id" validate:"required,min=0"`
	TaskID      uint   `json:"task_id" validate:"required,min=0"`
}

var validate = validator.New()

// PassiveScanHandler godoc
// @Summary Submit items for passive scanning
// @Description Receives a list of items and schedules them for passive scanning
// @Tags Scan
// @Accept  json
// @Produce  json
// @Param input body PassiveScanInput true "List of items"
// @Success 200 {object} ActionResponse
// @Failure 400 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scan/passive [post]
func PassiveScanHandler(c *fiber.Ctx) error {
	input := new(PassiveScanInput)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"message": err.Error(),
		})
	}

	workspaceExists, _ := db.Connection.WorkspaceExists(input.WorkspaceID)
	if !workspaceExists {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}

	taskExists, _ := db.Connection.TaskExists(input.TaskID)
	if !taskExists {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid task",
			"message": "The provided task ID does not seem valid",
		})

	}

	items, err := db.Connection.GetHistoriesByID(input.Items)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Cannot get history items with provided IDs",
			"message": err.Error(),
		})
	}

	engine := c.Locals("engine").(*scan.ScanEngine)
	for _, item := range items {
		engine.ScheduleHistoryItemScan(&item, scan.ScanJobTypePassive, input.WorkspaceID, input.TaskID)
	}

	return c.JSON(fiber.Map{
		"message": "Passive scan scheduled",
	})
}

type ActiveScanInput struct {
	Items       []uint `json:"items" validate:"required,dive,min=0"`
	WorkspaceID uint   `json:"workspace_id" validate:"required,min=0"`
	TaskID      uint   `json:"task_id" validate:"required,min=0"`
}

// ActiveScanHandler godoc
// @Summary Submit items for active scanning
// @Description Receives a list of items and schedules them for active scanning
// @Tags Scan
// @Accept  json
// @Produce  json
// @Param input body PassiveScanInput true "List of items"
// @Success 200 {object} ActionResponse
// @Failure 400 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scan/active [post]
func ActiveScanHandler(c *fiber.Ctx) error {
	input := new(ActiveScanInput)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"message": err.Error(),
		})
	}

	workspaceExists, _ := db.Connection.WorkspaceExists(input.WorkspaceID)
	if !workspaceExists {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}

	taskExists, _ := db.Connection.TaskExists(input.TaskID)
	if !taskExists {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid task",
			"message": "The provided task ID does not seem valid",
		})
	}

	items, err := db.Connection.GetHistoriesByID(input.Items)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Cannot get history items with provided IDs",
			"message": err.Error(),
		})
	}

	engine := c.Locals("engine").(*scan.ScanEngine)
	for _, item := range items {
		engine.ScheduleHistoryItemScan(&item, scan.ScanJobTypeActive, input.WorkspaceID, input.TaskID)
	}

	return c.JSON(fiber.Map{
		"message": "Active scan scheduled",
	})
}

type FullScanInput struct {
	StartURLs       []string `json:"start_urls" validate:"required,dive,url"`
	MaxDepth        int      `json:"max_depth" validate:"min=0"`
	MaxPagesToCrawl int      `json:"max_pages_to_crawl" validate:"min=0"`
	ExcludePatterns []string `json:"exclude_patterns"`
	WorkspaceID     uint     `json:"workspace_id" validate:"required,min=0"`
	PagesPoolSize   int      `json:"pages_pool_size" validate:"min=1,max=100"`
}

// FullScanHandler godoc
// @Summary Submit URLs for full scanning
// @Description Receives a list of URLs and other parameters and schedules them for a full scan
// @Tags Scan
// @Accept  json
// @Produce  json
// @Param input body FullScanInput true "Configuration for full scan"
// @Success 200 {object} ActionResponse
// @Failure 400 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scan/full [post]
func FullScanHandler(c *fiber.Ctx) error {
	input := new(FullScanInput)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"message": err.Error(),
		})
	}

	workspaceExists, _ := db.Connection.WorkspaceExists(input.WorkspaceID)
	if !workspaceExists {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}

	engine := c.Locals("engine").(*scan.ScanEngine)
	go engine.CrawlAndAudit(
		input.StartURLs,
		input.MaxPagesToCrawl,
		input.MaxDepth,
		input.PagesPoolSize,
		false,
		input.ExcludePatterns,
		input.WorkspaceID,
	)

	return c.JSON(fiber.Map{
		"message": "Full scan scheduled",
	})
}
