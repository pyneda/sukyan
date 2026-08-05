package api

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/spf13/viper"
)

func currentUserIDTestApp(t *testing.T, capture func(uuid.UUID, error)) *fiber.App {
	t.Helper()
	viper.Set("api.auth.jwt_secret_key", testJWTSecret)
	t.Cleanup(viper.Reset)

	app := fiber.New()
	app.Get("/private", JWTProtected(), func(c fiber.Ctx) error {
		capture(currentUserID(c))
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

func TestCurrentUserIDReadsTheIDClaim(t *testing.T) {
	want := uuid.New()
	var got uuid.UUID
	var gotErr error

	app := currentUserIDTestApp(t, func(id uuid.UUID, err error) { got, gotErr = id, err })
	token := signTestToken(t, jwt.MapClaims{"id": want.String(), "exp": time.Now().Add(time.Hour).Unix()})

	req := httptest.NewRequest("GET", "/private", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if _, err := app.Test(req, fiber.TestConfig{}); err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if gotErr != nil {
		t.Fatalf("currentUserID error = %v", gotErr)
	}
	if got != want {
		t.Errorf("currentUserID = %v, want %v", got, want)
	}
}

func TestCurrentUserIDRejectsANonUUIDClaim(t *testing.T) {
	var gotErr error

	app := currentUserIDTestApp(t, func(_ uuid.UUID, err error) { gotErr = err })
	token := signTestToken(t, jwt.MapClaims{"id": "not-a-uuid", "exp": time.Now().Add(time.Hour).Unix()})

	req := httptest.NewRequest("GET", "/private", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if _, err := app.Test(req, fiber.TestConfig{}); err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if gotErr == nil {
		t.Error("currentUserID error = nil, want an error for a malformed id claim")
	}
}
