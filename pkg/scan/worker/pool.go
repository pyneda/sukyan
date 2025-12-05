package worker

import (
	"fmt"
	"sync"

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
}

// PoolConfig holds pool configuration.
type PoolConfig struct {
	WorkerCount      int
	WorkerIDPrefix   string
	Queue            queue.JobQueue
	Registry         *control.Registry
	ExecutorRegistry *executor.ExecutorRegistry
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

	p := &Pool{
		workers:          make([]*Worker, cfg.WorkerCount),
		queue:            cfg.Queue,
		registry:         cfg.Registry,
		executorRegistry: cfg.ExecutorRegistry,
	}

	for i := 0; i < cfg.WorkerCount; i++ {
		p.workers[i] = New(Config{
			ID:               fmt.Sprintf("%s-%d", cfg.WorkerIDPrefix, i),
			Queue:            cfg.Queue,
			Registry:         cfg.Registry,
			ExecutorRegistry: cfg.ExecutorRegistry,
		})
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

	log.Info().Int("worker_count", len(p.workers)).Msg("Starting worker pool")

	for _, w := range p.workers {
		w.Start()
	}

	p.started = true
}

// Stop stops all workers in the pool and waits for them to finish.
func (p *Pool) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started {
		return
	}

	log.Info().Int("worker_count", len(p.workers)).Msg("Stopping worker pool")

	// Signal all workers to stop
	for _, w := range p.workers {
		w.cancel()
	}

	// Wait for all workers to finish
	for _, w := range p.workers {
		w.wg.Wait()
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
