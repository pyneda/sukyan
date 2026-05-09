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
	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// PlaygroundWsStreamUpgrade godoc
// @Summary Stream control events for a playground WebSocket session
// @Description Upgrades the connection to a WebSocket that streams interactive state changes,
// @Description frame events and run progress for the given playground session. Authenticates via
// @Description a `?token=<jwt>` query param because browsers cannot set Authorization headers on
// @Description WebSocket handshakes. Optional `?since=<seq>` replays events with seq > since.
// @Tags Playground
// @Param id path int true "Playground Session ID"
// @Param token query string true "JWT auth token"
// @Param since query int false "Replay events with seq greater than this value"
// @Success 101 {string} string "Switching Protocols"
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 426 {object} map[string]string
// @Router /api/v1/playground/ws/sessions/{id}/stream [get]
func PlaygroundWsStreamUpgrade(c *fiber.Ctx) error {
	if !websocket.IsWebSocketUpgrade(c) {
		return c.Status(fiber.StatusUpgradeRequired).JSON(fiber.Map{"error": "WebSocket upgrade required"})
	}
	tokenString := c.Query("token")
	if tokenString == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing token"})
	}
	secret := viper.GetString("api.auth.jwt_secret_key")
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token"})
	}
	idParam, err := c.ParamsInt("id")
	if err != nil || idParam <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid id"})
	}
	since, _ := strconv.ParseInt(c.Query("since", "0"), 10, 64)
	c.Locals("playgroundSessionID", uint(idParam))
	c.Locals("since", since)
	return c.Next()
}

// PlaygroundWsStream is the post-upgrade callback. It writes a snapshot frame, then pumps
// broadcaster events to the client until either side closes the connection.
func PlaygroundWsStream(c *websocket.Conn) {
	sessionID := c.Locals("playgroundSessionID").(uint)
	since, _ := c.Locals("since").(int64)

	wsSess, err := db.Connection().GetPlaygroundWsSessionBySessionID(sessionID)
	if err != nil {
		data, _ := json.Marshal(map[string]string{"message": "session not found"})
		_ = c.WriteJSON(wsreplay.Event{
			Type:     "error",
			Instance: wsreplay.InteractiveInstance(),
			Data:     data,
			Ts:       time.Now(),
		})
		c.Close()
		return
	}
	mgr := wsreplay.Default()
	bcast := mgr.BroadcasterFor(wsSess.ID)
	ch, _ := bcast.Subscribe(since)
	// Release the broadcaster slot when the client disconnects. Unsubscribe is
	// idempotent and safe even if Publish already dropped+closed the subscriber
	// for being slow (the channel close is what surfaces ok=false from <-ch).
	defer bcast.Unsubscribe(ch)

	// Snapshot. ActiveRuns is intentionally left nil for now: the engine does not
	// yet expose a per-session run listing; populating it is deferred to a later
	// task. Empty/null is the documented contract.
	snap := wsreplay.Snapshot{LastSeq: bcast.LastSeq()}
	if iv := mgr.GetInteractive(wsSess.ID); iv != nil {
		snap.Interactive.State = iv.State()
		cid := iv.ConnectionID()
		snap.Interactive.WebSocketConnectionID = &cid
	} else {
		snap.Interactive.State = wsreplay.StateDisconnected
	}
	raw, _ := json.Marshal(snap)
	_ = c.WriteJSON(wsreplay.Event{
		Type:     "snapshot",
		Instance: wsreplay.InteractiveInstance(),
		Data:     raw,
		Ts:       time.Now(),
	})

	// Pump events. A read-detection goroutine watches for client disconnect; on
	// any read error it closes `closed` so the for-select returns.
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
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := c.WriteJSON(ev); err != nil {
				log.Warn().Err(err).Msg("playground ws stream write")
				return
			}
		case <-closed:
			return
		}
	}
}
