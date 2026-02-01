package db

import (
	"fmt"
	"time"

	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
)

// ScanStatus represents the status of a scan
type ScanStatus string

const (
	ScanStatusPending   ScanStatus = "pending"
	ScanStatusCrawling  ScanStatus = "crawling"
	ScanStatusScanning  ScanStatus = "scanning"
	ScanStatusPaused    ScanStatus = "paused"
	ScanStatusCompleted ScanStatus = "completed"
	ScanStatusCancelled ScanStatus = "cancelled"
	ScanStatusFailed    ScanStatus = "failed"
)

// ScanPhase represents the current phase of a scan
type ScanPhase string

const (
	ScanPhaseCrawl        ScanPhase = "crawl"
	ScanPhaseFingerprint  ScanPhase = "fingerprint"
	ScanPhaseSiteBehavior ScanPhase = "site_behavior"
	ScanPhaseAPIBehavior  ScanPhase = "api_behavior"
	ScanPhaseDiscovery    ScanPhase = "discovery"
	ScanPhaseNuclei       ScanPhase = "nuclei"
	ScanPhaseActiveScan   ScanPhase = "active_scan"
	ScanPhaseWebSocket    ScanPhase = "websocket"
)

// ScanCheckpoint stores scan-level state for restart recovery
type ScanCheckpoint struct {
	Phase           ScanPhase        `json:"phase"`
	CrawlCheckpoint *CrawlCheckpoint `json:"crawl_checkpoint,omitempty"`
	// IDs of history items that have been processed for fingerprinting
	ProcessedHistoryIDs []uint `json:"processed_history_ids,omitempty"`
	// Base URLs that have completed discovery
	CompletedDiscoveryURLs []string `json:"completed_discovery_urls,omitempty"`
	// Whether nuclei phase has completed
	NucleiCompleted bool `json:"nuclei_completed,omitempty"`
	// Fingerprint tags discovered during fingerprinting phase
	FingerprintTags []string `json:"fingerprint_tags,omitempty"`
	// Fingerprints discovered during fingerprinting phase
	Fingerprints []lib.Fingerprint `json:"fingerprints,omitempty"`
	// Scope domains derived from start URLs
	ScopeDomains []string `json:"scope_domains,omitempty"`
	// Site behaviors for each base URL
	SiteBehaviors map[string]*SiteBehavior `json:"site_behaviors,omitempty"`
}

type SiteBehavior struct {
	NotFoundReturns404 bool   `json:"not_found_returns_404"`
	NotFoundChanges    bool   `json:"not_found_changes"`
	NotFoundCommonHash string `json:"not_found_common_hash"`
}

// CrawlCheckpoint stores crawl phase state
type CrawlCheckpoint struct {
	VisitedURLs  []string `json:"visited_urls"`
	PendingURLs  []string `json:"pending_urls"`
	CurrentDepth int      `json:"current_depth"`
	PageCount    int      `json:"page_count"`
}

// Scan represents a vulnerability scanning session
type Scan struct {
	BaseModel

	// Core fields
	WorkspaceID    uint                    `json:"workspace_id" gorm:"index;not null"`
	Workspace      Workspace               `json:"-" gorm:"foreignKey:WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Title          string                  `json:"title" gorm:"size:255"`
	Status         ScanStatus              `json:"status" gorm:"index;size:50;not null;default:'pending'"`
	Phase          ScanPhase               `json:"phase" gorm:"size:50"`
	PreviousStatus ScanStatus              `json:"previous_status,omitempty" gorm:"size:50"`
	Options        options.FullScanOptions `json:"options" gorm:"serializer:json"`

	// Rate limiting and circuit breaker fields
	MaxRPS              *int       `json:"max_rps,omitempty"`
	MaxConcurrentJobs   *int       `json:"max_concurrent_jobs,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures" gorm:"default:0"`
	LastFailureAt       *time.Time `json:"last_failure_at,omitempty"`
	ThrottledUntil      *time.Time `json:"throttled_until,omitempty"`

	// Job counters for progress tracking
	TotalJobsCount     int `json:"total_jobs_count" gorm:"default:0"`
	PendingJobsCount   int `json:"pending_jobs_count" gorm:"default:0"`
	RunningJobsCount   int `json:"running_jobs_count" gorm:"default:0"`
	CompletedJobsCount int `json:"completed_jobs_count" gorm:"default:0"`
	FailedJobsCount    int `json:"failed_jobs_count" gorm:"default:0"`

	// Timing fields
	StartedAt   *time.Time `json:"started_at,omitempty"`
	PausedAt    *time.Time `json:"paused_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Checkpoint for restart recovery
	Checkpoint *ScanCheckpoint `json:"checkpoint,omitempty" gorm:"serializer:json"`

	// Isolation flag - when true, only workers with matching scan ID filter can claim jobs
	// This is used for CLI scans to prevent API workers from claiming their jobs
	Isolated bool `json:"isolated" gorm:"default:false;index"`

	// Browser event capture - when true, browser events are captured and stored during scanning
	CaptureBrowserEvents bool `json:"capture_browser_events" gorm:"default:false"`

	// Relationships
	Jobs           []ScanJob       `json:"-" gorm:"foreignKey:ScanID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Histories      []History       `json:"-" gorm:"foreignKey:ScanID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	APIDefinitions []APIDefinition `json:"-" gorm:"many2many:scan_api_definitions;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

// IsPausable returns true if the scan can be paused
func (s *Scan) IsPausable() bool {
	return s.Status == ScanStatusCrawling || s.Status == ScanStatusScanning
}

// IsResumable returns true if the scan can be resumed
func (s *Scan) IsResumable() bool {
	return s.Status == ScanStatusPaused
}

// IsCancellable returns true if the scan can be cancelled
func (s *Scan) IsCancellable() bool {
	return s.Status != ScanStatusCompleted &&
		s.Status != ScanStatusCancelled &&
		s.Status != ScanStatusFailed
}

// IsActive returns true if the scan is currently active
func (s *Scan) IsActive() bool {
	return s.Status == ScanStatusCrawling || s.Status == ScanStatusScanning
}

// IsTerminal returns true if the scan is in a terminal state
func (s *Scan) IsTerminal() bool {
	return s.Status == ScanStatusCompleted ||
		s.Status == ScanStatusCancelled ||
		s.Status == ScanStatusFailed
}

// Progress returns the scan progress as a percentage
func (s *Scan) Progress() float64 {
	if s.TotalJobsCount == 0 {
		return 0
	}
	completed := s.CompletedJobsCount + s.FailedJobsCount
	return float64(completed) / float64(s.TotalJobsCount) * 100
}

// TableHeaders returns table headers for CLI output
func (s Scan) TableHeaders() []string {
	return []string{"ID", "Title", "Status", "Phase", "Progress", "Workspace", "Created At"}
}

// TableRow returns table row for CLI output
func (s Scan) TableRow() []string {
	progress := fmt.Sprintf("%.1f%% (%d/%d)", s.Progress(), s.CompletedJobsCount+s.FailedJobsCount, s.TotalJobsCount)
	return []string{
		fmt.Sprintf("%d", s.ID),
		s.Title,
		string(s.Status),
		string(s.Phase),
		progress,
		fmt.Sprintf("%d", s.WorkspaceID),
		s.CreatedAt.Format(time.RFC3339),
	}
}

// String provides a basic textual representation
func (s Scan) String() string {
	return fmt.Sprintf("ID: %d, Title: %s, Status: %s, Phase: %s, Progress: %.1f%%",
		s.ID, s.Title, s.Status, s.Phase, s.Progress())
}

// Pretty provides a formatted representation
func (s Scan) Pretty() string {
	return fmt.Sprintf(
		"%sID:%s %d\n%sTitle:%s %s\n%sStatus:%s %s\n%sPhase:%s %s\n%sProgress:%s %.1f%% (%d/%d)\n%sWorkspace:%s %d\n%sCreated:%s %s\n",
		lib.Blue, lib.ResetColor, s.ID,
		lib.Blue, lib.ResetColor, s.Title,
		lib.Blue, lib.ResetColor, s.Status,
		lib.Blue, lib.ResetColor, s.Phase,
		lib.Blue, lib.ResetColor, s.Progress(), s.CompletedJobsCount+s.FailedJobsCount, s.TotalJobsCount,
		lib.Blue, lib.ResetColor, s.WorkspaceID,
		lib.Blue, lib.ResetColor, s.CreatedAt.Format(time.RFC3339),
	)
}

// ScanFilter represents available scan filters
type ScanFilter struct {
	Query       string       `json:"query" validate:"omitempty,ascii"`
	Statuses    []ScanStatus `json:"statuses" validate:"omitempty"`
	Phases      []ScanPhase  `json:"phases" validate:"omitempty"`
	WorkspaceID uint         `json:"workspace_id" validate:"omitempty,numeric"`
	Pagination  Pagination   `json:"pagination"`
	SortBy      string       `json:"sort_by" validate:"omitempty,oneof=id created_at updated_at status title"`
	SortOrder   string       `json:"sort_order" validate:"omitempty,oneof=asc desc"`
}

// CreateScan creates a new scan
func (d *DatabaseConnection) CreateScan(scan *Scan) (*Scan, error) {
	result := d.db.Create(scan)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("scan", scan).Msg("Scan creation failed")
	}
	return scan, result.Error
}

// GetScanByID retrieves a scan by ID
func (d *DatabaseConnection) GetScanByID(id uint) (*Scan, error) {
	var scan Scan
	err := d.db.Where("id = ?", id).First(&scan).Error
	if err != nil {
		log.Error().Err(err).Uint("id", id).Msg("Unable to fetch scan by ID")
		return nil, err
	}
	return &scan, nil
}

// UpdateScan updates a scan
func (d *DatabaseConnection) UpdateScan(scan *Scan) (*Scan, error) {
	result := d.db.Save(scan)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("scan", scan).Msg("Scan update failed")
	}
	return scan, result.Error
}

// DeleteScan deletes a scan and all its jobs
func (d *DatabaseConnection) DeleteScan(id uint) error {
	if err := d.db.Delete(&Scan{}, id).Error; err != nil {
		log.Error().Err(err).Uint("id", id).Msg("Error deleting scan")
		return err
	}
	return nil
}

// ListScans lists scans with filters
func (d *DatabaseConnection) ListScans(filter ScanFilter) (items []*Scan, count int64, err error) {
	query := d.db.Model(&Scan{})

	if filter.Query != "" {
		likeQuery := "%" + filter.Query + "%"
		query = query.Where("title ILIKE ?", likeQuery)
	}

	if len(filter.Statuses) > 0 {
		query = query.Where("status IN ?", filter.Statuses)
	}

	if len(filter.Phases) > 0 {
		query = query.Where("phase IN ?", filter.Phases)
	}

	if filter.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	}

	if err := query.Count(&count).Error; err != nil {
		return nil, 0, err
	}

	// Sorting
	validSortBy := map[string]bool{
		"id":         true,
		"created_at": true,
		"updated_at": true,
		"status":     true,
		"title":      true,
	}

	order := "id desc"
	if validSortBy[filter.SortBy] {
		sortOrder := "asc"
		if filter.SortOrder == "desc" {
			sortOrder = "desc"
		}
		order = filter.SortBy + " " + sortOrder
	}

	err = query.Scopes(Paginate(&filter.Pagination)).Order(order).Find(&items).Error
	return items, count, err
}

// PauseScan pauses a running scan
func (d *DatabaseConnection) PauseScan(scanID uint) (*Scan, error) {
	scan, err := d.GetScanByID(scanID)
	if err != nil {
		return nil, err
	}

	if !scan.IsPausable() {
		return nil, fmt.Errorf("scan %d cannot be paused (status: %s)", scanID, scan.Status)
	}

	scan.PreviousStatus = scan.Status
	scan.Status = ScanStatusPaused
	now := time.Now()
	scan.PausedAt = &now

	return d.UpdateScan(scan)
}

// ResumeScan resumes a paused scan
func (d *DatabaseConnection) ResumeScan(scanID uint) (*Scan, error) {
	scan, err := d.GetScanByID(scanID)
	if err != nil {
		return nil, err
	}

	if !scan.IsResumable() {
		return nil, fmt.Errorf("scan %d cannot be resumed (status: %s)", scanID, scan.Status)
	}

	if scan.PreviousStatus != "" {
		scan.Status = scan.PreviousStatus
	} else {
		scan.Status = ScanStatusScanning
	}
	scan.PreviousStatus = ""
	scan.PausedAt = nil

	return d.UpdateScan(scan)
}

// CancelScan cancels a scan and all its pending jobs
func (d *DatabaseConnection) CancelScan(scanID uint) (*Scan, error) {
	scan, err := d.GetScanByID(scanID)
	if err != nil {
		return nil, err
	}

	if !scan.IsCancellable() {
		return nil, fmt.Errorf("scan %d cannot be cancelled (status: %s)", scanID, scan.Status)
	}

	scan.Status = ScanStatusCancelled
	now := time.Now()
	scan.CompletedAt = &now

	// Cancel all pending and claimed jobs
	d.db.Model(&ScanJob{}).
		Where("scan_id = ? AND status IN ?", scanID, []ScanJobStatus{ScanJobStatusPending, ScanJobStatusClaimed}).
		Update("status", ScanJobStatusCancelled)

	return d.UpdateScan(scan)
}

// GetActiveScans returns all scans that are currently running
func (d *DatabaseConnection) GetActiveScans() ([]*Scan, error) {
	var scans []*Scan
	err := d.db.Where("status IN ?", []ScanStatus{ScanStatusCrawling, ScanStatusScanning}).Find(&scans).Error
	return scans, err
}

// GetPausedScans returns all paused scans
func (d *DatabaseConnection) GetPausedScans() ([]*Scan, error) {
	var scans []*Scan
	err := d.db.Where("status = ?", ScanStatusPaused).Find(&scans).Error
	return scans, err
}

// GetInterruptedScans returns scans that were interrupted (for restart recovery)
func (d *DatabaseConnection) GetInterruptedScans() ([]*Scan, error) {
	var scans []*Scan
	err := d.db.Where("status IN ?", []ScanStatus{ScanStatusCrawling, ScanStatusScanning, ScanStatusPaused}).Find(&scans).Error
	return scans, err
}

// ScanExists checks if a scan exists
func (d *DatabaseConnection) ScanExists(id uint) (bool, error) {
	var count int64
	err := d.db.Model(&Scan{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}

// UpdateScanJobCounts updates the job counts for a scan
func (d *DatabaseConnection) UpdateScanJobCounts(scanID uint) error {
	var pending, running, completed, failed int64

	d.db.Model(&ScanJob{}).Where("scan_id = ? AND status = ?", scanID, ScanJobStatusPending).Count(&pending)
	d.db.Model(&ScanJob{}).Where("scan_id = ? AND status = ?", scanID, ScanJobStatusRunning).Count(&running)
	d.db.Model(&ScanJob{}).Where("scan_id = ? AND status = ?", scanID, ScanJobStatusCompleted).Count(&completed)
	d.db.Model(&ScanJob{}).Where("scan_id = ? AND status = ?", scanID, ScanJobStatusFailed).Count(&failed)

	total := pending + running + completed + failed

	return d.db.Model(&Scan{}).Where("id = ?", scanID).Updates(map[string]interface{}{
		"total_jobs_count":     total,
		"pending_jobs_count":   pending,
		"running_jobs_count":   running,
		"completed_jobs_count": completed,
		"failed_jobs_count":    failed,
	}).Error
}

// SetScanPhase updates the scan phase
func (d *DatabaseConnection) SetScanPhase(scanID uint, phase ScanPhase) error {
	return d.db.Model(&Scan{}).Where("id = ?", scanID).Update("phase", phase).Error
}

// AtomicSetScanPhase atomically transitions a scan from expectedPhase to newPhase.
// Returns true if the transition succeeded (phase was expectedPhase and is now newPhase).
// Returns false if another process already transitioned the phase.
// This is safe for distributed systems where multiple orchestrators may be running.
func (d *DatabaseConnection) AtomicSetScanPhase(scanID uint, expectedPhase, newPhase ScanPhase) (bool, error) {
	result := d.db.Model(&Scan{}).Where("id = ? AND phase = ?", scanID, expectedPhase).Update("phase", newPhase)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// SetScanStatus updates the scan status
func (d *DatabaseConnection) SetScanStatus(scanID uint, status ScanStatus) error {
	updates := map[string]interface{}{
		"status": status,
	}

	if status == ScanStatusCrawling || status == ScanStatusScanning {
		now := time.Now()
		updates["started_at"] = now
	}

	if status == ScanStatusCompleted || status == ScanStatusFailed || status == ScanStatusCancelled {
		now := time.Now()
		updates["completed_at"] = now
	}

	return d.db.Model(&Scan{}).Where("id = ?", scanID).Updates(updates).Error
}

// ScanStatsResponse contains statistics for a scan
type ScanStatsResponse struct {
	Requests RequestsStats `json:"requests"`
	Issues   IssuesStats   `json:"issues"`
}

// GetScanStats retrieves request and issue statistics for a scan
func (d *DatabaseConnection) GetScanStats(scanID uint) (ScanStatsResponse, error) {
	var stats ScanStatsResponse

	// Get request counts by source
	historyCounts := map[string]int64{}
	rows, err := d.db.Model(&History{}).
		Select("source, COUNT(*) as count").
		Where("scan_id = ?", scanID).
		Group("source").Rows()
	if err != nil {
		return stats, err
	}
	for rows.Next() {
		var source string
		var count int64
		rows.Scan(&source, &count)
		historyCounts[source] = count
	}
	rows.Close()

	stats.Requests = RequestsStats{
		Crawler: historyCounts["Crawler"],
		Scanner: historyCounts["Scanner"],
	}

	// Get issue counts by severity
	issueCounts := map[severity]int64{}
	rows, err = d.db.Model(&Issue{}).
		Select("severity, COUNT(*) as count").
		Where("scan_id = ?", scanID).
		Group("severity").Rows()
	if err != nil {
		return stats, err
	}
	for rows.Next() {
		var sev severity
		var count int64
		rows.Scan(&sev, &count)
		issueCounts[sev] = count
	}
	rows.Close()

	stats.Issues = IssuesStats{
		Unknown:  issueCounts[Unknown],
		Info:     issueCounts[Info],
		Low:      issueCounts[Low],
		Medium:   issueCounts[Medium],
		High:     issueCounts[High],
		Critical: issueCounts[Critical],
	}

	return stats, nil
}
