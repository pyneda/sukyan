package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/stretchr/testify/require"
)

// presetTestCounter disambiguates workspaces inside a single test that needs
// two. GetOrCreateWorkspace is keyed on `code` — without a per-call suffix,
// both calls collapse onto the same row and cross-workspace isolation
// assertions become vacuously true.
var presetTestCounter atomic.Uint64

// matcherPresetTestApp mounts the four preset routes against a fresh Fiber
// app so each test gets an isolated middleware chain.
func matcherPresetTestApp() *fiber.App {
	app := fiber.New()
	app.Get("/api/v1/workspaces/:workspace_id/matcher-presets", ListMatcherPresets)
	app.Post("/api/v1/workspaces/:workspace_id/matcher-presets", CreateMatcherPreset)
	app.Put("/api/v1/workspaces/:workspace_id/matcher-presets/:id", UpdateMatcherPreset)
	app.Delete("/api/v1/workspaces/:workspace_id/matcher-presets/:id", DeleteMatcherPreset)
	return app
}

// matcherPresetTestWorkspace creates a throwaway workspace registered for
// cleanup so each test runs in its own isolated context.
func matcherPresetTestWorkspace(t *testing.T) uint {
	t.Helper()
	conn := db.Connection()
	suffix := fmt.Sprintf("_%d", presetTestCounter.Add(1))
	ws, err := conn.GetOrCreateWorkspace(&db.Workspace{
		Title: "preset-api-" + t.Name() + suffix,
		Code:  "preset_" + sanitizeForCode(t.Name()) + suffix,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.DeleteWorkspace(ws.ID) })
	return ws.ID
}

func sampleMatcherSet() fuzz.MatcherSet {
	rawValue, _ := json.Marshal(404)
	return fuzz.MatcherSet{
		Mode: fuzz.MatcherModeHide,
		Rules: []fuzz.MatcherRule{
			{Field: fuzz.FieldStatusCode, Operator: fuzz.OpEq, Value: rawValue},
		},
	}
}

func TestCreateAndListMatcherPreset_RoundTrip(t *testing.T) {
	app := matcherPresetTestApp()
	wsID := matcherPresetTestWorkspace(t)
	input := MatcherPresetInput{
		Name:       "404 hider",
		Domain:     db.MatcherPresetDomainHTTPFuzz,
		MatcherSet: sampleMatcherSet(),
	}
	createResp := doJSON(t, app, "POST",
		fmt.Sprintf("/api/v1/workspaces/%d/matcher-presets", wsID), input)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	var created db.MatcherPreset
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))
	require.Equal(t, "404 hider", created.Name)
	require.Equal(t, wsID, created.WorkspaceID)

	listResp := doJSON(t, app, "GET",
		fmt.Sprintf("/api/v1/workspaces/%d/matcher-presets?domain=http_fuzz", wsID), nil)
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	var presets []db.MatcherPreset
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&presets))
	require.Len(t, presets, 1)
	require.Equal(t, created.ID, presets[0].ID)
}

func TestCreateMatcherPreset_DuplicateNameReturns409(t *testing.T) {
	app := matcherPresetTestApp()
	wsID := matcherPresetTestWorkspace(t)
	input := MatcherPresetInput{
		Name:       "dupe",
		Domain:     db.MatcherPresetDomainHTTPFuzz,
		MatcherSet: sampleMatcherSet(),
	}
	first := doJSON(t, app, "POST",
		fmt.Sprintf("/api/v1/workspaces/%d/matcher-presets", wsID), input)
	require.Equal(t, http.StatusCreated, first.StatusCode)
	second := doJSON(t, app, "POST",
		fmt.Sprintf("/api/v1/workspaces/%d/matcher-presets", wsID), input)
	require.Equal(t, http.StatusConflict, second.StatusCode)
}

func TestCreateMatcherPreset_SameNameDifferentDomainOK(t *testing.T) {
	// The unique index keys on (workspace, domain, name) — the same label
	// should coexist across http_fuzz and ws_fuzz pickers.
	app := matcherPresetTestApp()
	wsID := matcherPresetTestWorkspace(t)
	input := MatcherPresetInput{
		Name:       "shared label",
		Domain:     db.MatcherPresetDomainHTTPFuzz,
		MatcherSet: sampleMatcherSet(),
	}
	first := doJSON(t, app, "POST",
		fmt.Sprintf("/api/v1/workspaces/%d/matcher-presets", wsID), input)
	require.Equal(t, http.StatusCreated, first.StatusCode)
	input.Domain = db.MatcherPresetDomainWsFuzz
	second := doJSON(t, app, "POST",
		fmt.Sprintf("/api/v1/workspaces/%d/matcher-presets", wsID), input)
	require.Equal(t, http.StatusCreated, second.StatusCode)
}

func TestCreateMatcherPreset_InvalidDomain(t *testing.T) {
	app := matcherPresetTestApp()
	wsID := matcherPresetTestWorkspace(t)
	bad := MatcherPresetInput{
		Name:       "x",
		Domain:     "scan_fuzz", // not in the enum
		MatcherSet: sampleMatcherSet(),
	}
	resp := doJSON(t, app, "POST",
		fmt.Sprintf("/api/v1/workspaces/%d/matcher-presets", wsID), bad)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUpdateMatcherPreset_RenameAndReplaceSet(t *testing.T) {
	app := matcherPresetTestApp()
	wsID := matcherPresetTestWorkspace(t)
	created := mustCreatePreset(t, app, wsID, "original", db.MatcherPresetDomainHTTPFuzz)

	newSet := sampleMatcherSet()
	newSet.Mode = fuzz.MatcherModeShow
	updResp := doJSON(t, app, "PUT",
		fmt.Sprintf("/api/v1/workspaces/%d/matcher-presets/%d", wsID, created.ID),
		map[string]any{"name": "renamed", "matcher_set": newSet})
	require.Equal(t, http.StatusOK, updResp.StatusCode)
	var updated db.MatcherPreset
	require.NoError(t, json.NewDecoder(updResp.Body).Decode(&updated))
	require.Equal(t, "renamed", updated.Name)
}

func TestUpdateMatcherPreset_CrossWorkspaceReturns404(t *testing.T) {
	// Editing a preset under a workspace it does not belong to must look
	// indistinguishable from "not found" to avoid leaking existence.
	app := matcherPresetTestApp()
	wsA := matcherPresetTestWorkspace(t)
	wsB := matcherPresetTestWorkspace(t)
	created := mustCreatePreset(t, app, wsA, "x", db.MatcherPresetDomainHTTPFuzz)
	resp := doJSON(t, app, "PUT",
		fmt.Sprintf("/api/v1/workspaces/%d/matcher-presets/%d", wsB, created.ID),
		map[string]any{"name": "y", "matcher_set": sampleMatcherSet()})
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestDeleteMatcherPreset_RemovesFromList(t *testing.T) {
	app := matcherPresetTestApp()
	wsID := matcherPresetTestWorkspace(t)
	created := mustCreatePreset(t, app, wsID, "to-delete", db.MatcherPresetDomainHTTPFuzz)
	delResp := doJSON(t, app, "DELETE",
		fmt.Sprintf("/api/v1/workspaces/%d/matcher-presets/%d", wsID, created.ID), nil)
	require.Equal(t, http.StatusNoContent, delResp.StatusCode)
	listResp := doJSON(t, app, "GET",
		fmt.Sprintf("/api/v1/workspaces/%d/matcher-presets", wsID), nil)
	body, _ := io.ReadAll(listResp.Body)
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	require.Equal(t, "[]", string(body))
}

func TestDeleteMatcherPreset_CrossWorkspaceReturns404(t *testing.T) {
	app := matcherPresetTestApp()
	wsA := matcherPresetTestWorkspace(t)
	wsB := matcherPresetTestWorkspace(t)
	created := mustCreatePreset(t, app, wsA, "x", db.MatcherPresetDomainHTTPFuzz)
	resp := doJSON(t, app, "DELETE",
		fmt.Sprintf("/api/v1/workspaces/%d/matcher-presets/%d", wsB, created.ID), nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func mustCreatePreset(t *testing.T, app *fiber.App, wsID uint, name string, domain db.MatcherPresetDomain) db.MatcherPreset {
	t.Helper()
	resp := doJSON(t, app, "POST", fmt.Sprintf("/api/v1/workspaces/%d/matcher-presets", wsID), MatcherPresetInput{
		Name:       name,
		Domain:     domain,
		MatcherSet: sampleMatcherSet(),
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var p db.MatcherPreset
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&p))
	return p
}
