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
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
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
		engine.ScheduleHistoryItemScan(&item, scan.ScanJobTypePassive)
	}

	return c.JSON(fiber.Map{
		"message": "Passive scan scheduled",
	})
}

type ActiveScanInput struct {
	Items []uint `json:"items" validate:"required,dive,min=0"`
}

// ActiveScanHandler godoc
// @Summary Submit items for active scanning
// @Description Receives a list of items and schedules them for active scanning
// @Tags Scan
// @Accept  json
// @Produce  json
// @Param input body PassiveScanInput true "List of items"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
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

	items, err := db.Connection.GetHistoriesByID(input.Items)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Cannot get history items with provided IDs",
			"message": err.Error(),
		})
	}

	engine := c.Locals("engine").(*scan.ScanEngine)
	for _, item := range items {
		engine.ScheduleHistoryItemScan(&item, scan.ScanJobTypeActive)
	}

	return c.JSON(fiber.Map{
		"message": "Active scan scheduled",
	})
}
