package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/auth"
	"github.com/rs/zerolog/log"
	"time"
)

type Renew struct {
	RefreshToken string `json:"refresh_token"`
}

// RenewTokens method for renew access and refresh tokens.
// @Description Renew access and refresh tokens.
// @Summary renew access and refresh tokens
// @Tags Auth
// @Accept json
// @Produce json
// @Param refresh_token body Renew true "Refresh token"
// @Success 200 {string} status "ok"
// @Security ApiKeyAuth
// @Router /api/v1/auth/token/renew [post]
func RenewTokens(c *fiber.Ctx) error {
	now := time.Now().Unix()
	claims, err := auth.ExtractTokenMetadata(c)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	expiresAccessToken := claims.Expires

	if now > expiresAccessToken {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "unauthorized, check expiration time of your token",
		})
	}

	renew := &Renew{}
	if err := c.BodyParser(renew); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	expiresRefreshToken, err := auth.ParseRefreshToken(renew.RefreshToken)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if now < expiresRefreshToken {
		// Parse UUID from string.
		// userID, err := uuid.Parse(claims.UserID)
		// if err != nil {
		// 	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		// 		"error": true,
		// 		"msg":   err.Error(),
		// 	})
		// }
		userID := claims.UserID
		_, err := db.Connection.GetUserByID(userID)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": true,
				"msg":   "user with the given ID is not found",
			})
		}

		credentials := []string{}

		tokens, err := auth.GenerateNewTokens(userID.String(), credentials)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": true,
				"msg":   err.Error(),
			})
		}

		// Delete old refresh token
		if err := db.Connection.DeleteRefreshToken(userID); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": true,
				"msg":   err.Error(),
			})
		}

		// Save new refresh token
		if err := db.Connection.SaveRefreshToken(userID, tokens.Refresh); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": true,
				"msg":   err.Error(),
			})
		}
		log.Info().Str("user", claims.UserID.String()).Msg("Renewed JWT token")

		return c.JSON(fiber.Map{
			"error": false,
			"msg":   nil,
			"tokens": fiber.Map{
				"access":  tokens.Access,
				"refresh": tokens.Refresh,
			},
		})
	} else {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "unauthorized, your session was ended earlier",
		})
	}
}
