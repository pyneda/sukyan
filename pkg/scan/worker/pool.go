package worker

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/scan/control"
	"github.com/pyneda/sukyan/pkg/scan/executor"
	"github.com/pyneda/sukyan/pkg/scan/queue"
	"github.com/rs/zerolog/log"
)

// Pool manages a group of workers.
type Pool struct {
	workers          []*Worker
	mu               sync.RWMutex
	started          bool
	queue            queue.JobQueue
	registry         *control.Registry
	executorRegistry *executor.ExecutorRegistry

	// Node registration
	nodeID            string
	heartbeatInterval time.Duration
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
}

// PoolConfig holds pool configuration.
type PoolConfig struct {
	WorkerCount       int
	WorkerIDPrefix    string
	NodeID            string // Custom node ID (auto-generated if empty)
	Queue             queue.JobQueue
	Registry          *control.Registry
	ExecutorRegistry  *executor.ExecutorRegistry
	HeartbeatInterval time.Duration
	Version           string // Application version for tracking
}

// NewPool creates a new worker pool.
func NewPool(cfg PoolConfig) *Pool {
	if cfg.WorkerCount < 1 {
		cfg.WorkerCount = 5
	}
	if cfg.WorkerIDPrefix == "" {
		cfg.WorkerIDPrefix = "worker"
	}
	if cfg.ExecutorRegistry == nil {
		cfg.ExecutorRegistry = executor.DefaultRegistry
	}
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = 30 * time.Second
	}

	// Generate node ID if not provided
	nodeID := cfg.NodeID
	if nodeID == "" {
		nodeID = db.GenerateWorkerNodeID(cfg.WorkerIDPrefix)
	}

	ctx, cancel := context.WithCancel(context.Background())

	p := &Pool{
		workers:           make([]*Worker, cfg.WorkerCount),
		queue:             cfg.Queue,
		registry:          cfg.Registry,
		executorRegistry:  cfg.ExecutorRegistry,
		nodeID:            nodeID,
		heartbeatInterval: cfg.HeartbeatInterval,
		ctx:               ctx,
		cancel:            cancel,
	}

	// Create workers with node-prefixed IDs
	for i := 0; i < cfg.WorkerCount; i++ {
		p.workers[i] = New(Config{
			ID:               fmt.Sprintf("%s-%d", nodeID, i),
			Queue:            cfg.Queue,
			Registry:         cfg.Registry,
			ExecutorRegistry: cfg.ExecutorRegistry,
		})
	}

	// Register worker node in database
	hostname, _ := os.Hostname()
	node := &db.WorkerNode{
		ID:          nodeID,
		Hostname:    hostname,
		WorkerCount: cfg.WorkerCount,
		Version:     cfg.Version,
	}
	if err := db.Connection().RegisterWorkerNode(node); err != nil {
		log.Error().Err(err).Str("node_id", nodeID).Msg("Failed to register worker node")
	}

	return p
}

// Start starts all workers in the pool.
func (p *Pool) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return
	}

	log.Info().
		Int("worker_count", len(p.workers)).
		Str("node_id", p.nodeID).
		Msg("Starting worker pool")

	for _, w := range p.workers {
		w.Start()
	}

	// Start heartbeat goroutine
	p.wg.Add(1)
	go p.heartbeatLoop()

	p.started = true
}

// heartbeatLoop periodically updates the worker node's last_seen_at timestamp.
func (p *Pool) heartbeatLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			log.Debug().Str("node_id", p.nodeID).Msg("Heartbeat loop stopped")
			return
		case <-ticker.C:
			if err := db.Connection().UpdateWorkerHeartbeat(p.nodeID); err != nil {
				log.Warn().Err(err).Str("node_id", p.nodeID).Msg("Failed to update heartbeat")
			} else {
				log.Trace().Str("node_id", p.nodeID).Msg("Heartbeat updated")
			}
		}
	}
}

// Stop stops all workers in the pool and waits for them to finish.
func (p *Pool) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started {
		return
	}

	log.Info().
		Int("worker_count", len(p.workers)).
		Str("node_id", p.nodeID).
		Msg("Stopping worker pool")

	// Stop heartbeat loop
	p.cancel()

	// Signal all workers to stop
	for _, w := range p.workers {
		w.cancel()
	}

	// Wait for all workers to finish
	for _, w := range p.workers {
		w.wg.Wait()
	}

	// Wait for heartbeat loop to finish
	p.wg.Wait()

	// Release any jobs that were being processed by this worker pool
	// This ensures jobs are immediately available for other workers rather than
	// waiting for stale job detection
	releasedCount, affectedScanIDs, err := db.Connection().ReleaseJobsByWorkerNode(p.nodeID)
	if err != nil {
		log.Warn().Err(err).Str("node_id", p.nodeID).Msg("Failed to release jobs during graceful shutdown")
	} else if releasedCount > 0 {
		log.Info().
			Str("node_id", p.nodeID).
			Int64("released_jobs", releasedCount).
			Msg("Released jobs during graceful shutdown")
		// Update job counts for affected scans
		for _, scanID := range affectedScanIDs {
			db.Connection().UpdateScanJobCounts(scanID)
		}
	}

	// Deregister worker node
	if err := db.Connection().DeregisterWorkerNode(p.nodeID); err != nil {
		log.Warn().Err(err).Str("node_id", p.nodeID).Msg("Failed to deregister worker node")
	} else {
		log.Info().Str("node_id", p.nodeID).Msg("Worker node deregistered")
	}

	p.started = false
	log.Info().Msg("Worker pool stopped")
}

// WorkerCount returns the number of workers in the pool.
func (p *Pool) WorkerCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.workers)
}

// IsRunning returns true if the pool is running.
func (p *Pool) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.started
}

// WorkerIDs returns the IDs of all workers in the pool.
func (p *Pool) WorkerIDs() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ids := make([]string, len(p.workers))
	for i, w := range p.workers {
		ids[i] = w.id
	}
	return ids
}

// NodeID returns the unique identifier for this worker pool node.
func (p *Pool) NodeID() string {
	return p.nodeID
}
