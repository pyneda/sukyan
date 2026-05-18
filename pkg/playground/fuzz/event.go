package fuzz

import (
	"time"
)

// FuzzEventType is the discriminator on the WS control stream. Mirrors the
// streaming spec exactly so the frontend can rely on these names.
type FuzzEventType string

const (
	FuzzEventSnapshot FuzzEventType = "snapshot"
	FuzzEventResult   FuzzEventType = "result"
	FuzzEventProgress FuzzEventType = "progress"
	FuzzEventStatus   FuzzEventType = "status"
	FuzzEventBaseline FuzzEventType = "baseline"
	FuzzEventDone     FuzzEventType = "done"
	FuzzEventError    FuzzEventType = "error"
)

// FuzzEvent is the envelope sent over the per-run broadcaster. Implements
// stream.Sequenced so it can flow through the shared broadcaster.
//
// Only one of Result/Progress/Status/Baseline/Done/Snapshot/Err is non-nil
// at a time; the wire format JSON-marshals the active one.
type FuzzEvent struct {
	Type     FuzzEventType  `json:"type"`
	Seq      int64          `json:"seq"`
	RunID    uint           `json:"run_id"`
	At       time.Time      `json:"ts"`
	Result   *FuzzResult    `json:"result,omitempty"`
	Progress *FuzzProgress  `json:"progress,omitempty"`
	Status   *FuzzStatusEv  `json:"status,omitempty"`
	Baseline *FuzzBaselineEv `json:"baseline,omitempty"`
	Done     *FuzzDoneEv    `json:"done,omitempty"`
	Snapshot *FuzzSnapshot  `json:"snapshot,omitempty"`
	Err      *FuzzErrorEv   `json:"error,omitempty"`
}

// GetSeq satisfies stream.Sequenced.
func (e *FuzzEvent) GetSeq() int64 { return e.Seq }

// SetSeq satisfies stream.Sequenced. Called by the broadcaster on Publish.
func (e *FuzzEvent) SetSeq(s int64) { e.Seq = s }

// FuzzProgress is the periodic progress tick.
type FuzzProgress struct {
	Sent               int     `json:"sent"`
	Errors             int     `json:"errors"`
	Planned            int     `json:"planned"`
	CurrentRPS         float64 `json:"current_rps"`
	ElapsedSeconds     int     `json:"elapsed_seconds"`
	EstimatedRemaining *int    `json:"estimated_remaining,omitempty"`
}

// FuzzStatusEv reports a lifecycle transition.
type FuzzStatusEv struct {
	From   string  `json:"from"`
	To     string  `json:"to"`
	Reason *string `json:"reason,omitempty"`
}

// FuzzBaselineEv is published during the calibrating phase; data shape is
// filled in by the baseline package (task 15).
type FuzzBaselineEv struct {
	Fingerprints any      `json:"fingerprints"`
	Warnings     []string `json:"warnings,omitempty"`
}

// FuzzDoneEv is the terminal event; emitted once before the broadcaster closes.
type FuzzDoneEv struct {
	FinalStatus     string  `json:"final_status"`
	TotalSent       int     `json:"total_sent"`
	TotalErrors     int     `json:"total_errors"`
	DurationSeconds int     `json:"duration_seconds"`
	Reason          *string `json:"reason,omitempty"`
}

// FuzzSnapshot is the first frame sent to a new subscriber.
type FuzzSnapshot struct {
	RunID               uint   `json:"run_id"`
	Status              string `json:"status"`
	Mode                string `json:"mode"`
	PlannedRequestCount int    `json:"planned_request_count"`
	StartedAt           *time.Time `json:"started_at,omitempty"`
	FinishedAt          *time.Time `json:"finished_at,omitempty"`
	LastSeq             int64  `json:"last_seq"`
	Progress            FuzzProgress `json:"progress"`
}

// FuzzErrorEv carries a server-side error message (e.g. run not found at
// subscribe time).
type FuzzErrorEv struct {
	Message string `json:"message"`
}
