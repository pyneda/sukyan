package wsreplay

import (
	"encoding/json"
	"time"
)

// SessionState describes the lifecycle of an upstream WS connection.
type SessionState string

const (
	StateDisconnected SessionState = "disconnected"
	StateConnecting   SessionState = "connecting"
	StateConnected    SessionState = "connected"
	StateClosing      SessionState = "closing"
	StateErrored      SessionState = "errored"
)

// Instance distinguishes the interactive socket from per-run sockets within a single session.
type Instance struct {
	Kind  string `json:"kind"` // "interactive" or "run"
	RunID uint   `json:"run_id,omitempty"`
}

func InteractiveInstance() Instance   { return Instance{Kind: "interactive"} }
func RunInstance(runID uint) Instance { return Instance{Kind: "run", RunID: runID} }

// ScriptEntry mirrors the frontend zod schema. JSON-encoded in DB.
type ScriptEntry struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Content   string       `json:"content"`
	Opcode    int          `json:"opcode"` // 1 text, 2 binary
	DelayMs   int          `json:"delay_ms"`
	WaitFor   *WaitForSpec `json:"wait_for,omitempty"`
	OnTimeout string       `json:"on_timeout"`  // "abort" | "continue"
	OnNoMatch string       `json:"on_no_match"` // "abort" | "continue"
}

type WaitForSpec struct {
	MatchType string `json:"match_type"` // any | contains | regex | json_path
	Pattern   string `json:"pattern"`
	TimeoutMs int    `json:"timeout_ms"`
}

type SessionOptions struct {
	ConnectionTimeoutMs int `json:"connection_timeout_ms"`
	SendTimeoutMs       int `json:"send_timeout_ms"`
	InterStepDelayMs    int `json:"inter_step_delay_ms"`
}

// HeaderSpec is one row of the request_headers JSON.
type HeaderSpec struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

// Event is the discriminated union sent over the control WS.
type Event struct {
	Type     string          `json:"type"`
	Seq      int64           `json:"seq"`
	Instance Instance        `json:"instance"`
	Data     json.RawMessage `json:"data,omitempty"`
	Ts       time.Time       `json:"ts"`
}

// Snapshot is the first frame delivered to a new control-WS subscriber.
type Snapshot struct {
	Interactive struct {
		State                 SessionState `json:"state"`
		WebSocketConnectionID *uint        `json:"websocket_connection_id,omitempty"`
	} `json:"interactive"`
	ActiveRuns []ActiveRunSummary `json:"active_runs"`
	LastSeq    int64              `json:"last_seq"`
}

type ActiveRunSummary struct {
	RunID                 uint   `json:"run_id"`
	Status                string `json:"status"`
	CurrentStepIndex      *int   `json:"current_step_index,omitempty"`
	WebSocketConnectionID *uint  `json:"websocket_connection_id,omitempty"`
}
