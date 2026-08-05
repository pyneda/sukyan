package api

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/spf13/viper"
)

func superuserTestApp(t *testing.T, load userLoader) *fiber.App {
	t.Helper()
	viper.Set("api.auth.jwt_secret_key", testJWTSecret)
	t.Cleanup(viper.Reset)

	app := fiber.New()
	app.Get("/admin", JWTProtected(), superuserProtectedWith(load), func(c fiber.Ctx) error {
		return c.SendString("admin")
	})
	return app
}

func superuserRequest(t *testing.T, app *fiber.App) int {
	t.Helper()
	token := signTestToken(t, jwt.MapClaims{
		"id":  uuid.New().String(),
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req, fiber.TestConfig{})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp.StatusCode
}

func loaderReturning(user *db.User) userLoader {
	return func(uuid.UUID) (*db.User, error) { return user, nil }
}

func TestSuperuserProtectedAllowsAnActiveSuperuser(t *testing.T) {
	app := superuserTestApp(t, loaderReturning(&db.User{Active: true, Superuser: true}))

	if status := superuserRequest(t, app); status != fiber.StatusOK {
		t.Errorf("status = %d, want %d", status, fiber.StatusOK)
	}
}

func TestSuperuserProtectedRejectsAMember(t *testing.T) {
	app := superuserTestApp(t, loaderReturning(&db.User{Active: true, Superuser: false}))

	if status := superuserRequest(t, app); status != fiber.StatusForbidden {
		t.Errorf("status = %d, want %d", status, fiber.StatusForbidden)
	}
}

// A superuser who has been deactivated must lose access immediately, which is
// only possible because the flag is read per request rather than from the token.
func TestSuperuserProtectedRejectsADeactivatedSuperuser(t *testing.T) {
	app := superuserTestApp(t, loaderReturning(&db.User{Active: false, Superuser: true}))

	if status := superuserRequest(t, app); status != fiber.StatusForbidden {
		t.Errorf("status = %d, want %d", status, fiber.StatusForbidden)
	}
}

func TestSuperuserProtectedRejectsAnUnresolvableUser(t *testing.T) {
	app := superuserTestApp(t, func(uuid.UUID) (*db.User, error) {
		return nil, errors.New("record not found")
	})

	if status := superuserRequest(t, app); status != fiber.StatusForbidden {
		t.Errorf("status = %d, want %d", status, fiber.StatusForbidden)
	}
}
