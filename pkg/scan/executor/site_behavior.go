package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/scan/control"
	"github.com/rs/zerolog/log"
)

type SiteBehaviorJobData struct {
	BaseURL     string              `json:"base_url"`
	Headers     map[string][]string `json:"headers,omitempty"`
	Concurrency int                 `json:"concurrency,omitempty"`
}

type SiteBehaviorExecutor struct{}

func NewSiteBehaviorExecutor() *SiteBehaviorExecutor {
	return &SiteBehaviorExecutor{}
}

func (e *SiteBehaviorExecutor) JobType() db.ScanJobType {
	return db.ScanJobTypeSiteBehavior
}

func (e *SiteBehaviorExecutor) Execute(ctx context.Context, job *db.ScanJob, ctrl *control.ScanControl) error {
	taskLog := log.With().
		Uint("scan_id", job.ScanID).
		Uint("job_id", job.ID).
		Str("job_type", string(job.JobType)).
		Logger()

	taskLog.Info().Msg("Starting site behavior check")

	var jobData SiteBehaviorJobData
	if err := json.Unmarshal(job.Payload, &jobData); err != nil {
		return fmt.Errorf("failed to parse job payload: %w", err)
	}

	if !ctrl.CheckpointWithContext(ctx) {
		return context.Canceled
	}

	scan, err := db.Connection().GetScanByID(job.ScanID)
	if err != nil {
		return fmt.Errorf("failed to get scan %d: %w", job.ScanID, err)
	}

	httpClient := http_utils.CreateHTTPClientFromConfig(http_utils.HTTPClientConfig{
		Timeout:             scan.Options.HTTPTimeout,
		MaxIdleConns:        scan.Options.HTTPMaxIdleConns,
		MaxIdleConnsPerHost: scan.Options.HTTPMaxIdleConnsPerHost,
		MaxConnsPerHost:     scan.Options.HTTPMaxConnsPerHost,
		DisableKeepAlives:   scan.Options.HTTPDisableKeepAlives,
	})

	if transport, ok := httpClient.Transport.(*http.Transport); ok {
		transport.ForceAttemptHTTP2 = true
	}

	historyOptions := http_utils.HistoryCreationOptions{
		Source:              db.SourceScanner,
		WorkspaceID:         job.WorkspaceID,
		ScanID:              job.ScanID,
		ScanJobID:           job.ID,
		CreateNewBodyStream: true,
	}

	concurrency := jobData.Concurrency
	if concurrency == 0 {
		concurrency = 10
	}

	siteBehavior, err := http_utils.CheckSiteBehavior(http_utils.SiteBehaviourCheckOptions{
		BaseURL:                jobData.BaseURL,
		Client:                 httpClient,
		HistoryCreationOptions: historyOptions,
		Concurrency:            concurrency,
		Headers:                jobData.Headers,
	})
	if err != nil {
		return fmt.Errorf("site behavior check failed for %s: %w", jobData.BaseURL, err)
	}

	if !ctrl.CheckpointWithContext(ctx) {
		return context.Canceled
	}

	result := &db.SiteBehaviorResult{
		ScanID:             job.ScanID,
		ScanJobID:          &job.ID,
		WorkspaceID:        job.WorkspaceID,
		BaseURL:            jobData.BaseURL,
		NotFoundReturns404: siteBehavior.NotFoundReturns404,
		NotFoundChanges:    siteBehavior.NotFoundChanges,
		NotFoundCommonHash: siteBehavior.NotFoundCommonHash,
		NotFoundStatusCode: siteBehavior.NotFoundStatusCode,
	}

	if siteBehavior.BaseURLSample != nil {
		result.BaseURLSampleID = &siteBehavior.BaseURLSample.ID
	}

	if _, err = db.Connection().CreateSiteBehaviorResult(result); err != nil {
		return fmt.Errorf("failed to store site behavior result: %w", err)
	}

	for _, sample := range siteBehavior.NotFoundSamples {
		if sample == nil {
			continue
		}
		nfSample := &db.SiteBehaviorNotFoundSample{
			SiteBehaviorResultID: result.ID,
			HistoryID:            sample.ID,
		}
		if err := db.Connection().CreateSiteBehaviorNotFoundSample(nfSample); err != nil {
			taskLog.Warn().Err(err).Uint("history_id", sample.ID).Msg("Failed to store not-found sample reference")
		}
	}

	taskLog.Info().
		Str("base_url", jobData.BaseURL).
		Bool("not_found_returns_404", siteBehavior.NotFoundReturns404).
		Bool("not_found_changes", siteBehavior.NotFoundChanges).
		Int("not_found_samples", len(siteBehavior.NotFoundSamples)).
		Msg("Site behavior check completed")

	return nil
}
