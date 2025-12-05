package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGenerateWorkerNodeID(t *testing.T) {
	// Test without prefix
	id1 := GenerateWorkerNodeID("")
	assert.NotEmpty(t, id1, "Generated ID should not be empty")
	assert.Contains(t, id1, "-", "ID should contain hostname-pid separator")

	// Test with prefix
	id2 := GenerateWorkerNodeID("test")
	assert.NotEmpty(t, id2, "Generated ID should not be empty")
	assert.Contains(t, id2, "test-", "ID should contain the prefix")

	// IDs should be different (include timestamp component - actually they include PID which is same)
	// In same process, IDs with same prefix will be the same
	id3 := GenerateWorkerNodeID("test")
	assert.Equal(t, id2, id3, "Same prefix in same process should generate same ID")
}

func TestWorkerNodeStatus(t *testing.T) {
	assert.Equal(t, WorkerNodeStatusRunning, WorkerNodeStatus("running"))
	assert.Equal(t, WorkerNodeStatusDraining, WorkerNodeStatus("draining"))
	assert.Equal(t, WorkerNodeStatusStopped, WorkerNodeStatus("stopped"))
}

func TestWorkerNode_TableName(t *testing.T) {
	node := WorkerNode{}
	assert.Equal(t, "worker_nodes", node.TableName())
}

func TestWorkerNodeStats(t *testing.T) {
	stats := WorkerNodeStats{
		TotalNodes:     10,
		RunningNodes:   5,
		StoppedNodes:   5,
		TotalClaimed:   1000,
		TotalCompleted: 900,
		TotalFailed:    50,
	}

	assert.Equal(t, 10, stats.TotalNodes)
	assert.Equal(t, 5, stats.RunningNodes)
	assert.Equal(t, 5, stats.StoppedNodes)
	assert.Equal(t, int64(1000), stats.TotalClaimed)
	assert.Equal(t, int64(900), stats.TotalCompleted)
	assert.Equal(t, int64(50), stats.TotalFailed)
}

func TestWorkerNode_Fields(t *testing.T) {
	now := time.Now()
	node := WorkerNode{
		ID:            "test-node-1",
		Hostname:      "localhost",
		WorkerCount:   5,
		Status:        WorkerNodeStatusRunning,
		StartedAt:     now,
		LastSeenAt:    now,
		JobsClaimed:   100,
		JobsCompleted: 90,
		JobsFailed:    5,
		Version:       "1.0.0",
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	assert.Equal(t, "test-node-1", node.ID)
	assert.Equal(t, "localhost", node.Hostname)
	assert.Equal(t, 5, node.WorkerCount)
	assert.Equal(t, WorkerNodeStatusRunning, node.Status)
	assert.Equal(t, 100, node.JobsClaimed)
	assert.Equal(t, 90, node.JobsCompleted)
	assert.Equal(t, 5, node.JobsFailed)
	assert.Equal(t, "1.0.0", node.Version)
}
