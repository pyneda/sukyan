package db

// Source values for WebSocketConnection.Source. These mirror the existing
// History.Source taxonomy in db/history_sources.go but for WebSocket
// connections. Use these constants instead of string literals.
// SourcePlayground tags connections opened by the manual WS playground
// or by the wsreplay engine (default).
var SourcePlayground = "playground"

// SourceWsFuzz tags connections opened by per-iteration wsfuzz runs.
var SourceWsFuzz = "ws_fuzz"

// WebSocketSources is the full taxonomy a WebSocketConnection.Source may hold:
// the HTTP sources plus the two WebSocket-only ones. Note the mixed casing is
// load-bearing, these are the values written to the column.
var WebSocketSources = append(append([]string{}, Sources...), SourcePlayground, SourceWsFuzz)

// IsValidWebSocketSource reports whether source is a known WebSocket connection
// source. IsValidSource covers only the HTTP taxonomy and so rejects the
// WebSocket-only values.
func IsValidWebSocketSource(source string) bool {
	for _, s := range WebSocketSources {
		if s == source {
			return true
		}
	}
	return false
}
