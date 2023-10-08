package db

import (
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
	"time"
)

type WebSocketConnection struct {
	BaseModel
	URL             string             `json:"url"`
	RequestHeaders  datatypes.JSON     `json:"request_headers" swaggerignore:"true"`
	ResponseHeaders datatypes.JSON     `json:"response_headers" swaggerignore:"true"`
	StatusCode      int                `gorm:"index" json:"status_code"`
	StatusText      string             `json:"status_text"`
	Messages        []WebSocketMessage `json:"messages" gorm:"foreignKey:ConnectionID"`
	ClosedAt        time.Time          `json:"closed_at"` // timestamp for when the connection is closed
	Workspace       Workspace          `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WorkspaceID     *uint              `json:"workspace_id"`
	TaskID          *uint              `json:"task_id" gorm:"index" `
	Task            Task               `json:"-" gorm:"foreignKey:TaskID"`
	Source          string             `json:"source"`
}

type MessageDirection string

const (
	MessageSent     MessageDirection = "sent"
	MessageReceived MessageDirection = "received"
)

type WebSocketMessage struct {
	BaseModel
	ConnectionID uint             `json:"connection_id"`
	Opcode       float64          `json:"opcode"`
	Mask         bool             `gorm:"index" json:"mask"`
	PayloadData  string           `json:"payload_data"`
	Timestamp    time.Time        `json:"timestamp"`              // timestamp for when the message was sent/received
	Direction    MessageDirection `gorm:"index" json:"direction"` // direction of the message
}

func (d *DatabaseConnection) CreateWebSocketConnection(connection *WebSocketConnection) error {
	err := d.db.Create(connection).Error
	return err
}

func (d *DatabaseConnection) CreateWebSocketMessage(message *WebSocketMessage) error {
	err := d.db.Create(message).Error
	return err
}

func (d *DatabaseConnection) UpdateWebSocketConnection(connection *WebSocketConnection) error {
	err := d.db.Save(connection).Error
	return err
}

type WebSocketConnectionFilter struct {
	Pagination
	WorkspaceID uint     `json:"workspace_id" validate:"required"`
	TaskID      uint     `json:"task_id"`
	Sources     []string `json:"sources" validate:"omitempty,dive,ascii"`
}

func (d *DatabaseConnection) ListWebSocketConnections(filter WebSocketConnectionFilter) ([]WebSocketConnection, int64, error) {
	var connections []WebSocketConnection
	var count int64

	query := d.db.Model(&WebSocketConnection{}).
		Where("workspace_id = ?", filter.WorkspaceID)

	if len(filter.Sources) > 0 {
		query = query.Where("source IN ?", filter.Sources)
	}

	if filter.TaskID > 0 {
		query = query.Where("task_id = ?", filter.TaskID)
	}

	err := query.Count(&count).
		Order("id desc").
		Limit(filter.PageSize).
		Offset((filter.Page - 1) * filter.PageSize).
		Find(&connections).
		Error

	if err != nil {
		log.Error().Err(err).Msg("Failed to list WebSocket connections")
		return nil, 0, err
	}

	return connections, count, nil
}

type WebSocketMessageFilter struct {
	Pagination
	ConnectionID uint
}

func (d *DatabaseConnection) ListWebSocketMessages(filter WebSocketMessageFilter) ([]WebSocketMessage, int64, error) {
	var messages []WebSocketMessage
	var count int64

	query := d.db.Model(&WebSocketMessage{})
	if filter.ConnectionID != 0 {
		query = query.Where("connection_id = ?", filter.ConnectionID)
	}

	err := query.
		Count(&count).
		// Order("id asc").
		Limit(filter.PageSize).
		Offset((filter.Page - 1) * filter.PageSize).
		Find(&messages).
		Error
	if err != nil {
		log.Error().Err(err).Msg("Failed to list WebSocket messages")
		return nil, 0, err
	}

	return messages, count, nil
}
