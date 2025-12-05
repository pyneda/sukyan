package db

import (
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// WorkerNodeStatus represents the current status of a worker node.
type WorkerNodeStatus string

const (
	WorkerNodeStatusRunning  WorkerNodeStatus = "running"
	WorkerNodeStatusDraining WorkerNodeStatus = "draining"
	WorkerNodeStatusStopped  WorkerNodeStatus = "stopped"
)

// WorkerNode tracks registered workers for monitoring and stale job recovery.
// This enables distributed worker deployments where multiple processes can
// claim and execute scan jobs.
type WorkerNode struct {
	ID            string           `json:"id" gorm:"primaryKey;size:255"`
	Hostname      string           `json:"hostname" gorm:"size:255;index"`
	WorkerCount   int              `json:"worker_count"`
	Status        WorkerNodeStatus `json:"status" gorm:"size:50;index"`
	StartedAt     time.Time        `json:"started_at"`
	LastSeenAt    time.Time        `json:"last_seen_at" gorm:"index"`
	JobsClaimed   int              `json:"jobs_claimed"`
	JobsCompleted int              `json:"jobs_completed"`
	JobsFailed    int              `json:"jobs_failed"`
	Version       string           `json:"version" gorm:"size:50"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// TableName returns the table name for WorkerNode.
func (WorkerNode) TableName() string {
	return "worker_nodes"
}

// GenerateWorkerNodeID generates a unique worker node ID.
// Format: hostname-pid or custom prefix-pid
func GenerateWorkerNodeID(prefix string) string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	if prefix != "" {
		return fmt.Sprintf("%s-%s-%d", prefix, hostname, os.Getpid())
	}
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}

// RegisterWorkerNode registers a new worker node or updates an existing one.
// If the node already exists, it updates the status and timestamps.
func (d *DatabaseConnection) RegisterWorkerNode(node *WorkerNode) error {
	now := time.Now()
	node.StartedAt = now
	node.LastSeenAt = now
	node.Status = WorkerNodeStatusRunning
	node.CreatedAt = now
	node.UpdatedAt = now

	// Use upsert to handle restarts of the same worker
	result := d.db.Where("id = ?", node.ID).Assign(map[string]interface{}{
		"hostname":       node.Hostname,
		"worker_count":   node.WorkerCount,
		"status":         WorkerNodeStatusRunning,
		"started_at":     now,
		"last_seen_at":   now,
		"jobs_claimed":   0,
		"jobs_completed": 0,
		"jobs_failed":    0,
		"version":        node.Version,
		"updated_at":     now,
	}).FirstOrCreate(node)

	if result.Error != nil {
		return fmt.Errorf("failed to register worker node: %w", result.Error)
	}

	log.Info().
		Str("node_id", node.ID).
		Str("hostname", node.Hostname).
		Int("worker_count", node.WorkerCount).
		Msg("Worker node registered")

	return nil
}

// UpdateWorkerHeartbeat updates the last_seen_at timestamp for a worker node.
func (d *DatabaseConnection) UpdateWorkerHeartbeat(nodeID string) error {
	result := d.db.Model(&WorkerNode{}).
		Where("id = ?", nodeID).
		Updates(map[string]interface{}{
			"last_seen_at": time.Now(),
			"updated_at":   time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update worker heartbeat: %w", result.Error)
	}
	return nil
}

// IncrementWorkerJobsClaimed increments the jobs_claimed counter.
func (d *DatabaseConnection) IncrementWorkerJobsClaimed(nodeID string) error {
	result := d.db.Model(&WorkerNode{}).
		Where("id = ?", nodeID).
		Updates(map[string]interface{}{
			"jobs_claimed": gorm.Expr("jobs_claimed + 1"),
			"last_seen_at": time.Now(),
			"updated_at":   time.Now(),
		})
	return result.Error
}

// IncrementWorkerJobsCompleted increments the jobs_completed counter.
func (d *DatabaseConnection) IncrementWorkerJobsCompleted(nodeID string) error {
	result := d.db.Model(&WorkerNode{}).
		Where("id = ?", nodeID).
		Updates(map[string]interface{}{
			"jobs_completed": gorm.Expr("jobs_completed + 1"),
			"last_seen_at":   time.Now(),
			"updated_at":     time.Now(),
		})
	return result.Error
}

// IncrementWorkerJobsFailed increments the jobs_failed counter.
func (d *DatabaseConnection) IncrementWorkerJobsFailed(nodeID string) error {
	result := d.db.Model(&WorkerNode{}).
		Where("id = ?", nodeID).
		Updates(map[string]interface{}{
			"jobs_failed":  gorm.Expr("jobs_failed + 1"),
			"last_seen_at": time.Now(),
			"updated_at":   time.Now(),
		})
	return result.Error
}

// SetWorkerNodeStatus updates the status of a worker node.
func (d *DatabaseConnection) SetWorkerNodeStatus(nodeID string, status WorkerNodeStatus) error {
	result := d.db.Model(&WorkerNode{}).
		Where("id = ?", nodeID).
		Updates(map[string]interface{}{
			"status":       status,
			"last_seen_at": time.Now(),
			"updated_at":   time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to set worker node status: %w", result.Error)
	}
	return nil
}

// DeregisterWorkerNode marks a worker node as stopped.
func (d *DatabaseConnection) DeregisterWorkerNode(nodeID string) error {
	return d.SetWorkerNodeStatus(nodeID, WorkerNodeStatusStopped)
}

// GetWorkerNode retrieves a worker node by ID.
func (d *DatabaseConnection) GetWorkerNode(nodeID string) (*WorkerNode, error) {
	var node WorkerNode
	result := d.db.Where("id = ?", nodeID).First(&node)
	if result.Error != nil {
		return nil, result.Error
	}
	return &node, nil
}

// GetActiveWorkerNodes retrieves all worker nodes that are currently running
// and have sent a heartbeat within the threshold.
func (d *DatabaseConnection) GetActiveWorkerNodes(heartbeatThreshold time.Duration) ([]*WorkerNode, error) {
	var nodes []*WorkerNode
	threshold := time.Now().Add(-heartbeatThreshold)

	result := d.db.Where("status = ? AND last_seen_at > ?", WorkerNodeStatusRunning, threshold).
		Order("started_at ASC").
		Find(&nodes)

	if result.Error != nil {
		return nil, result.Error
	}
	return nodes, nil
}

// GetAllWorkerNodes retrieves all worker nodes regardless of status.
func (d *DatabaseConnection) GetAllWorkerNodes() ([]*WorkerNode, error) {
	var nodes []*WorkerNode
	result := d.db.Order("started_at DESC").Find(&nodes)
	if result.Error != nil {
		return nil, result.Error
	}
	return nodes, nil
}

// GetStaleWorkerNodes retrieves worker nodes that haven't sent a heartbeat
// within the threshold.
func (d *DatabaseConnection) GetStaleWorkerNodes(heartbeatThreshold time.Duration) ([]*WorkerNode, error) {
	var nodes []*WorkerNode
	threshold := time.Now().Add(-heartbeatThreshold)

	result := d.db.Where("status = ? AND last_seen_at < ?", WorkerNodeStatusRunning, threshold).
		Find(&nodes)

	if result.Error != nil {
		return nil, result.Error
	}
	return nodes, nil
}

// CleanupStaleWorkerNodes marks stale worker nodes as stopped and returns their IDs.
func (d *DatabaseConnection) CleanupStaleWorkerNodes(heartbeatThreshold time.Duration) ([]string, error) {
	staleNodes, err := d.GetStaleWorkerNodes(heartbeatThreshold)
	if err != nil {
		return nil, err
	}

	var staleIDs []string
	for _, node := range staleNodes {
		staleIDs = append(staleIDs, node.ID)
		if err := d.SetWorkerNodeStatus(node.ID, WorkerNodeStatusStopped); err != nil {
			log.Warn().Err(err).Str("node_id", node.ID).Msg("Failed to mark stale worker as stopped")
		} else {
			log.Info().Str("node_id", node.ID).Msg("Marked stale worker node as stopped")
		}
	}

	return staleIDs, nil
}

// ResetJobsFromStaleWorkers resets claimed jobs from workers that are no longer active.
// This is more intelligent than time-based reset as it knows which workers are dead.
func (d *DatabaseConnection) ResetJobsFromStaleWorkers(heartbeatThreshold time.Duration) (int64, error) {
	// First, cleanup stale workers
	staleIDs, err := d.CleanupStaleWorkerNodes(heartbeatThreshold)
	if err != nil {
		return 0, err
	}

	if len(staleIDs) == 0 {
		return 0, nil
	}

	// Reset jobs claimed by stale workers
	result := d.db.Model(&ScanJob{}).
		Where("status IN ? AND worker_id IN ?",
			[]ScanJobStatus{ScanJobStatusClaimed, ScanJobStatusRunning},
			staleIDs).
		Updates(map[string]interface{}{
			"status":     ScanJobStatusPending,
			"worker_id":  nil,
			"claimed_at": nil,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return 0, fmt.Errorf("failed to reset jobs from stale workers: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		log.Info().
			Int64("jobs_reset", result.RowsAffected).
			Int("stale_workers", len(staleIDs)).
			Msg("Reset jobs from stale workers")
	}

	return result.RowsAffected, nil
}

// GetWorkerNodeStats returns aggregate statistics for all workers.
type WorkerNodeStats struct {
	TotalNodes     int   `json:"total_nodes"`
	RunningNodes   int   `json:"running_nodes"`
	StoppedNodes   int   `json:"stopped_nodes"`
	TotalClaimed   int64 `json:"total_claimed"`
	TotalCompleted int64 `json:"total_completed"`
	TotalFailed    int64 `json:"total_failed"`
}

// GetWorkerNodeStats retrieves aggregate statistics for worker nodes.
func (d *DatabaseConnection) GetWorkerNodeStats() (*WorkerNodeStats, error) {
	var stats WorkerNodeStats

	// Count nodes by status
	var runningCount, stoppedCount int64
	d.db.Model(&WorkerNode{}).Where("status = ?", WorkerNodeStatusRunning).Count(&runningCount)
	d.db.Model(&WorkerNode{}).Where("status = ?", WorkerNodeStatusStopped).Count(&stoppedCount)

	stats.RunningNodes = int(runningCount)
	stats.StoppedNodes = int(stoppedCount)
	stats.TotalNodes = stats.RunningNodes + stats.StoppedNodes

	// Sum job counts
	type sumResult struct {
		TotalClaimed   int64
		TotalCompleted int64
		TotalFailed    int64
	}
	var sums sumResult
	d.db.Model(&WorkerNode{}).
		Select("COALESCE(SUM(jobs_claimed), 0) as total_claimed, COALESCE(SUM(jobs_completed), 0) as total_completed, COALESCE(SUM(jobs_failed), 0) as total_failed").
		Scan(&sums)

	stats.TotalClaimed = sums.TotalClaimed
	stats.TotalCompleted = sums.TotalCompleted
	stats.TotalFailed = sums.TotalFailed

	return &stats, nil
}

// DeleteOldWorkerNodes removes worker nodes that have been stopped for longer than the retention period.
func (d *DatabaseConnection) DeleteOldWorkerNodes(retentionPeriod time.Duration) (int64, error) {
	threshold := time.Now().Add(-retentionPeriod)

	result := d.db.Where("status = ? AND updated_at < ?", WorkerNodeStatusStopped, threshold).
		Delete(&WorkerNode{})

	if result.Error != nil {
		return 0, fmt.Errorf("failed to delete old worker nodes: %w", result.Error)
	}

	return result.RowsAffected, nil
}
