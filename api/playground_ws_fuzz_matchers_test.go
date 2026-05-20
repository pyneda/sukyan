package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
)

func TestGetWsFuzzMatcherFields_HasExpectedShape(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/playground/ws-fuzz/matcher-fields", GetWsFuzzMatcherFields)
	resp := doJSON(t, app, "GET", "/api/v1/playground/ws-fuzz/matcher-fields", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out struct {
		Fields []map[string]any `json:"fields"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.NotEmpty(t, out.Fields)

	// Confirm a known WS field is present.
	hasReceivedFrameCount := false
	hasHandshakeStatus := false
	for _, f := range out.Fields {
		switch f["name"] {
		case "received_frame_count":
			hasReceivedFrameCount = true
			require.Equal(t, "int", f["type"])
		case "handshake.status":
			hasHandshakeStatus = true
		}
	}
	require.True(t, hasReceivedFrameCount, "received_frame_count must be exposed")
	require.True(t, hasHandshakeStatus, "handshake.status must be exposed")
}

func TestGetWsFuzzMatcherFields_AllFieldsHaveOperators(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/playground/ws-fuzz/matcher-fields", GetWsFuzzMatcherFields)
	resp := doJSON(t, app, "GET", "/api/v1/playground/ws-fuzz/matcher-fields", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out struct {
		Fields []map[string]any `json:"fields"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	for _, f := range out.Fields {
		require.NotEmpty(t, f["operators"], "field %q must have at least one operator", f["name"])
		require.NotEmpty(t, f["label"], "field %q must have a label", f["name"])
		require.Contains(t, []any{"string", "int", "bool"}, f["type"], "field %q has unexpected type %v", f["name"], f["type"])
	}
}
