package scan

import (
	"encoding/json"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"gorm.io/datatypes"
)

func wsReceived(payload string) db.WebSocketMessage {
	return db.WebSocketMessage{PayloadData: payload, Direction: db.MessageReceived}
}

// TestWebSocketResponseCheckBaselineSuppression encodes the /trap/echo-error
// scenario: an endpoint that returns a constant SQL error on every reply must
// NOT be flagged, while a real injection (error only after the payload) must be.
func TestWebSocketResponseCheckBaselineSuppression(t *testing.T) {
	s := &WebSocketScanner{}
	method := &generation.ResponseCheckDetectionMethod{Check: generation.DatabaseErrorCondition, Confidence: 80}

	sqliteError := `{"found":false,"error":"SQL logic error: unrecognized token: \"'\" (1)"}`
	constantError := `{"found":false,"error":"SQL logic error: near \"x\": syntax error"}`

	tests := []struct {
		name     string
		response []db.WebSocketMessage
		baseline []db.WebSocketMessage
		wantVuln bool
	}{
		{
			name:     "payload-triggered error with clean baseline is a finding",
			response: []db.WebSocketMessage{wsReceived(sqliteError)},
			baseline: []db.WebSocketMessage{wsReceived(`{"found":true,"name":"user1"}`)},
			wantVuln: true,
		},
		{
			name:     "constant error also present in baseline is suppressed",
			response: []db.WebSocketMessage{wsReceived(sqliteError)},
			baseline: []db.WebSocketMessage{wsReceived(constantError)},
			wantVuln: false,
		},
		{
			name:     "no baseline captured still reports (fail open)",
			response: []db.WebSocketMessage{wsReceived(sqliteError)},
			baseline: nil,
			wantVuln: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WebSocketScannerResult{
				ResponseMessages:  tt.response,
				BaselineResponses: tt.baseline,
				Payload:           generation.Payload{IssueCode: string(db.SqlInjectionCode)},
			}
			gotVuln, _, _, _, err := s.evaluateResponseCheck(result, method)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotVuln != tt.wantVuln {
				t.Errorf("evaluateResponseCheck vulnerable = %v, want %v", gotVuln, tt.wantVuln)
			}
		})
	}
}

// TestWebSocketResponseConditionBaselineSuppression covers the contains-based
// oracle: a marker present in the baseline is constant, not payload-triggered.
func TestWebSocketResponseConditionBaselineSuppression(t *testing.T) {
	s := &WebSocketScanner{}
	method := &generation.ResponseConditionDetectionMethod{Contains: "SQLITE_ERROR", Confidence: 90}

	t.Run("marker only after payload is a finding", func(t *testing.T) {
		result := WebSocketScannerResult{
			ResponseMessages:  []db.WebSocketMessage{wsReceived("boom [SQLITE_ERROR] boom")},
			BaselineResponses: []db.WebSocketMessage{wsReceived(`{"ok":true}`)},
		}
		if v, _, _, _, _ := s.evaluateResponseCondition(result, method); !v {
			t.Error("expected a finding when the marker is absent from the baseline")
		}
	})

	t.Run("marker constant in baseline is suppressed", func(t *testing.T) {
		result := WebSocketScannerResult{
			ResponseMessages:  []db.WebSocketMessage{wsReceived("boom [SQLITE_ERROR] boom")},
			BaselineResponses: []db.WebSocketMessage{wsReceived("greeting [SQLITE_ERROR]")},
		}
		if v, _, _, _, _ := s.evaluateResponseCondition(result, method); v {
			t.Error("expected suppression when the marker is already in the baseline")
		}
	})
}

// TestWebSocketDialerNegotiatesCapturedSubprotocol verifies FIX 3: the captured
// Sec-WebSocket-Protocol is re-negotiated via dialer.Subprotocols instead of
// being stripped, while Origin and custom headers are preserved and the
// auto-managed handshake headers are dropped.
func TestWebSocketDialerNegotiatesCapturedSubprotocol(t *testing.T) {
	headers := map[string][]string{
		"Origin":                 {"http://example.test"},
		"Sec-WebSocket-Protocol": {"graphql-transport-ws, chat"},
		"Sec-WebSocket-Key":      {"dGhlIHNhbXBsZSBub25jZQ=="},
		"Sec-WebSocket-Version":  {"13"},
		"Connection":             {"Upgrade"},
		"Upgrade":                {"websocket"},
		"X-Custom":               {"keep-me"},
	}
	reqHeaders, err := json.Marshal(headers)
	if err != nil {
		t.Fatalf("marshal headers: %v", err)
	}
	conn := &db.WebSocketConnection{URL: "ws://example.test/graphql", RequestHeaders: datatypes.JSON(reqHeaders)}

	dialer, httpHeaders, err := dialerAndHeadersForConnection(conn)
	if err != nil {
		t.Fatalf("dialerAndHeadersForConnection: %v", err)
	}

	wantProtocols := []string{"graphql-transport-ws", "chat"}
	if len(dialer.Subprotocols) != len(wantProtocols) {
		t.Fatalf("Subprotocols = %v, want %v", dialer.Subprotocols, wantProtocols)
	}
	for i, p := range wantProtocols {
		if dialer.Subprotocols[i] != p {
			t.Errorf("Subprotocols[%d] = %q, want %q", i, dialer.Subprotocols[i], p)
		}
	}

	if got := httpHeaders.Get("Origin"); got != "http://example.test" {
		t.Errorf("Origin header = %q, want it preserved", got)
	}
	if got := httpHeaders.Get("X-Custom"); got != "keep-me" {
		t.Errorf("X-Custom header = %q, want it preserved", got)
	}
	for _, dropped := range []string{"Sec-WebSocket-Protocol", "Sec-WebSocket-Key", "Sec-WebSocket-Version", "Connection", "Upgrade"} {
		if got := httpHeaders.Get(dropped); got != "" {
			t.Errorf("%s header = %q, want it dropped from raw headers", dropped, got)
		}
	}
}
