package api

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/playground/stream"
	"github.com/pyneda/sukyan/pkg/playground/wsfuzz"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// PlaygroundWsFuzzStreamUpgrade godoc
// @Summary Stream live events for a playground ws-fuzz run
// @Description Upgrades the connection to a WebSocket that streams snapshot,
// @Description result, progress, status, baseline, warning, and done events
// @Description for the given run. Authenticates via `?token=<jwt>` because
// @Description browsers cannot set Authorization on WS handshakes. Optional
// @Description `?since=<seq>` replays events with seq > since (reconnect-with-cursor).
// @Tags Playground
// @Param run_id path int true "WS Fuzz Run ID"
// @Param token query string true "JWT auth token"
// @Param since query int false "Replay events with seq greater than this value"
// @Success 101 {string} string "Switching Protocols"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 426 {object} ErrorResponse
// @Router /api/v1/playground/ws-fuzz/runs/{run_id}/stream [get]
func PlaygroundWsFuzzStreamUpgrade(c *fiber.Ctx) error {
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
	c.Locals("playgroundWsFuzzRunID", uint(runID))
	c.Locals("playgroundWsFuzzSince", since)
	return c.Next()
}

// PlaygroundWsFuzzStream is the post-upgrade callback. Writes a snapshot,
// subscribes to the per-run broadcaster, and pumps events until either side
// disconnects.
func PlaygroundWsFuzzStream(c *websocket.Conn) {
	runID := c.Locals("playgroundWsFuzzRunID").(uint)
	since, _ := c.Locals("playgroundWsFuzzSince").(int64)

	run, err := db.Connection().GetPlaygroundWsFuzzRun(runID)
	if err != nil {
		writeWsFuzzErrorAndClose(c, runID, "run not found")
		return
	}

	bcast := wsFuzzBroadcastersDefault.Lookup(runID)
	snapshot := buildWsFuzzSnapshot(run, bcast)
	if err := c.WriteJSON(snapshot); err != nil {
		log.Warn().Err(err).Msg("ws-fuzz stream: write snapshot")
		_ = c.Close()
		return
	}
	if bcast == nil {
		// Finished run, no live broadcaster — UI falls back to paginated iterations endpoint.
		_ = c.Close()
		return
	}

	ch, _ := bcast.Subscribe(since)
	defer bcast.Unsubscribe(ch)

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
				log.Warn().Err(err).Msg("ws-fuzz stream: write event")
				return
			}
		case <-closed:
			return
		}
	}
}

// buildWsFuzzSnapshot constructs the initial snapshot event from the persisted
// run row + the broadcaster's last-seen seq (so the client knows where to
// resume from on reconnect).
func buildWsFuzzSnapshot(run *db.PlaygroundWsFuzzRun, bcast *stream.Broadcaster) *wsfuzz.WsFuzzEvent {
	var lastSeq int64
	if bcast != nil {
		lastSeq = bcast.LastSeq()
	}
	startedAt := time.Time{}
	if run.StartedAt != nil {
		startedAt = *run.StartedAt
	}
	return &wsfuzz.WsFuzzEvent{
		Type:  wsfuzz.EventSnapshot,
		RunID: run.ID,
		Ts:    time.Now(),
		Snapshot: &wsfuzz.WsFuzzSnapshot{
			RunID:             run.ID,
			Status:            run.Status,
			PlannedIterations: run.IterationCount,
			StartedAt:         startedAt,
			LastSeq:           lastSeq,
		},
	}
}

// writeWsFuzzErrorAndClose sends an error event and closes the socket.
func writeWsFuzzErrorAndClose(c *websocket.Conn, runID uint, msg string) {
	_ = c.WriteJSON(&wsfuzz.WsFuzzEvent{
		Type:  wsfuzz.EventError,
		RunID: runID,
		Ts:    time.Now(),
		Error: &wsfuzz.WsFuzzError{Message: msg},
	})
	_ = c.Close()
}
