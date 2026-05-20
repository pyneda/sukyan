package api

import (
	"testing"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/playground/stream"
	"github.com/pyneda/sukyan/pkg/playground/wsfuzz"
	"github.com/stretchr/testify/require"
)

func TestBuildWsFuzzSnapshot_FromTerminalRunNoBroadcaster(t *testing.T) {
	started := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	run := &db.PlaygroundWsFuzzRun{
		Status:         "succeeded",
		IterationCount: 100,
		StartedAt:      &started,
	}
	run.ID = 42 // BaseModel.ID

	snap := buildWsFuzzSnapshot(run, nil)
	require.Equal(t, wsfuzz.EventSnapshot, snap.Type)
	require.Equal(t, uint(42), snap.RunID)
	require.NotNil(t, snap.Snapshot)
	require.Equal(t, "succeeded", snap.Snapshot.Status)
	require.Equal(t, 100, snap.Snapshot.PlannedIterations)
	require.Equal(t, started, snap.Snapshot.StartedAt)
	require.Equal(t, int64(0), snap.Snapshot.LastSeq, "no broadcaster → LastSeq is 0")
}

func TestBuildWsFuzzSnapshot_WithLiveBroadcaster(t *testing.T) {
	bcast := stream.NewBroadcaster(64, 1000)
	// Publish a couple of events so LastSeq > 0.
	bcast.Publish(&wsfuzz.WsFuzzEvent{Type: wsfuzz.EventStatus})
	bcast.Publish(&wsfuzz.WsFuzzEvent{Type: wsfuzz.EventStatus})

	run := &db.PlaygroundWsFuzzRun{Status: "running", IterationCount: 50}
	run.ID = 7

	snap := buildWsFuzzSnapshot(run, bcast)
	require.Equal(t, int64(2), snap.Snapshot.LastSeq)
	require.Equal(t, "running", snap.Snapshot.Status)
	require.Equal(t, time.Time{}, snap.Snapshot.StartedAt, "nil StartedAt should serialize as zero-value")
}
