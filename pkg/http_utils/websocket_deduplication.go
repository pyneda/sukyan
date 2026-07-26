package http_utils

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/scan/options"
)

// EndpointKey reduces a WebSocket URL to the endpoint identity used for
// deduplication: scheme, host and path. The query string is dropped so that
// per-connection noise (session tokens and similar) does not defeat
// deduplication for a single endpoint.
//
// Keying on the endpoint matters because deduplication exists to avoid
// rescanning the SAME endpoint reached over repeated connections. Without it,
// distinct endpoints that happen to share a frame shape - which is the norm for
// an app whose sockets all speak one protocol - compete for the same budget and
// most of them are silently never scanned.
func EndpointKey(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return rawURL
	}
	return parsed.Scheme + "://" + parsed.Host + parsed.Path
}

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

func (m *WebSocketDeduplicationManager) ShouldScanMessage(connectionID uint, connectionURL string, message *db.WebSocketMessage) bool {
	if m.mode == options.ScanModeFuzz {
		return true
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	exactHash := m.hashExactMessage(connectionURL, message)
	if connections, exists := m.scannedExactMessages[exactHash]; exists {
		if m.mode == options.ScanModeFast {
			return false
		}
		if m.mode == options.ScanModeSmart && len(connections) >= SmartModeMaxRepeatedExactMessages {
			return false
		}
	}

	patternHash := m.hashMessagePattern(connectionURL, message)
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

func (m *WebSocketDeduplicationManager) MarkMessageAsScanned(connectionID uint, connectionURL string, message *db.WebSocketMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()

	exactHash := m.hashExactMessage(connectionURL, message)
	if _, exists := m.scannedExactMessages[exactHash]; !exists {
		m.scannedExactMessages[exactHash] = make([]uint, 0)
	}
	if !lib.SliceContainsUint(m.scannedExactMessages[exactHash], connectionID) {
		m.scannedExactMessages[exactHash] = append(m.scannedExactMessages[exactHash], connectionID)
	}

	patternHash := m.hashMessagePattern(connectionURL, message)
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

func (m *WebSocketDeduplicationManager) hashExactMessage(connectionURL string, message *db.WebSocketMessage) string {
	h := sha256.New()
	h.Write([]byte(EndpointKey(connectionURL)))
	h.Write([]byte{0})
	h.Write([]byte(message.PayloadData))
	h.Write([]byte(fmt.Sprintf(":%d", int(message.Opcode))))
	return hex.EncodeToString(h.Sum(nil))
}

func (m *WebSocketDeduplicationManager) hashMessagePattern(connectionURL string, message *db.WebSocketMessage) string {
	pattern := extractMessagePattern(message)
	h := sha256.New()
	h.Write([]byte(EndpointKey(connectionURL)))
	h.Write([]byte{0})
	h.Write([]byte(pattern.Structure))
	h.Write([]byte(pattern.MessageType))
	return hex.EncodeToString(h.Sum(nil))
}

// extractMessagePattern is retained as a method for existing callers and tests;
// it delegates to the package-level function so the same normalization backs both
// runtime deduplication and scheduling-time deduplication.
func (m *WebSocketDeduplicationManager) extractMessagePattern(message *db.WebSocketMessage) WebSocketMessagePattern {
	return extractMessagePattern(message)
}

func extractMessagePattern(message *db.WebSocketMessage) WebSocketMessagePattern {
	pattern := WebSocketMessagePattern{
		MessageType: fmt.Sprintf("%d", int(message.Opcode)),
	}

	var jsonData interface{}
	if err := json.Unmarshal([]byte(message.PayloadData), &jsonData); err == nil {
		pattern.Structure = normalizeJSONStructure(jsonData)
		return pattern
	}

	pattern.Structure = normalizeTextStructure(message.PayloadData)
	return pattern
}

// SentFramesSignature returns a stable signature over the DISTINCT client->server
// frames of a connection, by EXACT content (opcode + payload). Crucially it does
// NOT normalize away values: two connections that differ only in a discriminator
// value - an "action"/"cmd" field or an id - get DISTINCT signatures and are both
// scheduled. This is the multiplexed-endpoint case (many operations over one
// /ws keyed by a JSON field) where a structural, value-erasing signature would
// collapse distinct operations and silently lose recall. Byte-identical captures
// (the same page re-crawled) still share a signature and collapse.
func SentFramesSignature(messages []db.WebSocketMessage) string {
	frames := make(map[string]struct{})
	for i := range messages {
		if messages[i].Direction != db.MessageSent {
			continue
		}
		frames[fmt.Sprintf("%d\x1f%s", int(messages[i].Opcode), messages[i].PayloadData)] = struct{}{}
	}
	if len(frames) == 0 {
		return "no-client-frames"
	}
	sorted := make([]string, 0, len(frames))
	for f := range frames {
		sorted = append(sorted, f)
	}
	sort.Strings(sorted)
	// Length-prefix each frame so the combined hash is injective even when a
	// (binary) payload contains the internal delimiters - otherwise a crafted
	// single frame could collide with a two-frame set and collapse distinct
	// connections, the exact recall loss this signature exists to prevent.
	h := sha256.New()
	var lenBuf [8]byte
	for _, f := range sorted {
		binary.BigEndian.PutUint64(lenBuf[:], uint64(len(f)))
		h.Write(lenBuf[:])
		h.Write([]byte(f))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// EndpointScanKey is the unit of WebSocket scan scheduling: the endpoint identity
// combined with its exact client-frame signature. Scheduling one job per unique
// key avoids re-scanning the same endpoint over byte-identical captured
// connections while still covering every distinct frame.
func EndpointScanKey(rawURL string, messages []db.WebSocketMessage) string {
	return EndpointKey(rawURL) + "\x00" + SentFramesSignature(messages)
}

// DeduplicateWebSocketConnectionsForScheduling returns the connection IDs to
// schedule so each (endpoint, client-frame shape) is scanned exactly once. A
// crawl captures the same socket once per page visit, so scheduling every
// connection would re-run passive + CSWSH + message dedup hundreds of times for
// work that is ultimately skipped. Keying on the frame shape preserves recall
// when different pages send genuinely different frames to the same endpoint.
//
// A frameless capture of an endpoint that also has a frame-bearing capture is
// dropped: its only value is the endpoint-level CSWSH/passive checks, which the
// frame-bearing job already performs. Input order determines which connection
// represents each key, so callers should pass a stable order.
//
// In Fuzz mode nothing is deduplicated: the user asked for an exhaustive scan, so
// every in-scope connection is scheduled and the runtime dedup (per message) is
// the only limiter. Deduplicating here would drop distinct frames the user
// explicitly wanted scanned.
func DeduplicateWebSocketConnectionsForScheduling(connections []db.WebSocketConnection, messagesByConn map[uint][]db.WebSocketMessage, mode options.ScanMode) []uint {
	if mode == options.ScanModeFuzz {
		ids := make([]uint, 0, len(connections))
		for i := range connections {
			ids = append(ids, connections[i].ID)
		}
		return ids
	}

	endpointHasFrames := make(map[string]bool)
	for i := range connections {
		if len(messagesByConn[connections[i].ID]) > 0 {
			endpointHasFrames[EndpointKey(connections[i].URL)] = true
		}
	}

	seen := make(map[string]bool)
	var scheduled []uint
	for i := range connections {
		conn := connections[i]
		msgs := messagesByConn[conn.ID]
		if len(msgs) == 0 && endpointHasFrames[EndpointKey(conn.URL)] {
			continue
		}
		key := EndpointScanKey(conn.URL, msgs)
		if seen[key] {
			continue
		}
		seen[key] = true
		scheduled = append(scheduled, conn.ID)
	}
	return scheduled
}

func normalizeJSONStructure(data interface{}) string {
	switch v := data.(type) {
	case map[string]interface{}:
		normalized := make(map[string]string)
		for key, value := range v {
			normalized[key] = normalizeJSONStructure(value)
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
			return fmt.Sprintf("[%s...]", normalizeJSONStructure(v[0]))
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

func normalizeTextStructure(text string) string {
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
