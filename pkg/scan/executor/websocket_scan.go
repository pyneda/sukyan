package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/active"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/passive"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan"
	"github.com/pyneda/sukyan/pkg/scan/control"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
)

// WebSocketScanJobData represents the payload data for a WebSocket scan job
type WebSocketScanJobData struct {
	WebSocketConnectionID uint                  `json:"websocket_connection_id"`
	TargetMessageIndex    int                   `json:"target_message_index"`
	Mode                  scan_options.ScanMode `json:"mode"`
	ReplayMessages        bool                  `json:"replay_messages"`
	Concurrency           int                   `json:"concurrency,omitempty"`
	ObservationWindow     int                   `json:"observation_window,omitempty"` // in seconds
	FingerprintTags       []string              `json:"fingerprint_tags,omitempty"`
	RunPassiveScan        bool                  `json:"run_passive_scan"`
}

// WebSocketScanExecutor executes WebSocket scan jobs
type WebSocketScanExecutor struct {
	interactionsManager   *integrations.InteractionsManager
	payloadGenerators     []*generation.PayloadGenerator
	deduplicationManagers map[uint]*http_utils.WebSocketDeduplicationManager // per-scan dedup managers
	deduplicationMu       sync.RWMutex
}

// NewWebSocketScanExecutor creates a new WebSocket scan executor
func NewWebSocketScanExecutor(
	interactionsManager *integrations.InteractionsManager,
	payloadGenerators []*generation.PayloadGenerator,
) *WebSocketScanExecutor {
	return &WebSocketScanExecutor{
		interactionsManager:   interactionsManager,
		payloadGenerators:     payloadGenerators,
		deduplicationManagers: make(map[uint]*http_utils.WebSocketDeduplicationManager),
	}
}

// getOrCreateDeduplicationManager gets or creates a deduplication manager for a scan
func (e *WebSocketScanExecutor) getOrCreateDeduplicationManager(scanID uint, mode scan_options.ScanMode) *http_utils.WebSocketDeduplicationManager {
	e.deduplicationMu.Lock()
	defer e.deduplicationMu.Unlock()

	if manager, exists := e.deduplicationManagers[scanID]; exists {
		return manager
	}

	manager := http_utils.NewWebSocketDeduplicationManager(mode)
	e.deduplicationManagers[scanID] = manager
	return manager
}

// CleanupDeduplicationManager removes the deduplication manager for a scan
// Call this when a scan is complete to free memory
func (e *WebSocketScanExecutor) CleanupDeduplicationManager(scanID uint) {
	e.deduplicationMu.Lock()
	defer e.deduplicationMu.Unlock()

	if manager, exists := e.deduplicationManagers[scanID]; exists {
		stats := manager.GetStatistics()
		log.Info().
			Uint("scan_id", scanID).
			Interface("stats", stats).
			Msg("WebSocket deduplication statistics before cleanup")
	}
	delete(e.deduplicationManagers, scanID)
}

// JobType returns the job type this executor handles
func (e *WebSocketScanExecutor) JobType() db.ScanJobType {
	return db.ScanJobTypeWebSocketScan
}

// Execute runs the WebSocket scan job
func (e *WebSocketScanExecutor) Execute(ctx context.Context, job *db.ScanJob, ctrl *control.ScanControl) error {
	taskLog := log.With().
		Uint("scan_id", job.ScanID).
		Uint("job_id", job.ID).
		Str("job_type", string(job.JobType)).
		Logger()

	taskLog.Info().Msg("Starting WebSocket scan job execution")

	// Parse job payload
	var jobData WebSocketScanJobData
	if err := json.Unmarshal(job.Payload, &jobData); err != nil {
		return fmt.Errorf("failed to parse job payload: %w", err)
	}

	// Checkpoint: check before starting
	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled before starting")
		return context.Canceled
	}

	// Fetch the WebSocket connection
	wsConnection, err := db.Connection().GetWebSocketConnection(jobData.WebSocketConnectionID)
	if err != nil {
		return fmt.Errorf("failed to get WebSocket connection %d: %w", jobData.WebSocketConnectionID, err)
	}

	// Build scan options
	concurrency := jobData.Concurrency
	if concurrency == 0 {
		concurrency = 10
	}

	observationWindow := time.Duration(jobData.ObservationWindow) * time.Second
	if observationWindow <= 0 {
		observationWindow = 10 * time.Second
	}

	opts := scan_options.WebSocketScanOptions{
		WorkspaceID:       job.WorkspaceID,
		TaskID:            0, // New scan system doesn't use tasks
		TaskJobID:         0,
		ScanID:            job.ScanID,
		ScanJobID:         job.ID,
		Mode:              jobData.Mode,
		ReplayMessages:    jobData.ReplayMessages,
		Concurrency:       concurrency,
		ObservationWindow: observationWindow,
		FingerprintTags:   jobData.FingerprintTags,
	}

	// Run passive scan first if enabled
	if jobData.RunPassiveScan {
		passiveResult := passive.ScanWebSocketConnection(wsConnection)
		if passiveResult != nil && len(passiveResult.Issues) > 0 {
			taskLog.Info().
				Uint("connection_id", passiveResult.ConnectionID).
				Int("issues_found", len(passiveResult.Issues)).
				Msg("WebSocket passive scan completed with issues")
		}
	}

	// Checkpoint: check before connection-level security checks
	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled before security checks")
		return context.Canceled
	}

	taskLog.Info().Msg("Running CSWSH detection")
	cswshOpts := active.CSWSHScanOptions{
		WebSocketScanOptions: opts,
		TestNullOrigin:       true,
		TestMissingOrigin:    true,
		TestSubdomains:       true,
		MessageTimeout:       5 * time.Second,
		ConnectionTimeout:    30 * time.Second,
	}
	cswshResult, err := active.ScanForCSWSH(wsConnection, cswshOpts, e.interactionsManager)
	if err != nil {
		taskLog.Warn().Err(err).Msg("CSWSH check failed")
	} else if cswshResult != nil && cswshResult.Vulnerable {
		taskLog.Warn().
			Int("confidence", cswshResult.Confidence).
			Int("origins_tested", len(cswshResult.CrossOriginTests)).
			Msg("CSWSH vulnerability detected")
	}

	// Checkpoint: check before message scanning
	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled before message scanning")
		return context.Canceled
	}

	// Get or create deduplication manager for this scan
	deduplicationManager := e.getOrCreateDeduplicationManager(job.ScanID, jobData.Mode)

	scan.ActiveScanWebSocketConnection(
		wsConnection,
		e.interactionsManager,
		e.payloadGenerators,
		opts,
		deduplicationManager,
	)

	// Checkpoint: check after completion
	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled after scanning")
		return context.Canceled
	}

	taskLog.Info().Msg("WebSocket scan job completed successfully")
	return nil
}
