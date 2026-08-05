package api

import (
	"crypto/subtle"

	fiberzerolog "github.com/gofiber/contrib/v3/zerolog"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/extractors"
	"github.com/gofiber/fiber/v3/middleware/basicauth"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	jwtMiddleware "github.com/gofiber/contrib/v3/jwt"
)

// registerBaseMiddleware installs the middleware every request passes through.
// Order matters: fiberzerolog logs after c.Next() returns rather than in a
// defer, so recover must sit inside it for a recovered panic to be logged.
func registerBaseMiddleware(app *fiber.App, logger *zerolog.Logger) {
	app.Use(cors.New(cors.Config{
		AllowOrigins:  viper.GetStringSlice("api.cors.origins"),
		AllowHeaders:  []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders: []string{"Content-Disposition"},
	}))

	app.Use(fiberzerolog.New(fiberzerolog.Config{
		Logger: logger,
	}))

	app.Use(recover.New(recover.Config{EnableStackTrace: true}))
}

// JWTProtected func for specify routes group with JWT authentication.
// See: https://github.com/gofiber/contrib/tree/main/v3/jwt
//
// The extractor chain also accepts the JWT from the "auth" cookie so that
// navigator.sendBeacon and similar unload-time helpers — which cannot set a
// custom Authorization header — can still authenticate using the cookie the
// UI already sets at sign-in. The header is checked first, preserving the
// existing Bearer flow for normal API clients.
//
// Expiry is mandatory: a token minted before the registered exp claim existed
// would otherwise be honoured forever.
func JWTProtected() fiber.Handler {
	jwtSecret := viper.GetString("api.auth.jwt_secret_key")
	config := jwtMiddleware.Config{
		SigningKey: jwtMiddleware.SigningKey{Key: []byte(jwtSecret)},
		Extractor: extractors.Chain(
			extractors.FromAuthHeader("Bearer"),
			extractors.FromCookie("auth"),
		),
		ParserOptions: []jwt.ParserOption{jwt.WithExpirationRequired()},
		ErrorHandler:  jwtError,
	}

	return jwtMiddleware.New(config)
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
	return func(c fiber.Ctx) error {
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

func jwtError(c fiber.Ctx, err error) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"error": true,
		"msg":   err.Error(),
	})
}

// DashboardBasicAuth creates a basic auth middleware for the dashboard.
//
// The credential is configured in clear text, so the comparison happens here and
// Users is left unset: basicauth reads every Users value as a password hash and
// panics on anything it cannot parse. Both halves are compared before combining
// so a wrong username costs the same as a wrong password.
func DashboardBasicAuth() fiber.Handler {
	username := viper.GetString("api.dashboard.basic_auth.username")
	password := viper.GetString("api.dashboard.basic_auth.password")

	authorize := func(user, pass string, _ fiber.Ctx) bool {
		userOK := subtle.ConstantTimeCompare([]byte(user), []byte(username)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(password)) == 1
		return userOK && passOK
	}

	if username == "" || password == "" {
		log.Error().Msg("Dashboard basic auth is not configured - every dashboard request will be rejected")
		authorize = func(string, string, fiber.Ctx) bool { return false }
	}

	return basicauth.New(basicauth.Config{
		Realm:      "Dashboard Access",
		Authorizer: authorize,
	})
}
