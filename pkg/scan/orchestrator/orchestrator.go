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
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// ScanPhase represents a phase in the full scan lifecycle
type ScanPhase string

const (
	PhaseCrawl       ScanPhase = "crawl"
	PhaseFingerprint ScanPhase = "fingerprint"
	PhaseDiscovery   ScanPhase = "discovery"
	PhaseNuclei      ScanPhase = "nuclei"
	PhaseActiveScan  ScanPhase = "active_scan"
	PhaseWebSocket   ScanPhase = "websocket"
	PhaseComplete    ScanPhase = "complete"
)

// PhaseOrder defines the sequence of phases in a full scan
var PhaseOrder = []ScanPhase{
	PhaseCrawl,
	PhaseFingerprint,
	PhaseDiscovery,
	PhaseNuclei,
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

	// Get history items discovered during crawl (filtered by scope)
	items, err := o.getInScopeHistoryItemsForScan(scanEntity, scanScope)
	if err != nil {
		return fmt.Errorf("failed to get history items: %w", err)
	}

	if len(items) == 0 {
		scanLog.Warn().Msg("No in-scope history items for fingerprint phase")
		return nil
	}

	scanLog.Info().Int("history_count", len(items)).Msg("Processing history items for fingerprinting")

	// Group histories by base URL
	historiesByBaseURL := o.separateHistoriesByBaseURL(items)
	fingerprints := make([]lib.Fingerprint, 0)

	// Process each base URL
	for baseURL, histories := range historiesByBaseURL {
		// Analyze headers
		passive.AnalyzeHeaders(baseURL, histories)

		// Fingerprint the history items
		newFingerprints := passive.FingerprintHistoryItems(histories)
		passive.ReportFingerprints(baseURL, newFingerprints, scanEntity.WorkspaceID, 0)
		fingerprints = append(fingerprints, newFingerprints...)

		// CDN Check
		_, err := integrations.CDNCheck(baseURL, scanEntity.WorkspaceID, 0)
		if err != nil {
			scanLog.Debug().Err(err).Str("base_url", baseURL).Msg("CDN check failed")
		}
	}

	// Run retire.js scanning on all items
	for _, item := range items {
		go o.retireScanner.HistoryScan(item)
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

// getInScopeHistoryItemsForScan retrieves in-scope history items for a scan
func (o *Orchestrator) getInScopeHistoryItemsForScan(scanEntity *db.Scan, scanScope scope.Scope) ([]*db.History, error) {
	filter := db.HistoryFilter{
		WorkspaceID: scanEntity.WorkspaceID,
		ScanID:      scanEntity.ID,
		Pagination: db.Pagination{
			PageSize: 0, // No pagination - get all items
		},
	}

	items, _, err := db.Connection().ListHistory(filter)
	if err != nil {
		return nil, err
	}

	// Filter by scope
	inScopeItems := make([]*db.History, 0)
	for _, item := range items {
		if scanScope.IsInScope(item.URL) {
			inScopeItems = append(inScopeItems, item)
		}
	}

	return inScopeItems, nil
}

// separateHistoriesByBaseURL groups history items by their base URL
func (o *Orchestrator) separateHistoriesByBaseURL(items []*db.History) map[string][]*db.History {
	result := make(map[string][]*db.History)

	for _, item := range items {
		baseURL, err := lib.GetBaseURL(item.URL)
		if err != nil {
			continue
		}
		result[baseURL] = append(result[baseURL], item)
	}

	return result
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

	// Check site behavior for each base URL before discovery
	o.siteBehaviorMu.Lock()
	if o.siteBehaviors[scanEntity.ID] == nil {
		o.siteBehaviors[scanEntity.ID] = make(map[string]*http_utils.SiteBehavior)
	}
	o.siteBehaviorMu.Unlock()

	for _, baseURL := range baseURLs {
		createOpts := http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: scanEntity.WorkspaceID,
			TaskID:      0,
			ScanID:      scanEntity.ID,
		}

		siteBehavior, err := http_utils.CheckSiteBehavior(http_utils.SiteBehaviourCheckOptions{
			BaseURL:                baseURL,
			Client:                 o.httpClient,
			HistoryCreationOptions: createOpts,
			Concurrency:            10,
			Headers:                scanEntity.Options.Headers,
		})
		if err != nil {
			scanLog.Error().Err(err).Str("base_url", baseURL).Msg("Could not check site behavior")
			continue
		}

		o.siteBehaviorMu.Lock()
		o.siteBehaviors[scanEntity.ID][baseURL] = siteBehavior
		o.siteBehaviorMu.Unlock()
	}

	// Store site behaviors in checkpoint
	if scanEntity.Checkpoint == nil {
		scanEntity.Checkpoint = &db.ScanCheckpoint{}
	}
	if scanEntity.Checkpoint.SiteBehaviors == nil {
		scanEntity.Checkpoint.SiteBehaviors = make(map[string]*db.SiteBehavior)
	}

	o.siteBehaviorMu.RLock()
	for baseURL, behavior := range o.siteBehaviors[scanEntity.ID] {
		scanEntity.Checkpoint.SiteBehaviors[baseURL] = &db.SiteBehavior{
			NotFoundReturns404: behavior.NotFoundReturns404,
			NotFoundChanges:    behavior.NotFoundChanges,
			CommonHash:         behavior.CommonHash,
		}
	}
	o.siteBehaviorMu.RUnlock()

	if _, err := db.Connection().UpdateScan(scanEntity); err != nil {
		scanLog.Error().Err(err).Msg("Failed to update scan checkpoint with site behaviors")
	}

	return o.scheduler.ScheduleDiscovery(o.ctx, scanEntity.ID, baseURLs)
}

// getInScopeBaseURLsForScan extracts unique in-scope base URLs from scan history
func (o *Orchestrator) getInScopeBaseURLsForScan(scanEntity *db.Scan, scanScope scope.Scope) ([]string, error) {
	filter := db.HistoryFilter{
		WorkspaceID: scanEntity.WorkspaceID,
		ScanID:      scanEntity.ID,
		Pagination: db.Pagination{
			PageSize: 0, // No pagination - get all items
		},
	}

	items, _, err := db.Connection().ListHistory(filter)
	if err != nil {
		return nil, err
	}

	// Extract unique base URLs that are in scope
	baseURLs := make(map[string]bool)
	for _, item := range items {
		if item.URL != "" && scanScope.IsInScope(item.URL) {
			baseURL, err := lib.GetBaseURL(item.URL)
			if err != nil {
				log.Debug().Err(err).Str("url", item.URL).Msg("Failed to get base URL")
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

// startActiveScanPhase initiates the active scanning phase
func (o *Orchestrator) startActiveScanPhase(scanEntity *db.Scan) error {
	scanLog := log.With().Uint("scan_id", scanEntity.ID).Logger()
	scanLog.Info().Msg("Starting active scan phase")

	// Update scan status to scanning
	db.Connection().SetScanStatus(scanEntity.ID, db.ScanStatusScanning)

	scanScope := o.getScopeForScan(scanEntity)

	// Get in-scope history items to scan
	items, err := o.getInScopeHistoryItemsForScan(scanEntity, scanScope)
	if err != nil {
		return fmt.Errorf("failed to get history items: %w", err)
	}

	if len(items) == 0 {
		scanLog.Warn().Msg("No in-scope history items for active scan phase")
		return nil
	}

	// Remove duplicate history items to avoid scanning the same request/response multiple times
	originalCount := len(items)
	items = removeDuplicateHistoryItems(items)
	if len(items) < originalCount {
		scanLog.Info().
			Int("original_count", originalCount).
			Int("unique_count", len(items)).
			Msg("Removed duplicate history items before active scanning")
	}

	// Filter items based on the same logic as FullScan
	// Separate items into two groups:
	// 1. fullInsertionPointIDs: items that should be scanned with all insertion points (first occurrence of each URL path)
	// 2. reducedInsertionPointIDs: items that should be scanned without urlpath (duplicate URL paths)
	ignoredExtensions := viper.GetStringSlice("crawl.ignored_extensions")
	scheduledURLPaths := make(map[string]bool)
	fullInsertionPointIDs := make([]uint, 0)
	reducedInsertionPointIDs := make([]uint, 0)

	urlpathEnabled := lib.SliceContains(scanEntity.Options.InsertionPoints, "urlpath")

	for _, item := range items {
		// Skip 404 responses
		if item.StatusCode == 404 {
			continue
		}

		// Skip ignored extensions
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

		// URL path deduplication when urlpath insertion point is enabled
		if urlpathEnabled {
			normalizedURLPath, err := lib.NormalizeURLPath(item.URL)
			if err != nil {
				scanLog.Error().Err(err).Str("url", item.URL).Uint("history", item.ID).Msg("Skipping history item as could not normalize URL path")
				continue
			}
			if _, exists := scheduledURLPaths[normalizedURLPath]; exists {
				// URL path already scheduled - scan with other insertion points (excluding urlpath)
				reducedInsertionPointIDs = append(reducedInsertionPointIDs, item.ID)
			} else {
				// First occurrence of this URL path - scan with all insertion points
				fullInsertionPointIDs = append(fullInsertionPointIDs, item.ID)
				scheduledURLPaths[normalizedURLPath] = true
			}
		} else {
			fullInsertionPointIDs = append(fullInsertionPointIDs, item.ID)
		}
	}

	totalFiltered := len(fullInsertionPointIDs) + len(reducedInsertionPointIDs)
	if totalFiltered == 0 {
		scanLog.Warn().Msg("No history items remaining after filtering for active scan phase")
		return nil
	}

	scanLog.Info().
		Int("total_items", len(items)).
		Int("full_insertion_point_items", len(fullInsertionPointIDs)).
		Int("reduced_insertion_point_items", len(reducedInsertionPointIDs)).
		Msg("Scheduling active scans for filtered history items")

	// Schedule items with full insertion points
	if len(fullInsertionPointIDs) > 0 {
		if err := o.scheduler.ScheduleActiveScan(o.ctx, scanEntity.ID, fullInsertionPointIDs); err != nil {
			return fmt.Errorf("failed to schedule full insertion point scans: %w", err)
		}
	}

	// Schedule items with reduced insertion points (excluding urlpath)
	if len(reducedInsertionPointIDs) > 0 {
		if err := o.scheduler.ScheduleActiveScanWithOptions(o.ctx, scanEntity.ID, reducedInsertionPointIDs, []string{"urlpath"}); err != nil {
			return fmt.Errorf("failed to schedule reduced insertion point scans: %w", err)
		}
	}

	return nil
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
			var scanID uint = scanEntity.ID
			db.CreateIssueFromWebSocketConnectionAndTemplate(
				&conn,
				db.UnencryptedWebsocketConnectionCode,
				fmt.Sprintf("Cleartext WebSocket connections detected on host: %s", u.Host),
				100,
				"",
				&scanEntity.WorkspaceID,
				nil, // No taskID in new system
				nil, // No taskJobID
				&scanID,
				nil, // No scanJobID at orchestrator level
			)
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

// getHistoryItemsForScan retrieves history item IDs for a scan (backward compatibility)
func (o *Orchestrator) getHistoryItemsForScan(scanEntity *db.Scan) ([]uint, error) {
	scanScope := o.getScopeForScan(scanEntity)
	items, err := o.getInScopeHistoryItemsForScan(scanEntity, scanScope)
	if err != nil {
		return nil, err
	}

	ids := make([]uint, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}

	return ids, nil
}

// getBaseURLsForScan extracts unique base URLs from scan history (backward compatibility)
func (o *Orchestrator) getBaseURLsForScan(scanEntity *db.Scan) ([]string, error) {
	scanScope := o.getScopeForScan(scanEntity)
	return o.getInScopeBaseURLsForScan(scanEntity, scanScope)
}

// getWebSocketHistoryForScan retrieves websocket connection IDs for a scan (backward compatibility)
func (o *Orchestrator) getWebSocketHistoryForScan(scanEntity *db.Scan) ([]uint, error) {
	scanScope := o.getScopeForScan(scanEntity)

	// Get websocket connections for this scan
	connections, _, err := db.Connection().ListWebSocketConnections(db.WebSocketConnectionFilter{
		WorkspaceID: scanEntity.WorkspaceID,
		ScanID:      scanEntity.ID,
	})
	if err != nil {
		return nil, err
	}

	// Filter by scope
	ids := make([]uint, 0)
	for _, conn := range connections {
		if scanScope.IsInScope(conn.URL) {
			ids = append(ids, conn.ID)
		}
	}

	return ids, nil
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
