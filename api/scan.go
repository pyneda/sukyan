package api

import (
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/scan"
)

type PassiveScanInput struct {
	Items []uint `json:"items" validate:"required,dive,min=0"`
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

	items, err := db.Connection.GetHistoriesByID(input.Items)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Cannot get history items with provided IDs",
			"message": err.Error(),
		})
	}

	engine := c.Locals("engine").(*scan.ScanEngine)
	for _, item := range items {
		// engine.ScheduleHistoryItemScan(&item, scan.ScanJobTypePassive, input.WorkspaceID, input.TaskID)
		// NOTE: By now, passive scans do not create task jobs, so we pass 0 as task ID
		engine.ScheduleHistoryItemScan(&item, scan.ScanJobTypePassive, *item.WorkspaceID, 0)

	}

	return c.JSON(fiber.Map{
		"message": "Passive scan scheduled",
	})
}

type ActiveScanInput struct {
	Items       []uint `json:"items" validate:"required,dive,min=0"`
	WorkspaceID uint   `json:"workspace" validate:"omitempty,min=0"`
	TaskID      uint   `json:"task" validate:"omitempty,min=0"`
}

// ActiveScanHandler godoc
// @Summary Submit items for active scanning
// @Description Receives a list of items and schedules them for active scanning. Either the workspace ID or task ID must be provided.
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

	// if !workspaceExists {
	// 	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
	// 		"error":   "Invalid workspace",
	// 		"message": "The provided workspace ID does not seem valid",
	// 	})
	// }

	taskExists, _ := db.Connection.TaskExists(input.TaskID)
	if !taskExists {
		workspaceExists, _ := db.Connection.WorkspaceExists(input.WorkspaceID)
		if !workspaceExists {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Invalid task",
				"message": "The provided task ID does not seem valid and we can't create a default one because the workspace ID is either not provided or invalid",
			})
		}

		task, err := db.Connection.GetOrCreateDefaultWorkspaceTask(input.WorkspaceID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Error creating default task",
				"message": "We couldnt create a default task for the provided workspace",
			})
		}
		input.TaskID = task.ID
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
		// TODO: maybe should validate that the history item and task belongs to the same workspace
		engine.ScheduleHistoryItemScan(&item, scan.ScanJobTypeActive, *item.WorkspaceID, input.TaskID)
	}

	return c.JSON(fiber.Map{
		"message": "Active scan scheduled",
	})
}

type FullScanInput struct {
	Title           string              `json:"title" validate:"omitempty,min=1,max=255"`
	StartURLs       []string            `json:"start_urls" validate:"required,dive,url"`
	MaxDepth        int                 `json:"max_depth" validate:"min=0"`
	MaxPagesToCrawl int                 `json:"max_pages_to_crawl" validate:"min=0"`
	ExcludePatterns []string            `json:"exclude_patterns"`
	WorkspaceID     uint                `json:"workspace_id" validate:"required,min=0"`
	PagesPoolSize   int                 `json:"pages_pool_size" validate:"min=1,max=100"`
	Headers         map[string][]string `json:"headers" validate:"omitempty"`
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

	if input.Title == "" {
		input.Title = "Full scan"
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
		input.Title,
		input.Headers,
	)

	return c.JSON(fiber.Map{
		"message": "Full scan scheduled",
	})
}
