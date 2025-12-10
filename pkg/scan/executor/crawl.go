package executor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/crawl"
	"github.com/pyneda/sukyan/pkg/scan/control"
	"github.com/rs/zerolog/log"
)

// CrawlJobData represents the payload data for a crawl job
type CrawlJobData struct {
	StartURLs       []string            `json:"start_urls"`
	MaxPagesToCrawl int                 `json:"max_pages_to_crawl"`
	MaxDepth        int                 `json:"max_depth"`
	PoolSize        int                 `json:"pool_size"`
	ExcludePatterns []string            `json:"exclude_patterns,omitempty"`
	ExtraHeaders    map[string][]string `json:"extra_headers,omitempty"`
}

// CrawlExecutor executes crawl jobs
type CrawlExecutor struct{}

// NewCrawlExecutor creates a new crawl executor
func NewCrawlExecutor() *CrawlExecutor {
	return &CrawlExecutor{}
}

// JobType returns the job type this executor handles
func (e *CrawlExecutor) JobType() db.ScanJobType {
	return db.ScanJobTypeCrawl
}

// Execute runs the crawl job
func (e *CrawlExecutor) Execute(ctx context.Context, job *db.ScanJob, ctrl *control.ScanControl) error {
	taskLog := log.With().
		Uint("scan_id", job.ScanID).
		Uint("job_id", job.ID).
		Str("job_type", string(job.JobType)).
		Logger()

	taskLog.Info().Msg("Starting crawl job execution")

	// Parse job payload
	var jobData CrawlJobData
	if err := json.Unmarshal(job.Payload, &jobData); err != nil {
		return fmt.Errorf("failed to parse job payload: %w", err)
	}

	// Checkpoint: check before starting
	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled before starting")
		return context.Canceled
	}

	// Set defaults
	if jobData.PoolSize == 0 {
		jobData.PoolSize = 5
	}

	// Fetch scan to get CaptureBrowserEvents setting
	captureBrowserEvents := false
	scan, err := db.Connection().GetScanByID(job.ScanID)
	if err != nil {
		taskLog.Warn().Err(err).Msg("Failed to get scan for browser events setting, defaulting to disabled")
	} else {
		captureBrowserEvents = scan.CaptureBrowserEvents
	}

	// Create and run crawler with scanID and scanJobID for history association
	crawler := crawl.NewCrawler(
		jobData.StartURLs,
		jobData.MaxPagesToCrawl,
		jobData.MaxDepth,
		jobData.PoolSize,
		jobData.ExcludePatterns,
		job.WorkspaceID,
		0,          // taskID - not used in new system
		job.ScanID, // scanID - used to associate history items with the scan
		job.ID,     // scanJobID - used to associate history items with this specific job
		jobData.ExtraHeaders,
		captureBrowserEvents,
	)

	// Checkpoint: check before heavy operation
	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled before crawling")
		return context.Canceled
	}

	// Execute the crawl with context for cancellation support
	historyItems := crawler.RunWithContext(ctx)

	taskLog.Info().Int("crawled_items", len(historyItems)).Msg("Crawl completed")

	// Checkpoint: check after completion
	if !ctrl.CheckpointWithContext(ctx) {
		taskLog.Info().Msg("Job cancelled after crawling")
		return context.Canceled
	}

	taskLog.Info().Msg("Crawl job completed successfully")
	return nil
}
