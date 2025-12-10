// Package manager provides the ScanManager which coordinates all scanning operations.
package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan"
	"github.com/pyneda/sukyan/pkg/scan/circuitbreaker"
	"github.com/pyneda/sukyan/pkg/scan/control"
	"github.com/pyneda/sukyan/pkg/scan/executor"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/pyneda/sukyan/pkg/scan/orchestrator"
	"github.com/pyneda/sukyan/pkg/scan/queue"
	"github.com/pyneda/sukyan/pkg/scan/ratelimit"
	"github.com/pyneda/sukyan/pkg/scan/worker"
	"github.com/rs/zerolog/log"
)

// Compile-time assertion that ScanManager implements orchestrator.JobScheduler
var _ orchestrator.JobScheduler = (*ScanManager)(nil)

// Config holds configuration for the ScanManager.
type Config struct {
	// WorkerCount is the number of workers to run.
	WorkerCount int
	// WorkerIDPrefix is the prefix for worker IDs.
	WorkerIDPrefix string
	// NodeID is the unique identifier for this worker node. If empty, auto-generated.
	NodeID string
	// Version is the application version for tracking.
	Version string
	// PollInterval is how often workers poll for jobs.
	PollInterval time.Duration
	// RefreshInterval is how often to refresh control state from DB.
	RefreshInterval time.Duration
	// StaleJobThreshold is how long before a claimed job is considered stale.
	StaleJobThreshold time.Duration
	// HeartbeatInterval is how often to update the worker node heartbeat.
	HeartbeatInterval time.Duration
	// StaleRecoveryInterval is how often to run the stale job recovery loop.
	StaleRecoveryInterval time.Duration
	// ScanID limits workers to only process jobs for this scan (isolated mode for CLI).
	// If nil, workers process jobs for all scans.
	ScanID *uint
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		WorkerCount:           5,
		WorkerIDPrefix:        "worker",
		NodeID:                "", // Auto-generated if empty
		Version:               "",
		PollInterval:          100 * time.Millisecond,
		RefreshInterval:       3 * time.Second,
		StaleJobThreshold:     2 * time.Minute,
		HeartbeatInterval:     30 * time.Second,
		StaleRecoveryInterval: 30 * time.Second,
	}
}

// ScanManager coordinates all scanning operations within a process.
// It manages the worker pool, control registry, and provides methods for
// creating and controlling scans.
type ScanManager struct {
	config              Config
	dbConn              *db.DatabaseConnection
	queue               *queue.PostgresQueue
	registry            *control.Registry
	workerPool          *worker.Pool
	orchestrator        *orchestrator.Orchestrator
	executorRegistry    *executor.ExecutorRegistry
	rateLimiter         ratelimit.RateLimiter
	circuitBreaker      circuitbreaker.CircuitBreaker
	interactionsManager *integrations.InteractionsManager
	payloadGenerators   []*generation.PayloadGenerator

	ctx       context.Context
	cancel    context.CancelFunc
	stopCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	mu        sync.RWMutex
	started   bool
}

// New creates a new ScanManager.
func New(cfg Config, dbConn *db.DatabaseConnection, interactionsManager *integrations.InteractionsManager, payloadGenerators []*generation.PayloadGenerator) *ScanManager {
	if cfg.WorkerCount < 1 {
		cfg.WorkerCount = DefaultConfig().WorkerCount
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = DefaultConfig().PollInterval
	}
	if cfg.RefreshInterval == 0 {
		cfg.RefreshInterval = DefaultConfig().RefreshInterval
	}
	if cfg.StaleJobThreshold == 0 {
		cfg.StaleJobThreshold = DefaultConfig().StaleJobThreshold
	}
	if cfg.StaleRecoveryInterval == 0 {
		cfg.StaleRecoveryInterval = DefaultConfig().StaleRecoveryInterval
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create executor registry and register executors
	execRegistry := executor.NewExecutorRegistry()

	sm := &ScanManager{
		config:              cfg,
		dbConn:              dbConn,
		queue:               queue.NewPostgresQueue(dbConn),
		registry:            control.NewRegistry(dbConn),
		executorRegistry:    execRegistry,
		rateLimiter:         ratelimit.NewNoOpRateLimiter(),
		circuitBreaker:      circuitbreaker.NewNoOpCircuitBreaker(),
		interactionsManager: interactionsManager,
		payloadGenerators:   payloadGenerators,
		ctx:                 ctx,
		cancel:              cancel,
		stopCh:              make(chan struct{}),
	}

	// Register executors
	sm.registerExecutors()

	return sm
}

// registerExecutors sets up all the job executors.
func (sm *ScanManager) registerExecutors() {
	// Register active scan executor
	sm.executorRegistry.Register(executor.NewActiveScanExecutor(
		sm.interactionsManager,
		sm.payloadGenerators,
	))

	// Register WebSocket scan executor
	sm.executorRegistry.Register(executor.NewWebSocketScanExecutor(
		sm.interactionsManager,
		sm.payloadGenerators,
	))

	// Register discovery executor
	sm.executorRegistry.Register(executor.NewDiscoveryExecutor())

	// Register crawl executor
	sm.executorRegistry.Register(executor.NewCrawlExecutor())

	log.Debug().Msg("Registered all scan executors")
}

// Start initializes and starts the scan manager.
// This recovers state from the database and starts the worker pool.
func (sm *ScanManager) Start() error {
	var startErr error
	sm.startOnce.Do(func() {
		log.Info().Msg("Starting ScanManager")

		// Recover state from database
		if err := sm.recover(); err != nil {
			log.Error().Err(err).Msg("Failed to recover scan manager state")
			startErr = err
			return
		}

		// Create and start worker pool
		sm.workerPool = worker.NewPool(worker.PoolConfig{
			WorkerCount:       sm.config.WorkerCount,
			WorkerIDPrefix:    sm.config.WorkerIDPrefix,
			NodeID:            sm.config.NodeID,
			Queue:             sm.queue,
			Registry:          sm.registry,
			ExecutorRegistry:  sm.executorRegistry,
			HeartbeatInterval: sm.config.HeartbeatInterval,
			Version:           sm.config.Version,
			ScanID:            sm.config.ScanID,
		})
		sm.workerPool.Start()

		// Create and start orchestrator for phase management
		// ScanManager implements orchestrator.JobScheduler interface
		sm.orchestrator = orchestrator.New(sm, orchestrator.DefaultConfig())
		sm.orchestrator.Start(sm.ctx)

		// Start periodic refresh
		sm.registry.StartPeriodicRefresh(sm.config.RefreshInterval, sm.stopCh)

		// Start stale job recovery loop
		go sm.startStaleJobRecoveryLoop()

		sm.mu.Lock()
		sm.started = true
		sm.mu.Unlock()

		log.Info().Int("workers", sm.config.WorkerCount).Msg("ScanManager started")
	})
	return startErr
}

// Stop gracefully stops the scan manager.
func (sm *ScanManager) Stop() {
	sm.stopOnce.Do(func() {
		log.Info().Msg("Stopping ScanManager")

		// Signal stop to all goroutines
		close(sm.stopCh)
		sm.cancel()

		// Stop orchestrator
		if sm.orchestrator != nil {
			sm.orchestrator.Stop()
		}

		// Stop worker pool
		if sm.workerPool != nil {
			sm.workerPool.Stop()
		}

		sm.mu.Lock()
		sm.started = false
		sm.mu.Unlock()

		log.Info().Msg("ScanManager stopped")
	})
}

// recover initializes state from the database on startup.
func (sm *ScanManager) recover() error {
	log.Info().Msg("Recovering scan manager state from database")

	// Recover control registry state
	if err := sm.registry.RecoverFromDB(); err != nil {
		return fmt.Errorf("failed to recover control registry: %w", err)
	}

	// Reset jobs from stale workers (workers that haven't sent heartbeat)
	// This is more intelligent than pure time-based reset
	workerResetCount, staleWorkerScanIDs, err := sm.dbConn.ResetJobsFromStaleWorkers(sm.config.StaleJobThreshold)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to reset jobs from stale workers")
	} else if workerResetCount > 0 {
		log.Info().Int64("count", workerResetCount).Msg("Reset jobs from stale workers during recovery")
		// Update job counts for affected scans
		for _, scanID := range staleWorkerScanIDs {
			sm.dbConn.UpdateScanJobCounts(scanID)
		}
	}

	// Reset jobs that have exceeded their max_duration
	resetCount, failedCount, affectedScanIDs, err := sm.dbConn.ResetTimedOutJobs()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to reset timed out jobs during recovery")
	} else if resetCount > 0 || failedCount > 0 {
		log.Info().
			Int64("reset", resetCount).
			Int64("failed", failedCount).
			Msg("Processed timed out jobs during recovery")
		// Update job counts for affected scans
		for _, scanID := range affectedScanIDs {
			sm.dbConn.UpdateScanJobCounts(scanID)
		}
	}

	// Also reset any remaining stale claimed jobs (fallback for edge cases)
	count, err := sm.queue.ResetAllStaleJobs(sm.ctx, sm.config.StaleJobThreshold)
	if err != nil {
		return fmt.Errorf("failed to reset stale jobs: %w", err)
	}
	if count > 0 {
		log.Info().Int64("count", count).Msg("Reset stale jobs during recovery")
	}

	return nil
}

// StaleJobRecoveryAdvisoryLockID is the PostgreSQL advisory lock ID for the stale job recovery loop.
// This ensures only one ScanManager instance runs the recovery loop at a time.
const StaleJobRecoveryAdvisoryLockID = 8675309 // Arbitrary unique ID

// startStaleJobRecoveryLoop runs periodically to recover jobs from stale workers and timed out jobs.
// Uses PostgreSQL advisory locks to ensure only one instance runs the recovery loop.
func (sm *ScanManager) startStaleJobRecoveryLoop() {
	ticker := time.NewTicker(sm.config.StaleRecoveryInterval)
	defer ticker.Stop()

	log.Info().
		Dur("interval", sm.config.StaleRecoveryInterval).
		Msg("Started stale job recovery loop")

	for {
		select {
		case <-sm.stopCh:
			log.Debug().Msg("Stale job recovery loop stopped")
			return
		case <-ticker.C:
			sm.runStaleJobRecovery()
		}
	}
}

// runStaleJobRecovery attempts to acquire an advisory lock and run recovery operations.
func (sm *ScanManager) runStaleJobRecovery() {
	// Try to acquire advisory lock (non-blocking)
	var acquired bool
	err := sm.dbConn.DB().Raw("SELECT pg_try_advisory_lock(?)", StaleJobRecoveryAdvisoryLockID).Scan(&acquired).Error
	if err != nil {
		log.Warn().Err(err).Msg("Failed to acquire advisory lock for stale job recovery")
		return
	}

	if !acquired {
		// Another instance is running recovery, skip this iteration
		log.Trace().Msg("Skipping stale job recovery - another instance holds the lock")
		return
	}

	// Ensure we release the lock when done
	defer func() {
		if err := sm.dbConn.DB().Exec("SELECT pg_advisory_unlock(?)", StaleJobRecoveryAdvisoryLockID).Error; err != nil {
			log.Warn().Err(err).Msg("Failed to release advisory lock for stale job recovery")
		}
	}()

	// 1. Cleanup stale worker nodes and reset their jobs
	workerResetCount, staleWorkerScanIDs, err := sm.dbConn.ResetJobsFromStaleWorkers(sm.config.StaleJobThreshold)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to reset jobs from stale workers")
	} else if workerResetCount > 0 {
		log.Info().Int64("count", workerResetCount).Msg("Reset jobs from stale workers during periodic recovery")
		// Update job counts for affected scans
		for _, scanID := range staleWorkerScanIDs {
			sm.dbConn.UpdateScanJobCounts(scanID)
		}
	}

	// 2. Reset jobs that have exceeded their max_duration
	resetCount, failedCount, affectedScanIDs, err := sm.dbConn.ResetTimedOutJobs()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to reset timed out jobs")
	} else if resetCount > 0 || failedCount > 0 {
		log.Info().
			Int64("reset", resetCount).
			Int64("failed", failedCount).
			Msg("Processed timed out jobs during periodic recovery")
		// Update job counts for affected scans
		for _, scanID := range affectedScanIDs {
			sm.dbConn.UpdateScanJobCounts(scanID)
		}
	}

	// 3. Fallback: reset any remaining stale claimed jobs
	count, err := sm.queue.ResetAllStaleJobs(sm.ctx, sm.config.StaleJobThreshold)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to reset stale claimed jobs")
	} else if count > 0 {
		log.Info().Int64("count", count).Msg("Reset stale claimed jobs during periodic recovery")
	}
}

// CreateScanRecord creates a scan in the database without requiring a running manager.
// If isolated is true, the scan's jobs can only be claimed by workers with matching scan ID filter.
// This is used by CLI to create the scan before starting the manager for isolation.
func CreateScanRecord(dbConn *db.DatabaseConnection, opts options.FullScanOptions, isolated bool) (*db.Scan, error) {
	now := time.Now()
	scan := &db.Scan{
		WorkspaceID:          opts.WorkspaceID,
		Title:                opts.Title,
		Status:               db.ScanStatusPending,
		Options:              opts,
		StartedAt:            &now,
		MaxConcurrentJobs:    opts.MaxConcurrentJobs,
		MaxRPS:               opts.MaxRPS,
		Isolated:             isolated,
		CaptureBrowserEvents: opts.CaptureBrowserEvents,
	}

	scan, err := dbConn.CreateScan(scan)
	if err != nil {
		return nil, fmt.Errorf("failed to create scan: %w", err)
	}

	log.Info().
		Uint("scan_id", scan.ID).
		Str("title", scan.Title).
		Uint("workspace_id", scan.WorkspaceID).
		Bool("isolated", isolated).
		Msg("Created new scan record")

	return scan, nil
}

// CreateScan creates a new scan and registers it with the control registry.
// Scans created through the manager are not isolated (can be processed by any worker).
// Requires the manager to be started.
func (sm *ScanManager) CreateScan(opts options.FullScanOptions) (*db.Scan, error) {
	sm.mu.RLock()
	if !sm.started {
		sm.mu.RUnlock()
		return nil, fmt.Errorf("scan manager not started")
	}
	sm.mu.RUnlock()

	scan, err := CreateScanRecord(sm.dbConn, opts, false)
	if err != nil {
		return nil, err
	}

	// Register scan control
	sm.registry.Register(scan.ID, control.StateRunning)

	return scan, nil
}

// StartFullScan creates and starts a full scan using the orchestrator for phase management.
// This is the preferred method for starting scans as it manages the complete lifecycle:
// crawl → fingerprint → discovery → nuclei → active_scan → websocket
func (sm *ScanManager) StartFullScan(opts options.FullScanOptions) (*db.Scan, error) {
	// Create the scan first
	scan, err := sm.CreateScan(opts)
	if err != nil {
		return nil, err
	}

	// Start the scan through the orchestrator
	if err := sm.orchestrator.StartScan(scan.ID); err != nil {
		// If orchestrator fails to start, update scan status to failed
		sm.dbConn.SetScanStatus(scan.ID, db.ScanStatusFailed)
		return nil, fmt.Errorf("failed to start scan through orchestrator: %w", err)
	}

	// Refresh scan from DB to get updated status
	scan, _ = sm.dbConn.GetScanByID(scan.ID)

	log.Info().
		Uint("scan_id", scan.ID).
		Str("title", scan.Title).
		Str("status", string(scan.Status)).
		Str("phase", string(scan.Phase)).
		Msg("Started full scan through orchestrator")

	return scan, nil
}

// StartScan starts an existing scan through the orchestrator.
// Use this when the scan was created separately (e.g., for CLI isolation where
// the scan must be created before the manager starts to configure worker filters).
func (sm *ScanManager) StartScan(scanID uint) error {
	sm.mu.RLock()
	if !sm.started {
		sm.mu.RUnlock()
		return fmt.Errorf("scan manager not started")
	}
	sm.mu.RUnlock()

	// Register scan control for pause/resume/cancel support
	sm.registry.Register(scanID, control.StateRunning)

	// Start the scan through the orchestrator
	if err := sm.orchestrator.StartScan(scanID); err != nil {
		sm.dbConn.SetScanStatus(scanID, db.ScanStatusFailed)
		return fmt.Errorf("failed to start scan through orchestrator: %w", err)
	}

	log.Info().Uint("scan_id", scanID).Msg("Started existing scan through orchestrator")
	return nil
}

// PauseScan pauses a running scan.
func (sm *ScanManager) PauseScan(scanID uint) error {
	// Update database first
	_, err := sm.dbConn.PauseScan(scanID)
	if err != nil {
		return err
	}

	// Then update in-memory state
	sm.registry.SetPaused(scanID)

	log.Info().Uint("scan_id", scanID).Msg("Scan paused")
	return nil
}

// ResumeScan resumes a paused scan.
func (sm *ScanManager) ResumeScan(scanID uint) error {
	// Update database first
	_, err := sm.dbConn.ResumeScan(scanID)
	if err != nil {
		return err
	}

	// Then update in-memory state
	sm.registry.SetRunning(scanID)

	log.Info().Uint("scan_id", scanID).Msg("Scan resumed")
	return nil
}

// CancelScan cancels a scan and all its pending jobs.
func (sm *ScanManager) CancelScan(scanID uint) error {
	// Update database first (this also cancels pending jobs)
	_, err := sm.dbConn.CancelScan(scanID)
	if err != nil {
		return err
	}

	// Then update in-memory state
	sm.registry.SetCancelled(scanID)

	// Unregister after cancellation (with a small delay to let workers finish)
	go func() {
		time.Sleep(5 * time.Second)
		sm.registry.Unregister(scanID)
	}()

	log.Info().Uint("scan_id", scanID).Msg("Scan cancelled")
	return nil
}

// GetScan retrieves a scan by ID.
func (sm *ScanManager) GetScan(scanID uint) (*db.Scan, error) {
	return sm.dbConn.GetScanByID(scanID)
}

// ListScans lists scans matching the filter.
func (sm *ScanManager) ListScans(filter db.ScanFilter) ([]*db.Scan, int64, error) {
	return sm.dbConn.ListScans(filter)
}

// CancelJobs cancels jobs matching the filter for a scan.
func (sm *ScanManager) CancelJobs(scanID uint, filter db.ScanJobFilter) (int64, error) {
	return sm.dbConn.CancelScanJobs(scanID, filter)
}

// ScheduleHistoryItemScan schedules active scans for history items.
func (sm *ScanManager) ScheduleHistoryItemScan(scanID uint, workspaceID uint, items []*db.History, opts options.HistoryItemScanOptions) error {
	if len(items) == 0 {
		return nil
	}

	// Get fingerprints from scan checkpoint if available
	scan, scanErr := sm.dbConn.GetScanByID(scanID)
	var fingerprintTags []string
	var fingerprints []lib.Fingerprint
	if scanErr == nil && scan.Checkpoint != nil {
		fingerprintTags = scan.Checkpoint.FingerprintTags
		fingerprints = scan.Checkpoint.Fingerprints
	}

	jobs := make([]*db.ScanJob, 0, len(items))
	for _, item := range items {
		// Extract target host
		targetHost := ""
		hasQueryParams := false
		if u, err := url.Parse(item.URL); err == nil {
			targetHost = u.Host
			hasQueryParams = u.RawQuery != ""
		}

		// Calculate priority: higher for non-GET requests or requests with query parameters
		priority := 0
		if item.Method != "GET" || hasQueryParams {
			priority = 2
		}

		// Build payload for the executor
		jobData := executor.ActiveScanJobData{
			HistoryID:          item.ID,
			Mode:               opts.Mode,
			InsertionPoints:    opts.InsertionPoints,
			AuditCategories:    opts.AuditCategories,
			ExperimentalAudits: opts.ExperimentalAudits,
			FingerprintTags:    fingerprintTags,
			Fingerprints:       fingerprints,
			MaxRetries:         opts.MaxRetries,
		}
		payload, _ := json.Marshal(jobData)

		job := &db.ScanJob{
			ScanID:      scanID,
			WorkspaceID: workspaceID,
			Status:      db.ScanJobStatusPending,
			JobType:     db.ScanJobTypeActiveScan,
			Priority:    priority,
			TargetHost:  targetHost,
			URL:         item.URL,
			Method:      item.Method,
			HistoryID:   &item.ID,
			Payload:     payload,
		}
		jobs = append(jobs, job)
	}

	if err := sm.queue.EnqueueBatch(sm.ctx, jobs); err != nil {
		return fmt.Errorf("failed to enqueue jobs: %w", err)
	}

	// Update job counts
	sm.dbConn.UpdateScanJobCounts(scanID)

	log.Debug().
		Uint("scan_id", scanID).
		Int("count", len(jobs)).
		Msg("Scheduled history item scans")

	return nil
}

// scheduleWebSocketScanForConnection schedules a WebSocket scan job for a single connection.
func (sm *ScanManager) scheduleWebSocketScanForConnection(scanID uint, workspaceID uint, conn *db.WebSocketConnection, opts scan.WebSocketScanOptions) error {
	targetHost := ""
	if u, err := url.Parse(conn.URL); err == nil {
		targetHost = u.Host
	}

	// Get fingerprint tags from scan checkpoint if available
	scan, scanErr := sm.dbConn.GetScanByID(scanID)
	var fingerprintTags []string
	if scanErr == nil && scan.Checkpoint != nil {
		fingerprintTags = scan.Checkpoint.FingerprintTags
	}

	// Build payload for the executor
	jobData := executor.WebSocketScanJobData{
		WebSocketConnectionID: conn.ID,
		TargetMessageIndex:    0, // Default to first message, can be customized
		Mode:                  opts.Mode,
		ReplayMessages:        opts.ReplayMessages,
		Concurrency:           opts.Concurrency,
		ObservationWindow:     int(opts.ObservationWindow.Seconds()),
		FingerprintTags:       fingerprintTags,
		RunPassiveScan:        true, // Always run passive scan with active scan
	}
	payload, _ := json.Marshal(jobData)

	job := &db.ScanJob{
		ScanID:                scanID,
		WorkspaceID:           workspaceID,
		Status:                db.ScanJobStatusPending,
		JobType:               db.ScanJobTypeWebSocketScan,
		Priority:              0,
		TargetHost:            targetHost,
		URL:                   conn.URL,
		Method:                "GET",
		WebSocketConnectionID: &conn.ID,
		Payload:               payload,
	}

	if err := sm.queue.Enqueue(sm.ctx, job); err != nil {
		return fmt.Errorf("failed to enqueue websocket job: %w", err)
	}

	sm.dbConn.UpdateScanJobCounts(scanID)

	return nil
}

// scheduleDiscoveryForURL schedules a discovery job for a single base URL.
func (sm *ScanManager) scheduleDiscoveryForURL(scanID uint, workspaceID uint, baseURL string, scanMode options.ScanMode) error {
	// Check if a discovery job already exists for this scan + URL combination
	// This prevents duplicate jobs in distributed environments where multiple orchestrators may run
	exists, err := sm.dbConn.DiscoveryJobExistsForURL(scanID, baseURL)
	if err != nil {
		log.Warn().Err(err).Uint("scan_id", scanID).Str("url", baseURL).Msg("Failed to check for existing discovery job")
		// Continue anyway - worst case we create a duplicate that will be handled
	} else if exists {
		log.Debug().Uint("scan_id", scanID).Str("url", baseURL).Msg("Discovery job already exists, skipping")
		return nil
	}

	targetHost := ""
	if u, err := url.Parse(baseURL); err == nil {
		targetHost = u.Host
	}

	// Get scan to retrieve headers and site behavior
	scan, scanErr := sm.dbConn.GetScanByID(scanID)
	var baseHeaders map[string][]string
	var siteBehavior *http_utils.SiteBehavior
	if scanErr == nil {
		baseHeaders = scan.Options.Headers
		// Get site behavior from checkpoint if available
		if scan.Checkpoint != nil && scan.Checkpoint.SiteBehaviors != nil {
			if sb, ok := scan.Checkpoint.SiteBehaviors[baseURL]; ok {
				siteBehavior = &http_utils.SiteBehavior{
					NotFoundReturns404: sb.NotFoundReturns404,
					NotFoundChanges:    sb.NotFoundChanges,
					CommonHash:         sb.CommonHash,
				}
			}
		}
	}

	// Build payload for the executor
	jobData := executor.DiscoveryJobData{
		BaseURL:      baseURL,
		Module:       "all", // Run all discovery modules
		ScanMode:     scanMode,
		BaseHeaders:  baseHeaders,
		SiteBehavior: siteBehavior,
	}
	payload, _ := json.Marshal(jobData)

	job := &db.ScanJob{
		ScanID:      scanID,
		WorkspaceID: workspaceID,
		Status:      db.ScanJobStatusPending,
		JobType:     db.ScanJobTypeDiscovery,
		Priority:    10, // Higher priority than regular scans
		TargetHost:  targetHost,
		URL:         baseURL,
		Method:      "GET",
		Payload:     payload,
	}

	if err := sm.queue.Enqueue(sm.ctx, job); err != nil {
		return fmt.Errorf("failed to enqueue discovery job: %w", err)
	}

	sm.dbConn.UpdateScanJobCounts(scanID)

	return nil
}

// ScheduleCrawlWithOptions schedules a crawl job with specific options.
// This method is used directly by the API when creating scans with custom crawl settings.
func (sm *ScanManager) ScheduleCrawlWithOptions(scanID uint, workspaceID uint, startURLs []string, maxPages, maxDepth, poolSize int, excludePatterns []string, headers map[string][]string) error {
	targetHost := ""
	if len(startURLs) > 0 {
		if u, err := url.Parse(startURLs[0]); err == nil {
			targetHost = u.Host
		}
	}

	// Build payload for the executor
	jobData := executor.CrawlJobData{
		StartURLs:       startURLs,
		MaxPagesToCrawl: maxPages,
		MaxDepth:        maxDepth,
		PoolSize:        poolSize,
		ExcludePatterns: excludePatterns,
		ExtraHeaders:    headers,
	}
	payload, _ := json.Marshal(jobData)

	job := &db.ScanJob{
		ScanID:      scanID,
		WorkspaceID: workspaceID,
		Status:      db.ScanJobStatusPending,
		JobType:     db.ScanJobTypeCrawl,
		Priority:    20, // Highest priority - crawl first
		TargetHost:  targetHost,
		URL:         startURLs[0],
		Method:      "GET",
		Payload:     payload,
	}

	if err := sm.queue.Enqueue(sm.ctx, job); err != nil {
		return fmt.Errorf("failed to enqueue crawl job: %w", err)
	}

	sm.dbConn.UpdateScanJobCounts(scanID)

	return nil
}

// ScheduleNuclei schedules a Nuclei scan job.
func (sm *ScanManager) ScheduleNuclei(scanID uint, workspaceID uint, baseURLs []string) error {
	// Nuclei runs against all URLs at once
	targetHost := ""
	targetURL := ""
	if len(baseURLs) > 0 {
		targetURL = baseURLs[0] // Store first URL for reference
		if u, err := url.Parse(baseURLs[0]); err == nil {
			targetHost = u.Host
		}
	}

	// Note: Nuclei executor not implemented yet, but job structure is ready
	job := &db.ScanJob{
		ScanID:      scanID,
		WorkspaceID: workspaceID,
		Status:      db.ScanJobStatusPending,
		JobType:     db.ScanJobTypeNuclei,
		Priority:    5,
		TargetHost:  targetHost,
		URL:         targetURL,
	}

	if err := sm.queue.Enqueue(sm.ctx, job); err != nil {
		return fmt.Errorf("failed to enqueue nuclei job: %w", err)
	}

	sm.dbConn.UpdateScanJobCounts(scanID)

	return nil
}

// GetQueueStats returns queue statistics for a scan.
func (sm *ScanManager) GetQueueStats(scanID uint) (*queue.QueueStats, error) {
	return sm.queue.Stats(sm.ctx, scanID)
}

// IsStarted returns whether the manager has been started.
func (sm *ScanManager) IsStarted() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.started
}

// WorkerCount returns the number of workers.
func (sm *ScanManager) WorkerCount() int {
	if sm.workerPool == nil {
		return 0
	}
	return sm.workerPool.WorkerCount()
}

// NodeID returns the unique identifier for this worker pool node.
func (sm *ScanManager) NodeID() string {
	if sm.workerPool == nil {
		return ""
	}
	return sm.workerPool.NodeID()
}

// SetScanIDFilter sets the scan ID filter for all workers.
// When set, workers will only claim jobs for this specific scan (isolated mode).
// This is typically called after creating a scan to bind the workers to it.
func (sm *ScanManager) SetScanIDFilter(scanID uint) {
	if sm.workerPool != nil {
		sm.workerPool.SetScanID(scanID)
	}
}

// GetControlRegistry returns the control registry.
func (sm *ScanManager) GetControlRegistry() *control.Registry {
	return sm.registry
}

// GetOrchestrator returns the orchestrator instance.
func (sm *ScanManager) GetOrchestrator() *orchestrator.Orchestrator {
	return sm.orchestrator
}

// SetRateLimiter sets a custom rate limiter.
func (sm *ScanManager) SetRateLimiter(rl ratelimit.RateLimiter) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.rateLimiter = rl
}

// SetCircuitBreaker sets a custom circuit breaker.
func (sm *ScanManager) SetCircuitBreaker(cb circuitbreaker.CircuitBreaker) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.circuitBreaker = cb
}

// =============================================================================
// orchestrator.JobScheduler interface implementation
// =============================================================================

// ScheduleActiveScan implements orchestrator.JobScheduler interface.
// It schedules active scanning jobs for the given history item IDs.
func (sm *ScanManager) ScheduleActiveScan(ctx context.Context, scanID uint, historyIDs []uint) error {
	// Get scan to retrieve workspace and options
	scanEntity, err := sm.dbConn.GetScanByID(scanID)
	if err != nil {
		return fmt.Errorf("failed to get scan: %w", err)
	}

	// Get history items
	items := make([]*db.History, 0, len(historyIDs))
	for _, hid := range historyIDs {
		item, err := sm.dbConn.GetHistory(hid)
		if err != nil {
			log.Warn().Err(err).Uint("history_id", hid).Msg("Failed to get history item, skipping")
			continue
		}
		items = append(items, &item)
	}

	// Build options from scan configuration
	opts := options.HistoryItemScanOptions{
		WorkspaceID:        scanEntity.WorkspaceID,
		Mode:               scanEntity.Options.Mode,
		InsertionPoints:    scanEntity.Options.InsertionPoints,
		ExperimentalAudits: scanEntity.Options.ExperimentalAudits,
		AuditCategories:    scanEntity.Options.AuditCategories,
		MaxRetries:         scanEntity.Options.MaxRetries,
	}

	return sm.ScheduleHistoryItemScan(scanID, scanEntity.WorkspaceID, items, opts)
}

// ScheduleActiveScanWithOptions implements orchestrator.JobScheduler interface.
// It schedules active scanning jobs for history items with custom options.
// This is used when different items need different insertion points (e.g., URL path deduplication).
func (sm *ScanManager) ScheduleActiveScanWithOptions(ctx context.Context, scanID uint, historyIDs []uint, excludeInsertionPoints []string) error {
	// Get scan to retrieve workspace and options
	scanEntity, err := sm.dbConn.GetScanByID(scanID)
	if err != nil {
		return fmt.Errorf("failed to get scan: %w", err)
	}

	// Get history items
	items := make([]*db.History, 0, len(historyIDs))
	for _, hid := range historyIDs {
		item, err := sm.dbConn.GetHistory(hid)
		if err != nil {
			log.Warn().Err(err).Uint("history_id", hid).Msg("Failed to get history item, skipping")
			continue
		}
		items = append(items, &item)
	}

	// Filter out excluded insertion points
	filteredInsertionPoints := make([]string, 0, len(scanEntity.Options.InsertionPoints))
	for _, ip := range scanEntity.Options.InsertionPoints {
		excluded := false
		for _, excludeIP := range excludeInsertionPoints {
			if ip == excludeIP {
				excluded = true
				break
			}
		}
		if !excluded {
			filteredInsertionPoints = append(filteredInsertionPoints, ip)
		}
	}

	// Build options from scan configuration with filtered insertion points
	opts := options.HistoryItemScanOptions{
		WorkspaceID:        scanEntity.WorkspaceID,
		Mode:               scanEntity.Options.Mode,
		InsertionPoints:    filteredInsertionPoints,
		ExperimentalAudits: scanEntity.Options.ExperimentalAudits,
		AuditCategories:    scanEntity.Options.AuditCategories,
		MaxRetries:         scanEntity.Options.MaxRetries,
	}

	return sm.ScheduleHistoryItemScan(scanID, scanEntity.WorkspaceID, items, opts)
}

// ScheduleWebSocketScan implements orchestrator.JobScheduler interface.
// It schedules websocket scanning jobs for the given connection IDs.
func (sm *ScanManager) ScheduleWebSocketScan(ctx context.Context, scanID uint, connectionIDs []uint) error {
	// Get scan to retrieve workspace and options
	scanEntity, err := sm.dbConn.GetScanByID(scanID)
	if err != nil {
		return fmt.Errorf("failed to get scan: %w", err)
	}

	// Schedule each websocket connection
	for _, connID := range connectionIDs {
		conn, err := sm.dbConn.GetWebSocketConnection(connID)
		if err != nil {
			log.Warn().Err(err).Uint("connection_id", connID).Msg("Failed to get websocket connection, skipping")
			continue
		}

		// Calculate observation window
		observationWindow := time.Duration(scanEntity.Options.WebSocketOptions.ObservationWindow) * time.Second
		if observationWindow <= 0 {
			observationWindow = 10 * time.Second
		}

		wsOpts := scan.WebSocketScanOptions{
			Mode:              scanEntity.Options.Mode,
			ReplayMessages:    scanEntity.Options.WebSocketOptions.ReplayMessages,
			Concurrency:       scanEntity.Options.WebSocketOptions.Concurrency,
			ObservationWindow: observationWindow,
		}

		if err := sm.scheduleWebSocketScanForConnection(scanID, scanEntity.WorkspaceID, conn, wsOpts); err != nil {
			log.Warn().Err(err).Uint("connection_id", connID).Msg("Failed to schedule websocket scan")
		}
	}

	return nil
}

// ScheduleDiscovery implements orchestrator.JobScheduler interface.
// It schedules discovery/fuzzing jobs for the given base URLs.
func (sm *ScanManager) ScheduleDiscovery(ctx context.Context, scanID uint, baseURLs []string) error {
	// Get scan to retrieve workspace and options
	scanEntity, err := sm.dbConn.GetScanByID(scanID)
	if err != nil {
		return fmt.Errorf("failed to get scan: %w", err)
	}

	for _, baseURL := range baseURLs {
		if err := sm.scheduleDiscoveryForURL(scanID, scanEntity.WorkspaceID, baseURL, scanEntity.Options.Mode); err != nil {
			log.Warn().Err(err).Str("url", baseURL).Msg("Failed to schedule discovery")
		}
	}

	return nil
}

// ScheduleCrawl implements orchestrator.JobScheduler interface.
// It schedules crawling jobs for the given URLs.
func (sm *ScanManager) ScheduleCrawl(ctx context.Context, scanID uint, urls []string) error {
	// Get scan to retrieve workspace and options
	scanEntity, err := sm.dbConn.GetScanByID(scanID)
	if err != nil {
		return fmt.Errorf("failed to get scan: %w", err)
	}

	return sm.ScheduleCrawlWithOptions(
		scanID,
		scanEntity.WorkspaceID,
		urls,
		scanEntity.Options.MaxPagesToCrawl,
		scanEntity.Options.MaxDepth,
		scanEntity.Options.PagesPoolSize,
		scanEntity.Options.ExcludePatterns,
		scanEntity.Options.Headers,
	)
}
