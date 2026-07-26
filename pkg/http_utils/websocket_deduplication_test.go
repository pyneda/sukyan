package http_utils

import (
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/stretchr/testify/assert"
)

// testEndpoint is the single endpoint these cases operate on; deduplication
// is keyed per endpoint, so using one URL preserves their original intent.
const testEndpoint = "ws://127.0.0.1:20300/ws"

func TestNewWebSocketDeduplicationManager(t *testing.T) {
	manager := NewWebSocketDeduplicationManager(options.ScanModeSmart)
	assert.NotNil(t, manager)
	assert.Equal(t, options.ScanModeSmart, manager.mode)
	assert.NotNil(t, manager.scannedPatterns)
	assert.NotNil(t, manager.scannedExactMessages)
}

func TestShouldScanMessage_FuzzMode(t *testing.T) {
	manager := NewWebSocketDeduplicationManager(options.ScanModeFuzz)

	message := &db.WebSocketMessage{
		PayloadData: `{"action": "test", "value": 123}`,
		Opcode:      1,
	}

	// In fuzz mode, should always return true
	assert.True(t, manager.ShouldScanMessage(1, testEndpoint, message))

	// Mark as scanned
	manager.MarkMessageAsScanned(1, testEndpoint, message)

	// Should still return true in fuzz mode
	assert.True(t, manager.ShouldScanMessage(1, testEndpoint, message))
	assert.True(t, manager.ShouldScanMessage(2, testEndpoint, message))
}

func TestShouldScanMessage_FastMode(t *testing.T) {
	manager := NewWebSocketDeduplicationManager(options.ScanModeFast)

	message := &db.WebSocketMessage{
		PayloadData: `{"action": "test", "value": 123}`,
		Opcode:      1,
	}

	// First time should return true
	assert.True(t, manager.ShouldScanMessage(1, testEndpoint, message))

	// Mark as scanned
	manager.MarkMessageAsScanned(1, testEndpoint, message)

	// In fast mode, exact duplicate should return false
	assert.False(t, manager.ShouldScanMessage(2, testEndpoint, message))

	// Different message with same pattern should also return false
	similarMessage := &db.WebSocketMessage{
		PayloadData: `{"action": "test", "value": 456}`,
		Opcode:      1,
	}
	assert.False(t, manager.ShouldScanMessage(3, testEndpoint, similarMessage))
}

func TestShouldScanMessage_SmartMode(t *testing.T) {
	manager := NewWebSocketDeduplicationManager(options.ScanModeSmart)

	message := &db.WebSocketMessage{
		PayloadData: `{"action": "test", "value": 123}`,
		Opcode:      1,
	}

	// First connection should scan
	assert.True(t, manager.ShouldScanMessage(1, testEndpoint, message))
	manager.MarkMessageAsScanned(1, testEndpoint, message)

	// Second connection should scan (smart mode allows 2 exact duplicates)
	assert.True(t, manager.ShouldScanMessage(2, testEndpoint, message))
	manager.MarkMessageAsScanned(2, testEndpoint, message)

	// Third connection should NOT scan (exceeded limit)
	assert.False(t, manager.ShouldScanMessage(3, testEndpoint, message))

	// Test pattern-based deduplication with a different message that has same pattern
	similarMessage := &db.WebSocketMessage{
		PayloadData: `{"action": "test", "value": 456}`,
		Opcode:      1,
	}

	// Should allow up to 3 connections with same pattern (but first two exact messages count towards pattern limit)
	assert.True(t, manager.ShouldScanMessage(4, testEndpoint, similarMessage))
	manager.MarkMessageAsScanned(4, testEndpoint, similarMessage)

	// Fourth pattern match should be skipped (we already have 3: conn1, conn2, conn4)
	assert.False(t, manager.ShouldScanMessage(5, testEndpoint, similarMessage))
}

func TestMarkMessageAsScanned(t *testing.T) {
	manager := NewWebSocketDeduplicationManager(options.ScanModeSmart)

	message := &db.WebSocketMessage{
		PayloadData: `{"test": "data"}`,
		Opcode:      1,
	}

	// Mark message for connection 1
	manager.MarkMessageAsScanned(1, testEndpoint, message)

	// Verify it's tracked
	exactHash := manager.hashExactMessage(testEndpoint, message)
	patternHash := manager.hashMessagePattern(testEndpoint, message)

	assert.Contains(t, manager.scannedExactMessages[exactHash], uint(1))
	assert.Contains(t, manager.scannedPatterns[patternHash], uint(1))

	// Mark same message for connection 2
	manager.MarkMessageAsScanned(2, testEndpoint, message)

	assert.Len(t, manager.scannedExactMessages[exactHash], 2)
	assert.Contains(t, manager.scannedExactMessages[exactHash], uint(2))

	// Marking same connection again shouldn't duplicate
	manager.MarkMessageAsScanned(1, testEndpoint, message)
	assert.Len(t, manager.scannedExactMessages[exactHash], 2)
}

func TestNormalizeJSONStructure(t *testing.T) {
	manager := NewWebSocketDeduplicationManager(options.ScanModeSmart)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple object",
			input:    `{"name": "John", "age": 30}`,
			expected: `{"age":"<number>","name":"<string>"}`,
		},
		{
			name:     "Nested object",
			input:    `{"user": {"id": 123, "name": "test"}, "active": true}`,
			expected: `{"active":"<bool>","user":"{\"id\":\"<number>\",\"name\":\"<string>\"}"}`,
		},
		{
			name:     "Array",
			input:    `{"items": [1, 2, 3], "empty": []}`,
			expected: `{"empty":"[]","items":"[<number>...]"}`,
		},
		{
			name:     "Mixed types",
			input:    `{"str": "test", "num": 42, "bool": false, "null": null}`,
			expected: `{"bool":"<bool>","null":"<null>","num":"<number>","str":"<string>"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := &db.WebSocketMessage{
				PayloadData: tt.input,
				Opcode:      1,
			}
			pattern := manager.extractMessagePattern(message)
			assert.Equal(t, tt.expected, pattern.Structure)
		})
	}
}

func TestNormalizeTextStructure(t *testing.T) {
	manager := NewWebSocketDeduplicationManager(options.ScanModeSmart)

	tests := []struct {
		name        string
		input       string
		shouldMatch bool
		similar     string
	}{
		{
			name:        "UUID replacement",
			input:       "Session: 550e8400-e29b-41d4-a716-446655440000",
			similar:     "Session: 660e9500-f39c-52e5-b827-557766551111",
			shouldMatch: true,
		},
		{
			name:        "Timestamp replacement",
			input:       "Time: 2024-01-15T10:30:00Z",
			similar:     "Time: 2024-02-20T15:45:00Z",
			shouldMatch: true,
		},
		{
			name:        "Number replacement",
			input:       "Count: 42, Price: 99.99",
			similar:     "Count: 100, Price: 150.50",
			shouldMatch: true,
		},
		{
			name:        "Email replacement",
			input:       "Contact: user@example.com",
			similar:     "Contact: admin@test.org",
			shouldMatch: true,
		},
		{
			name:        "Short message",
			input:       "ping",
			similar:     "ping",
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg1 := &db.WebSocketMessage{PayloadData: tt.input, Opcode: 1}
			msg2 := &db.WebSocketMessage{PayloadData: tt.similar, Opcode: 1}

			pattern1 := manager.extractMessagePattern(msg1)
			pattern2 := manager.extractMessagePattern(msg2)

			if tt.shouldMatch {
				assert.Equal(t, pattern1.Structure, pattern2.Structure)
			} else {
				assert.NotEqual(t, pattern1.Structure, pattern2.Structure)
			}
		})
	}
}

func TestGetStatistics(t *testing.T) {
	manager := NewWebSocketDeduplicationManager(options.ScanModeSmart)

	// Add some messages
	msg1 := &db.WebSocketMessage{PayloadData: `{"type": "ping"}`, Opcode: 1}
	msg2 := &db.WebSocketMessage{PayloadData: `{"type": "ping"}`, Opcode: 1}
	msg3 := &db.WebSocketMessage{PayloadData: `{"action": "test", "value": 123}`, Opcode: 1}

	manager.MarkMessageAsScanned(1, testEndpoint, msg1)
	manager.MarkMessageAsScanned(2, testEndpoint, msg1) // Same exact message, different connection
	manager.MarkMessageAsScanned(3, testEndpoint, msg2) // Same pattern as msg1
	manager.MarkMessageAsScanned(4, testEndpoint, msg3) // Different message with different pattern

	stats := manager.GetStatistics()

	assert.Equal(t, "smart", stats["mode"])
	assert.Equal(t, 2, stats["unique_patterns"])
	assert.Equal(t, 2, stats["exact_messages"])
	assert.GreaterOrEqual(t, stats["estimated_skipped"].(int), 1)
}

func TestConcurrentAccess(t *testing.T) {
	manager := NewWebSocketDeduplicationManager(options.ScanModeSmart)

	message := &db.WebSocketMessage{
		PayloadData: `{"concurrent": "test"}`,
		Opcode:      1,
	}

	// Test concurrent reads and writes
	done := make(chan bool)

	// Multiple goroutines marking messages
	for i := 0; i < 10; i++ {
		go func(connID uint) {
			manager.MarkMessageAsScanned(connID, testEndpoint, message)
			done <- true
		}(uint(i))
	}

	// Multiple goroutines checking if should scan
	for i := 0; i < 10; i++ {
		go func(connID uint) {
			manager.ShouldScanMessage(connID, testEndpoint, message)
			done <- true
		}(uint(i + 10))
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify state is consistent
	stats := manager.GetStatistics()
	assert.NotNil(t, stats)
}

func TestDifferentOpcodes(t *testing.T) {
	manager := NewWebSocketDeduplicationManager(options.ScanModeFast)

	textMessage := &db.WebSocketMessage{
		PayloadData: "Hello",
		Opcode:      1, // Text
	}

	binaryMessage := &db.WebSocketMessage{
		PayloadData: "Hello",
		Opcode:      2, // Binary
	}

	// Same payload but different opcodes should be treated as different
	assert.True(t, manager.ShouldScanMessage(1, testEndpoint, textMessage))
	manager.MarkMessageAsScanned(1, testEndpoint, textMessage)

	assert.True(t, manager.ShouldScanMessage(2, testEndpoint, binaryMessage))
}

// TestShouldScanMessage_DistinctEndpointsNotDeduplicated encodes the ws-sqli
// case: ten different WebSocket endpoints (/raw, /raw/safe and eight /trap/*
// arms) all send the identical baseline frame {"id":"1"}. Deduplication is
// meant to stop rescanning the SAME endpoint reached over repeated
// connections - it must not make distinct endpoints compete for the same
// budget, or most endpoints are silently never scanned.
func TestShouldScanMessage_DistinctEndpointsNotDeduplicated(t *testing.T) {
	manager := NewWebSocketDeduplicationManager(options.ScanModeSmart)

	message := &db.WebSocketMessage{PayloadData: `{"id":"1"}`, Opcode: 1}

	endpoints := []string{
		"ws://127.0.0.1:20300/raw",
		"ws://127.0.0.1:20300/raw/safe",
		"ws://127.0.0.1:20300/trap/reflect",
		"ws://127.0.0.1:20300/trap/echo-error",
		"ws://127.0.0.1:20300/trap/always-true",
		"ws://127.0.0.1:20300/trap/heartbeat",
		"ws://127.0.0.1:20300/trap/broadcast",
		"ws://127.0.0.1:20300/trap/varying-broadcast",
		"ws://127.0.0.1:20300/trap/slow",
		"ws://127.0.0.1:20300/trap/silent",
	}

	for i, endpoint := range endpoints {
		connID := uint(i + 1)
		assert.True(t, manager.ShouldScanMessage(connID, endpoint, message),
			"endpoint %s must be scannable even though earlier endpoints sent the same frame", endpoint)
		manager.MarkMessageAsScanned(connID, endpoint, message)
	}
}

// TestShouldScanMessage_SameEndpointStillDeduplicated is the control: repeated
// connections to the SAME endpoint carrying the same frame must still be
// capped, which is the behaviour deduplication exists to provide.
func TestShouldScanMessage_SameEndpointStillDeduplicated(t *testing.T) {
	manager := NewWebSocketDeduplicationManager(options.ScanModeSmart)

	message := &db.WebSocketMessage{PayloadData: `{"id":"1"}`, Opcode: 1}
	const endpoint = "ws://127.0.0.1:20300/raw"

	assert.True(t, manager.ShouldScanMessage(1, endpoint, message))
	manager.MarkMessageAsScanned(1, endpoint, message)

	assert.True(t, manager.ShouldScanMessage(2, endpoint, message))
	manager.MarkMessageAsScanned(2, endpoint, message)

	assert.False(t, manager.ShouldScanMessage(3, endpoint, message),
		"a third connection to the same endpoint with the same frame should be skipped")
}

// TestShouldScanMessage_QueryStringIgnored ensures per-connection query noise
// (session tokens and similar) does not defeat deduplication for one endpoint.
func TestShouldScanMessage_QueryStringIgnored(t *testing.T) {
	manager := NewWebSocketDeduplicationManager(options.ScanModeSmart)

	message := &db.WebSocketMessage{PayloadData: `{"id":"1"}`, Opcode: 1}

	manager.MarkMessageAsScanned(1, "ws://127.0.0.1:20300/ws?token=aaa", message)
	manager.MarkMessageAsScanned(2, "ws://127.0.0.1:20300/ws?token=bbb", message)

	assert.False(t, manager.ShouldScanMessage(3, "ws://127.0.0.1:20300/ws?token=ccc", message),
		"the same endpoint with a different query string must still deduplicate")
}

func TestSentFramesSignature(t *testing.T) {
	frameA := []db.WebSocketMessage{{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"id":"1"}`}}
	frameAdup := []db.WebSocketMessage{{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"id":"1"}`}}
	assert.Equal(t, SentFramesSignature(frameA), SentFramesSignature(frameAdup),
		"byte-identical frames must share a signature")

	// The key behaviour of the fix: different VALUES in the same JSON shape must
	// produce DIFFERENT signatures so distinct operations are not collapsed.
	frameDifferentValue := []db.WebSocketMessage{{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"id":"2"}`}}
	assert.NotEqual(t, SentFramesSignature(frameA), SentFramesSignature(frameDifferentValue),
		"same shape, different value must NOT share a signature (multiplexed recall)")

	actionExec := []db.WebSocketMessage{{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"cmd":"exec","arg":"x"}`}}
	actionLookup := []db.WebSocketMessage{{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"cmd":"lookup","arg":"x"}`}}
	assert.NotEqual(t, SentFramesSignature(actionExec), SentFramesSignature(actionLookup),
		"distinct action/cmd values must NOT collapse")

	withReceived := []db.WebSocketMessage{
		{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"id":"1"}`},
		{Direction: db.MessageReceived, Opcode: 1, PayloadData: `{"unrelated":"x"}`},
	}
	assert.Equal(t, SentFramesSignature(frameA), SentFramesSignature(withReceived),
		"received frames must be ignored")

	assert.Equal(t, "no-client-frames", SentFramesSignature(nil))
	assert.Equal(t, "no-client-frames", SentFramesSignature([]db.WebSocketMessage{{Direction: db.MessageReceived, PayloadData: "x"}}))

	setOrderA := []db.WebSocketMessage{
		{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"a":"1"}`},
		{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"b":"2"}`},
	}
	setOrderB := []db.WebSocketMessage{
		{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"b":"2"}`},
		{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"a":"1"}`},
	}
	assert.Equal(t, SentFramesSignature(setOrderA), SentFramesSignature(setOrderB),
		"the distinct set of frames must be order-independent")

	// A payload containing the internal delimiter bytes must not let a single
	// crafted frame collide with a two-frame set (length-prefixed hashing).
	collidingSingle := []db.WebSocketMessage{{Direction: db.MessageSent, Opcode: 1, PayloadData: "X\x001\x1fY"}}
	twoFrames := []db.WebSocketMessage{
		{Direction: db.MessageSent, Opcode: 1, PayloadData: "X"},
		{Direction: db.MessageSent, Opcode: 1, PayloadData: "Y"},
	}
	assert.NotEqual(t, SentFramesSignature(collidingSingle), SentFramesSignature(twoFrames),
		"delimiter bytes in a payload must not collapse distinct frame sets")
}

func TestEndpointScanKey_QueryIgnoredSameContent(t *testing.T) {
	msgs := []db.WebSocketMessage{{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"id":"1"}`}}
	assert.Equal(t,
		EndpointScanKey("ws://host/chat", msgs),
		EndpointScanKey("ws://host/chat?token=abc", msgs),
		"per-connection query tokens must not create a distinct scan key")
	assert.NotEqual(t,
		EndpointScanKey("ws://host/chat", msgs),
		EndpointScanKey("ws://host/notify", msgs),
		"distinct endpoints must have distinct scan keys")
}

func TestDeduplicateWebSocketConnectionsForScheduling(t *testing.T) {
	// Two byte-identical captures of /chat (conn 1,2 and 3-with-query), plus a
	// DISTINCT action value on /chat (conn 5), plus a different endpoint (4).
	conns := []db.WebSocketConnection{
		{BaseModel: db.BaseModel{ID: 1}, URL: "ws://host/chat"},
		{BaseModel: db.BaseModel{ID: 2}, URL: "ws://host/chat"},
		{BaseModel: db.BaseModel{ID: 3}, URL: "ws://host/chat?token=abc"},
		{BaseModel: db.BaseModel{ID: 4}, URL: "ws://host/notify"},
		{BaseModel: db.BaseModel{ID: 5}, URL: "ws://host/chat"},
	}
	msgs := map[uint][]db.WebSocketMessage{
		1: {{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"cmd":"exec"}`}},
		2: {{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"cmd":"exec"}`}}, // identical to 1 -> collapse
		3: {{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"cmd":"exec"}`}}, // identical (query dropped) -> collapse
		4: {{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"cmd":"exec"}`}}, // different endpoint -> keep
		5: {{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"cmd":"lookup"}`}}, // distinct action on /chat -> keep
	}
	got := DeduplicateWebSocketConnectionsForScheduling(conns, msgs, options.ScanModeSmart)
	assert.ElementsMatch(t, []uint{1, 4, 5}, got,
		"collapse identical /chat exec captures; keep distinct /chat lookup and the notify endpoint")
}

func TestDeduplicate_FuzzModeSchedulesAll(t *testing.T) {
	// In Fuzz mode even byte-identical captures must all be scheduled.
	conns := []db.WebSocketConnection{
		{BaseModel: db.BaseModel{ID: 1}, URL: "ws://host/chat"},
		{BaseModel: db.BaseModel{ID: 2}, URL: "ws://host/chat"},
		{BaseModel: db.BaseModel{ID: 3}, URL: "ws://host/chat"},
	}
	msgs := map[uint][]db.WebSocketMessage{
		1: {{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"cmd":"exec"}`}},
		2: {{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"cmd":"exec"}`}},
		3: {{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"cmd":"exec"}`}},
	}
	got := DeduplicateWebSocketConnectionsForScheduling(conns, msgs, options.ScanModeFuzz)
	assert.ElementsMatch(t, []uint{1, 2, 3}, got, "Fuzz mode must schedule every connection")
}

func TestDeduplicate_FramelessDroppedWhenEndpointHasFrames(t *testing.T) {
	conns := []db.WebSocketConnection{
		{BaseModel: db.BaseModel{ID: 1}, URL: "ws://host/ws"},
		{BaseModel: db.BaseModel{ID: 2}, URL: "ws://host/ws"},
	}
	msgs := map[uint][]db.WebSocketMessage{
		1: {{Direction: db.MessageSent, Opcode: 1, PayloadData: `{"id":"1"}`}},
	}
	got := DeduplicateWebSocketConnectionsForScheduling(conns, msgs, options.ScanModeSmart)
	assert.Equal(t, []uint{1}, got, "frameless capture dropped when endpoint has a frame-bearing one")
}

func TestDeduplicate_FramelessKeptWhenEndpointHasNoFrames(t *testing.T) {
	conns := []db.WebSocketConnection{
		{BaseModel: db.BaseModel{ID: 1}, URL: "ws://host/ws"},
		{BaseModel: db.BaseModel{ID: 2}, URL: "ws://host/ws"},
	}
	msgs := map[uint][]db.WebSocketMessage{}
	got := DeduplicateWebSocketConnectionsForScheduling(conns, msgs, options.ScanModeSmart)
	assert.Equal(t, []uint{1}, got, "one frameless job kept for endpoint-level CSWSH/passive")
}
