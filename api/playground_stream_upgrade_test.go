package api

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/viper"
)

// The upgrade handler stashes ids on the request and the websocket handler reads
// them back off Conn.locals, which contrib/websocket fills by copying every
// string-keyed fasthttp user value at upgrade time. Reading the user values here
// covers that hop without binding a real socket.
func captureUpgradeLocals(t *testing.T, route, target string, upgrade fiber.Handler) map[string]any {
	t.Helper()

	locals := map[string]any{}
	app := fiber.New()
	app.Get(route, upgrade, func(c fiber.Ctx) error {
		c.RequestCtx().VisitUserValues(func(k []byte, v any) { locals[string(k)] = v })
		return c.SendStatus(fiber.StatusOK)
	})

	resp := requestUpgrade(t, app, target)
	if resp != fiber.StatusOK {
		t.Fatalf("upgrade handler returned %d, want %d", resp, fiber.StatusOK)
	}
	return locals
}

func requestUpgrade(t *testing.T, app *fiber.App, target string) int {
	t.Helper()
	req := httptest.NewRequest("GET", target, nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	resp, err := app.Test(req, fiber.TestConfig{})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp.StatusCode
}

func streamTestToken(t *testing.T) string {
	t.Helper()
	viper.Set("api.auth.jwt_secret_key", testJWTSecret)
	t.Cleanup(viper.Reset)
	return signTestToken(t, jwt.MapClaims{"id": "user-1", "exp": time.Now().Add(time.Hour).Unix()})
}

func requireLocal(t *testing.T, locals map[string]any, key string, want any) {
	t.Helper()
	got, ok := locals[key]
	if !ok {
		t.Fatalf("local %q did not survive the upgrade", key)
	}
	if got != want {
		t.Errorf("local %q = %#v, want %#v", key, got, want)
	}
}

func TestPlaygroundWsStreamUpgradeCarriesLocals(t *testing.T) {
	token := streamTestToken(t)
	locals := captureUpgradeLocals(t,
		"/playground/ws/sessions/:id/stream",
		"/playground/ws/sessions/7/stream?token="+token+"&since=12",
		PlaygroundWsStreamUpgrade)

	requireLocal(t, locals, "playgroundSessionID", uint(7))
	requireLocal(t, locals, "since", int64(12))
}

func TestPlaygroundFuzzStreamUpgradeCarriesLocals(t *testing.T) {
	token := streamTestToken(t)
	locals := captureUpgradeLocals(t,
		"/playground/fuzz/runs/:run_id/stream",
		"/playground/fuzz/runs/42/stream?token="+token+"&since=3",
		PlaygroundFuzzStreamUpgrade)

	requireLocal(t, locals, "playgroundFuzzRunID", uint(42))
	requireLocal(t, locals, "since", int64(3))
}

func TestPlaygroundWsFuzzStreamUpgradeCarriesLocals(t *testing.T) {
	token := streamTestToken(t)
	locals := captureUpgradeLocals(t,
		"/playground/ws-fuzz/runs/:run_id/stream",
		"/playground/ws-fuzz/runs/99/stream?token="+token+"&since=5",
		PlaygroundWsFuzzStreamUpgrade)

	requireLocal(t, locals, "playgroundWsFuzzRunID", uint(99))
	requireLocal(t, locals, "playgroundWsFuzzSince", int64(5))
}

func TestStreamUpgradeRequiresWebSocketHeaders(t *testing.T) {
	token := streamTestToken(t)

	app := fiber.New()
	app.Get("/playground/ws/sessions/:id/stream", PlaygroundWsStreamUpgrade, func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/playground/ws/sessions/7/stream?token="+token, nil)
	resp, err := app.Test(req, fiber.TestConfig{})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusUpgradeRequired {
		t.Errorf("status = %d, want %d", resp.StatusCode, fiber.StatusUpgradeRequired)
	}
}

func TestStreamUpgradeRejectsMissingToken(t *testing.T) {
	streamTestToken(t)

	app := fiber.New()
	app.Get("/playground/ws/sessions/:id/stream", PlaygroundWsStreamUpgrade, func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	if status := requestUpgrade(t, app, "/playground/ws/sessions/7/stream"); status != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want %d", status, fiber.StatusUnauthorized)
	}
}

// The stream routes parse the token themselves rather than going through
// JWTProtected, so the expiry requirement has to be asserted separately.
func TestStreamUpgradeRejectsTokenWithoutExpiry(t *testing.T) {
	viper.Set("api.auth.jwt_secret_key", testJWTSecret)
	t.Cleanup(viper.Reset)
	token := signTestToken(t, jwt.MapClaims{"id": "user-1"})

	app := fiber.New()
	app.Get("/playground/ws/sessions/:id/stream", PlaygroundWsStreamUpgrade, func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	status := requestUpgrade(t, app, "/playground/ws/sessions/7/stream?token="+token)
	if status != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want %d", status, fiber.StatusUnauthorized)
	}
}
