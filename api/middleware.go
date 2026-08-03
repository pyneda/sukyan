package api

import (
	"errors"
	"strings"

	"github.com/gofiber/contrib/fiberzerolog"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/basicauth"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"

	jwtMiddleware "github.com/gofiber/contrib/jwt"
)

// registerBaseMiddleware installs the middleware every request passes through.
// Order matters: fiberzerolog logs after c.Next() returns rather than in a
// defer, so recover must sit inside it for a recovered panic to be logged.
func registerBaseMiddleware(app *fiber.App, logger *zerolog.Logger) {
	app.Use(cors.New(cors.Config{
		AllowOrigins:  strings.Join(viper.GetStringSlice("api.cors.origins"), ","),
		AllowHeaders:  "Origin, Content-Type, Accept, Authorization",
		ExposeHeaders: "Content-Disposition",
	}))

	app.Use(fiberzerolog.New(fiberzerolog.Config{
		Logger: logger,
	}))

	app.Use(recover.New(recover.Config{EnableStackTrace: true}))
}

// JWTProtected func for specify routes group with JWT authentication.
// See: https://github.com/gofiber/contrib/jwt
//
// TokenLookup also accepts the JWT from the "auth" cookie so that
// navigator.sendBeacon and similar unload-time helpers — which cannot set a
// custom Authorization header — can still authenticate using the cookie the
// UI already sets at sign-in. The header is checked first, preserving the
// existing Bearer flow for normal API clients.
func JWTProtected() func(*fiber.Ctx) error {
	// Create config for JWT authentication middleware.
	jwtSecret := viper.GetString("api.auth.jwt_secret_key")
	config := jwtMiddleware.Config{
		SigningKey:     jwtMiddleware.SigningKey{Key: []byte(jwtSecret)},
		ContextKey:     "jwt", // used in private routes
		TokenLookup:    "header:Authorization,cookie:auth",
		AuthScheme:     "Bearer",
		SuccessHandler: requireTokenExpiry,
		ErrorHandler:   jwtError,
	}

	return jwtMiddleware.New(config)
}

// requireTokenExpiry rejects tokens carrying no expiration. contrib/jwt parses
// without parser options, so expiry cannot be made mandatory through config, and
// a token minted before the registered claim existed would otherwise be honoured
// forever.
func requireTokenExpiry(c *fiber.Ctx) error {
	token, ok := c.Locals("jwt").(*jwt.Token)
	if !ok {
		return jwtError(c, errors.New("invalid token"))
	}

	expires, err := token.Claims.GetExpirationTime()
	if err != nil || expires == nil {
		return jwtError(c, errors.New("token has no expiration"))
	}

	return c.Next()
}

// userLoader resolves the authenticated account. Injected so the superuser gate
// can be exercised without a database.
type userLoader func(uuid.UUID) (*db.User, error)

// SuperuserProtected restricts a route to active superusers. Chain it after
// JWTProtected().
//
// The flag is read from the database on every request rather than from the
// token, so revoking a superuser or deactivating them takes effect at once
// instead of at the next token renewal.
func SuperuserProtected() fiber.Handler {
	return superuserProtectedWith(func(id uuid.UUID) (*db.User, error) {
		return db.Connection().GetUserByID(id)
	})
}

func superuserProtectedWith(load userLoader) fiber.Handler {
	return func(c *fiber.Ctx) error {
		forbidden := func() error {
			return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
				Error:   "Forbidden",
				Message: "This resource requires a superuser account.",
			})
		}

		id, err := currentUserID(c)
		if err != nil {
			return forbidden()
		}
		user, err := load(id)
		if err != nil || user == nil || !user.Active || !user.Superuser {
			return forbidden()
		}
		return c.Next()
	}
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
