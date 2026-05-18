package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/require"
)

func TestFuzzMatchersGetPutRoundtrip(t *testing.T) {
	ensureFuzzRunTableApi(t)
	wsID, sessID := createFuzzWorkspaceSession(t)
	run := &db.PlaygroundFuzzRun{
		PlaygroundSessionID: sessID, WorkspaceID: wsID,
		ConfigSnapshot: []byte(`{}`), Status: db.FuzzRunSucceeded,
	}
	require.NoError(t, db.Connection().CreatePlaygroundFuzzRun(run))

	app := fiber.New()
	app.Get("/api/v1/playground/fuzz/runs/:run_id/matchers", GetFuzzRunMatchers)
	app.Put("/api/v1/playground/fuzz/runs/:run_id/matchers", PutFuzzRunMatchers)

	// Initial GET — no matchers yet.
	resp := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/fuzz/runs/%d/matchers", run.ID), nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// PUT a set with a body matcher.
	set := map[string]any{
		"mode": "hide",
		"rules": []map[string]any{
			{"field": "status_code", "operator": "eq", "value": 404},
			{"field": "response_body", "operator": "contains", "value": "Not Found"},
		},
	}
	put := doJSON(t, app, "PUT", fmt.Sprintf("/api/v1/playground/fuzz/runs/%d/matchers", run.ID), set)
	require.Equal(t, http.StatusNoContent, put.StatusCode)

	// GET roundtrip.
	got := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/fuzz/runs/%d/matchers", run.ID), nil)
	require.Equal(t, http.StatusOK, got.StatusCode)
	var roundTrip map[string]any
	require.NoError(t, json.NewDecoder(got.Body).Decode(&roundTrip))
	require.Equal(t, "hide", roundTrip["mode"])
	require.Len(t, roundTrip["rules"].([]any), 2)
}

func TestFuzzMatchersRejectsInvalidRule(t *testing.T) {
	ensureFuzzRunTableApi(t)
	wsID, sessID := createFuzzWorkspaceSession(t)
	run := &db.PlaygroundFuzzRun{PlaygroundSessionID: sessID, WorkspaceID: wsID, ConfigSnapshot: []byte(`{}`), Status: db.FuzzRunSucceeded}
	require.NoError(t, db.Connection().CreatePlaygroundFuzzRun(run))

	app := fiber.New()
	app.Put("/api/v1/playground/fuzz/runs/:run_id/matchers", PutFuzzRunMatchers)

	// status_code + contains is invalid
	resp := doJSON(t, app, "PUT", fmt.Sprintf("/api/v1/playground/fuzz/runs/%d/matchers", run.ID), map[string]any{
		"rules": []map[string]any{{"field": "status_code", "operator": "contains", "value": "x"}},
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestMatchFuzzRunServerSide(t *testing.T) {
	ensureFuzzRunTableApi(t)
	wsID, sessID := createFuzzWorkspaceSession(t)
	run := &db.PlaygroundFuzzRun{PlaygroundSessionID: sessID, WorkspaceID: wsID, ConfigSnapshot: []byte(`{}`), Status: db.FuzzRunSucceeded}
	require.NoError(t, db.Connection().CreatePlaygroundFuzzRun(run))

	// Create 3 history rows: 2 match, 1 doesn't.
	rows := []struct {
		url  string
		body string
	}{
		{"http://a/1", "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<html>found-the-needle</html>"},
		{"http://a/2", "HTTP/1.1 200 OK\r\n\r\nno needle here"},
		{"http://a/3", "HTTP/1.1 200 OK\r\n\r\nfound-the-needle again"},
	}
	var ids []uint
	for _, r := range rows {
		h := &db.History{URL: r.url, Method: "GET", StatusCode: 200, RawResponse: []byte(r.body),
			Source: db.SourceFuzzer, WorkspaceID: &wsID, PlaygroundFuzzRunID: &run.ID}
		require.NoError(t, db.Connection().DB().Create(h).Error)
		ids = append(ids, h.ID)
	}

	app := fiber.New()
	app.Post("/api/v1/playground/fuzz/runs/:run_id/match", MatchFuzzRun)

	resp := doJSON(t, app, "POST", fmt.Sprintf("/api/v1/playground/fuzz/runs/%d/match", run.ID), map[string]any{
		"matchers": map[string]any{
			"rules": []map[string]any{
				{"field": "response_body", "operator": "contains", "value": "found-the-needle"},
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got MatchFuzzRunResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, 2, got.Total)
}

func TestMatchFuzzRunWithOnlyClientSideRulesEchoesIDs(t *testing.T) {
	ensureFuzzRunTableApi(t)
	wsID, sessID := createFuzzWorkspaceSession(t)
	run := &db.PlaygroundFuzzRun{PlaygroundSessionID: sessID, WorkspaceID: wsID, ConfigSnapshot: []byte(`{}`), Status: db.FuzzRunSucceeded}
	require.NoError(t, db.Connection().CreatePlaygroundFuzzRun(run))

	app := fiber.New()
	app.Post("/api/v1/playground/fuzz/runs/:run_id/match", MatchFuzzRun)

	resp := doJSON(t, app, "POST", fmt.Sprintf("/api/v1/playground/fuzz/runs/%d/match", run.ID), map[string]any{
		"matchers": map[string]any{
			"rules": []map[string]any{
				{"field": "status_code", "operator": "eq", "value": 200},
			},
		},
		"history_ids": []uint{1, 2, 3},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got MatchFuzzRunResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, 3, got.Total, "no server-side rules → caller's ids passed through")
}
