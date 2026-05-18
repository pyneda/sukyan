package db

import (
	"encoding/json"
	"time"
)

// PlaygroundFuzzRunStatus is the lifecycle of one HTTP fuzz execution.
//
// pending → calibrating → running → (succeeded | failed | cancelled | stopped_error_rate | stopped_max_duration)
//
// aborted_server_restart is set by the boot-time recovery sweep on rows left
// in pending/calibrating/running by a process that died.
type PlaygroundFuzzRunStatus string

const (
	FuzzRunPending              PlaygroundFuzzRunStatus = "pending"
	FuzzRunCalibrating          PlaygroundFuzzRunStatus = "calibrating"
	FuzzRunRunning              PlaygroundFuzzRunStatus = "running"
	FuzzRunSucceeded            PlaygroundFuzzRunStatus = "succeeded"
	FuzzRunFailed               PlaygroundFuzzRunStatus = "failed"
	FuzzRunCancelled            PlaygroundFuzzRunStatus = "cancelled"
	FuzzRunStoppedErrorRate     PlaygroundFuzzRunStatus = "stopped_error_rate"
	FuzzRunStoppedMaxDuration   PlaygroundFuzzRunStatus = "stopped_max_duration"
	FuzzRunAbortedServerRestart PlaygroundFuzzRunStatus = "aborted_server_restart"
)

// IsTerminal reports whether the status is one the engine never transitions out of.
func (s PlaygroundFuzzRunStatus) IsTerminal() bool {
	switch s {
	case FuzzRunSucceeded, FuzzRunFailed, FuzzRunCancelled,
		FuzzRunStoppedErrorRate, FuzzRunStoppedMaxDuration, FuzzRunAbortedServerRestart:
		return true
	}
	return false
}

// PlaygroundFuzzRun is one execution of a fuzz against a target. Replaces the
// previous use of db.Task with TaskTypePlaygroundFuzzer; the typed run table
// owns lifecycle, config snapshot, baseline, matchers, and progress.
//
// Pattern mirrors PlaygroundWsRun: a session holds the live editable config,
// each launch snapshots the config into a new run row.
type PlaygroundFuzzRun struct {
	BaseModel
	PlaygroundSessionID uint              `json:"playground_session_id" gorm:"index;not null"`
	PlaygroundSession   PlaygroundSession `json:"-" gorm:"foreignKey:PlaygroundSessionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	WorkspaceID         uint              `json:"workspace_id" gorm:"index;not null"`

	// ConfigSnapshot is the FuzzerConfig used at launch — frozen, so editing
	// the session's live config does not retroactively change run history.
	ConfigSnapshot json.RawMessage `json:"config_snapshot" gorm:"type:jsonb;not null"`

	// Baseline is the per-run BaselineFingerprint set computed during the
	// calibrating phase. nil when baseline mode = off, or when calibration
	// has not yet run.
	Baseline json.RawMessage `json:"baseline,omitempty" gorm:"type:jsonb"`

	// Matchers is the user's matcher set as of the last save. nil = no matchers.
	Matchers json.RawMessage `json:"matchers,omitempty" gorm:"type:jsonb"`

	// Lifecycle
	Status        PlaygroundFuzzRunStatus `json:"status" gorm:"not null;default:'pending';index"`
	FailureReason *string                 `json:"failure_reason"`
	StartedAt     *time.Time              `json:"started_at"`
	FinishedAt    *time.Time              `json:"finished_at"`

	// Progress — updated periodically by the engine, not per-result, to avoid DB write storms.
	PlannedRequestCount int `json:"planned_request_count"`
	SentRequestCount    int `json:"sent_request_count"`
	ErrorCount          int `json:"error_count"`
}

// CreatePlaygroundFuzzRun inserts a new run.
func (d *DatabaseConnection) CreatePlaygroundFuzzRun(r *PlaygroundFuzzRun) error {
	return d.db.Create(r).Error
}

// UpdatePlaygroundFuzzRun persists changes to an existing run.
func (d *DatabaseConnection) UpdatePlaygroundFuzzRun(r *PlaygroundFuzzRun) error {
	return d.db.Save(r).Error
}

// GetPlaygroundFuzzRun fetches a run by ID.
func (d *DatabaseConnection) GetPlaygroundFuzzRun(id uint) (*PlaygroundFuzzRun, error) {
	var r PlaygroundFuzzRun
	err := d.db.First(&r, id).Error
	return &r, err
}

// ListPlaygroundFuzzRuns returns recent runs for a session, newest first.
// page/pageSize are 1-based; pass 0 for both to disable pagination.
func (d *DatabaseConnection) ListPlaygroundFuzzRuns(sessionID uint, page, pageSize int) ([]*PlaygroundFuzzRun, int64, error) {
	q := d.db.Model(&PlaygroundFuzzRun{}).Where("playground_session_id = ?", sessionID).Order("id DESC")
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	if page > 0 && pageSize > 0 {
		q = q.Offset((page - 1) * pageSize).Limit(pageSize)
	}
	var runs []*PlaygroundFuzzRun
	err := q.Find(&runs).Error
	return runs, count, err
}

// MarkOrphanedFuzzRunsAborted is the recovery sweep run on backend boot.
// Rows still in pending/calibrating/running are stamped aborted, since no
// engine is alive to make further progress on them.
func (d *DatabaseConnection) MarkOrphanedFuzzRunsAborted() error {
	now := time.Now()
	reason := "server restarted while run was in progress"
	return d.db.Model(&PlaygroundFuzzRun{}).
		Where("status IN ?", []PlaygroundFuzzRunStatus{FuzzRunPending, FuzzRunCalibrating, FuzzRunRunning}).
		Updates(map[string]any{
			"status":         FuzzRunAbortedServerRestart,
			"finished_at":    &now,
			"failure_reason": &reason,
		}).Error
}
