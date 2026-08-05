package api

import (
	"errors"
	"net/http"

	jwtMiddleware "github.com/gofiber/contrib/v3/jwt"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

var errNoAuthenticatedUser = errors.New("no authenticated user on the request")

// currentUserID reads the subject of the access token JWTProtected validated.
// The token carries the user id under a bespoke "id" claim, not "sub".
func currentUserID(c fiber.Ctx) (uuid.UUID, error) {
	token := jwtMiddleware.FromContext(c)
	if token == nil {
		return uuid.Nil, errNoAuthenticatedUser
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return uuid.Nil, errNoAuthenticatedUser
	}
	raw, ok := claims["id"].(string)
	if !ok {
		return uuid.Nil, errNoAuthenticatedUser
	}
	return uuid.Parse(raw)
}

func currentUser(c fiber.Ctx) (*db.User, error) {
	id, err := currentUserID(c)
	if err != nil {
		return nil, err
	}
	return db.Connection().GetUserByID(id)
}

// GetCurrentUserHandler returns the account the request authenticated as.
//
// @Summary Returns the authenticated user.
// @Description Returns the account behind the presented access token. Every
// authenticated user may call it; it never exposes another user's record and
// never includes the password hash.
// @Tags Auth
// @Accept json
// @Produce json
// @Success 200 {object} db.User "Successfully retrieved the current user"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Security ApiKeyAuth
// @Router /api/v1/auth/me [get]
func GetCurrentUserHandler(c fiber.Ctx) error {
	user, err := currentUser(c)
	if err != nil {
		log.Debug().Err(err).Msg("Could not resolve the authenticated user")
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error:   "Unauthorized",
			Message: "The presented token does not identify a known account.",
		})
	}
	return c.Status(http.StatusOK).JSON(user)
}
