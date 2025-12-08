package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
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
	interactionsManager *integrations.InteractionsManager
	payloadGenerators   []*generation.PayloadGenerator
}

// NewWebSocketScanExecutor creates a new WebSocket scan executor
func NewWebSocketScanExecutor(
	interactionsManager *integrations.InteractionsManager,
	payloadGenerators []*generation.PayloadGenerator,
) *WebSocketScanExecutor {
	return &WebSocketScanExecutor{
		interactionsManager: interactionsManager,
		payloadGenerators:   payloadGenerators,
	}
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

	// Create WebSocket scanner
	scanner := scan.WebSocketScanner{
		InteractionsManager: e.interactionsManager,
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

	options := scan.WebSocketScanOptions{
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

	// Get insertion points from the target message
	var insertionPoints []scan.InsertionPoint
	if jobData.TargetMessageIndex >= 0 && jobData.TargetMessageIndex < len(wsConnection.Messages) {
		targetMsg := wsConnection.Messages[jobData.TargetMessageIndex]
		insertionPoints, _ = scan.GetWebSocketMessageInsertionPoints(&targetMsg, nil)
	}

	// Checkpoint: check before heavy operation
	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled before scanning")
		return context.Canceled
	}

	// Execute the scan
	// NOTE: Currently WebSocketScanner.Run doesn't support checkpointing internally.
	// For now, we checkpoint before and after. In phase 3, we'll add internal checkpoints.
	scanner.Run(
		wsConnection,
		wsConnection.Messages,
		jobData.TargetMessageIndex,
		e.payloadGenerators,
		insertionPoints,
		options,
	)

	// Checkpoint: check after completion
	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled after scanning")
		return context.Canceled
	}

	taskLog.Info().Msg("WebSocket scan job completed successfully")
	return nil
}
