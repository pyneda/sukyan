package api

import (
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
)

type FeatureAvailability struct {
	Enabled bool `json:"enabled"`
}

type APIFeatures struct {
	ProxyServices FeatureAvailability `json:"proxy_services"`
}

type APILimits struct {
	BodyLimit int `json:"body_limit"`
}

type APICapabilities struct {
	Features APIFeatures `json:"features"`
	Limits   APILimits   `json:"limits"`
}

func BuildAPICapabilities(options APIServerOptions) APICapabilities {
	return APICapabilities{
		Features: APIFeatures{
			ProxyServices: FeatureAvailability{Enabled: options.EnableProxyServices},
		},
		Limits: APILimits{
			BodyLimit: viper.GetInt("api.body_limit"),
		},
	}
}

func GetAPICapabilitiesHandler(capabilities APICapabilities) fiber.Handler {
	return func(c fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"data": capabilities,
		})
	}
}
