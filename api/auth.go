package api

import (
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/auth"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	"github.com/gofiber/fiber/v2"
)

// The refresh cookie is scoped to the auth routes so it never rides along with
// ordinary API traffic, and is HttpOnly so a script injected into a rendered
// response cannot read a credential that outlives the page.
const (
	refreshCookieName = "refresh"
	refreshCookiePath = "/api/v1/auth"
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

func setRefreshCookie(c *fiber.Ctx, token string) {
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     refreshCookiePath,
		MaxAge:   int(auth.RefreshCookieMaxAge().Seconds()),
		HTTPOnly: true,
		Secure:   viper.GetBool("api.auth.cookie_secure"),
		SameSite: fiber.CookieSameSiteStrictMode,
	})
}

func clearRefreshCookie(c *fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		MaxAge:   -1,
		Expires:  time.Now().Add(-time.Hour),
		HTTPOnly: true,
		Secure:   viper.GetBool("api.auth.cookie_secure"),
		SameSite: fiber.CookieSameSiteStrictMode,
	})
}

// UserSignIn method to auth user and return access and refresh tokens.
// @Description Auth user and return access and refresh token. The refresh token
// @Description is also set as an HttpOnly cookie scoped to /api/v1/auth; browser
// @Description clients should rely on that cookie rather than storing the token.
// @Summary auth user and return access and refresh token
// @Tags Auth
// @Accept json
// @Produce json
// @Param signIn body SignIn true "SignIn payload"
// @Success 200 {object} SignInResponse
// @Router /api/v1/auth/user/sign/in [post]
func UserSignIn(c *fiber.Ctx) error {
	signIn := &SignIn{}

	if err := c.BodyParser(signIn); err != nil {
		return c.JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	foundUser, err := db.Connection().GetUserByEmail(signIn.Email)
	if err != nil {
		return c.JSON(fiber.Map{
			"error": true,
			"msg":   "wrong user email address or password",
		})
	}

	if !auth.ComparePasswords(foundUser.PasswordHash, signIn.Password) {
		return c.JSON(fiber.Map{
			"error": true,
			"msg":   "wrong user email address or password",
		})
	}

	if !foundUser.Active {
		return c.JSON(fiber.Map{
			"error": true,
			"msg":   "wrong user email address or password",
		})
	}

	tokens, err := auth.GenerateNewTokens(foundUser.ID.String())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	setRefreshCookie(c, tokens.Refresh)
	log.Info().Str("user", foundUser.ID.String()).Msg("Signed in")

	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"tokens": fiber.Map{
			"access":  tokens.Access,
			"refresh": tokens.Refresh,
		},
	})
}

// UserSignOut method to de-authorize user and clear the refresh token.
// @Description Clears the refresh token cookie, ending the session. Deliberately
// @Description unauthenticated: clearing a cookie needs no identity, and requiring
// @Description a live access token would leave an expired session unable to sign out.
// @Summary de-authorize user and clear the refresh token
// @Tags Auth
// @Accept json
// @Produce json
// @Success 204 {string} status "ok"
// @Router /api/v1/auth/user/sign/out [post]
func UserSignOut(c *fiber.Ctx) error {
	clearRefreshCookie(c)
	return c.SendStatus(fiber.StatusNoContent)
}
