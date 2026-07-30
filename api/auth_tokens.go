package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/auth"
	"github.com/rs/zerolog/log"
)

type Renew struct {
	RefreshToken string `json:"refresh_token"`
}

type RenewTokensResponse struct {
	Error  bool         `json:"error"`
	Msg    *string      `json:"msg"`
	Tokens SignInTokens `json:"tokens"`
}

// Every rejection is reported identically so the endpoint cannot be used to
// probe which accounts exist or which sessions were revoked.
func renewUnauthorized(c *fiber.Ctx) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"error": true,
		"msg":   "unauthorized, please sign in again",
	})
}

// RenewTokens method for renew access and refresh tokens.
// @Description Renew access and refresh tokens. The refresh token is read from the
// @Description HttpOnly cookie set at sign-in, falling back to the request body for
// @Description non-browser clients. No access token is required, so a session resumes
// @Description even after the access token has expired.
// @Summary renew access and refresh tokens
// @Tags Auth
// @Accept json
// @Produce json
// @Param refresh_token body Renew false "Refresh token, when not supplied as a cookie"
// @Success 200 {object} RenewTokensResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/v1/auth/token/renew [post]
func RenewTokens(c *fiber.Ctx) error {
	refreshToken := c.Cookies(refreshCookieName)
	if refreshToken == "" {
		renew := &Renew{}
		if err := c.BodyParser(renew); err == nil {
			refreshToken = renew.RefreshToken
		}
	}
	if refreshToken == "" {
		return renewUnauthorized(c)
	}

	claims, err := auth.ParseRefreshToken(refreshToken)
	if err != nil {
		log.Debug().Err(err).Msg("Rejected refresh token")
		return renewUnauthorized(c)
	}

	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		return renewUnauthorized(c)
	}

	user, err := db.Connection().GetUserByID(userID)
	if err != nil || !user.Active {
		return renewUnauthorized(c)
	}

	if claims.AuthTime.Before(user.TokensValidFrom) {
		log.Info().Str("user", claims.UserID).Msg("Rejected revoked session")
		return renewUnauthorized(c)
	}

	tokens, err := auth.RenewTokens(claims.UserID, claims.AuthTime)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	setRefreshCookie(c, tokens.Refresh)
	log.Debug().Str("user", claims.UserID).Msg("Renewed JWT token")

	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"tokens": fiber.Map{
			"access":  tokens.Access,
			"refresh": tokens.Refresh,
		},
	})
}
