package executor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/active"
	"github.com/pyneda/sukyan/pkg/passive"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan/control"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
)

// ActiveScanJobData represents the payload data for an active scan job
type ActiveScanJobData struct {
	HistoryID          uint                         `json:"history_id"`
	Mode               scan_options.ScanMode        `json:"mode"`
	InsertionPoints    []string                     `json:"insertion_points,omitempty"`
	AuditCategories    scan_options.AuditCategories `json:"audit_categories"`
	ExperimentalAudits bool                         `json:"experimental_audits,omitempty"`
	FingerprintTags    []string                     `json:"fingerprint_tags,omitempty"`
	Fingerprints       []lib.Fingerprint            `json:"fingerprints,omitempty"`
	MaxRetries         int                          `json:"max_retries,omitempty"`
}

// ActiveScanExecutor executes active scan jobs on history items
type ActiveScanExecutor struct {
	interactionsManager *integrations.InteractionsManager
	payloadGenerators   []*generation.PayloadGenerator
}

// NewActiveScanExecutor creates a new active scan executor
func NewActiveScanExecutor(
	interactionsManager *integrations.InteractionsManager,
	payloadGenerators []*generation.PayloadGenerator,
) *ActiveScanExecutor {
	return &ActiveScanExecutor{
		interactionsManager: interactionsManager,
		payloadGenerators:   payloadGenerators,
	}
}

// JobType returns the job type this executor handles
func (e *ActiveScanExecutor) JobType() db.ScanJobType {
	return db.ScanJobTypeActiveScan
}

// Execute runs the active scan job
func (e *ActiveScanExecutor) Execute(ctx context.Context, job *db.ScanJob, ctrl *control.ScanControl) error {
	taskLog := log.With().
		Uint("scan_id", job.ScanID).
		Uint("job_id", job.ID).
		Str("job_type", string(job.JobType)).
		Logger()

	taskLog.Info().Msg("Starting active scan job execution")

	// Parse job payload
	var jobData ActiveScanJobData
	if err := json.Unmarshal(job.Payload, &jobData); err != nil {
		return fmt.Errorf("failed to parse job payload: %w", err)
	}

	// Checkpoint: check before starting
	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled before starting")
		return context.Canceled
	}

	// Fetch the history item
	history, err := db.Connection().GetHistory(jobData.HistoryID)
	if err != nil {
		return fmt.Errorf("failed to get history item %d: %w", jobData.HistoryID, err)
	}

	// Build scan options
	options := scan_options.HistoryItemScanOptions{
		Ctx:                ctx,
		WorkspaceID:        job.WorkspaceID,
		TaskID:             0, // New scan system doesn't use tasks
		TaskJobID:          0,
		ScanID:             job.ScanID,
		ScanJobID:          job.ID,
		Mode:               jobData.Mode,
		InsertionPoints:    jobData.InsertionPoints,
		AuditCategories:    jobData.AuditCategories,
		ExperimentalAudits: jobData.ExperimentalAudits,
		FingerprintTags:    jobData.FingerprintTags,
		Fingerprints:       jobData.Fingerprints,
		MaxRetries:         jobData.MaxRetries,
	}

	// Checkpoint: check before heavy operation
	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled before scanning")
		return context.Canceled
	}

	// Run passive scan first if enabled (lightweight, no HTTP requests)
	if options.AuditCategories.Passive {
		taskLog.Debug().Msg("Running passive scan on history item")
		passive.ScanHistoryItem(&history)
	}

	// Execute the active scan
	// NOTE: Currently ScanHistoryItem doesn't support checkpointing internally.
	// For now, we checkpoint before and after. In phase 3, we'll add internal checkpoints.
	active.ScanHistoryItem(&history, e.interactionsManager, e.payloadGenerators, options)

	// Checkpoint: check after completion
	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled after scanning")
		return context.Canceled
	}

	taskLog.Info().Msg("Active scan job completed successfully")
	return nil
}

func init() {
	// Note: The executor will be registered by the manager after setting up dependencies
}
