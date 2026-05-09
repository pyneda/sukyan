package db

import (
	"encoding/json"
	"time"
)

// PlaygroundWsRunStatus represents the lifecycle status of a scripted WS run.
type PlaygroundWsRunStatus string

const (
	WsRunPending              PlaygroundWsRunStatus = "pending"
	WsRunRunning              PlaygroundWsRunStatus = "running"
	WsRunSucceeded            PlaygroundWsRunStatus = "succeeded"
	WsRunFailed               PlaygroundWsRunStatus = "failed"
	WsRunCancelled            PlaygroundWsRunStatus = "cancelled"
	WsRunAbortedServerRestart PlaygroundWsRunStatus = "aborted_server_restart"
)

// PlaygroundWsSession holds the WS-specific payload for a playground session of type ws_manual or ws_fuzz.
type PlaygroundWsSession struct {
	BaseModel
	PlaygroundSessionID      uint                 `json:"playground_session_id" gorm:"uniqueIndex;not null"`
	PlaygroundSession        PlaygroundSession    `json:"-" gorm:"foreignKey:PlaygroundSessionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	TargetURL                string               `json:"target_url"`
	RequestHeaders           json.RawMessage      `json:"request_headers" gorm:"type:jsonb"`
	Script                   json.RawMessage      `json:"script" gorm:"type:jsonb"`
	Options                  json.RawMessage      `json:"options" gorm:"type:jsonb"`
	ImportedFromConnectionID *uint                `json:"imported_from_connection_id" gorm:"index"`
	ImportedFromConnection   *WebSocketConnection `json:"-" gorm:"foreignKey:ImportedFromConnectionID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
}

func (d *DatabaseConnection) CreatePlaygroundWsSession(ws *PlaygroundWsSession) error {
	return d.db.Create(ws).Error
}

func (d *DatabaseConnection) GetPlaygroundWsSession(id uint) (*PlaygroundWsSession, error) {
	var ws PlaygroundWsSession
	err := d.db.First(&ws, id).Error
	return &ws, err
}

func (d *DatabaseConnection) GetPlaygroundWsSessionBySessionID(sessionID uint) (*PlaygroundWsSession, error) {
	var ws PlaygroundWsSession
	err := d.db.Where("playground_session_id = ?", sessionID).First(&ws).Error
	return &ws, err
}

func (d *DatabaseConnection) UpdatePlaygroundWsSession(ws *PlaygroundWsSession) error {
	return d.db.Save(ws).Error
}

func (d *DatabaseConnection) DeletePlaygroundWsSession(id uint) error {
	return d.db.Delete(&PlaygroundWsSession{}, id).Error
}

// PlaygroundWsRun is one execution of a script against a fresh upstream socket.
type PlaygroundWsRun struct {
	BaseModel
	PlaygroundWsSessionID uint                  `json:"playground_ws_session_id" gorm:"index;not null"`
	PlaygroundWsSession   PlaygroundWsSession   `json:"-" gorm:"foreignKey:PlaygroundWsSessionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	WebSocketConnectionID *uint                 `json:"websocket_connection_id" gorm:"index"`
	WebSocketConnection   *WebSocketConnection  `json:"-" gorm:"foreignKey:WebSocketConnectionID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	ScriptSnapshot        json.RawMessage       `json:"script_snapshot" gorm:"type:jsonb"`
	OptionsSnapshot       json.RawMessage       `json:"options_snapshot" gorm:"type:jsonb"`
	Status                PlaygroundWsRunStatus `json:"status" gorm:"not null;default:'pending'"`
	CurrentStepIndex      *int                  `json:"current_step_index"`
	FailureReason         *string               `json:"failure_reason"`
	StartedAt             *time.Time            `json:"started_at"`
	FinishedAt            *time.Time            `json:"finished_at"`
}

func (d *DatabaseConnection) CreatePlaygroundWsRun(r *PlaygroundWsRun) error {
	return d.db.Create(r).Error
}

func (d *DatabaseConnection) UpdatePlaygroundWsRun(r *PlaygroundWsRun) error {
	return d.db.Save(r).Error
}

func (d *DatabaseConnection) GetPlaygroundWsRun(id uint) (*PlaygroundWsRun, error) {
	var r PlaygroundWsRun
	err := d.db.First(&r, id).Error
	return &r, err
}

func (d *DatabaseConnection) ListPlaygroundWsRuns(wsSessionID uint, page, pageSize int) ([]*PlaygroundWsRun, int64, error) {
	q := d.db.Model(&PlaygroundWsRun{}).Where("playground_ws_session_id = ?", wsSessionID).Order("id DESC")
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	if page > 0 && pageSize > 0 {
		q = q.Offset((page - 1) * pageSize).Limit(pageSize)
	}
	var runs []*PlaygroundWsRun
	err := q.Find(&runs).Error
	return runs, count, err
}

// MarkOrphanedWsRunsAborted is the recovery sweep run on backend boot.
func (d *DatabaseConnection) MarkOrphanedWsRunsAborted() error {
	now := time.Now()
	reason := "server restarted while run was in progress"
	return d.db.Model(&PlaygroundWsRun{}).
		Where("status IN ?", []PlaygroundWsRunStatus{WsRunPending, WsRunRunning}).
		Updates(map[string]any{
			"status":         WsRunAbortedServerRestart,
			"finished_at":    &now,
			"failure_reason": &reason,
		}).Error
}

// CloseOrphanedPlaygroundConnections stamps closed_at on playground websocket_connections rows
// that have no closed_at, called from the same recovery sweep on boot.
func (d *DatabaseConnection) CloseOrphanedPlaygroundConnections() error {
	now := time.Now()
	return d.db.Model(&WebSocketConnection{}).
		Where("source = ? AND playground_session_id IS NOT NULL AND closed_at IS NULL", "playground").
		Update("closed_at", &now).Error
}
