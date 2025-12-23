package api

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
)

// GetScanOptionsPlatforms returns all available platforms for scanning.
//
// @Summary List all available platforms
// @Description Returns all unique platforms from loaded payload generators. Can be used to filter scans by target platform.
// @Tags Scan Options
// @Produce json
// @Success 200 {object} map[string][]string "List of platforms"
// @Security ApiKeyAuth
// @Router /api/v1/scan/options/platforms [get]
func GetScanOptionsPlatforms(c *fiber.Ctx) error {
	generators := c.Locals("generators").([]*generation.PayloadGenerator)
	platforms := generation.GetAllPlatforms(generators)
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"platforms": platforms,
	})
}

// GetScanOptionsCategories returns all available categories for scanning.
//
// @Summary List all available categories
// @Description Returns all unique categories from loaded payload generators. Can be used to filter scans by vulnerability category.
// @Tags Scan Options
// @Produce json
// @Success 200 {object} map[string][]string "List of categories"
// @Security ApiKeyAuth
// @Router /api/v1/scan/options/categories [get]
func GetScanOptionsCategories(c *fiber.Ctx) error {
	generators := c.Locals("generators").([]*generation.PayloadGenerator)
	categories := generation.GetAllCategories(generators)
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"categories": categories,
	})
}
