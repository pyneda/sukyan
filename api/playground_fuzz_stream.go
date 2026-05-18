package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/pyneda/sukyan/pkg/playground/stream"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// PlaygroundFuzzStreamUpgrade godoc
// @Summary Stream live events for a playground fuzz run
// @Description Upgrades the connection to a WebSocket that streams snapshot,
// @Description result, progress, status, and done events for the given run.
// @Description Authenticates via `?token=<jwt>` because browsers cannot set
// @Description Authorization on WS handshakes. Optional `?since=<seq>` replays
// @Description events with seq > since (for reconnect-with-cursor flow).
// @Tags Playground
// @Param run_id path int true "Fuzz Run ID"
// @Param token query string true "JWT auth token"
// @Param since query int false "Replay events with seq greater than this value"
// @Success 101 {string} string "Switching Protocols"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 426 {object} ErrorResponse
// @Router /api/v1/playground/fuzz/runs/{run_id}/stream [get]
func PlaygroundFuzzStreamUpgrade(c *fiber.Ctx) error {
	if !websocket.IsWebSocketUpgrade(c) {
		return c.Status(fiber.StatusUpgradeRequired).JSON(ErrorResponse{Error: "WebSocket upgrade required"})
	}
	tokenString := c.Query("token")
	if tokenString == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{Error: "Missing token"})
	}
	secret := viper.GetString("api.auth.jwt_secret_key")
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{Error: "Invalid token"})
	}
	runID, err := c.ParamsInt("run_id")
	if err != nil || runID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid run id"})
	}
	since, _ := strconv.ParseInt(c.Query("since", "0"), 10, 64)
	c.Locals("playgroundFuzzRunID", uint(runID))
	c.Locals("since", since)
	return c.Next()
}

// PlaygroundFuzzStream is the post-upgrade callback. Writes a snapshot, then
// pumps broadcaster events until either side disconnects or the run reaches
// terminal status.
func PlaygroundFuzzStream(c *websocket.Conn) {
	runID := c.Locals("playgroundFuzzRunID").(uint)
	since, _ := c.Locals("since").(int64)

	run, err := db.Connection().GetPlaygroundFuzzRun(runID)
	if err != nil {
		writeErrorAndClose(c, runID, "run not found")
		return
	}

	// Broadcaster lookup — may be nil for runs that finished before this
	// subscriber arrived (the engine closes the broadcaster on terminal
	// status). In that case we just send the snapshot derived from the DB
	// row and close; the UI falls back to the paginated results endpoint.
	bcast := fuzz.Default().Broadcaster(runID)
	snapshot := buildFuzzSnapshot(run, bcast)
	if err := c.WriteJSON(snapshot); err != nil {
		log.Warn().Err(err).Msg("fuzz stream: write snapshot")
		_ = c.Close()
		return
	}
	if bcast == nil {
		// Finished run, no live broadcaster — close after snapshot.
		_ = c.Close()
		return
	}

	ch, _ := bcast.Subscribe(since)
	defer bcast.Unsubscribe(ch)

	// Disconnect detection.
	closed := make(chan struct{})
	go func() {
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				close(closed)
				return
			}
		}
	}()

	for {
		select {
		case raw, ok := <-ch:
			if !ok {
				return
			}
			if err := c.WriteJSON(raw); err != nil {
				log.Warn().Err(err).Msg("fuzz stream: write event")
				return
			}
		case <-closed:
			return
		}
	}
}

// buildFuzzSnapshot constructs the snapshot event from the persisted run
// row + current broadcaster state.
func buildFuzzSnapshot(run *db.PlaygroundFuzzRun, bcast *stream.Broadcaster) *fuzz.FuzzEvent {
	var lastSeq int64
	if bcast != nil {
		lastSeq = bcast.LastSeq()
	}
	// Pull mode from the snapshot config; degrade gracefully if missing.
	mode := ""
	if len(run.ConfigSnapshot) > 0 {
		var c map[string]any
		if err := json.Unmarshal(run.ConfigSnapshot, &c); err == nil {
			if m, ok := c["mode"].(string); ok {
				mode = m
			}
		}
	}
	snap := &fuzz.FuzzSnapshot{
		RunID:               run.ID,
		Status:              string(run.Status),
		Mode:                mode,
		PlannedRequestCount: run.PlannedRequestCount,
		StartedAt:           run.StartedAt,
		FinishedAt:          run.FinishedAt,
		LastSeq:             lastSeq,
		Progress: fuzz.FuzzProgress{
			Sent:    run.SentRequestCount,
			Errors:  run.ErrorCount,
			Planned: run.PlannedRequestCount,
		},
	}
	return &fuzz.FuzzEvent{
		Type:     fuzz.FuzzEventSnapshot,
		RunID:    run.ID,
		At:       time.Now(),
		Snapshot: snap,
	}
}

func writeErrorAndClose(c *websocket.Conn, runID uint, msg string) {
	_ = c.WriteJSON(&fuzz.FuzzEvent{
		Type:  fuzz.FuzzEventError,
		RunID: runID,
		At:    time.Now(),
		Err:   &fuzz.FuzzErrorEv{Message: msg},
	})
	_ = c.Close()
}
