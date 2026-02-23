package api

import "github.com/gofiber/fiber/v2"

type FeatureAvailability struct {
	Enabled bool `json:"enabled"`
}

type APIFeatures struct {
	ProxyServices FeatureAvailability `json:"proxy_services"`
}

type APICapabilities struct {
	Features APIFeatures `json:"features"`
}

func BuildAPICapabilities(options APIServerOptions) APICapabilities {
	return APICapabilities{
		Features: APIFeatures{
			ProxyServices: FeatureAvailability{Enabled: options.EnableProxyServices},
		},
	}
}

func GetAPICapabilitiesHandler(capabilities APICapabilities) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"data": capabilities,
		})
	}
}

