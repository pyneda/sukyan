package scan

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/stretchr/testify/assert"
	"gorm.io/datatypes"
)

// slowConstantReplyServer upgrades any request and, for every frame it receives,
// replies with `ack` immediately and then `later` after `delay`. It models an
// endpoint that emits a CONSTANT signal (e.g. a DB error) but SLOWLY - after the
// first frame and a gap longer than the old 750ms baseline drain.
func slowConstantReplyServer(ack, later string, delay time.Duration) *httptest.Server {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(ack)); err != nil {
				return
			}
			time.Sleep(delay)
			if err := conn.WriteMessage(websocket.TextMessage, []byte(later)); err != nil {
				return
			}
		}
	}))
}

func baselineTestConnection(wsURL string) *db.WebSocketConnection {
	return &db.WebSocketConnection{
		URL:             wsURL,
		RequestHeaders:  datatypes.JSON(`{}`),
		ResponseHeaders: datatypes.JSON(`{}`),
	}
}

// TestCollectBaselineResponsesCapturesSlowConstantFrame is the regression for the
// baseline false-positive: when the endpoint returns a constant error slowly
// (ack fast, error after >750ms), the baseline must still capture the error so
// the evaluator can suppress it. Before the fix the baseline drained 750ms after
// the first frame and missed it, which turned a constant endpoint error into a
// payload-attributed false positive.
func TestCollectBaselineResponsesCapturesSlowConstantFrame(t *testing.T) {
	const errorFrame = "SQL logic error: unrecognized token"
	server := slowConstantReplyServer(`{"status":"processing"}`, errorFrame, 1500*time.Millisecond)
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn := baselineTestConnection(wsURL)
	messages := []db.WebSocketMessage{{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"id":"1"}`}}

	scanner := &WebSocketScanner{}
	opts := options.WebSocketScanOptions{ObservationWindow: 4 * time.Second}

	baseline := scanner.collectBaselineResponses(conn, messages, 0, opts)

	var sawError bool
	for _, m := range baseline {
		if strings.Contains(m.PayloadData, errorFrame) {
			sawError = true
		}
	}
	assert.True(t, sawError, "baseline must observe the full window and capture the slow constant error frame; got %d frames: %+v", len(baseline), baseline)
}

// TestCollectBaselineResponsesRespectsWindow verifies the observation window is
// still an upper bound: a frame that only arrives AFTER the window must not be
// captured (so the baseline never waits longer than the probe).
func TestCollectBaselineResponsesRespectsWindow(t *testing.T) {
	const lateFrame = "TOO-LATE-SHOULD-NOT-APPEAR"
	server := slowConstantReplyServer(`{"status":"processing"}`, lateFrame, 3*time.Second)
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn := baselineTestConnection(wsURL)
	messages := []db.WebSocketMessage{{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"id":"1"}`}}

	scanner := &WebSocketScanner{}
	opts := options.WebSocketScanOptions{ObservationWindow: 1 * time.Second}

	start := time.Now()
	baseline := scanner.collectBaselineResponses(conn, messages, 0, opts)
	elapsed := time.Since(start)

	for _, m := range baseline {
		assert.NotContains(t, m.PayloadData, lateFrame, "a frame arriving after the observation window must not be captured")
	}
	assert.Less(t, elapsed, 2500*time.Millisecond, "baseline must not observe beyond the observation window")
}
