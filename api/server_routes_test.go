package api

import (
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

// buildAPIApp registers every route the server exposes. Building it here is the
// assertion: the router accepts handlers as an untyped value, so a wrong handler
// shape is a registration-time panic rather than a compile error.
func buildRoutesTestApp(t *testing.T, options APIServerOptions) *fiber.App {
	t.Helper()
	logger := zerolog.Nop()
	return buildAPIApp(apiDeps{logger: &logger, options: options})
}

func routeSet(app *fiber.App) map[string]bool {
	set := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		set[r.Method+" "+r.Path] = true
	}
	return set
}

func requireRoute(t *testing.T, routes map[string]bool, route string) {
	t.Helper()
	if !routes[route] {
		t.Errorf("route %q is not registered", route)
	}
}

func requireNoRoute(t *testing.T, routes map[string]bool, route string) {
	t.Helper()
	if routes[route] {
		t.Errorf("route %q is registered but should not be", route)
	}
}

func TestBuildAPIAppRegistersCoreRoutes(t *testing.T) {
	t.Cleanup(viper.Reset)

	routes := routeSet(buildRoutesTestApp(t, APIServerOptions{}))
	if len(routes) == 0 {
		t.Fatal("no routes registered")
	}

	for _, route := range []string{
		"GET /",
		"GET /api/v1/capabilities",
		"GET /api/v1/history/:id",
		"GET /api/v1/issues/:id",
		"GET /api/v1/workspaces/:id/export",
		"POST /api/v1/workspaces/import",
		"POST /api/v1/auth/user/sign/in",
		"GET /api/v1/auth/me",
		"GET /api/v1/users",
		"POST /api/v1/scan/full",
		"GET /api/v1/scans/:id",
	} {
		requireRoute(t, routes, route)
	}
}

// These three carry a value from the upgrade handler into the websocket handler
// through Locals, so a routing change here breaks the stream silently.
func TestBuildAPIAppRegistersWebSocketStreams(t *testing.T) {
	t.Cleanup(viper.Reset)

	routes := routeSet(buildRoutesTestApp(t, APIServerOptions{}))
	for _, route := range []string{
		"GET /api/v1/playground/fuzz/runs/:run_id/stream",
		"GET /api/v1/playground/ws/sessions/:id/stream",
		"GET /api/v1/playground/ws-fuzz/runs/:run_id/stream",
	} {
		requireRoute(t, routes, route)
	}
}

func TestBuildAPIAppProxyServiceRoutesFollowTheFeatureFlag(t *testing.T) {
	t.Cleanup(viper.Reset)

	enabled := routeSet(buildRoutesTestApp(t, APIServerOptions{EnableProxyServices: true}))
	requireRoute(t, enabled, "GET /api/v1/proxy-services/:id")
	requireRoute(t, enabled, "POST /api/v1/proxy-services/:id/restart")
	requireRoute(t, enabled, "GET /api/v1/workspaces/:workspaceId/proxy-services")

	disabled := routeSet(buildRoutesTestApp(t, APIServerOptions{}))
	requireNoRoute(t, disabled, "GET /api/v1/proxy-services/:id")
	requireNoRoute(t, disabled, "POST /api/v1/proxy-services/:id/restart")
}

func TestBuildAPIAppDashboardRoutesFollowConfig(t *testing.T) {
	t.Cleanup(viper.Reset)

	viper.Set("api.dashboard.enabled", true)
	viper.Set("api.dashboard.path", "/dashboard")
	viper.Set("api.dashboard.basic_auth.username", "admin")
	viper.Set("api.dashboard.basic_auth.password", "s3cret1")

	enabled := routeSet(buildRoutesTestApp(t, APIServerOptions{}))
	requireRoute(t, enabled, "GET /dashboard")
	requireRoute(t, enabled, "GET /dashboard/stats")

	viper.Set("api.dashboard.enabled", false)
	disabled := routeSet(buildRoutesTestApp(t, APIServerOptions{}))
	requireNoRoute(t, disabled, "GET /dashboard")
	requireNoRoute(t, disabled, "GET /dashboard/stats")
}

func TestBuildAPIAppDashboardPathDefaultsWhenUnset(t *testing.T) {
	t.Cleanup(viper.Reset)

	viper.Set("api.dashboard.enabled", true)
	viper.Set("api.dashboard.path", "")
	viper.Set("api.dashboard.basic_auth.username", "admin")
	viper.Set("api.dashboard.basic_auth.password", "s3cret1")

	requireRoute(t, routeSet(buildRoutesTestApp(t, APIServerOptions{})), "GET /dashboard")
}

func TestBuildAPIAppOptionalMountsRegisterWithoutPanicking(t *testing.T) {
	t.Cleanup(viper.Reset)

	base := len(routeSet(buildRoutesTestApp(t, APIServerOptions{})))

	viper.Set("api.docs.enabled", true)
	viper.Set("api.docs.path", "/docs")
	viper.Set("api.pprof.enabled", true)
	viper.Set("api.pprof.prefix", "")

	routes := routeSet(buildRoutesTestApp(t, APIServerOptions{}))
	requireRoute(t, routes, "GET /docs/*")
	if len(routes) <= base {
		t.Errorf("enabling docs and pprof did not add routes: %d <= %d", len(routes), base)
	}
}
