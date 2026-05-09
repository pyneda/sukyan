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
