package engine

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/active"
	"github.com/pyneda/sukyan/pkg/crawl"
	"github.com/pyneda/sukyan/pkg/discovery"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/passive"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan"
	"github.com/pyneda/sukyan/pkg/scan/options"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"

	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc"
	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/viper"
)

type ScanJobType string

const (
	ScanJobTypePassive ScanJobType = "passive"
	ScanJobTypeActive  ScanJobType = "active"
	ScanJobTypeAll     ScanJobType = "all"
)

type ScanEngine struct {
	MaxConcurrentPassiveScans int
	MaxConcurrentActiveScans  int
	InteractionsManager       *integrations.InteractionsManager
	payloadGenerators         []*generation.PayloadGenerator
	passiveScanPool           *pool.Pool
	activeScanPool            *pool.Pool
	wg                        conc.WaitGroup
	ctx                       context.Context
	cancel                    context.CancelFunc
	isPaused                  bool
}

func NewScanEngine(payloadGenerators []*generation.PayloadGenerator, maxConcurrentPassiveScans, maxConcurrentActiveScans int, interactionsManager *integrations.InteractionsManager) *ScanEngine {
	ctx, cancel := context.WithCancel(context.Background())

	return &ScanEngine{
		MaxConcurrentPassiveScans: maxConcurrentPassiveScans,
		MaxConcurrentActiveScans:  maxConcurrentActiveScans,
		InteractionsManager:       interactionsManager,
		payloadGenerators:         payloadGenerators,
		passiveScanPool:           pool.New().WithMaxGoroutines(maxConcurrentPassiveScans),
		activeScanPool:            pool.New().WithMaxGoroutines(maxConcurrentActiveScans),
		ctx:                       ctx,
		cancel:                    cancel,
	}
}

func (s *ScanEngine) Stop() {
	s.cancel()
	s.wg.Wait()
}

func (s *ScanEngine) Pause() {
	s.isPaused = true
}

func (s *ScanEngine) Resume() {
	s.isPaused = false
}

func (s *ScanEngine) ScheduleHistoryItemScan(item *db.History, scanJobType ScanJobType, options options.HistoryItemScanOptions) {
	if s.isPaused {
		return
	}

	switch scanJobType {
	case ScanJobTypePassive:
		s.schedulePassiveScan(item, options.WorkspaceID)
	case ScanJobTypeActive:
		s.scheduleActiveScan(item, options)
	case ScanJobTypeAll:
		s.schedulePassiveScan(item, options.WorkspaceID)
		s.scheduleActiveScan(item, options)
	}
}

func (s *ScanEngine) schedulePassiveScan(item *db.History, workspaceID uint) {
	s.passiveScanPool.Go(func() {
		passive.ScanHistoryItem(item)
	})
}

func (s *ScanEngine) scheduleActiveScan(item *db.History, options scan_options.HistoryItemScanOptions) {
	s.activeScanPool.Go(func() {
		taskJob, err := db.Connection.NewTaskJob(options.TaskID, "Active scan to "+item.URL, db.TaskJobScheduled, item.ID)
		if err != nil {
			log.Error().Err(err).Uint("history", item.ID).Msg("Could not create task job")
			return
		}

		s.wg.Go(func() {
			options.TaskJobID = taskJob.ID
			taskJob.Status = db.TaskJobRunning
			db.Connection.UpdateTaskJob(taskJob)

			active.ScanHistoryItem(item, s.InteractionsManager, s.payloadGenerators, options)

			taskJob.Status = db.TaskJobFinished
			taskJob.CompletedAt = time.Now()
			db.Connection.UpdateTaskJob(taskJob)
		})
	})
}

func (s *ScanEngine) FullScan(options scan_options.FullScanOptions, waitCompletion bool) (*db.Task, error) {
	task, err := db.Connection.NewTask(options.WorkspaceID, nil, options.Title, db.TaskStatusCrawling, db.TaskTypeScan)
	if err != nil {
		log.Error().Err(err).Msg("Could not create task")
	}
	// NOTE: Optimally, we would refactor the NewTask to accept the options struct directly
	task.ScanOptions = options
	db.Connection.UpdateTask(task.ID, task)
	ignoredExtensions := viper.GetStringSlice("crawl.ignored_extensions")

	scanLog := log.With().Uint("task", task.ID).Str("title", options.Title).Uint("workspace", options.WorkspaceID).Logger()
	crawler := crawl.NewCrawler(options.StartURLs, options.MaxPagesToCrawl, options.MaxDepth, options.PagesPoolSize, options.ExcludePatterns, options.WorkspaceID, task.ID, options.Headers)
	historyItems := crawler.Run()
	if len(historyItems) == 0 {
		db.Connection.SetTaskStatus(task.ID, db.TaskStatusFinished)
		scanLog.Info().Msg("No history items gathered during crawl, exiting")
		return task, nil
	}
	uniqueHistoryItems := removeDuplicateHistoryItems(historyItems)
	scanLog.Info().Int("count", len(uniqueHistoryItems)).Msg("Crawling finished, scheduling active scans")
	fingerprints := make([]lib.Fingerprint, 0)
	scanLog.Info().Int("count", len(fingerprints)).Interface("fingerprints", fingerprints).Msg("Gathered fingerprints")

	historiesByBaseURL := separateHistoriesByBaseURL(uniqueHistoryItems)
	for baseURL, histories := range historiesByBaseURL {
		passive.AnalyzeHeaders(baseURL, histories)
		newFingerprints := passive.FingerprintHistoryItems(histories)
		passive.ReportFingerprints(baseURL, newFingerprints, options.WorkspaceID, task.ID)
		fingerprints = append(fingerprints, newFingerprints...)
		integrations.CDNCheck(baseURL, options.WorkspaceID, task.ID)
	}

	baseURLs, err := lib.GetUniqueBaseURLs(options.StartURLs)
	if err != nil {
		log.Error().Err(err).Msg("Could not get unique base urls")
	}

	fingerprintTags := passive.GetUniqueNucleiTags(fingerprints)

	if viper.GetBool("integrations.nuclei.enabled") {
		db.Connection.SetTaskStatus(task.ID, db.TaskStatusNuclei)
		scanLog.Info().Int("count", len(fingerprintTags)).Interface("tags", fingerprintTags).Msg("Gathered tags from fingerprints for Nuclei scan")
		nucleiScanErr := integrations.NucleiScan(baseURLs, options.WorkspaceID)
		if nucleiScanErr != nil {
			scanLog.Error().Err(nucleiScanErr).Msg("Error running nuclei scan")
		}
	}

	retireScanner := integrations.NewRetireScanner()

	db.Connection.SetTaskStatus(task.ID, db.TaskStatusScanning)

	transport := http_utils.CreateHttpTransport()
	transport.ForceAttemptHTTP2 = true
	discoveryClient := &http.Client{
		Transport: transport,
	}

	for _, baseURL := range baseURLs {
		createOpts := http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: options.WorkspaceID,
			TaskID:      task.ID,
		}
		siteBehaviour, err := http_utils.CheckSiteBehavior(http_utils.SiteBehaviourCheckOptions{
			BaseURL:                baseURL,
			Client:                 discoveryClient,
			HistoryCreationOptions: createOpts,
			Concurrency:            10,
			Headers:                options.Headers,
		})
		if err != nil {
			scanLog.Error().Err(err).Str("base_url", baseURL).Msg("Could not check site behavior")
			continue
		}
		discoverOpts := discovery.DiscoveryOptions{
			BaseURL:                baseURL,
			HistoryCreationOptions: createOpts,
			HttpClient:             discoveryClient,
			SiteBehavior:           siteBehaviour,
			BaseHeaders:            options.Headers,
			ScanMode:               options.Mode,
		}
		discovery.DiscoverAll(discoverOpts)
	}

	itemScanOptions := scan_options.HistoryItemScanOptions{
		WorkspaceID:        options.WorkspaceID,
		TaskID:             task.ID,
		Mode:               options.Mode,
		InsertionPoints:    options.InsertionPoints,
		FingerprintTags:    fingerprintTags,
		ExperimentalAudits: options.ExperimentalAudits,
		AuditCategories:    options.AuditCategories,
	}

	websocketConnections, count, _ := db.Connection.ListWebSocketConnections(db.WebSocketConnectionFilter{
		WorkspaceID: options.WorkspaceID,
		TaskID:      task.ID,
		Sources:     []string{db.SourceCrawler},
	})
	if count > 0 {
		go scan.EvaluateWebSocketConnections(websocketConnections, s.InteractionsManager, s.payloadGenerators, itemScanOptions)
		scanLog.Info().Int64("count", count).Msg("Scheduled scan to the WebSocket connections discovered during crawl")
	} else {
		scanLog.Info().Msg("No WebSocket connections discovered during crawl")
	}
	scheduledURLPaths := make(map[string]bool)

	s.wg.Go(func() {
		for _, historyItem := range uniqueHistoryItems {
			if historyItem.StatusCode == 404 {
				continue
			}

			go retireScanner.HistoryScan(historyItem)

			shouldSkip := false
			for _, extension := range ignoredExtensions {
				if strings.HasSuffix(historyItem.URL, extension) {
					shouldSkip = true
					break
				}
			}

			if shouldSkip {
				continue
			}

			// Schedule the active scan trying to avoid scanning the same URL path multiple times
			if lib.SliceContains(itemScanOptions.InsertionPoints, "urlpath") {
				normalizedURLPath, err := lib.NormalizeURLPath(historyItem.URL)
				if err != nil {
					scanLog.Error().Err(err).Str("url", historyItem.URL).Uint("history", historyItem.ID).Msg("Skipping scanning history item as could not normalize URL path")
					continue
				}
				if _, exists := scheduledURLPaths[normalizedURLPath]; exists {
					scanOptions := scan_options.HistoryItemScanOptions{
						WorkspaceID:        options.WorkspaceID,
						TaskID:             task.ID,
						Mode:               options.Mode,
						InsertionPoints:    lib.FilterOutString(options.InsertionPoints, "urlpath"),
						FingerprintTags:    fingerprintTags,
						ExperimentalAudits: options.ExperimentalAudits,
						AuditCategories:    options.AuditCategories,
					}
					s.ScheduleHistoryItemScan(historyItem, ScanJobTypeAll, scanOptions)
				} else {
					s.ScheduleHistoryItemScan(historyItem, ScanJobTypeAll, itemScanOptions)
					scheduledURLPaths[normalizedURLPath] = true
				}
			} else {
				s.ScheduleHistoryItemScan(historyItem, ScanJobTypeAll, itemScanOptions)
			}
		}
	})

	scanLog.Info().Msg("Active scans scheduled")

	if waitCompletion {
		time.Sleep(2 * time.Second)
		s.wg.Wait()
		scanLog.Info().Msg("Active scans finished")
		db.Connection.SetTaskStatus(task.ID, db.TaskStatusFinished)
	} else {
		go func() {
			s.wg.Wait()
			scanLog.Info().Msg("Active scans finished")
			db.Connection.SetTaskStatus(task.ID, db.TaskStatusFinished)
		}()
	}

	return task, nil
}
