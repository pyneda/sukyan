package db

import (
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
)

// PlaygroundWsFuzzRun is one execution of a WS fuzz session config.
type PlaygroundWsFuzzRun struct {
	BaseModel
	SessionID        uint              `gorm:"index" json:"session_id"`
	Session          PlaygroundSession `json:"-" gorm:"foreignKey:SessionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ConfigSnapshot   datatypes.JSON    `json:"config_snapshot" swaggerignore:"true"`
	BaselineSnapshot datatypes.JSON    `json:"baseline_snapshot,omitempty" swaggerignore:"true"`
	MatchersSnapshot datatypes.JSON    `json:"matchers_snapshot,omitempty" swaggerignore:"true"`
	Status           string            `gorm:"index" json:"status"`
	IterationCount   int               `json:"iteration_count"`
	SentCount        int               `json:"sent_count"`
	ErrorCount       int               `json:"error_count"`
	FindingCount     int               `json:"finding_count"`
	StartedAt        *time.Time        `json:"started_at"`
	FinishedAt       *time.Time        `json:"finished_at"`
	FailureReason    string            `json:"failure_reason,omitempty"`
}

// PlaygroundWsFuzzIteration is one iteration's per-row result.
type PlaygroundWsFuzzIteration struct {
	BaseModel
	RunID                 uint                 `gorm:"index" json:"run_id"`
	Run                   PlaygroundWsFuzzRun  `json:"-" gorm:"foreignKey:RunID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	IterationIndex        int                  `gorm:"index:idx_ws_fuzz_run_iter,unique;priority:2" json:"iteration_index"`
	Status                string               `gorm:"index" json:"status"`
	PayloadValues         datatypes.JSON       `json:"payload_values" swaggerignore:"true"`
	BaselineMatch         bool                 `gorm:"index" json:"baseline_match"`
	DurationMs            int                  `json:"duration_ms"`
	HandshakeStatusCode   int                  `json:"handshake_status_code"`
	HandshakeHeaders      datatypes.JSON       `json:"handshake_headers,omitempty" swaggerignore:"true"`
	WebSocketConnectionID *uint                `json:"websocket_connection_id,omitempty"`
	WebSocketConnection   *WebSocketConnection `json:"-" gorm:"foreignKey:WebSocketConnectionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	PeerCloseCode         *int                 `json:"peer_close_code,omitempty"`
	FailureReason         string               `json:"failure_reason,omitempty"`
	FailedStepIndex       *int                 `json:"failed_step_index,omitempty"`
	CheckResults          datatypes.JSON       `json:"check_results,omitempty" swaggerignore:"true"`
	VariablesSnapshot     datatypes.JSON       `json:"variables_snapshot,omitempty" swaggerignore:"true"`
}

// CreatePlaygroundWsFuzzRun inserts a new run.
func (d *DatabaseConnection) CreatePlaygroundWsFuzzRun(r *PlaygroundWsFuzzRun) error {
	return d.db.Create(r).Error
}

// GetPlaygroundWsFuzzRun fetches a run by ID.
func (d *DatabaseConnection) GetPlaygroundWsFuzzRun(id uint) (*PlaygroundWsFuzzRun, error) {
	var r PlaygroundWsFuzzRun
	if err := d.db.First(&r, id).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

// UpdatePlaygroundWsFuzzRun persists changes to an existing run.
func (d *DatabaseConnection) UpdatePlaygroundWsFuzzRun(r *PlaygroundWsFuzzRun) error {
	return d.db.Save(r).Error
}

// UpdatePlaygroundWsFuzzRunProgress writes only the per-tick counters so it
// doesn't clobber concurrent status changes (mirrors the HTTP fuzz helper).
func (d *DatabaseConnection) UpdatePlaygroundWsFuzzRunProgress(runID uint, sent, errs, findings int) error {
	return d.db.Model(&PlaygroundWsFuzzRun{}).Where("id = ?", runID).
		Updates(map[string]any{
			"sent_count":    sent,
			"error_count":   errs,
			"finding_count": findings,
		}).Error
}

// DeletePlaygroundWsFuzzRun removes a run (cascade-deletes its iterations).
func (d *DatabaseConnection) DeletePlaygroundWsFuzzRun(id uint) error {
	return d.db.Delete(&PlaygroundWsFuzzRun{}, id).Error
}

// CreatePlaygroundWsFuzzIteration inserts a new iteration row.
func (d *DatabaseConnection) CreatePlaygroundWsFuzzIteration(it *PlaygroundWsFuzzIteration) error {
	return d.db.Create(it).Error
}

// PlaygroundWsFuzzIterationFilter is the query filter for list endpoints.
type PlaygroundWsFuzzIterationFilter struct {
	RunID           uint
	Statuses        []string
	BaselineMatch   *bool
	PayloadContains string
	FailedStepIndex *int
	Page            int
	PageSize        int
}

// ListPlaygroundWsFuzzIterations fetches paginated iterations matching the filter.
func (d *DatabaseConnection) ListPlaygroundWsFuzzIterations(f PlaygroundWsFuzzIterationFilter) ([]PlaygroundWsFuzzIteration, int64, error) {
	q := d.db.Model(&PlaygroundWsFuzzIteration{}).Where("run_id = ?", f.RunID)
	if len(f.Statuses) > 0 {
		q = q.Where("status IN ?", f.Statuses)
	}
	if f.BaselineMatch != nil {
		q = q.Where("baseline_match = ?", *f.BaselineMatch)
	}
	if f.PayloadContains != "" {
		q = q.Where("payload_values::text ILIKE ?", "%"+f.PayloadContains+"%")
	}
	if f.FailedStepIndex != nil {
		q = q.Where("failed_step_index = ?", *f.FailedStepIndex)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if f.PageSize == 0 {
		f.PageSize = 50
	}
	if f.Page == 0 {
		f.Page = 1
	}
	var rows []PlaygroundWsFuzzIteration
	err := q.Order("iteration_index asc").
		Offset((f.Page - 1) * f.PageSize).
		Limit(f.PageSize).
		Find(&rows).Error
	return rows, total, err
}

// MarkOrphanedWsFuzzRunsAborted is the recovery sweep run on backend boot.
// Any run in a non-terminal status is flipped to "aborted_server_restart"
// because its in-process state was lost when the previous process exited.
// Mirrors the MarkOrphanedFuzzRunsAborted and MarkOrphanedWsRunsAborted
// helpers in this package for naming + signature parity.
func (d *DatabaseConnection) MarkOrphanedWsFuzzRunsAborted() error {
	nonTerminal := []string{"pending", "calibrating", "running", "paused", "pausing"}
	res := d.db.Model(&PlaygroundWsFuzzRun{}).
		Where("status IN ?", nonTerminal).
		Updates(map[string]any{
			"status":         "aborted_server_restart",
			"failure_reason": "server restarted while run in progress",
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		log.Info().Int64("count", res.RowsAffected).Msg("recovered orphaned ws_fuzz runs")
	}
	return nil
}
