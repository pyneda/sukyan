// Package wsreplay provides the WebSocket replay engine for the playground:
// session lifecycle, script-driven runs, and a control-event broadcaster.
package wsreplay

import (
	"encoding/json"
	"time"
)

// SessionState describes the lifecycle of an upstream WS connection.
// "disconnected" is the resting state both before connect and after close —
// chosen over the spec's "closed" because it reads better in UI status badges
// and avoids needing two terminal states to mean the same thing.
type SessionState string

const (
	StateDisconnected SessionState = "disconnected"
	StateConnecting   SessionState = "connecting"
	StateConnected    SessionState = "connected"
	StateClosing      SessionState = "closing"
	StateErrored      SessionState = "errored"
)

// Instance kinds.
const (
	InstanceKindInteractive = "interactive"
	InstanceKindRun         = "run"
)

// On-failure policies for run script entries.
const (
	PolicyAbort    = "abort"
	PolicyContinue = "continue"
)

// WaitFor match types.
const (
	MatchAny      = "any"
	MatchContains = "contains"
	MatchRegex    = "regex"
	MatchJSONPath = "json_path"
)

// Instance distinguishes the interactive socket from per-run sockets within a single session.
type Instance struct {
	Kind  string `json:"kind"` // "interactive" or "run"
	RunID uint   `json:"run_id,omitempty"`
}

func InteractiveInstance() Instance   { return Instance{Kind: InstanceKindInteractive} }
func RunInstance(runID uint) Instance { return Instance{Kind: InstanceKindRun, RunID: runID} }

// IsInteractive reports whether the instance is the long-lived interactive socket.
func (i Instance) IsInteractive() bool { return i.Kind == InstanceKindInteractive }

// IsRun reports whether the instance is a per-run socket.
func (i Instance) IsRun() bool { return i.Kind == InstanceKindRun }

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

// SnapshotInteractive describes the interactive socket's state in a snapshot frame.
type SnapshotInteractive struct {
	State                 SessionState `json:"state"`
	WebSocketConnectionID *uint        `json:"websocket_connection_id,omitempty"`
}

// Snapshot is the first frame delivered to a new control-WS subscriber.
// Wire format: when emitted on the control WS, it is wrapped as Event{Type: "snapshot", Data: <Snapshot JSON>}.
type Snapshot struct {
	Interactive SnapshotInteractive `json:"interactive"`
	ActiveRuns  []ActiveRunSummary  `json:"active_runs"`
	LastSeq     int64               `json:"last_seq"`
}

type ActiveRunSummary struct {
	RunID                 uint   `json:"run_id"`
	Status                string `json:"status"`
	CurrentStepIndex      *int   `json:"current_step_index,omitempty"`
	WebSocketConnectionID *uint  `json:"websocket_connection_id,omitempty"`
}
