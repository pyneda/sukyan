package wsfuzz

import "time"

// EventType enumerates wsfuzz live stream events.
type EventType string

const (
	EventSnapshot    EventType = "snapshot"
	EventCalibrating EventType = "calibrating"
	EventBaseline    EventType = "baseline"
	EventStatus      EventType = "status"
	EventProgress    EventType = "progress"
	EventResult      EventType = "result"
	EventDone        EventType = "done"
	EventError       EventType = "error"
	EventWarning     EventType = "warning"
)

// WsFuzzEvent is the envelope pushed to broadcaster subscribers. Implements
// stream.Sequenced via the GetSeq/SetSeq method pair on the pointer receiver.
type WsFuzzEvent struct {
	Type     EventType              `json:"type"`
	Seq      int64                  `json:"seq"`
	RunID    uint                   `json:"run_id"`
	Ts       time.Time              `json:"ts"`
	Result   *WsIterationResult     `json:"result,omitempty"`
	Progress *WsFuzzProgress        `json:"progress,omitempty"`
	Status   *WsFuzzStatus          `json:"status,omitempty"`
	Baseline *WsBaselineFingerprint `json:"baseline,omitempty"`
	Snapshot *WsFuzzSnapshot        `json:"snapshot,omitempty"`
	Done     *WsFuzzDone            `json:"done,omitempty"`
	Error    *WsFuzzError           `json:"error,omitempty"`
	Warning  *WsFuzzWarning         `json:"warning,omitempty"`
}

// GetSeq satisfies stream.Sequenced.
func (e *WsFuzzEvent) GetSeq() int64 { return e.Seq }

// SetSeq satisfies stream.Sequenced. Called by the broadcaster on Publish.
func (e *WsFuzzEvent) SetSeq(s int64) { e.Seq = s }

// WsFuzzProgress is the per-tick projection of run progress.
type WsFuzzProgress struct {
	Sent               int     `json:"sent"`
	Errors             int     `json:"errors"`
	Findings           int     `json:"findings"`
	InFlight           int     `json:"in_flight"`
	PlannedIterations  int     `json:"planned_iterations"`
	CurrentRate        float64 `json:"current_rate"` // iterations/sec
	ElapsedSeconds     int     `json:"elapsed_seconds"`
	EstimatedRemaining int     `json:"estimated_remaining,omitempty"`
}

// WsFuzzStatus is a lifecycle transition.
type WsFuzzStatus struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason,omitempty"`
}

// WsFuzzSnapshot is delivered first on subscribe.
type WsFuzzSnapshot struct {
	RunID             uint      `json:"run_id"`
	Status            string    `json:"status"`
	PlannedIterations int       `json:"planned_iterations"`
	StartedAt         time.Time `json:"started_at"`
	LastSeq           int64     `json:"last_seq"`
}

// WsFuzzDone is the terminal event.
type WsFuzzDone struct {
	Status          string    `json:"status"`
	Sent            int       `json:"sent"`
	Errors          int       `json:"errors"`
	Findings        int       `json:"findings"`
	DurationSeconds int       `json:"duration_seconds"`
	FailureReason   string    `json:"failure_reason,omitempty"`
	FinishedAt      time.Time `json:"finished_at"`
}

// WsFuzzError is a non-terminal error.
type WsFuzzError struct {
	Message string `json:"message"`
}

// WsFuzzWarning carries calibration / substitution warnings.
type WsFuzzWarning struct {
	Code    string `json:"code"` // e.g., "baseline_partial_nondeterminism"
	Message string `json:"message"`
	Context any    `json:"context,omitempty"`
}
