// Package orchestrator manages full scan lifecycle and phase transitions.
// It monitors running scans and automatically transitions them through phases:
// crawl → fingerprint → discovery → nuclei → active_scan → websocket
package orchestrator

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
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/passive"
	"github.com/pyneda/sukyan/pkg/scope"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// ScanPhase represents a phase in the full scan lifecycle
type ScanPhase string

const (
	PhaseCrawl        ScanPhase = "crawl"
	PhaseFingerprint  ScanPhase = "fingerprint"
	PhaseSiteBehavior ScanPhase = "site_behavior"
	PhaseDiscovery    ScanPhase = "discovery"
	PhaseNuclei       ScanPhase = "nuclei"
	PhaseAPIBehavior  ScanPhase = "api_behavior"
	PhaseActiveScan   ScanPhase = "active_scan"
	PhaseWebSocket    ScanPhase = "websocket"
	PhaseComplete     ScanPhase = "complete"
)

// PhaseOrder defines the sequence of phases in a full scan
var PhaseOrder = []ScanPhase{
	PhaseCrawl,
	PhaseFingerprint,
	PhaseSiteBehavior,
	PhaseDiscovery,
	PhaseNuclei,
	PhaseAPIBehavior,
	PhaseActiveScan,
	PhaseWebSocket,
	PhaseComplete,
}

// JobScheduler interface for scheduling jobs - decouples orchestrator from ScanManager
type JobScheduler interface {
	// ScheduleActiveScan schedules active scanning jobs for history items
	ScheduleActiveScan(ctx context.Context, scanID uint, historyIDs []uint) error
	// ScheduleActiveScanWithOptions schedules active scanning jobs for history items with custom options
	// This is used when different items need different insertion points (e.g., URL path deduplication)
	ScheduleActiveScanWithOptions(ctx context.Context, scanID uint, historyIDs []uint, excludeInsertionPoints []string) error
	// ScheduleWebSocketScan schedules websocket scanning jobs
	ScheduleWebSocketScan(ctx context.Context, scanID uint, connectionIDs []uint) error
	// ScheduleDiscovery schedules discovery/fuzzing jobs for URLs
	ScheduleDiscovery(ctx context.Context, scanID uint, baseURLs []string) error
	// ScheduleCrawl schedules crawling jobs
	ScheduleCrawl(ctx context.Context, scanID uint, urls []string) error
	// ScheduleSiteBehavior schedules site behavior check jobs for base URLs
	ScheduleSiteBehavior(ctx context.Context, scanID uint, baseURLs []string) error
	// ScheduleAPIBehavior schedules API behavior check jobs for API definitions
	ScheduleAPIBehavior(ctx context.Context, scanID uint) error
	// ScheduleAPIScan schedules API scanning jobs for discovered API definitions
	ScheduleAPIScan(ctx context.Context, scanID uint) error
}

// Config holds orchestrator configuration
type Config struct {
	// PollInterval is how often to check scan status
	PollInterval time.Duration
	// PhaseTimeout is the maximum duration for a phase before timeout
	PhaseTimeout time.Duration
	// EnableFingerprint enables fingerprinting phase
	EnableFingerprint bool
	// EnableDiscovery enables discovery/fuzzing phase
	EnableDiscovery bool
	// EnableAPIScan enables API scanning phase
	EnableAPIScan bool
	// EnableNuclei enables nuclei scanning phase
	EnableNuclei bool
	// EnableWebSocket enables websocket scanning phase
	EnableWebSocket bool
}

// DefaultConfig returns default orchestrator configuration
func DefaultConfig() Config {
	return Config{
		PollInterval:      10 * time.Second,
		PhaseTimeout:      2 * time.Hour,
		EnableFingerprint: true,
		EnableDiscovery:   true,
		EnableAPIScan:     true,
		EnableNuclei:      true,
		EnableWebSocket:   true,
	}
}

// Orchestrator manages full scan lifecycle and phase transitions
type Orchestrator struct {
	config         Config
	scheduler      JobScheduler
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	running        bool
	retireScanner  integrations.RetireScanner
	httpClient     *http.Client
	siteBehaviors  map[uint]map[string]*http_utils.SiteBehavior // scanID -> baseURL -> behavior
	siteBehaviorMu sync.RWMutex
}

// New creates a new Orchestrator
func New(scheduler JobScheduler, config Config) *Orchestrator {
	transport := http_utils.CreateHttpTransport()
	transport.ForceAttemptHTTP2 = true

	return &Orchestrator{
		config:        config,
		scheduler:     scheduler,
		retireScanner: integrations.NewRetireScanner(),
		httpClient: &http.Client{
			Transport: transport,
		},
		siteBehaviors: make(map[uint]map[string]*http_utils.SiteBehavior),
	}
}

// Start begins the orchestration loop
func (o *Orchestrator) Start(ctx context.Context) {
	o.mu.Lock()
	if o.running {
		o.mu.Unlock()
		return
	}
	o.ctx, o.cancel = context.WithCancel(ctx)
	o.running = true
	o.mu.Unlock()

	o.wg.Add(1)
	go o.monitorLoop()

	log.Info().Msg("Orchestrator started")
}

// Stop halts the orchestration loop
func (o *Orchestrator) Stop() {
	o.mu.Lock()
	if !o.running {
		o.mu.Unlock()
		return
	}
	o.cancel()
	o.running = false
	o.mu.Unlock()

	o.wg.Wait()
	log.Info().Msg("Orchestrator stopped")
}

// IsRunning returns whether the orchestrator is currently running.
func (o *Orchestrator) IsRunning() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.running
}

// GetConfig returns the orchestrator configuration.
func (o *Orchestrator) GetConfig() Config {
	return o.config
}

// monitorLoop periodically checks scans and manages phase transitions
func (o *Orchestrator) monitorLoop() {
	defer o.wg.Done()

	ticker := time.NewTicker(o.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			o.checkScans()
		}
	}
}

// checkScans reviews all running scans and processes phase transitions
func (o *Orchestrator) checkScans() {
	// Get all running scans (crawling or scanning status)
	scans, err := db.Connection().GetActiveScans()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get running scans")
		return
	}

	for _, s := range scans {
		if err := o.processScan(s); err != nil {
			log.Error().Err(err).Uint("scan_id", s.ID).Msg("Failed to process scan")
		}
	}
}

// processScan checks a scan's current phase and transitions if needed
func (o *Orchestrator) processScan(scanEntity *db.Scan) error {
	// Check if phase is complete (no pending, claimed, or running jobs)
	hasPendingJobs, err := db.Connection().ScanHasPendingJobs(scanEntity.ID)
	if err != nil {
		return fmt.Errorf("failed to check pending jobs: %w", err)
	}

	// If there are still pending/running jobs, wait for them to complete
	if hasPendingJobs {
		log.Debug().
			Uint("scan_id", scanEntity.ID).
			Str("phase", string(scanEntity.Phase)).
			Msg("Scan has pending or running jobs, waiting")
		return nil
	}

	// Phase complete, transition to next
	return o.transitionToNextPhase(scanEntity)
}

// transitionToNextPhase moves a scan to its next phase
func (o *Orchestrator) transitionToNextPhase(scanEntity *db.Scan) error {
	currentPhase := ScanPhase(scanEntity.Phase)
	nextPhase := o.getNextPhase(currentPhase, scanEntity)

	log.Info().
		Uint("scan_id", scanEntity.ID).
		Str("current_phase", string(currentPhase)).
		Str("next_phase", string(nextPhase)).
		Msg("Transitioning scan to next phase")

	// Atomically update scan phase - this is safe for distributed systems
	// where multiple orchestrators may be running
	transitioned, err := db.Connection().AtomicSetScanPhase(scanEntity.ID, db.ScanPhase(currentPhase), db.ScanPhase(nextPhase))
	if err != nil {
		return fmt.Errorf("failed to update scan phase: %w", err)
	}

	// Another orchestrator already transitioned this phase, skip
	if !transitioned {
		log.Debug().
			Uint("scan_id", scanEntity.ID).
			Str("expected_phase", string(currentPhase)).
			Str("target_phase", string(nextPhase)).
			Msg("Phase transition skipped - already transitioned by another process")
		return nil
	}
	scanEntity.Phase = db.ScanPhase(nextPhase)

	// Start the next phase
	switch nextPhase {
	case PhaseCrawl:
		return o.startCrawlPhase(scanEntity)
	case PhaseFingerprint:
		return o.startFingerprintPhase(scanEntity)
	case PhaseSiteBehavior:
		return o.startSiteBehaviorPhase(scanEntity)
	case PhaseAPIBehavior:
		return o.startAPIBehaviorPhase(scanEntity)
	case PhaseDiscovery:
		return o.startDiscoveryPhase(scanEntity)
	case PhaseNuclei:
		return o.startNucleiPhase(scanEntity)
	case PhaseActiveScan:
		return o.startActiveScanPhase(scanEntity)
	case PhaseWebSocket:
		return o.startWebSocketPhase(scanEntity)
	case PhaseComplete:
		return o.completeScan(scanEntity)
	default:
		return fmt.Errorf("unknown phase: %s", nextPhase)
	}
}

// getNextPhase returns the next phase after the current one
func (o *Orchestrator) getNextPhase(current ScanPhase, scanEntity *db.Scan) ScanPhase {
	for i, phase := range PhaseOrder {
		if phase == current && i < len(PhaseOrder)-1 {
			next := PhaseOrder[i+1]
			// Skip disabled phases based on config and scan options
			if next == PhaseFingerprint && !o.config.EnableFingerprint {
				return o.getNextPhase(PhaseFingerprint, scanEntity)
			}
			if next == PhaseDiscovery && (!o.config.EnableDiscovery || !scanEntity.Options.AuditCategories.Discovery) {
				return o.getNextPhase(PhaseDiscovery, scanEntity)
			}
			if next == PhaseNuclei && (!o.config.EnableNuclei || !viper.GetBool("integrations.nuclei.enabled")) {
				return o.getNextPhase(PhaseNuclei, scanEntity)
			}
			if next == PhaseWebSocket && (!o.config.EnableWebSocket || !scanEntity.Options.AuditCategories.WebSocket) {
				return o.getNextPhase(PhaseWebSocket, scanEntity)
			}
			return next
		}
	}
	return PhaseComplete
}

// getScopeForScan creates a scope from the scan's start URLs
func (o *Orchestrator) getScopeForScan(scanEntity *db.Scan) scope.Scope {
	s := scope.Scope{}
	s.CreateScopeItemsFromUrls(scanEntity.Options.StartURLs, "www")
	return s
}

// initializeScopeInCheckpoint stores scope domains in the scan checkpoint
func (o *Orchestrator) initializeScopeInCheckpoint(scanEntity *db.Scan) error {
	if scanEntity.Checkpoint == nil {
		scanEntity.Checkpoint = &db.ScanCheckpoint{}
	}

	// Extract domains from start URLs
	domains := make([]string, 0, len(scanEntity.Options.StartURLs))
	for _, startURL := range scanEntity.Options.StartURLs {
		u, err := url.Parse(startURL)
		if err == nil {
			domains = append(domains, u.Hostname())
		}
	}
	scanEntity.Checkpoint.ScopeDomains = domains

	_, err := db.Connection().UpdateScan(scanEntity)
	return err
}

// startCrawlPhase initiates the crawl phase
func (o *Orchestrator) startCrawlPhase(scanEntity *db.Scan) error {
	scanLog := log.With().Uint("scan_id", scanEntity.ID).Logger()
	scanLog.Info().Msg("Starting crawl phase")

	// Initialize scope in checkpoint
	if err := o.initializeScopeInCheckpoint(scanEntity); err != nil {
		scanLog.Error().Err(err).Msg("Failed to initialize scope in checkpoint")
	}

	// Get target URLs from scan configuration
	urls := o.getTargetURLs(scanEntity)
	if len(urls) == 0 {
		// Check if this is an API-only scan (has API definitions but no start URLs)
		hasAPIDefinitions, _ := db.Connection().HasLinkedAPIDefinitions(scanEntity.ID)
		if hasAPIDefinitions || scanEntity.Options.APIScanOptions.Enabled {
			scanLog.Info().Msg("API-only scan mode: skipping crawl phase")
			return nil
		}
		scanLog.Warn().Msg("No target URLs for crawl phase")
		return nil
	}

	return o.scheduler.ScheduleCrawl(o.ctx, scanEntity.ID, urls)
}

// startFingerprintPhase initiates the fingerprint phase
// This performs fingerprinting, header analysis, CDN checks, and retire.js scanning
func (o *Orchestrator) startFingerprintPhase(scanEntity *db.Scan) error {
	scanLog := log.With().Uint("scan_id", scanEntity.ID).Logger()
	scanLog.Info().Msg("Starting fingerprint phase")

	scanScope := o.getScopeForScan(scanEntity)

	// Get base URLs
	baseURLs, err := o.getInScopeBaseURLsForScan(scanEntity, scanScope)
	if err != nil {
		return fmt.Errorf("failed to get base URLs: %w", err)
	}

	if len(baseURLs) == 0 {
		scanLog.Warn().Msg("No in-scope base URLs for fingerprint phase")
		return nil
	}

	scanLog.Info().Int("base_url_count", len(baseURLs)).Msg("Processing base URLs for fingerprinting")

	fingerprints := make([]lib.Fingerprint, 0)
	const batchSize = 500 // Process 500 items at a time per base URL

	// Process each base URL separately to avoid loading all history at once
	for _, baseURL := range baseURLs {
		scanLog.Debug().Str("base_url", baseURL).Msg("Processing base URL")

		// Load history items for this base URL in batches
		offset := 0
		allHistoriesForBaseURL := make([]*db.History, 0)

		for {
			// Fetch a batch of history items for this base URL
			batch, err := o.getHistoryItemsForBaseURL(scanEntity, scanScope, baseURL, offset, batchSize)
			if err != nil {
				scanLog.Error().Err(err).Str("base_url", baseURL).Msg("Failed to get history items for base URL")
				break
			}

			if len(batch) == 0 {
				break
			}

			allHistoriesForBaseURL = append(allHistoriesForBaseURL, batch...)
			offset += batchSize

			// Stop if we got fewer items than batch size (end of data)
			if len(batch) < batchSize {
				break
			}
		}

		if len(allHistoriesForBaseURL) == 0 {
			continue
		}

		// Process this base URL's histories
		passive.AnalyzeHeaders(baseURL, allHistoriesForBaseURL)

		// Fingerprint the history items
		newFingerprints := passive.FingerprintHistoryItems(allHistoriesForBaseURL)
		passive.ReportFingerprints(baseURL, newFingerprints, scanEntity.WorkspaceID, 0, &scanEntity.ID)
		fingerprints = append(fingerprints, newFingerprints...)

		const maxConcurrentRetireJS = 10
		sem := make(chan struct{}, maxConcurrentRetireJS)
		var wg sync.WaitGroup

		uniqueHistories := removeDuplicateHistoryItems(allHistoriesForBaseURL)

		for _, item := range uniqueHistories {
			wg.Add(1)
			sem <- struct{}{} // Acquire semaphore

			go func(historyItem *db.History) {
				defer wg.Done()
				defer func() { <-sem }() // Release semaphore

				o.retireScanner.HistoryScan(historyItem)
			}(item)
		}

		// Wait for all retire.js scans for this base URL to complete
		wg.Wait()

		// CDN Check
		_, err := integrations.CDNCheck(baseURL, scanEntity.WorkspaceID, 0)
		if err != nil {
			scanLog.Debug().Err(err).Str("base_url", baseURL).Msg("CDN check failed")
		}

		// Clear the batch from memory before moving to next base URL
		allHistoriesForBaseURL = nil
	}

	// Get fingerprint tags for later use
	fingerprintTags := passive.GetUniqueNucleiTags(fingerprints)

	// Store fingerprints and tags in checkpoint
	if scanEntity.Checkpoint == nil {
		scanEntity.Checkpoint = &db.ScanCheckpoint{}
	}
	scanEntity.Checkpoint.Fingerprints = fingerprints
	scanEntity.Checkpoint.FingerprintTags = fingerprintTags

	if _, err := db.Connection().UpdateScan(scanEntity); err != nil {
		scanLog.Error().Err(err).Msg("Failed to update scan checkpoint with fingerprints")
	}

	scanLog.Info().
		Int("fingerprint_count", len(fingerprints)).
		Int("tag_count", len(fingerprintTags)).
		Msg("Fingerprint phase completed")

	return nil
}

// uniqueHistoryIdentifiers is used as a key for deduplicating history items
type uniqueHistoryIdentifiers struct {
	URL              string
	Method           string
	RequestBodySize  int
	ResponseBodySize int
	StatusCode       int
}

// removeDuplicateHistoryItems removes duplicate history items based on URL, Method,
// RequestBodySize, ResponseBodySize, and StatusCode. This prevents scanning the same
// request/response combination multiple times.
func removeDuplicateHistoryItems(histories []*db.History) []*db.History {
	keys := make(map[uniqueHistoryIdentifiers]bool)
	result := []*db.History{}

	for _, entry := range histories {
		key := uniqueHistoryIdentifiers{
			URL:              entry.URL,
			Method:           entry.Method,
			ResponseBodySize: entry.ResponseBodySize,
			RequestBodySize:  entry.RequestBodySize,
			StatusCode:       entry.StatusCode,
		}

		if _, exists := keys[key]; !exists {
			keys[key] = true
			result = append(result, entry)
		}
	}

	return result
}

// startSiteBehaviorPhase initiates the site behavior checking phase
func (o *Orchestrator) startSiteBehaviorPhase(scanEntity *db.Scan) error {
	scanLog := log.With().Uint("scan_id", scanEntity.ID).Logger()
	scanLog.Info().Msg("Starting site behavior phase")

	scanScope := o.getScopeForScan(scanEntity)

	baseURLs, err := o.getInScopeBaseURLsForScan(scanEntity, scanScope)
	if err != nil {
		return fmt.Errorf("failed to get base URLs: %w", err)
	}

	if len(baseURLs) == 0 {
		scanLog.Warn().Msg("No in-scope base URLs for site behavior phase")
		return nil
	}

	scanLog.Info().Int("base_url_count", len(baseURLs)).Msg("Scheduling site behavior checks")

	return o.scheduler.ScheduleSiteBehavior(o.ctx, scanEntity.ID, baseURLs)
}

// startAPIBehaviorPhase initiates the API behavior phase.
// Runs after discovery so all API definitions (user-imported, crawl-discovered,
// and discovery-found) are available.
func (o *Orchestrator) startAPIBehaviorPhase(scanEntity *db.Scan) error {
	scanLog := log.With().Uint("scan_id", scanEntity.ID).Logger()
	scanLog.Info().Msg("Starting API behavior phase")

	if err := o.scheduler.ScheduleAPIBehavior(o.ctx, scanEntity.ID); err != nil {
		scanLog.Error().Err(err).Msg("Failed to schedule API behavior checks")
		return err
	}

	return nil
}

// startDiscoveryPhase initiates the discovery/fuzzing phase
func (o *Orchestrator) startDiscoveryPhase(scanEntity *db.Scan) error {
	scanLog := log.With().Uint("scan_id", scanEntity.ID).Logger()
	scanLog.Info().Msg("Starting discovery phase")

	scanScope := o.getScopeForScan(scanEntity)

	// Get base URLs from crawled history (filtered by scope)
	baseURLs, err := o.getInScopeBaseURLsForScan(scanEntity, scanScope)
	if err != nil {
		return fmt.Errorf("failed to get base URLs: %w", err)
	}

	if len(baseURLs) == 0 {
		scanLog.Warn().Msg("No in-scope base URLs for discovery phase")
		return nil
	}

	// Site behaviors are already populated in the database by the site_behavior phase
	return o.scheduler.ScheduleDiscovery(o.ctx, scanEntity.ID, baseURLs)
}

// startNucleiPhase initiates the nuclei scanning phase
func (o *Orchestrator) startNucleiPhase(scanEntity *db.Scan) error {
	scanLog := log.With().Uint("scan_id", scanEntity.ID).Logger()
	scanLog.Info().Msg("Starting nuclei phase")

	if !viper.GetBool("integrations.nuclei.enabled") {
		scanLog.Info().Msg("Nuclei is disabled, skipping phase")
		return nil
	}

	scanScope := o.getScopeForScan(scanEntity)

	// Get base URLs for nuclei scan
	baseURLs, err := o.getInScopeBaseURLsForScan(scanEntity, scanScope)
	if err != nil {
		return fmt.Errorf("failed to get base URLs: %w", err)
	}

	if len(baseURLs) == 0 {
		scanLog.Warn().Msg("No base URLs for nuclei phase")
		return nil
	}

	// Get fingerprint tags from checkpoint
	var fingerprintTags []string
	if scanEntity.Checkpoint != nil {
		fingerprintTags = scanEntity.Checkpoint.FingerprintTags
	}

	scanLog.Info().
		Int("url_count", len(baseURLs)).
		Int("tag_count", len(fingerprintTags)).
		Interface("tags", fingerprintTags).
		Msg("Running nuclei scan")

	// Run nuclei scan synchronously (it manages its own concurrency)
	nucleiScanErr := integrations.NucleiScan(baseURLs, scanEntity.WorkspaceID)
	if nucleiScanErr != nil {
		scanLog.Error().Err(nucleiScanErr).Msg("Error running nuclei scan")
	}

	// Mark nuclei as completed in checkpoint
	if scanEntity.Checkpoint == nil {
		scanEntity.Checkpoint = &db.ScanCheckpoint{}
	}
	scanEntity.Checkpoint.NucleiCompleted = true
	if _, err := db.Connection().UpdateScan(scanEntity); err != nil {
		scanLog.Error().Err(err).Msg("Failed to update scan checkpoint after nuclei")
	}

	scanLog.Info().Msg("Nuclei phase completed")
	return nil
}

// startActiveScanPhase initiates the active scanning phase.
// This phase also schedules API scan jobs (if enabled) alongside regular active scan jobs.
// Both job types are scheduled before execution begins, so API scan baseline requests
// (created during execution) won't be picked up as additional active scan targets.
// API scan jobs use higher priority so workers process them first.
func (o *Orchestrator) startActiveScanPhase(scanEntity *db.Scan) error {
	scanLog := log.With().Uint("scan_id", scanEntity.ID).Logger()
	scanLog.Info().Msg("Starting active scan phase")

	// Update scan status to scanning
	db.Connection().SetScanStatus(scanEntity.ID, db.ScanStatusScanning)

	scanScope := o.getScopeForScan(scanEntity)

	// Get in-scope history metadata for deduplication and filtering
	items, err := o.getInScopeHistoryMetadata(scanEntity, scanScope)
	if err != nil {
		return fmt.Errorf("failed to get history metadata: %w", err)
	}

	// Schedule regular active scan jobs from crawled/discovered history
	if len(items) > 0 {
		o.scheduleActiveScanJobs(scanLog, scanEntity, items)
	} else {
		scanLog.Info().Msg("No in-scope history items for regular active scanning")
	}

	// Schedule API scan jobs in the same phase (higher priority, processed first by workers)
	o.scheduleAPIScanJobs(scanLog, scanEntity)

	return nil
}

func (o *Orchestrator) scheduleActiveScanJobs(scanLog zerolog.Logger, scanEntity *db.Scan, items []*db.History) {
	originalCount := len(items)
	items = removeDuplicateHistoryItems(items)
	if len(items) < originalCount {
		scanLog.Info().
			Int("original_count", originalCount).
			Int("unique_count", len(items)).
			Msg("Removed duplicate history items before active scanning")
	}

	ignoredExtensions := viper.GetStringSlice("crawl.ignored_extensions")
	scheduledURLPaths := make(map[string]bool)
	fullInsertionPointIDs := make([]uint, 0)
	reducedInsertionPointIDs := make([]uint, 0)

	urlpathEnabled := lib.SliceContains(scanEntity.Options.InsertionPoints, "urlpath")

	for _, item := range items {
		if item.StatusCode == 404 {
			continue
		}

		shouldSkip := false
		for _, extension := range ignoredExtensions {
			if strings.HasSuffix(item.URL, extension) {
				shouldSkip = true
				break
			}
		}
		if shouldSkip {
			continue
		}

		if urlpathEnabled {
			normalizedURLPath, err := lib.NormalizeURLPath(item.URL)
			if err != nil {
				scanLog.Error().Err(err).Str("url", item.URL).Uint("history", item.ID).Msg("Skipping history item as could not normalize URL path")
				continue
			}
			if _, exists := scheduledURLPaths[normalizedURLPath]; exists {
				reducedInsertionPointIDs = append(reducedInsertionPointIDs, item.ID)
			} else {
				fullInsertionPointIDs = append(fullInsertionPointIDs, item.ID)
				scheduledURLPaths[normalizedURLPath] = true
			}
		} else {
			fullInsertionPointIDs = append(fullInsertionPointIDs, item.ID)
		}
	}

	totalFiltered := len(fullInsertionPointIDs) + len(reducedInsertionPointIDs)
	if totalFiltered == 0 {
		scanLog.Info().Msg("No history items remaining after filtering for active scanning")
		return
	}

	scanLog.Info().
		Int("total_items", len(items)).
		Int("full_insertion_point_items", len(fullInsertionPointIDs)).
		Int("reduced_insertion_point_items", len(reducedInsertionPointIDs)).
		Msg("Scheduling active scans for filtered history items")

	if len(fullInsertionPointIDs) > 0 {
		if err := o.scheduler.ScheduleActiveScan(o.ctx, scanEntity.ID, fullInsertionPointIDs); err != nil {
			scanLog.Error().Err(err).Msg("Failed to schedule full insertion point scans")
		}
	}

	if len(reducedInsertionPointIDs) > 0 {
		if err := o.scheduler.ScheduleActiveScanWithOptions(o.ctx, scanEntity.ID, reducedInsertionPointIDs, []string{"urlpath"}); err != nil {
			scanLog.Error().Err(err).Msg("Failed to schedule reduced insertion point scans")
		}
	}
}

func (o *Orchestrator) scheduleAPIScanJobs(scanLog zerolog.Logger, scanEntity *db.Scan) {
	if !o.config.EnableAPIScan {
		return
	}

	if !scanEntity.Options.APIScanOptions.Enabled {
		hasDefinitions, _ := db.Connection().HasLinkedAPIDefinitions(scanEntity.ID)
		if !hasDefinitions {
			return
		}
	}

	scanLog.Info().Msg("Scheduling API scan jobs")
	if err := o.scheduler.ScheduleAPIScan(o.ctx, scanEntity.ID); err != nil {
		scanLog.Error().Err(err).Msg("Failed to schedule API scan jobs")
	}
}

// startWebSocketPhase initiates the websocket scanning phase
func (o *Orchestrator) startWebSocketPhase(scanEntity *db.Scan) error {
	scanLog := log.With().Uint("scan_id", scanEntity.ID).Logger()
	scanLog.Info().Msg("Starting websocket phase")

	scanScope := o.getScopeForScan(scanEntity)

	// Get websocket connections discovered during crawl
	connections, originalCount, err := db.Connection().ListWebSocketConnections(db.WebSocketConnectionFilter{
		WorkspaceID: scanEntity.WorkspaceID,
		ScanID:      scanEntity.ID,
		Sources:     []string{db.SourceCrawler},
	})
	if err != nil {
		return fmt.Errorf("failed to get websocket connections: %w", err)
	}

	// Filter to in-scope connections and detect cleartext WebSocket
	var inScopeConnectionIDs []uint
	cleartextHostsReported := make(map[string]bool)

	for _, conn := range connections {
		if !scanScope.IsInScope(conn.URL) {
			scanLog.Debug().Str("url", conn.URL).Msg("WebSocket connection discovered during crawler is out of scope, skipping scan")
			continue
		}

		inScopeConnectionIDs = append(inScopeConnectionIDs, conn.ID)

		// Check for cleartext WebSocket connections (ws:// instead of wss://)
		u, err := url.Parse(conn.URL)
		if err != nil {
			scanLog.Debug().Err(err).Str("url", conn.URL).Msg("Could not parse WebSocket URL")
			continue
		}

		if u.Scheme == "ws" && !cleartextHostsReported[u.Host] {
			cleartextHostsReported[u.Host] = true
			scanID := scanEntity.ID
			db.CreateWebSocketIssue(db.WebSocketIssueOptions{
				Connection:  &conn,
				Code:        db.UnencryptedWebsocketConnectionCode,
				Details:     fmt.Sprintf("Cleartext WebSocket connections detected on host: %s", u.Host),
				Confidence:  100,
				WorkspaceID: &scanEntity.WorkspaceID,
				ScanID:      &scanID,
			})
			scanLog.Info().Str("host", u.Host).Msg("Reported cleartext WebSocket connection issue")
		}
	}

	if originalCount > int64(len(inScopeConnectionIDs)) {
		scanLog.Warn().
			Int64("original_count", originalCount).
			Int("in_scope_count", len(inScopeConnectionIDs)).
			Msg("Some WebSocket connections discovered during crawl are out of scope, skipping scan for them")
	}

	if len(cleartextHostsReported) > 0 {
		scanLog.Warn().
			Int("cleartext_hosts", len(cleartextHostsReported)).
			Msg("Cleartext WebSocket connections detected")
	}

	if len(inScopeConnectionIDs) == 0 {
		scanLog.Info().Msg("No in-scope websocket connections for scanning")
		return nil
	}

	scanLog.Info().Int("count", len(inScopeConnectionIDs)).Msg("Scheduling WebSocket connection scans")

	return o.scheduler.ScheduleWebSocketScan(o.ctx, scanEntity.ID, inScopeConnectionIDs)
}

// completeScan marks a scan as complete
func (o *Orchestrator) completeScan(scanEntity *db.Scan) error {
	scanLog := log.With().Uint("scan_id", scanEntity.ID).Logger()
	scanLog.Info().Msg("Completing scan")

	now := time.Now()
	scanEntity.Status = db.ScanStatusCompleted
	scanEntity.CompletedAt = &now

	if _, err := db.Connection().UpdateScan(scanEntity); err != nil {
		return fmt.Errorf("failed to complete scan: %w", err)
	}

	// Calculate final stats
	o.updateScanStats(scanEntity)

	// Cleanup site behaviors for this scan
	o.siteBehaviorMu.Lock()
	delete(o.siteBehaviors, scanEntity.ID)
	o.siteBehaviorMu.Unlock()

	scanLog.Info().
		Uint("scan_id", scanEntity.ID).
		Time("completed_at", now).
		Msg("Scan completed successfully")

	return nil
}

// getTargetURLs extracts target URLs from scan configuration
func (o *Orchestrator) getTargetURLs(scanEntity *db.Scan) []string {
	// Get URLs from scan options
	if len(scanEntity.Options.StartURLs) > 0 {
		return scanEntity.Options.StartURLs
	}
	return nil
}

// updateScanStats calculates and updates final scan statistics
func (o *Orchestrator) updateScanStats(scanEntity *db.Scan) {
	// Update job counts
	if err := db.Connection().UpdateScanJobCounts(scanEntity.ID); err != nil {
		log.Error().Err(err).Uint("scan_id", scanEntity.ID).Msg("Failed to update scan job counts")
	}

	// Get job stats
	stats, err := db.Connection().GetScanJobStats(scanEntity.ID)
	if err != nil {
		log.Error().Err(err).Uint("scan_id", scanEntity.ID).Msg("Failed to get scan job stats")
		return
	}

	completedJobs := stats[db.ScanJobStatusCompleted]
	failedJobs := stats[db.ScanJobStatusFailed]

	log.Info().
		Uint("scan_id", scanEntity.ID).
		Int64("completed_jobs", completedJobs).
		Int64("failed_jobs", failedJobs).
		Msg("Scan statistics updated")
}

// StartScan initializes and starts a new full scan
func (o *Orchestrator) StartScan(scanID uint) error {
	scanEntity, err := db.Connection().GetScanByID(scanID)
	if err != nil {
		return fmt.Errorf("failed to get scan: %w", err)
	}

	// Initialize scan phase and checkpoint
	scanEntity.Status = db.ScanStatusCrawling
	scanEntity.Phase = db.ScanPhaseCrawl
	now := time.Now()
	scanEntity.StartedAt = &now

	// Initialize checkpoint with scope domains
	scanEntity.Checkpoint = &db.ScanCheckpoint{
		Phase: db.ScanPhaseCrawl,
	}

	if _, err := db.Connection().UpdateScan(scanEntity); err != nil {
		return fmt.Errorf("failed to update scan: %w", err)
	}

	// Initialize scope in checkpoint
	if err := o.initializeScopeInCheckpoint(scanEntity); err != nil {
		log.Error().Err(err).Uint("scan_id", scanID).Msg("Failed to initialize scope in checkpoint")
	}

	// Start the crawl phase
	return o.startCrawlPhase(scanEntity)
}

// GetSiteBehavior returns the cached site behavior for a base URL in a scan
func (o *Orchestrator) GetSiteBehavior(scanID uint, baseURL string) *http_utils.SiteBehavior {
	o.siteBehaviorMu.RLock()
	defer o.siteBehaviorMu.RUnlock()

	if behaviors, ok := o.siteBehaviors[scanID]; ok {
		return behaviors[baseURL]
	}
	return nil
}

// getInScopeBaseURLsForScan extracts unique in-scope base URLs from scan history
func (o *Orchestrator) getInScopeBaseURLsForScan(scanEntity *db.Scan, scanScope scope.Scope) ([]string, error) {
	var urlRecords []struct {
		URL string
	}

	err := db.Connection().DB().Model(&db.History{}).
		Select("DISTINCT url").
		Where("workspace_id = ? AND scan_id = ?", scanEntity.WorkspaceID, scanEntity.ID).
		Scan(&urlRecords).Error

	if err != nil {
		return nil, err
	}

	// Extract unique base URLs that are in scope
	baseURLs := make(map[string]bool)
	for _, record := range urlRecords {
		if record.URL != "" && scanScope.IsInScope(record.URL) {
			baseURL, err := lib.GetBaseURL(record.URL)
			if err != nil {
				log.Debug().Err(err).Str("url", record.URL).Msg("Failed to get base URL")
				continue
			}
			baseURLs[baseURL] = true
		}
	}

	result := make([]string, 0, len(baseURLs))
	for url := range baseURLs {
		result = append(result, url)
	}

	return result, nil
}

// getHistoryItemsForBaseURL retrieves history items for a specific base URL with pagination
func (o *Orchestrator) getHistoryItemsForBaseURL(scanEntity *db.Scan, scanScope scope.Scope, baseURL string, offset, limit int) ([]*db.History, error) {
	filter := db.HistoryFilter{
		WorkspaceID: scanEntity.WorkspaceID,
		ScanID:      scanEntity.ID,
		Pagination: db.Pagination{
			Page:     offset/limit + 1,
			PageSize: limit,
		},
	}

	items, _, err := db.Connection().ListHistory(filter)
	if err != nil {
		return nil, err
	}

	// Filter by base URL and scope
	result := make([]*db.History, 0)
	for _, item := range items {
		if !scanScope.IsInScope(item.URL) {
			continue
		}

		itemBaseURL, err := lib.GetBaseURL(item.URL)
		if err != nil {
			continue
		}

		if itemBaseURL == baseURL {
			result = append(result, item)
		}
	}

	return result, nil
}

// historyMetadata holds only the metadata fields needed for deduplication
type historyMetadata struct {
	ID               uint
	URL              string
	Method           string
	StatusCode       int
	RequestBodySize  int
	ResponseBodySize int
}

// getInScopeHistoryMetadata retrieves history metadata for deduplication in active scan phase
func (o *Orchestrator) getInScopeHistoryMetadata(scanEntity *db.Scan, scanScope scope.Scope) ([]*db.History, error) {
	var metadata []historyMetadata

	err := db.Connection().DB().Model(&db.History{}).
		Select("id, url, method, status_code, request_body_size, response_body_size").
		Where("workspace_id = ? AND scan_id = ?", scanEntity.WorkspaceID, scanEntity.ID).
		Scan(&metadata).Error

	if err != nil {
		return nil, err
	}

	// Convert to History objects and filter by scope
	items := make([]*db.History, 0)
	for _, m := range metadata {
		if scanScope.IsInScope(m.URL) {
			items = append(items, &db.History{
				BaseModel: db.BaseModel{
					ID: m.ID,
				},
				URL:              m.URL,
				Method:           m.Method,
				StatusCode:       m.StatusCode,
				RequestBodySize:  m.RequestBodySize,
				ResponseBodySize: m.ResponseBodySize,
			})
		}
	}

	return items, nil
}
