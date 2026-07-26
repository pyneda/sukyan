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

	// cswshTestedEndpoints tracks, per scan, the endpoints already probed for
	// CSWSH. A crawl records one connection per page visit, so the same socket
	// is captured many times; without this the handshake matrix is re-run - and
	// an issue re-reported - once per connection instead of once per endpoint.
	cswshTestedEndpoints map[uint]map[string]bool

	// permissiveOriginHosts tracks, per scan, the hosts for which the low-severity
	// permissive-origin note has already been raised. That finding is a host-level
	// hardening observation, so it is deduplicated per host rather than per
	// endpoint to avoid one low-value row per endpoint on the same server.
	permissiveOriginHosts map[uint]map[string]bool
	cswshMu               sync.Mutex
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
		cswshTestedEndpoints:  make(map[uint]map[string]bool),
		permissiveOriginHosts: make(map[uint]map[string]bool),
	}
}

// claimCSWSHEndpoint reports whether CSWSH still needs probing for this scan and
// endpoint, claiming it atomically so concurrent jobs for connections to the
// same socket probe it exactly once.
func (e *WebSocketScanExecutor) claimCSWSHEndpoint(scanID uint, connectionURL string) bool {
	key := http_utils.EndpointKey(connectionURL)

	e.cswshMu.Lock()
	defer e.cswshMu.Unlock()

	if e.cswshTestedEndpoints[scanID] == nil {
		e.cswshTestedEndpoints[scanID] = make(map[string]bool)
	}
	if e.cswshTestedEndpoints[scanID][key] {
		return false
	}
	e.cswshTestedEndpoints[scanID][key] = true
	return true
}

// claimPermissiveOriginHost reports whether the low-severity permissive-origin
// note still needs raising for this scan and host, claiming it atomically so the
// note is raised once per host rather than once per endpoint.
func (e *WebSocketScanExecutor) claimPermissiveOriginHost(scanID uint, host string) bool {
	if host == "" {
		return true
	}

	e.cswshMu.Lock()
	defer e.cswshMu.Unlock()

	if e.permissiveOriginHosts[scanID] == nil {
		e.permissiveOriginHosts[scanID] = make(map[string]bool)
	}
	if e.permissiveOriginHosts[scanID][host] {
		return false
	}
	e.permissiveOriginHosts[scanID][host] = true
	return true
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

	e.cswshMu.Lock()
	delete(e.cswshTestedEndpoints, scanID)
	delete(e.permissiveOriginHosts, scanID)
	e.cswshMu.Unlock()
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

	// CSWSH is a property of the endpoint, not of an individual connection, so
	// probe each endpoint once per scan rather than once per captured connection.
	if e.claimCSWSHEndpoint(job.ScanID, wsConnection.URL) {
		taskLog.Info().Msg("Running CSWSH detection")
		cswshOpts := active.CSWSHScanOptions{
			WebSocketScanOptions: opts,
			TestNullOrigin:       true,
			TestMissingOrigin:    false, // a real browser always sends Origin; testing it is a pointless request
			TestSubdomains:       true,
			MessageTimeout:       5 * time.Second,
			ConnectionTimeout:    30 * time.Second,
			PermissiveOriginGate: func(host string) bool {
				return e.claimPermissiveOriginHost(job.ScanID, host)
			},
		}
		cswshResult, err := active.ScanForCSWSH(wsConnection, cswshOpts, e.interactionsManager)
		if err != nil {
			taskLog.Warn().Err(err).Msg("CSWSH check failed")
		} else if cswshResult != nil {
			taskLog.Info().
				Str("verdict", string(cswshResult.Verdict)).
				Int("confidence", cswshResult.Confidence).
				Int("origins_tested", len(cswshResult.CrossOriginTests)).
				Msg("CSWSH check completed")
		}
	} else {
		taskLog.Debug().Str("url", wsConnection.URL).Msg("Skipping CSWSH, endpoint already probed in this scan")
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
