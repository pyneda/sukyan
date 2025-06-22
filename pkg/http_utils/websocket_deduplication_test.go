package http_utils

import (
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/stretchr/testify/assert"
)

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
	assert.True(t, manager.ShouldScanMessage(1, message))

	// Mark as scanned
	manager.MarkMessageAsScanned(1, message)

	// Should still return true in fuzz mode
	assert.True(t, manager.ShouldScanMessage(1, message))
	assert.True(t, manager.ShouldScanMessage(2, message))
}

func TestShouldScanMessage_FastMode(t *testing.T) {
	manager := NewWebSocketDeduplicationManager(options.ScanModeFast)

	message := &db.WebSocketMessage{
		PayloadData: `{"action": "test", "value": 123}`,
		Opcode:      1,
	}

	// First time should return true
	assert.True(t, manager.ShouldScanMessage(1, message))

	// Mark as scanned
	manager.MarkMessageAsScanned(1, message)

	// In fast mode, exact duplicate should return false
	assert.False(t, manager.ShouldScanMessage(2, message))

	// Different message with same pattern should also return false
	similarMessage := &db.WebSocketMessage{
		PayloadData: `{"action": "test", "value": 456}`,
		Opcode:      1,
	}
	assert.False(t, manager.ShouldScanMessage(3, similarMessage))
}

func TestShouldScanMessage_SmartMode(t *testing.T) {
	manager := NewWebSocketDeduplicationManager(options.ScanModeSmart)

	message := &db.WebSocketMessage{
		PayloadData: `{"action": "test", "value": 123}`,
		Opcode:      1,
	}

	// First connection should scan
	assert.True(t, manager.ShouldScanMessage(1, message))
	manager.MarkMessageAsScanned(1, message)

	// Second connection should scan (smart mode allows 2 exact duplicates)
	assert.True(t, manager.ShouldScanMessage(2, message))
	manager.MarkMessageAsScanned(2, message)

	// Third connection should NOT scan (exceeded limit)
	assert.False(t, manager.ShouldScanMessage(3, message))

	// Test pattern-based deduplication with a different message that has same pattern
	similarMessage := &db.WebSocketMessage{
		PayloadData: `{"action": "test", "value": 456}`,
		Opcode:      1,
	}

	// Should allow up to 3 connections with same pattern (but first two exact messages count towards pattern limit)
	assert.True(t, manager.ShouldScanMessage(4, similarMessage))
	manager.MarkMessageAsScanned(4, similarMessage)

	// Fourth pattern match should be skipped (we already have 3: conn1, conn2, conn4)
	assert.False(t, manager.ShouldScanMessage(5, similarMessage))
}

func TestMarkMessageAsScanned(t *testing.T) {
	manager := NewWebSocketDeduplicationManager(options.ScanModeSmart)

	message := &db.WebSocketMessage{
		PayloadData: `{"test": "data"}`,
		Opcode:      1,
	}

	// Mark message for connection 1
	manager.MarkMessageAsScanned(1, message)

	// Verify it's tracked
	exactHash := manager.hashExactMessage(message)
	patternHash := manager.hashMessagePattern(message)

	assert.Contains(t, manager.scannedExactMessages[exactHash], uint(1))
	assert.Contains(t, manager.scannedPatterns[patternHash], uint(1))

	// Mark same message for connection 2
	manager.MarkMessageAsScanned(2, message)

	assert.Len(t, manager.scannedExactMessages[exactHash], 2)
	assert.Contains(t, manager.scannedExactMessages[exactHash], uint(2))

	// Marking same connection again shouldn't duplicate
	manager.MarkMessageAsScanned(1, message)
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

	manager.MarkMessageAsScanned(1, msg1)
	manager.MarkMessageAsScanned(2, msg1) // Same exact message, different connection
	manager.MarkMessageAsScanned(3, msg2) // Same pattern as msg1
	manager.MarkMessageAsScanned(4, msg3) // Different message with different pattern

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
			manager.MarkMessageAsScanned(connID, message)
			done <- true
		}(uint(i))
	}

	// Multiple goroutines checking if should scan
	for i := 0; i < 10; i++ {
		go func(connID uint) {
			manager.ShouldScanMessage(connID, message)
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
	assert.True(t, manager.ShouldScanMessage(1, textMessage))
	manager.MarkMessageAsScanned(1, textMessage)

	assert.True(t, manager.ShouldScanMessage(2, binaryMessage))
}
