package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/discovery"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/scan/control"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
)

// DiscoveryJobData represents the payload data for a discovery scan job
type DiscoveryJobData struct {
	BaseURL      string                   `json:"base_url"`
	Module       string                   `json:"module"` // e.g., "all", "graphql", "openapi", etc.
	ScanMode     scan_options.ScanMode    `json:"scan_mode"`
	Timeout      int                      `json:"timeout,omitempty"`
	MaxDepth     int                      `json:"max_depth,omitempty"`
	BaseHeaders  map[string][]string      `json:"base_headers,omitempty"`
	SiteBehavior *http_utils.SiteBehavior `json:"site_behavior,omitempty"`
}

// DiscoveryExecutor executes discovery scan jobs
type DiscoveryExecutor struct{}

// NewDiscoveryExecutor creates a new discovery executor
func NewDiscoveryExecutor() *DiscoveryExecutor {
	return &DiscoveryExecutor{}
}

// JobType returns the job type this executor handles
func (e *DiscoveryExecutor) JobType() db.ScanJobType {
	return db.ScanJobTypeDiscovery
}

// Execute runs the discovery scan job
func (e *DiscoveryExecutor) Execute(ctx context.Context, job *db.ScanJob, ctrl *control.ScanControl) error {
	taskLog := log.With().
		Uint("scan_id", job.ScanID).
		Uint("job_id", job.ID).
		Str("job_type", string(job.JobType)).
		Logger()

	taskLog.Info().Msg("Starting discovery scan job execution")

	// Parse job payload
	var jobData DiscoveryJobData
	if err := json.Unmarshal(job.Payload, &jobData); err != nil {
		return fmt.Errorf("failed to parse job payload: %w", err)
	}

	// Checkpoint: check before starting
	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled before starting")
		return context.Canceled
	}

	historyOptions := http_utils.HistoryCreationOptions{
		Source:              db.SourceScanner,
		WorkspaceID:         job.WorkspaceID,
		TaskID:              0,
		TaskJobID:           0,
		ScanID:              job.ScanID,
		ScanJobID:           job.ID,
		CreateNewBodyStream: true,
	}

	// Create HTTP client for discovery
	transport := http_utils.CreateHttpTransport()
	transport.ForceAttemptHTTP2 = true
	httpClient := &http.Client{
		Transport: transport,
	}

	// Build discovery options
	options := discovery.DiscoveryOptions{
		BaseURL:                jobData.BaseURL,
		HistoryCreationOptions: historyOptions,
		ScanMode:               jobData.ScanMode,
		HttpClient:             httpClient,
		BaseHeaders:            jobData.BaseHeaders,
		SiteBehavior:           jobData.SiteBehavior,
	}

	// Run the appropriate discovery module(s)
	switch jobData.Module {
	case "all":
		// Checkpoint before running all modules
		if !ctrl.CheckpointWithContext(ctx) {
			return context.Canceled
		}
		_, err := discovery.DiscoverAll(options)
		if err != nil {
			taskLog.Warn().Err(err).Msg("Some discovery modules failed")
		}
	case "graphql":
		if !ctrl.CheckpointWithContext(ctx) {
			return context.Canceled
		}
		_, _ = discovery.DiscoverGraphQLEndpoints(options)
	case "openapi":
		if !ctrl.CheckpointWithContext(ctx) {
			return context.Canceled
		}
		_, _ = discovery.DiscoverOpenapiDefinitions(options)
	case "actuator":
		if !ctrl.CheckpointWithContext(ctx) {
			return context.Canceled
		}
		_, _ = discovery.DiscoverActuatorEndpoints(options)
	case "admin":
		if !ctrl.CheckpointWithContext(ctx) {
			return context.Canceled
		}
		_, _ = discovery.DiscoverAdminInterfaces(options)
	default:
		taskLog.Warn().Str("module", jobData.Module).Msg("Unknown discovery module, running all")
		if !ctrl.CheckpointWithContext(ctx) {
			return context.Canceled
		}
		_, err := discovery.DiscoverAll(options)
		if err != nil {
			taskLog.Warn().Err(err).Msg("Some discovery modules failed")
		}
	}

	// Checkpoint: check after completion
	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled after discovery")
		return context.Canceled
	}

	taskLog.Info().Msg("Discovery scan job completed successfully")
	return nil
}
