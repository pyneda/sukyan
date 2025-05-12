package db

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pyneda/sukyan/lib"

	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
)

type WebSocketConnection struct {
	BaseModel
	URL             string             `json:"url"`
	RequestHeaders  datatypes.JSON     `json:"request_headers" swaggerignore:"true"`
	ResponseHeaders datatypes.JSON     `json:"response_headers" swaggerignore:"true"`
	StatusCode      int                `gorm:"index" json:"status_code"`
	StatusText      string             `json:"status_text"`
	Messages        []WebSocketMessage `json:"messages" gorm:"foreignKey:ConnectionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ClosedAt        time.Time          `json:"closed_at"` // timestamp for when the connection is closed
	Workspace       Workspace          `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WorkspaceID     *uint              `json:"workspace_id"`
	TaskID          *uint              `json:"task_id" gorm:"index" `
	Task            Task               `json:"-" gorm:"foreignKey:TaskID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Source          string             `json:"source"`
}

func (c WebSocketConnection) TaskTitle() string {
	return fmt.Sprintf("WebSocket scan %s", c.URL)
}

func (c WebSocketConnection) TableHeaders() []string {
	return []string{"ID", "URL", "StatusCode", "StatusText", "ClosedAt", "WorkspaceID", "TaskID", "Source"}
}

func (c WebSocketConnection) TableRow() []string {
	return []string{
		fmt.Sprintf("%d", c.ID),
		c.URL,
		fmt.Sprintf("%d", c.StatusCode),
		c.StatusText,
		c.ClosedAt.Format(time.RFC3339),
		fmt.Sprintf("%d", *c.WorkspaceID),
		fmt.Sprintf("%d", *c.TaskID),
		c.Source,
	}
}

func (c WebSocketConnection) String() string {
	return fmt.Sprintf("ID: %d, URL: %s, StatusCode: %d, StatusText: %s, ClosedAt: %s, WorkspaceID: %d, TaskID: %d, Source: %s", c.ID, c.URL, c.StatusCode, c.StatusText, c.ClosedAt.Format(time.RFC3339), c.WorkspaceID, c.TaskID, c.Source)
}

func (c WebSocketConnection) Pretty() string {
	return fmt.Sprintf(
		"%sID:%s %d\n%sURL:%s %s\n%sStatusCode:%s %d\n%sStatusText:%s %s\n%sClosedAt:%s %s\n%sWorkspaceID:%s %d\n%sTaskID:%s %d\n%sSource:%s %s\n",
		lib.Blue, lib.ResetColor, c.ID,
		lib.Blue, lib.ResetColor, c.URL,
		lib.Blue, lib.ResetColor, c.StatusCode,
		lib.Blue, lib.ResetColor, c.StatusText,
		lib.Blue, lib.ResetColor, c.ClosedAt.Format(time.RFC3339),
		lib.Blue, lib.ResetColor, c.WorkspaceID,
		lib.Blue, lib.ResetColor, c.TaskID,
		lib.Blue, lib.ResetColor, c.Source)
}

func (c *WebSocketConnection) GetResponseHeadersAsMap() (map[string][]string, error) {
	intermediateMap := make(map[string]interface{})
	err := json.Unmarshal([]byte(c.ResponseHeaders), &intermediateMap)
	if err != nil {
		return nil, err
	}

	stringMap := make(map[string][]string)
	for key, value := range intermediateMap {
		switch v := value.(type) {
		case []interface{}:
			for _, item := range v {
				switch itemStr := item.(type) {
				case string:
					stringMap[key] = append(stringMap[key], itemStr)
				default:
					log.Warn().Interface("value", itemStr).Msg("value not a string")
				}
			}
		case string:
			stringMap[key] = append(stringMap[key], v)
		default:
			log.Warn().Interface("value", v).Msg("value not a []string")

		}
	}

	return stringMap, nil
}

func (c *WebSocketConnection) GetRequestHeadersAsMap() (map[string][]string, error) {
	intermediateMap := make(map[string]interface{})
	err := json.Unmarshal([]byte(c.RequestHeaders), &intermediateMap)
	if err != nil {
		return nil, err
	}

	stringMap := make(map[string][]string)
	for key, value := range intermediateMap {
		switch v := value.(type) {
		case []interface{}:
			for _, item := range v {
				switch itemStr := item.(type) {
				case string:
					stringMap[key] = append(stringMap[key], itemStr)
				default:
					log.Warn().Interface("value", itemStr).Msg("value not a string")
				}
			}
		case string:
			stringMap[key] = append(stringMap[key], v)
		default:
			log.Warn().Interface("value", v).Msg("value not a []string")

		}
	}

	return stringMap, nil
}

func (c *WebSocketConnection) GetResponseHeadersAsString() (string, error) {
	headersMap, err := c.GetResponseHeadersAsMap()
	if err != nil {
		log.Error().Err(err).Uint("history", c.ID).Msg("Error getting response headers as map")
		return "", err
	}
	headers := make([]string, 0, len(headersMap))
	for name, values := range headersMap {
		for _, value := range values {
			headers = append(headers, fmt.Sprintf("%s: %s", name, value))
		}
	}

	return strings.Join(headers, "\n"), nil
}

func (c *WebSocketConnection) GetRequestHeadersAsString() (string, error) {
	headersMap, err := c.GetRequestHeadersAsMap()
	if err != nil {
		log.Error().Err(err).Uint("history", c.ID).Msg("Error getting request headers as map")
		return "", err
	}
	headers := make([]string, 0, len(headersMap))
	for name, values := range headersMap {
		for _, value := range values {
			headers = append(headers, fmt.Sprintf("%s: %s", name, value))
		}
	}

	return strings.Join(headers, "\n"), nil
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

func (m WebSocketMessage) TableHeaders() []string {
	return []string{"ID", "ConnectionID", "Opcode", "Mask", "PayloadData", "Timestamp", "Direction"}
}

func (m WebSocketMessage) TableRow() []string {
	return []string{
		fmt.Sprintf("%d", m.ID),
		fmt.Sprintf("%d", m.ConnectionID),
		fmt.Sprintf("%f", m.Opcode),
		fmt.Sprintf("%t", m.Mask),
		m.PayloadData,
		m.Timestamp.Format(time.RFC3339),
		string(m.Direction),
	}
}

func (m WebSocketMessage) String() string {
	return fmt.Sprintf("ID: %d, ConnectionID: %d, Opcode: %f, Mask: %t, PayloadData: %s, Timestamp: %s, Direction: %s", m.ID, m.ConnectionID, m.Opcode, m.Mask, m.PayloadData, m.Timestamp.Format(time.RFC3339), m.Direction)
}

func (m WebSocketMessage) Pretty() string {
	return fmt.Sprintf(
		"%sID:%s %d\n%sConnectionID:%s %d\n%sOpcode:%s %f\n%sMask:%s %t\n%sPayloadData:%s %s\n%sTimestamp:%s %s\n%sDirection:%s %s\n",
		lib.Blue, lib.ResetColor, m.ID,
		lib.Blue, lib.ResetColor, m.ConnectionID,
		lib.Blue, lib.ResetColor, m.Opcode,
		lib.Blue, lib.ResetColor, m.Mask,
		lib.Blue, lib.ResetColor, m.PayloadData,
		lib.Blue, lib.ResetColor, m.Timestamp.Format(time.RFC3339),
		lib.Blue, lib.ResetColor, m.Direction)
}

func (d *DatabaseConnection) CreateWebSocketConnection(connection *WebSocketConnection) error {
	tx := d.db.Begin()

	if err := tx.Create(connection).Error; err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}
func (d *DatabaseConnection) GetWebSocketConnection(id uint) (*WebSocketConnection, error) {
	var connection WebSocketConnection
	err := d.db.Preload("Messages").First(&connection, id).Error
	return &connection, err
}

func (d *DatabaseConnection) CreateWebSocketMessage(message *WebSocketMessage) error {
	tx := d.db.Begin()

	if err := tx.Create(message).Error; err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

func (d *DatabaseConnection) UpdateWebSocketConnection(connection *WebSocketConnection) error {
	tx := d.db.Begin()

	if err := tx.Save(connection).Error; err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

type WebSocketConnectionFilter struct {
	Pagination
	WorkspaceID uint     `json:"workspace_id" validate:"required"`
	TaskID      uint     `json:"task_id"`
	Sources     []string `json:"sources" validate:"omitempty,dive,ascii"`
}

func (d *DatabaseConnection) ListWebSocketConnections(filter WebSocketConnectionFilter) ([]WebSocketConnection, int64, error) {
	query := d.db.Model(&WebSocketConnection{})

	if filter.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	}
	if len(filter.Sources) > 0 {
		query = query.Where("source IN ?", filter.Sources)
	}
	if filter.TaskID > 0 {
		query = query.Where("task_id = ?", filter.TaskID)
	}

	var connections []WebSocketConnection
	var count int64

	if err := query.Count(&count).Error; err != nil {
		log.Error().Err(err).Msg("Failed to count WebSocket connections")
		return nil, 0, err
	}

	if filter.PageSize > 0 && filter.Page > 0 {
		query = query.Scopes(Paginate(&filter.Pagination))
	}

	query = query.Order("id desc")

	if err := query.Find(&connections).Error; err != nil {
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
	query := d.db.Model(&WebSocketMessage{})

	if filter.ConnectionID != 0 {
		query = query.Where("connection_id = ?", filter.ConnectionID)
	}

	var messages []WebSocketMessage
	var count int64

	if err := query.Count(&count).Error; err != nil {
		log.Error().Err(err).Msg("Failed to count WebSocket messages")
		return nil, 0, err
	}

	if filter.PageSize > 0 && filter.Page > 0 {
		query = query.Scopes(Paginate(&filter.Pagination))
	}

	query = query.Order("id desc")

	if err := query.Find(&messages).Error; err != nil {
		log.Error().Err(err).Msg("Failed to list WebSocket messages")
		return nil, 0, err
	}

	return messages, count, nil
}
