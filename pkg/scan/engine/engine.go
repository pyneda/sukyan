package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
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
	"github.com/pyneda/sukyan/pkg/scope"

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
	MaxConcurrentPassiveScans      int
	MaxConcurrentActiveScans       int
	InteractionsManager            *integrations.InteractionsManager
	payloadGenerators              []*generation.PayloadGenerator
	passiveScanPool                *pool.Pool
	activeScanPool                 *pool.Pool
	wg                             conc.WaitGroup
	ctx                            context.Context
	cancel                         context.CancelFunc
	isPaused                       bool
	wsDeduplicationManagers        map[uint]*http_utils.WebSocketDeduplicationManager
	wsDeduplicationMu              sync.RWMutex
	wsPassiveDeduplicationManagers map[uint]*http_utils.WebSocketDeduplicationManager
	wsPassiveDeduplicationMu       sync.RWMutex
}

func NewScanEngine(payloadGenerators []*generation.PayloadGenerator, maxConcurrentPassiveScans, maxConcurrentActiveScans int, interactionsManager *integrations.InteractionsManager) *ScanEngine {
	ctx, cancel := context.WithCancel(context.Background())

	return &ScanEngine{
		MaxConcurrentPassiveScans:      maxConcurrentPassiveScans,
		MaxConcurrentActiveScans:       maxConcurrentActiveScans,
		InteractionsManager:            interactionsManager,
		payloadGenerators:              payloadGenerators,
		passiveScanPool:                pool.New().WithMaxGoroutines(maxConcurrentPassiveScans),
		activeScanPool:                 pool.New().WithMaxGoroutines(maxConcurrentActiveScans),
		ctx:                            ctx,
		cancel:                         cancel,
		wsDeduplicationManagers:        make(map[uint]*http_utils.WebSocketDeduplicationManager),
		wsPassiveDeduplicationManagers: make(map[uint]*http_utils.WebSocketDeduplicationManager),
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

// Get or create deduplication manager for a task
func (s *ScanEngine) getOrCreateWSDeduplicationManager(taskID uint, mode options.ScanMode) *http_utils.WebSocketDeduplicationManager {
	s.wsDeduplicationMu.Lock()
	defer s.wsDeduplicationMu.Unlock()

	if manager, exists := s.wsDeduplicationManagers[taskID]; exists {
		return manager
	}

	manager := http_utils.NewWebSocketDeduplicationManager(mode)
	s.wsDeduplicationManagers[taskID] = manager
	return manager
}

// Clean up deduplication manager for a task
func (s *ScanEngine) cleanupWSDeduplicationManager(taskID uint) {
	s.wsDeduplicationMu.Lock()
	defer s.wsDeduplicationMu.Unlock()

	delete(s.wsDeduplicationManagers, taskID)
}

// Get deduplication statistics for a task
func (s *ScanEngine) getWSDeduplicationStats(taskID uint) map[string]interface{} {
	s.wsDeduplicationMu.RLock()
	defer s.wsDeduplicationMu.RUnlock()

	if manager, exists := s.wsDeduplicationManagers[taskID]; exists {
		return manager.GetStatistics()
	}
	return nil
}

// Get or create passive deduplication manager for a workspace
func (s *ScanEngine) getOrCreateWSPassiveDeduplicationManager(workspaceID uint) *http_utils.WebSocketDeduplicationManager {
	s.wsPassiveDeduplicationMu.Lock()
	defer s.wsPassiveDeduplicationMu.Unlock()

	if manager, exists := s.wsPassiveDeduplicationManagers[workspaceID]; exists {
		return manager
	}
	// Use ScanModeFast for passive scanning by default to avoid too many duplicates
	manager := http_utils.NewWebSocketDeduplicationManager(options.ScanModeFast)
	s.wsPassiveDeduplicationManagers[workspaceID] = manager
	return manager
}

// Clean up passive deduplication manager for a workspace
func (s *ScanEngine) cleanupWSPassiveDeduplicationManager(workspaceID uint) {
	s.wsPassiveDeduplicationMu.Lock()
	defer s.wsPassiveDeduplicationMu.Unlock()

	delete(s.wsPassiveDeduplicationManagers, workspaceID)
}

// Get passive deduplication statistics for a workspace
func (s *ScanEngine) getWSPassiveDeduplicationStats(workspaceID uint) map[string]interface{} {
	s.wsPassiveDeduplicationMu.RLock()
	defer s.wsPassiveDeduplicationMu.RUnlock()

	if manager, exists := s.wsPassiveDeduplicationManagers[workspaceID]; exists {
		return manager.GetStatistics()
	}
	return nil
}

func (s *ScanEngine) schedulePassiveScan(item *db.History, workspaceID uint) {
	s.passiveScanPool.Go(func() {
		passive.ScanHistoryItem(item)
	})
}

func (s *ScanEngine) ScheduleWebSocketPassiveScan(item *db.WebSocketConnection, options scan.WebSocketScanOptions) {
	s.passiveScanPool.Go(func() {
		workspaceID := options.WorkspaceID
		if item.WorkspaceID != nil {
			workspaceID = *item.WorkspaceID
		}
		deduplicationManager := s.getOrCreateWSPassiveDeduplicationManager(workspaceID)

		result := passive.ScanWebSocketConnectionWithDeduplication(item, deduplicationManager)
		if result != nil && len(result.Issues) > 0 {
			log.Info().
				Uint("connection_id", result.ConnectionID).
				Int("message_count", result.MessageCount).
				Int("issues_found", len(result.Issues)).
				Msg("WebSocket passive scan completed with issues")
		} else if result != nil {
			log.Info().
				Uint("connection_id", result.ConnectionID).
				Int("message_count", result.MessageCount).
				Msg("WebSocket passive scan completed with no issues")
		} else {
			log.Warn().
				Uint("connection_id", item.ID).
				Msg("WebSocket passive scan returned nil result")
		}
	})
}

func (s *ScanEngine) scheduleActiveScan(item *db.History, options scan_options.HistoryItemScanOptions) {
	s.activeScanPool.Go(func() {
		taskJob, err := db.Connection().NewTaskJob(options.TaskID, item.TaskTitle(), db.TaskJobScheduled, item)
		if err != nil {
			log.Error().Err(err).Uint("history", item.ID).Msg("Could not create task job")
			return
		}

		s.wg.Go(func() {
			options.TaskJobID = taskJob.ID
			taskJob.Status = db.TaskJobRunning
			db.Connection().UpdateTaskJob(taskJob)

			active.ScanHistoryItem(item, s.InteractionsManager, s.payloadGenerators, options)

			taskJob.Status = db.TaskJobFinished
			taskJob.CompletedAt = time.Now()
			db.Connection().UpdateTaskJob(taskJob)
		})
	})
}

func (s *ScanEngine) FullScan(options scan_options.FullScanOptions, waitCompletion bool) (*db.Task, error) {

	task, err := db.Connection().NewTask(options.WorkspaceID, nil, options.Title, db.TaskStatusCrawling, db.TaskTypeScan)
	if err != nil {
		log.Error().Err(err).Msg("Could not create task")
	}

	s.getOrCreateWSDeduplicationManager(task.ID, options.Mode)

	// NOTE: Optimally, we would refactor the NewTask to accept the options struct directly
	task.ScanOptions = options
	db.Connection().UpdateTask(task.ID, task)
	ignoredExtensions := viper.GetStringSlice("crawl.ignored_extensions")

	scope := scope.Scope{}
	scope.CreateScopeItemsFromUrls(options.StartURLs, "www")

	scanLog := log.With().Uint("task", task.ID).Str("title", options.Title).Uint("workspace", options.WorkspaceID).Logger()
	crawler := crawl.NewCrawler(options.StartURLs, options.MaxPagesToCrawl, options.MaxDepth, options.PagesPoolSize, options.ExcludePatterns, options.WorkspaceID, task.ID, options.Headers)
	historyItems := crawler.Run()
	if len(historyItems) == 0 {
		db.Connection().SetTaskStatus(task.ID, db.TaskStatusFinished)
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
		db.Connection().SetTaskStatus(task.ID, db.TaskStatusNuclei)
		scanLog.Info().Int("count", len(fingerprintTags)).Interface("tags", fingerprintTags).Msg("Gathered tags from fingerprints for Nuclei scan")
		nucleiScanErr := integrations.NucleiScan(baseURLs, options.WorkspaceID)
		if nucleiScanErr != nil {
			scanLog.Error().Err(nucleiScanErr).Msg("Error running nuclei scan")
		}
	}

	retireScanner := integrations.NewRetireScanner()

	db.Connection().SetTaskStatus(task.ID, db.TaskStatusScanning)

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
		if options.AuditCategories.Discovery {
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

	}

	itemScanOptions := scan_options.HistoryItemScanOptions{
		WorkspaceID:        options.WorkspaceID,
		TaskID:             task.ID,
		Mode:               options.Mode,
		InsertionPoints:    options.InsertionPoints,
		FingerprintTags:    fingerprintTags,
		Fingerprints:       fingerprints,
		ExperimentalAudits: options.ExperimentalAudits,
		AuditCategories:    options.AuditCategories,
		MaxRetries:         options.MaxRetries,
	}

	websocketConnections, originalCount, _ := db.Connection().ListWebSocketConnections(db.WebSocketConnectionFilter{
		WorkspaceID: options.WorkspaceID,
		TaskID:      task.ID,
		Sources:     []string{db.SourceCrawler},
	})
	var inScopeWebsocketConnections []db.WebSocketConnection
	for _, conn := range websocketConnections {
		if scope.IsInScope(conn.URL) {
			inScopeWebsocketConnections = append(inScopeWebsocketConnections, conn)
		} else {
			scanLog.Debug().Str("url", conn.URL).Msg("WebSocket connection discovered during crawler is out of scope, skipping scan")
		}
	}
	count := int64(len(inScopeWebsocketConnections))
	if originalCount > count {
		scanLog.Warn().Int64("original_count", originalCount).Int64("count", count).Msg("Some WebSocket connections discovered during crawl are out of scope, skipping scan for them")
	} else {
		scanLog.Info().Int64("count", count).Msg("WebSocket connections discovered during crawl, scheduling scan")
	}

	if count > 0 {
		if options.AuditCategories.WebSocket {
			s.wg.Go(func() {
				scanLog.Info().Int64("count", count).Msg("Scheduling scan to the WebSocket connections discovered during crawl")
				websocketScanOptions := scan.WebSocketScanOptions{
					WorkspaceID:     options.WorkspaceID,
					TaskID:          task.ID,
					Mode:            options.Mode,
					FingerprintTags: fingerprintTags,
					ReplayMessages:  options.WebSocketOptions.ReplayMessages,
					Concurrency:     options.WebSocketOptions.Concurrency,
				}
				observationWindow := options.WebSocketOptions.ObservationWindow
				if observationWindow > 0 {
					websocketScanOptions.ObservationWindow = time.Duration(observationWindow) * time.Second
				} else {
					websocketScanOptions.ObservationWindow = 10 * time.Second
				}

				scanLog.Info().Int64("count", count).Msg("Starting WebSocket connections scan")

				scanLog.Info().Int64("count", count).Msg("Starting WebSocket passive scans")
				for _, connection := range inScopeWebsocketConnections {
					s.ScheduleWebSocketPassiveScan(&connection, websocketScanOptions)
				}
				scanLog.Info().Int64("count", count).Msg("WebSocket passive scans scheduled")

				s.EvaluateWebSocketConnections(inScopeWebsocketConnections, websocketScanOptions)
				scanLog.Info().Int64("count", count).Msg("WebSocket connections scan finished")
			})
		} else {
			scanLog.Info().Int64("count", count).Msg("WebSocket connections discovered during crawl, skipping scanning as WebSocket audit category is disabled")
		}
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
						Fingerprints:       fingerprints,
						ExperimentalAudits: options.ExperimentalAudits,
						AuditCategories:    options.AuditCategories,
						MaxRetries:         options.MaxRetries,
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
		s.activeScanPool.Wait()
		waitForTaskCompletion(task.ID)
		scanLog.Info().Msg("Active scans finished")
		db.Connection().SetTaskStatus(task.ID, db.TaskStatusFinished)
		// Log WebSocket deduplication statistics if available and cleanup the manager
		if stats := s.getWSDeduplicationStats(task.ID); stats != nil {
			scanLog.Info().Interface("websocket_dedup_stats", stats).Msg("WebSocket deduplication statistics")
		}
		s.cleanupWSDeduplicationManager(task.ID)

	} else {
		go func() {
			s.wg.Wait()
			s.activeScanPool.Wait()
			waitForTaskCompletion(task.ID)
			scanLog.Info().Msg("Active scans finished")
			db.Connection().SetTaskStatus(task.ID, db.TaskStatusFinished)
			// Log WebSocket deduplication statistics if available and cleanup the manager
			if stats := s.getWSDeduplicationStats(task.ID); stats != nil {
				scanLog.Info().Interface("websocket_dedup_stats", stats).Msg("WebSocket deduplication statistics")
			}
			s.cleanupWSDeduplicationManager(task.ID)
		}()
	}

	return task, nil
}

func (s *ScanEngine) EvaluateWebSocketConnections(connections []db.WebSocketConnection, options scan.WebSocketScanOptions) {
	cleartextHostsReported := make(map[string]bool)

	for i := range connections {
		item := &connections[i]
		u, err := url.Parse(item.URL)
		if err != nil {
			log.Error().Err(err).Str("url", item.URL).Uint("connection", item.ID).Msg("Could not parse websocket connection url URL")
			continue
		}

		if u.Scheme == "ws" && !cleartextHostsReported[u.Host] {
			cleartextHostsReported[u.Host] = true
			var taskJobID uint
			db.CreateIssueFromWebSocketConnectionAndTemplate(
				item,
				db.UnencryptedWebsocketConnectionCode,
				fmt.Sprintf("Cleartext WebSocket connections detected on host: %s", u.Host),
				100,
				"",
				&options.WorkspaceID,
				&options.TaskID,
				&taskJobID,
			)
		}

		s.scheduleWebSocketConnectionScan(item, options)
	}

	log.Info().
		Int("total_connections", len(connections)).
		Int("cleartext_hosts", len(cleartextHostsReported)).
		Msg("Completed scheduling active WebSocket connection scans")
}

func (s *ScanEngine) scheduleWebSocketConnectionScan(item *db.WebSocketConnection, options scan.WebSocketScanOptions) {
	s.activeScanPool.Go(func() {
		connectionOptions := options
		taskJob, err := db.Connection().NewWebSocketTaskJob(
			options.TaskID,
			item.TaskTitle(),
			db.TaskJobRunning,
			item,
		)
		if err != nil {
			log.Error().Err(err).Str("url", item.URL).Uint("connection", item.ID).Msg("Could not create task job for websocket connection")
			return
		}

		connectionOptions.TaskJobID = taskJob.ID

		taskJob.Status = db.TaskJobRunning
		db.Connection().UpdateTaskJob(taskJob)
		deduplicationManager := s.getOrCreateWSDeduplicationManager(options.TaskID, options.Mode)

		scan.ActiveScanWebSocketConnection(item, s.InteractionsManager, s.payloadGenerators, connectionOptions, deduplicationManager)

		taskJob.Status = db.TaskJobFinished
		taskJob.CompletedAt = time.Now()
		db.Connection().UpdateTaskJob(taskJob)
	})
}

func waitForTaskCompletion(taskID uint) {
	scanLog := log.With().Uint("task", taskID).Logger()
	for {
		hasPending, err := db.Connection().TaskHasPendingJobs(taskID)
		if err != nil {
			scanLog.Error().Err(err).Msg("Error checking pending task jobs")
			return
		}
		if !hasPending {
			break
		}
		time.Sleep(2 * time.Second)
	}
	db.Connection().SetTaskStatus(taskID, db.TaskStatusFinished)
}
