package http_utils

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/scan/options"
)

type WebSocketMessagePattern struct {
	Structure   string
	MessageType string
}

type WebSocketDeduplicationManager struct {
	mu                   sync.RWMutex
	scannedPatterns      map[string][]uint
	scannedExactMessages map[string][]uint
	mode                 options.ScanMode
}

func NewWebSocketDeduplicationManager(mode options.ScanMode) *WebSocketDeduplicationManager {
	return &WebSocketDeduplicationManager{
		scannedPatterns:      make(map[string][]uint),
		scannedExactMessages: make(map[string][]uint),
		mode:                 mode,
	}
}

const SmartModeMaxRepeatedExactMessages = 2
const SmartModeMaxRepeatedPatterns = 3

func (m *WebSocketDeduplicationManager) ShouldScanMessage(connectionID uint, message *db.WebSocketMessage) bool {
	if m.mode == options.ScanModeFuzz {
		return true
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	exactHash := m.hashExactMessage(message)
	if connections, exists := m.scannedExactMessages[exactHash]; exists {
		if m.mode == options.ScanModeFast {
			return false
		}
		if m.mode == options.ScanModeSmart && len(connections) >= SmartModeMaxRepeatedExactMessages {
			return false
		}
	}

	patternHash := m.hashMessagePattern(message)
	if connections, exists := m.scannedPatterns[patternHash]; exists {
		if m.mode == options.ScanModeFast {
			return false
		}
		if m.mode == options.ScanModeSmart && len(connections) >= SmartModeMaxRepeatedPatterns {
			return false
		}
	}

	return true
}

func (m *WebSocketDeduplicationManager) MarkMessageAsScanned(connectionID uint, message *db.WebSocketMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()

	exactHash := m.hashExactMessage(message)
	if _, exists := m.scannedExactMessages[exactHash]; !exists {
		m.scannedExactMessages[exactHash] = make([]uint, 0)
	}
	if !lib.SliceContainsUint(m.scannedExactMessages[exactHash], connectionID) {
		m.scannedExactMessages[exactHash] = append(m.scannedExactMessages[exactHash], connectionID)
	}

	patternHash := m.hashMessagePattern(message)
	if _, exists := m.scannedPatterns[patternHash]; !exists {
		m.scannedPatterns[patternHash] = make([]uint, 0)
	}
	if !lib.SliceContainsUint(m.scannedPatterns[patternHash], connectionID) {
		m.scannedPatterns[patternHash] = append(m.scannedPatterns[patternHash], connectionID)
	}
}

func (m *WebSocketDeduplicationManager) GetStatistics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	totalPatterns := len(m.scannedPatterns)
	totalExactMessages := len(m.scannedExactMessages)
	totalSkipped := 0

	for _, connections := range m.scannedPatterns {
		if len(connections) > 1 {
			totalSkipped += len(connections) - 1
		}
	}

	return map[string]interface{}{
		"mode":              m.mode.String(),
		"unique_patterns":   totalPatterns,
		"exact_messages":    totalExactMessages,
		"estimated_skipped": totalSkipped,
	}
}

func (m *WebSocketDeduplicationManager) hashExactMessage(message *db.WebSocketMessage) string {
	h := sha256.New()
	h.Write([]byte(message.PayloadData))
	h.Write([]byte(fmt.Sprintf(":%d", int(message.Opcode))))
	return hex.EncodeToString(h.Sum(nil))
}

func (m *WebSocketDeduplicationManager) hashMessagePattern(message *db.WebSocketMessage) string {
	pattern := m.extractMessagePattern(message)
	h := sha256.New()
	h.Write([]byte(pattern.Structure))
	h.Write([]byte(pattern.MessageType))
	return hex.EncodeToString(h.Sum(nil))
}

func (m *WebSocketDeduplicationManager) extractMessagePattern(message *db.WebSocketMessage) WebSocketMessagePattern {
	pattern := WebSocketMessagePattern{
		MessageType: fmt.Sprintf("%d", int(message.Opcode)),
	}

	var jsonData interface{}
	if err := json.Unmarshal([]byte(message.PayloadData), &jsonData); err == nil {
		pattern.Structure = m.normalizeJSONStructure(jsonData)
		return pattern
	}

	pattern.Structure = m.normalizeTextStructure(message.PayloadData)
	return pattern
}

func (m *WebSocketDeduplicationManager) normalizeJSONStructure(data interface{}) string {
	switch v := data.(type) {
	case map[string]interface{}:
		normalized := make(map[string]string)
		for key, value := range v {
			normalized[key] = m.normalizeJSONStructure(value)
		}
		// Use a buffer to avoid HTML escaping
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		encoder.Encode(normalized)
		result := buf.String()
		// Remove the trailing newline that Encode adds
		return strings.TrimSuffix(result, "\n")
	case []interface{}:
		if len(v) > 0 {
			return fmt.Sprintf("[%s...]", m.normalizeJSONStructure(v[0]))
		}
		return "[]"
	case string:
		return "<string>"
	case float64:
		return "<number>"
	case bool:
		return "<bool>"
	case nil:
		return "<null>"
	default:
		return "<unknown>"
	}
}

func (m *WebSocketDeduplicationManager) normalizeTextStructure(text string) string {
	normalized := text

	uuidRegex := regexp.MustCompile(`[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}`)
	normalized = uuidRegex.ReplaceAllString(normalized, "<uuid>")

	timestampRegexes := []*regexp.Regexp{
		regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T\s]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?`),
		regexp.MustCompile(`\d{1,2}/\d{1,2}/\d{2,4}\s+\d{1,2}:\d{2}:\d{2}`),
		regexp.MustCompile(`\d{10,13}`),
	}
	for _, re := range timestampRegexes {
		normalized = re.ReplaceAllString(normalized, "<timestamp>")
	}

	numberRegex := regexp.MustCompile(`\b\d+\.?\d*\b`)
	normalized = numberRegex.ReplaceAllString(normalized, "<number>")

	hexRegex := regexp.MustCompile(`\b[a-fA-F0-9]{16,}\b`)
	normalized = hexRegex.ReplaceAllString(normalized, "<token>")

	emailRegex := regexp.MustCompile(`[\w\.-]+@[\w\.-]+\.\w+`)
	normalized = emailRegex.ReplaceAllString(normalized, "<email>")

	urlRegex := regexp.MustCompile(`https?://[^\s<>"{}|\\^` + "`" + `\[\]]+`)
	normalized = urlRegex.ReplaceAllString(normalized, "<url>")

	// For very short messages, just return the normalized text itself
	if len(normalized) < 200 {
		return normalized
	}

	// For longer messages, create a pattern signature
	// Include length bucket to differentiate between messages of very different sizes
	lengthBucket := (len(text) / 100) * 100
	pattern := fmt.Sprintf("len:%d|%s", lengthBucket, normalized)

	h := sha256.New()
	h.Write([]byte(pattern))
	return hex.EncodeToString(h.Sum(nil))
}
