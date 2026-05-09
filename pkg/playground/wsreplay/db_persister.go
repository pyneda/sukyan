package wsreplay

import (
	"encoding/json"
	"time"

	"github.com/pyneda/sukyan/db"
	"gorm.io/datatypes"
)

// DBPersister adapts the engine's Persister interface to the db package.
type DBPersister struct{ conn *db.DatabaseConnection }

// NewDBPersister returns a Persister backed by the project's DB connection.
func NewDBPersister(c *db.DatabaseConnection) *DBPersister { return &DBPersister{conn: c} }

func (p *DBPersister) CreateConnection(url string, headers []HeaderSpec, statusCode int, source string, playgroundSessionID *uint) (uint, error) {
	hdrJSON, _ := json.Marshal(headers)
	rec := &db.WebSocketConnection{
		URL:                 url,
		RequestHeaders:      datatypes.JSON(hdrJSON),
		StatusCode:          statusCode,
		Source:              source,
		PlaygroundSessionID: playgroundSessionID,
	}
	if err := p.conn.CreateWebSocketConnection(rec); err != nil {
		return 0, err
	}
	return rec.ID, nil
}

func (p *DBPersister) RecordMessage(connID uint, opcode int, content string, direction string) (uint, error) {
	msg := &db.WebSocketMessage{
		ConnectionID: connID,
		Opcode:       float64(opcode),
		PayloadData:  content,
		Direction:    db.MessageDirection(direction),
		IsBinary:     opcode == 2,
		Timestamp:    time.Now(),
	}
	if err := p.conn.CreateWebSocketMessage(msg); err != nil {
		return 0, err
	}
	return msg.ID, nil
}

func (p *DBPersister) CloseConnection(connID uint) error {
	return p.conn.CloseWebSocketConnection(connID, time.Now())
}
