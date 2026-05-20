package api

import (
	"encoding/json"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/playground/wsfuzz"
	"gorm.io/datatypes"
)

// dbRunPersister adapts db.DatabaseConnection to the wsfuzz.RunPersister
// contract. The engine talks to this interface; the api layer wires up the
// real Postgres-backed implementation.
type dbRunPersister struct{ conn *db.DatabaseConnection }

// newDBRunPersister returns a RunPersister backed by the given DB connection.
func newDBRunPersister(conn *db.DatabaseConnection) *dbRunPersister {
	return &dbRunPersister{conn: conn}
}

func (p *dbRunPersister) UpdateRunStatus(runID uint, status string, reason string) error {
	r, err := p.conn.GetPlaygroundWsFuzzRun(runID)
	if err != nil {
		return err
	}
	r.Status = status
	if reason != "" {
		r.FailureReason = reason
	}
	return p.conn.UpdatePlaygroundWsFuzzRun(r)
}

func (p *dbRunPersister) UpdateRunProgress(runID uint, sent, errs, findings int) error {
	return p.conn.UpdatePlaygroundWsFuzzRunProgress(runID, sent, errs, findings)
}

func (p *dbRunPersister) UpdateRunStartedAt(runID uint, t time.Time) error {
	r, err := p.conn.GetPlaygroundWsFuzzRun(runID)
	if err != nil {
		return err
	}
	r.StartedAt = &t
	return p.conn.UpdatePlaygroundWsFuzzRun(r)
}

func (p *dbRunPersister) UpdateRunFinishedAt(runID uint, t time.Time) error {
	r, err := p.conn.GetPlaygroundWsFuzzRun(runID)
	if err != nil {
		return err
	}
	r.FinishedAt = &t
	return p.conn.UpdatePlaygroundWsFuzzRun(r)
}

func (p *dbRunPersister) UpdateRunBaseline(runID uint, baselineJSON []byte) error {
	r, err := p.conn.GetPlaygroundWsFuzzRun(runID)
	if err != nil {
		return err
	}
	r.BaselineSnapshot = datatypes.JSON(baselineJSON)
	return p.conn.UpdatePlaygroundWsFuzzRun(r)
}

func (p *dbRunPersister) SaveIteration(it wsfuzz.WsIterationResult) error {
	payloadJSON, _ := json.Marshal(it.PayloadValues)
	varsJSON, _ := json.Marshal(it.VariablesSnapshot)
	// HandshakeHeaders capture from the engine is future work; persist an
	// empty JSON object for now so the column stays NOT-NULL-friendly.
	hdrJSON, _ := json.Marshal(map[string]string{})
	row := &db.PlaygroundWsFuzzIteration{
		RunID:                 it.RunID,
		IterationIndex:        it.IterationIndex,
		Status:                string(it.Status),
		PayloadValues:         datatypes.JSON(payloadJSON),
		BaselineMatch:         it.BaselineMatch,
		DurationMs:            it.DurationMs,
		HandshakeStatusCode:   it.HandshakeStatusCode,
		HandshakeHeaders:      datatypes.JSON(hdrJSON),
		WebSocketConnectionID: it.WebSocketConnectionID,
		PeerCloseCode:         it.PeerCloseCode,
		FailureReason:         it.FailureReason,
		FailedStepIndex:       it.FailedStepIndex,
		CheckResults:          datatypes.JSON(it.CheckResults),
		VariablesSnapshot:     datatypes.JSON(varsJSON),
	}
	return p.conn.CreatePlaygroundWsFuzzIteration(row)
}
