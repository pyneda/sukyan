package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/basicauth"
	"github.com/spf13/viper"

	jwtMiddleware "github.com/gofiber/contrib/jwt"
)

// JWTProtected func for specify routes group with JWT authentication.
// See: https://github.com/gofiber/contrib/jwt
func JWTProtected() func(*fiber.Ctx) error {
	// Create config for JWT authentication middleware.
	jwtSecret := viper.GetString("api.auth.jwt_secret_key")
	config := jwtMiddleware.Config{
		SigningKey:   jwtMiddleware.SigningKey{Key: []byte(jwtSecret)},
		ContextKey:   "jwt", // used in private routes
		ErrorHandler: jwtError,
	}

	return jwtMiddleware.New(config)
}

func jwtError(c *fiber.Ctx, err error) error {
	// Return status 401 and failed authentication error.
	if err.Error() == "Missing or malformed JWT" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 401 and failed authentication error.
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"error": true,
		"msg":   err.Error(),
	})
}

// DashboardBasicAuth creates a basic auth middleware for the dashboard
func DashboardBasicAuth() fiber.Handler {
	username := viper.GetString("api.dashboard.basic_auth.username")
	password := viper.GetString("api.dashboard.basic_auth.password")

	return basicauth.New(basicauth.Config{
		Users: map[string]string{
			username: password,
		},
		Realm: "Dashboard Access",
		Unauthorized: func(c *fiber.Ctx) error {
			c.Set("WWW-Authenticate", "Basic realm=\"Dashboard Access\"")
			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
		},
	})
}
