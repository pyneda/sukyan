package api

import (
	jwtMiddleware "github.com/gofiber/contrib/jwt"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
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
