package db

// Source values for WebSocketConnection.Source. These mirror the existing
// History.Source taxonomy in db/history_sources.go but for WebSocket
// connections. Use these constants instead of string literals.
const (
	// SourcePlayground tags connections opened by the manual WS playground
	// or by the wsreplay engine (default).
	SourcePlayground = "playground"

	// SourceWsFuzz tags connections opened by per-iteration wsfuzz runs.
	// The captures UI excludes these by default.
	SourceWsFuzz = "ws_fuzz"
)
