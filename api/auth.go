package api

import (
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/auth"
	"github.com/rs/zerolog/log"

	"github.com/gofiber/fiber/v2"
)

// SignIn struct to describe login user.
type SignIn struct {
	Email    string `json:"email" validate:"required,email,lte=255"`
	Password string `json:"password" validate:"required,lte=255"`
}

type SignInTokens struct {
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
}

// SignInResponse represents the response from the UserSignIn endpoint.
type SignInResponse struct {
	Error  bool         `json:"error"`
	Msg    *string      `json:"msg"`
	Tokens SignInTokens `json:"tokens"`
}

// UserSignIn method to auth user and return access and refresh tokens.
// @Description Auth user and return access and refresh token.
// @Summary auth user and return access and refresh token
// @Tags Auth
// @Accept json
// @Produce json
// @Param signIn body SignIn true "SignIn payload"
// @Success 200 {object} SignInResponse
// @Router /api/v1/auth/user/sign/in [post]
func UserSignIn(c *fiber.Ctx) error {
	// Create a new user auth struct.
	signIn := &SignIn{}

	// Checking received data from JSON body.
	if err := c.BodyParser(signIn); err != nil {
		// Return status 400 and error message.
		return c.JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get user by email.
	foundedUser, err := db.Connection.GetUserByEmail(signIn.Email)
	if err != nil {
		// Return, if user not found.
		return c.JSON(fiber.Map{
			"error": true,
			"msg":   "wrong user email address or password", // "user with the given email is not found",
		})
	}

	// Compare given user password with stored in found user.
	compareUserPassword := auth.ComparePasswords(foundedUser.PasswordHash, signIn.Password)
	if !compareUserPassword {
		// Return, if password is not compare to stored in database.
		return c.JSON(fiber.Map{
			"error": true,
			"msg":   "wrong user email address or password",
		})
	}

	// Generate a new pair of access and refresh tokens.
	credentials := []string{}
	tokens, err := auth.GenerateNewTokens(foundedUser.ID.String(), credentials)
	if err != nil {
		// Return status 500 and token generation error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Define user ID.
	userID := foundedUser.ID.String()

	// Save refresh token to database.
	refreshToken := &db.RefreshToken{UserID: foundedUser.ID, Token: tokens.Refresh}
	if err := db.Connection.CreateRefreshToken(refreshToken); err != nil {
		// Return status 500 and token save error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	log.Info().Str("user", userID).Msg("Signed in")
	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"tokens": fiber.Map{
			"access":  tokens.Access,
			"refresh": tokens.Refresh,
		},
	})
}

// UserSignOut method to de-authorize user and delete refresh token.
// @Description De-authorize user and delete refresh token.
// @Summary de-authorize user and delete refresh token
// @Tags Auth
// @Accept json
// @Produce json
// @Success 204 {string} status "ok"
// @Security ApiKeyAuth
// @Router /api/v1/auth/user/sign/out [post]
func UserSignOut(c *fiber.Ctx) error {
	// Get claims from JWT.
	claims, err := auth.ExtractTokenMetadata(c)
	if err != nil {
		// Return status 500 and JWT parse error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Define user ID.
	userID := claims.UserID.String()
	userIDUUID, err := uuid.Parse(userID)
	if err != nil {
		// Return status 500 and parsing error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Delete refresh token from database.
	if err := db.Connection.DeleteRefreshToken(userIDUUID); err != nil {
		// Return status 500 and token deletion error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 204 no content.
	return c.SendStatus(fiber.StatusNoContent)
}

// WhoAmI method to get details of the authenticated user.
// @Description Get details of the authenticated user using JWT token.
// @Summary get details of the authenticated user
// @Tags Auth
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Success 200 {object} db.User "Returns the authenticated user data"
// @Failure 500 {object} ErrorResponse "Returns error message and status code 500 when an error occurs while processing the request"
// @Security ApiKeyAuth
// @Router /api/v1/auth/user/whoami [get]
func WhoAmI(c *fiber.Ctx) error {
	// Get claims from JWT.
	claims, err := auth.ExtractTokenMetadata(c)
	if err != nil {
		// Return status 500 and JWT parse error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	user, err := db.Connection.GetUserByID(claims.UserID)
	user.PasswordHash = ""
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": user, "count": 1})
}
