package scan

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"gorm.io/datatypes"
)

// TestWebSocketSQLiEndToEndLive drives the real WS scanner against the ws-sqli
// testbed (port 20300) by feeding it a connection + a baseline client message,
// exactly as the crawler would. It confirms end-to-end that error-based SQLi is
// detected on the vulnerable /raw endpoint while the parameterized /raw/safe arm
// and the constant-error /trap/echo-error decoy are NOT flagged.
//
// Opt-in: requires the test DB (POSTGRES_DSN) and the running testbed. Run with:
//
//	SUKYAN_WS_LIVE=1 GOWORK=off go test ./pkg/scan/ -run TestWebSocketSQLiEndToEndLive -v
func TestWebSocketSQLiEndToEndLive(t *testing.T) {
	if os.Getenv("SUKYAN_WS_LIVE") == "" {
		t.Skip("set SUKYAN_WS_LIVE=1 (needs ws-sqli testbed on :20300 and the test DB) to run")
	}

	generators, err := generation.LoadLocalGenerators()
	if err != nil {
		t.Fatalf("load generators: %v", err)
	}
	var sqliErr []*generation.PayloadGenerator
	for _, g := range generators {
		if g.ID == "sqli-error" {
			sqliErr = append(sqliErr, g)
		}
	}
	if len(sqliErr) == 0 {
		t.Fatal("sqli-error generator not found among embedded templates")
	}

	interactionsManager := &integrations.InteractionsManager{}

	// The graphql-transport-ws frame the crawler captures on /graphql: the
	// injectable id lives inside the query string (payload.query), preceded by a
	// connection_init handshake frame - both are sent frames the scanner replays.
	const gqlSubscribe = `{"id":"1","type":"subscribe","payload":{"query":"query{ user(id:\"1\"){ name } }"}}`

	cases := []struct {
		name        string
		url         string
		subprotocol string   // Sec-WebSocket-Protocol to negotiate, "" for none
		frames      []string // client->server frames, in order
		wantIssue   bool
	}{
		{"raw_vulnerable", "ws://127.0.0.1:20300/raw", "", []string{`{"id":"1"}`}, true},
		{"raw_safe_arm", "ws://127.0.0.1:20300/raw/safe", "", []string{`{"id":"1"}`}, false},
		{"trap_echo_error", "ws://127.0.0.1:20300/trap/echo-error", "", []string{`{"id":"1"}`}, false},
		// graphql-transport-ws: the gap this test guards. Detection requires
		// fuzzing the inline id inside payload.query, not the opaque JSON value.
		{"graphql_vulnerable", "ws://127.0.0.1:20300/graphql", "graphql-transport-ws",
			[]string{`{"type":"connection_init"}`, gqlSubscribe}, true},
		{"graphql_safe_arm", "ws://127.0.0.1:20300/graphql/safe", "graphql-transport-ws",
			[]string{`{"type":"connection_init"}`, gqlSubscribe}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
				Code:  "ws-live-eval-" + tc.name,
				Title: "ws live eval " + tc.name,
			})
			if err != nil {
				t.Fatalf("workspace: %v", err)
			}
			wsID := ws.ID

			headerMap := map[string][]string{"Origin": {"http://127.0.0.1:20300"}}
			if tc.subprotocol != "" {
				headerMap["Sec-WebSocket-Protocol"] = []string{tc.subprotocol}
			}
			reqHeaders, _ := json.Marshal(headerMap)

			conn := &db.WebSocketConnection{
				URL:            tc.url,
				RequestHeaders: datatypes.JSON(reqHeaders),
				Source:         db.SourceScanner,
				WorkspaceID:    &wsID,
			}
			if err := db.Connection().CreateWebSocketConnection(conn); err != nil {
				t.Fatalf("create connection: %v", err)
			}
			var msgs []db.WebSocketMessage
			for _, frame := range tc.frames {
				m := &db.WebSocketMessage{
					ConnectionID: conn.ID,
					Opcode:       1,
					Direction:    db.MessageSent,
					PayloadData:  frame,
					Timestamp:    time.Now(),
				}
				if err := db.Connection().CreateWebSocketMessage(m); err != nil {
					t.Fatalf("create message: %v", err)
				}
				msgs = append(msgs, *m)
			}
			conn.Messages = msgs

			opts := scan_options.WebSocketScanOptions{
				WorkspaceID:       wsID,
				Mode:              scan_options.ScanModeSmart,
				ObservationWindow: 2 * time.Second,
				Concurrency:       8,
			}
			dedup := http_utils.NewWebSocketDeduplicationManager(scan_options.ScanModeSmart)

			ActiveScanWebSocketConnection(conn, interactionsManager, sqliErr, opts, dedup)

			_, count, err := db.Connection().ListIssues(db.IssueFilter{
				WorkspaceID: wsID,
				Codes:       []string{string(db.SqlInjectionCode)},
			})
			if err != nil {
				t.Fatalf("list issues: %v", err)
			}
			gotIssue := count > 0
			if gotIssue != tc.wantIssue {
				t.Errorf("%s: sql_injection issue present = %v (count=%d), want %v", tc.url, gotIssue, count, tc.wantIssue)
			} else {
				t.Logf("%s: sql_injection issues = %d (want issue=%v) ✓", tc.url, count, tc.wantIssue)
			}
		})
	}
}
