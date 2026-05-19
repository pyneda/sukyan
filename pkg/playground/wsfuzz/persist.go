package wsfuzz

import "time"

// RunPersister is what the engine uses to write run + iteration rows to the DB.
// Implemented in api/ by a thin adapter over db.DatabaseConnection so this
// package doesn't import db directly.
type RunPersister interface {
	UpdateRunStatus(runID uint, status string, reason string) error
	UpdateRunProgress(runID uint, sent, errors, findings int) error
	UpdateRunStartedAt(runID uint, t time.Time) error
	UpdateRunFinishedAt(runID uint, t time.Time) error
	UpdateRunBaseline(runID uint, baselineJSON []byte) error
	SaveIteration(it WsIterationResult) error
}
