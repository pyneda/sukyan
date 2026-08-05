package api

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

func TestRegisterBaseMiddlewareRecoversPanics(t *testing.T) {
	logger := zerolog.Nop()
	app := fiber.New()
	registerBaseMiddleware(app, &logger)

	app.Get("/boom", func(c *fiber.Ctx) error {
		parts := []string{"only-one"}
		return c.SendString(parts[1])
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/boom", nil), -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, fiber.StatusInternalServerError)
	}
}

func TestRegisterBaseMiddlewareServesNormalRequests(t *testing.T) {
	logger := zerolog.Nop()
	app := fiber.New()
	registerBaseMiddleware(app, &logger)

	app.Get("/ok", func(c *fiber.Ctx) error {
		return c.SendString("fine")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/ok", nil), -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
}

const testJWTSecret = "middleware-test-secret"

func signTestToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}
	return token
}

func protectedTestApp(t *testing.T) *fiber.App {
	t.Helper()
	viper.Set("api.auth.jwt_secret_key", testJWTSecret)
	t.Cleanup(viper.Reset)

	app := fiber.New()
	app.Get("/private", JWTProtected(), func(c *fiber.Ctx) error {
		return c.SendString("secret")
	})
	return app
}

func requestWithToken(t *testing.T, app *fiber.App, token string) int {
	t.Helper()
	req := httptest.NewRequest("GET", "/private", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp.StatusCode
}

func TestJWTProtectedAcceptsUnexpiredToken(t *testing.T) {
	app := protectedTestApp(t)
	token := signTestToken(t, jwt.MapClaims{"id": "user-1", "exp": time.Now().Add(time.Hour).Unix()})

	if status := requestWithToken(t, app, token); status != fiber.StatusOK {
		t.Errorf("status = %d, want %d", status, fiber.StatusOK)
	}
}

func TestJWTProtectedRejectsExpiredToken(t *testing.T) {
	app := protectedTestApp(t)
	token := signTestToken(t, jwt.MapClaims{"id": "user-1", "exp": time.Now().Add(-time.Hour).Unix()})

	if status := requestWithToken(t, app, token); status != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want %d", status, fiber.StatusUnauthorized)
	}
}

// Tokens predating the registered exp claim carried only a bespoke "expires"
// value that nothing validated, leaving them usable indefinitely.
func TestJWTProtectedRejectsTokenWithoutExpiry(t *testing.T) {
	app := protectedTestApp(t)
	token := signTestToken(t, jwt.MapClaims{"id": "user-1", "expires": time.Now().Add(-time.Hour).Unix()})

	if status := requestWithToken(t, app, token); status != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want %d", status, fiber.StatusUnauthorized)
	}
}

// The auth cookie is the only credential navigator.sendBeacon and other
// unload-time helpers can present, so it has to authenticate on its own.
func TestJWTProtectedAcceptsTokenFromAuthCookie(t *testing.T) {
	app := protectedTestApp(t)
	token := signTestToken(t, jwt.MapClaims{"id": "user-1", "exp": time.Now().Add(time.Hour).Unix()})

	req := httptest.NewRequest("GET", "/private", nil)
	req.AddCookie(&http.Cookie{Name: "auth", Value: token})
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
}

func TestJWTProtectedPrefersHeaderOverCookie(t *testing.T) {
	app := protectedTestApp(t)
	token := signTestToken(t, jwt.MapClaims{"id": "user-1", "exp": time.Now().Add(time.Hour).Unix()})

	req := httptest.NewRequest("GET", "/private", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.AddCookie(&http.Cookie{Name: "auth", Value: "not-a-token"})
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
}

func dashboardTestApp(t *testing.T, username, password string) *fiber.App {
	t.Helper()
	viper.Set("api.dashboard.basic_auth.username", username)
	viper.Set("api.dashboard.basic_auth.password", password)
	t.Cleanup(viper.Reset)

	app := fiber.New()
	app.Get("/dashboard", DashboardBasicAuth(), func(c *fiber.Ctx) error {
		return c.SendString("dashboard")
	})
	return app
}

func dashboardRequest(t *testing.T, app *fiber.App, user, pass string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("GET", "/dashboard", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func TestDashboardBasicAuthAcceptsConfiguredCredentials(t *testing.T) {
	app := dashboardTestApp(t, "admin", "s3cret1")

	resp := dashboardRequest(t, app, "admin", "s3cret1")
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
}

func TestDashboardBasicAuthRejectsWrongPassword(t *testing.T) {
	app := dashboardTestApp(t, "admin", "s3cret1")

	resp := dashboardRequest(t, app, "admin", "wrong")
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, fiber.StatusUnauthorized)
	}
	if challenge := resp.Header.Get("WWW-Authenticate"); !strings.Contains(challenge, `realm="Dashboard Access"`) {
		t.Errorf("WWW-Authenticate = %q, want it to carry realm=\"Dashboard Access\"", challenge)
	}
}

func TestDashboardBasicAuthRejectsWrongUsername(t *testing.T) {
	app := dashboardTestApp(t, "admin", "s3cret1")

	resp := dashboardRequest(t, app, "intruder", "s3cret1")
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, fiber.StatusUnauthorized)
	}
}
