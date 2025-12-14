package db

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
)

// BrowserEventType represents the specific type of browser event
type BrowserEventType string

const (
	BrowserEventConsole           BrowserEventType = "console"
	BrowserEventDialog            BrowserEventType = "dialog"
	BrowserEventDOMStorage        BrowserEventType = "dom_storage"
	BrowserEventSecurity          BrowserEventType = "security"
	BrowserEventCertificate       BrowserEventType = "certificate"
	BrowserEventAudit             BrowserEventType = "audit"
	BrowserEventIndexedDB         BrowserEventType = "indexeddb"
	BrowserEventCacheStorage      BrowserEventType = "cache_storage"
	BrowserEventBackgroundService BrowserEventType = "background_service"
	BrowserEventDatabase          BrowserEventType = "database"
	BrowserEventNetworkAuth       BrowserEventType = "network_auth"
)

// BrowserEventCategory represents the broad category of browser event
type BrowserEventCategory string

const (
	BrowserEventCategoryRuntime  BrowserEventCategory = "runtime"
	BrowserEventCategoryStorage  BrowserEventCategory = "storage"
	BrowserEventCategorySecurity BrowserEventCategory = "security"
	BrowserEventCategoryNetwork  BrowserEventCategory = "network"
	BrowserEventCategoryAudit    BrowserEventCategory = "audit"
)

// BrowserEvent represents a captured browser event during scanning
type BrowserEvent struct {
	BaseUUIDModel

	// Core event data
	EventType   BrowserEventType     `json:"event_type" gorm:"index;size:50;not null"`
	Category    BrowserEventCategory `json:"category" gorm:"index;size:50;not null"`
	URL         string               `json:"url" gorm:"index"`
	Description string               `json:"description" gorm:"type:text"`
	Data        datatypes.JSON       `json:"data"` // Event-specific data as JSONB

	// Aggregation fields
	ContentHash     string    `json:"content_hash" gorm:"index;size:64;not null"`
	OccurrenceCount int       `json:"occurrence_count" gorm:"default:1"`
	FirstSeenAt     time.Time `json:"first_seen_at"`
	LastSeenAt      time.Time `json:"last_seen_at"`

	// Context relationships
	WorkspaceID uint      `json:"workspace_id" gorm:"index;not null"`
	Workspace   Workspace `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ScanID      *uint     `json:"scan_id" gorm:"index"`
	Scan        *Scan     `json:"-" gorm:"foreignKey:ScanID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ScanJobID   *uint     `json:"scan_job_id" gorm:"index"`
	ScanJob     *ScanJob  `json:"-" gorm:"foreignKey:ScanJobID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	HistoryID   *uint     `json:"history_id" gorm:"index"`
	History     *History  `json:"-" gorm:"foreignKey:HistoryID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	TaskID      *uint     `json:"task_id" gorm:"index"`
	Task        *Task     `json:"-" gorm:"foreignKey:TaskID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`

	// Source tracking
	Source string `json:"source" gorm:"size:50;index"` // crawler, replay, audit, etc.
}

// TableName returns the table name for the BrowserEvent model
func (BrowserEvent) TableName() string {
	return "browser_events"
}

// ComputeContentHash generates a SHA256 hash of event_type + url + data for aggregation
func (e *BrowserEvent) ComputeContentHash() string {
	dataBytes, _ := json.Marshal(e.Data)
	hashInput := fmt.Sprintf("%s|%s|%s", e.EventType, e.URL, string(dataBytes))
	hash := sha256.Sum256([]byte(hashInput))
	return hex.EncodeToString(hash[:])
}

// BrowserEventFilter represents available filters for listing browser events
type BrowserEventFilter struct {
	Pagination
	WorkspaceID uint                   `json:"workspace_id" validate:"required"`
	ScanID      *uint                  `json:"scan_id"`
	ScanJobID   *uint                  `json:"scan_job_id"`
	HistoryID   *uint                  `json:"history_id"`
	TaskID      *uint                  `json:"task_id"`
	EventTypes  []BrowserEventType     `json:"event_types"`
	Categories  []BrowserEventCategory `json:"categories"`
	URL         string                 `json:"url"` // Partial match with ILIKE
	Sources     []string               `json:"sources"`
	SortBy      string                 `json:"sort_by" validate:"omitempty,oneof=id created_at event_type category occurrence_count last_seen_at first_seen_at"`
	SortOrder   string                 `json:"sort_order" validate:"omitempty,oneof=asc desc"`
}

// SaveBrowserEvent saves a browser event with aggregation logic.
// If an event with the same content hash already exists for the same scan (or workspace if no scan),
// it increments the occurrence count and updates last_seen_at.
// Otherwise, it creates a new record.
func (d *DatabaseConnection) SaveBrowserEvent(event *BrowserEvent) error {
	// Compute content hash if not already set
	if event.ContentHash == "" {
		event.ContentHash = event.ComputeContentHash()
	}

	now := time.Now()

	// Build query to find existing event with same hash
	query := d.db.Model(&BrowserEvent{}).
		Where("content_hash = ?", event.ContentHash).
		Where("workspace_id = ?", event.WorkspaceID)

	// Scope to scan if provided
	if event.ScanID != nil {
		query = query.Where("scan_id = ?", *event.ScanID)
	} else {
		query = query.Where("scan_id IS NULL")
	}

	// Try to find and update existing event
	var existingEvent BrowserEvent
	err := query.First(&existingEvent).Error

	log.Debug().Str("content_hash", event.ContentHash).Uint("workspace_id", event.WorkspaceID).Err(err).Msg("Query result for existing browser event")

	if err == nil {
		// Event exists, update occurrence count and last_seen_at
		log.Debug().Str("content_hash", event.ContentHash).Uint("workspace_id", event.WorkspaceID).Msg("Browser event aggregating with existing event")
		return d.db.Model(&existingEvent).Updates(map[string]interface{}{
			"occurrence_count": existingEvent.OccurrenceCount + 1,
			"last_seen_at":     now,
		}).Error
	}

	// Event doesn't exist, create new one
	// Ensure ID is generated if not already set
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}

	event.FirstSeenAt = now
	event.LastSeenAt = now
	if event.OccurrenceCount == 0 {
		event.OccurrenceCount = 1
	}

	log.Debug().Str("id", event.ID.String()).Str("content_hash", event.ContentHash).Int("occurrence_count", event.OccurrenceCount).Msg("Creating new browser event")
	result := d.db.Create(event)
	log.Debug().Str("id", event.ID.String()).Str("content_hash", event.ContentHash).Int("occurrence_count", event.OccurrenceCount).Err(result.Error).Msg("Browser event created")
	return result.Error
}

// SaveBrowserEvents saves multiple browser events with aggregation logic
func (d *DatabaseConnection) SaveBrowserEvents(events []*BrowserEvent) error {
	for _, event := range events {
		if err := d.SaveBrowserEvent(event); err != nil {
			log.Error().Err(err).Str("event_type", string(event.EventType)).Msg("Failed to save browser event")
			// Continue with other events even if one fails
		}
	}
	return nil
}

// GetBrowserEvent retrieves a browser event by ID
func (d *DatabaseConnection) GetBrowserEvent(id uuid.UUID) (*BrowserEvent, error) {
	var event BrowserEvent
	err := d.db.Where("id = ?", id).First(&event).Error
	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Unable to fetch browser event by ID")
		return nil, err
	}
	return &event, nil
}

// ListBrowserEvents lists browser events with filters and pagination
func (d *DatabaseConnection) ListBrowserEvents(filter BrowserEventFilter) (items []BrowserEvent, count int64, err error) {
	query := d.db.Model(&BrowserEvent{})

	// Required filter
	if filter.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	}

	// Optional filters
	if filter.ScanID != nil && *filter.ScanID > 0 {
		query = query.Where("scan_id = ?", *filter.ScanID)
	}

	if filter.ScanJobID != nil && *filter.ScanJobID > 0 {
		query = query.Where("scan_job_id = ?", *filter.ScanJobID)
	}

	if filter.HistoryID != nil && *filter.HistoryID > 0 {
		query = query.Where("history_id = ?", *filter.HistoryID)
	}

	if filter.TaskID != nil && *filter.TaskID > 0 {
		query = query.Where("task_id = ?", *filter.TaskID)
	}

	if len(filter.EventTypes) > 0 {
		query = query.Where("event_type IN ?", filter.EventTypes)
	}

	if len(filter.Categories) > 0 {
		query = query.Where("category IN ?", filter.Categories)
	}

	if filter.URL != "" {
		query = query.Where("url ILIKE ?", "%"+filter.URL+"%")
	}

	if len(filter.Sources) > 0 {
		query = query.Where("source IN ?", filter.Sources)
	}

	// Count before pagination
	if err := query.Count(&count).Error; err != nil {
		log.Error().Err(err).Msg("Failed to count browser events")
		return nil, 0, err
	}

	// Sorting
	validSortBy := map[string]bool{
		"id":               true,
		"created_at":       true,
		"event_type":       true,
		"category":         true,
		"occurrence_count": true,
		"last_seen_at":     true,
		"first_seen_at":    true,
	}

	order := "last_seen_at desc"
	if validSortBy[filter.SortBy] {
		sortOrder := "asc"
		if filter.SortOrder == "desc" {
			sortOrder = "desc"
		}
		order = filter.SortBy + " " + sortOrder
	}

	// Apply pagination and execute query
	if filter.PageSize > 0 && filter.Page > 0 {
		query = query.Scopes(Paginate(&filter.Pagination))
	}

	err = query.Order(order).Find(&items).Error
	if err != nil {
		log.Error().Err(err).Msg("Failed to list browser events")
		return nil, 0, err
	}

	return items, count, nil
}

// DeleteBrowserEventsByScanID deletes all browser events for a scan
func (d *DatabaseConnection) DeleteBrowserEventsByScanID(scanID uint) error {
	result := d.db.Where("scan_id = ?", scanID).Delete(&BrowserEvent{})
	if result.Error != nil {
		log.Error().Err(result.Error).Uint("scan_id", scanID).Msg("Failed to delete browser events by scan ID")
		return result.Error
	}
	log.Debug().Uint("scan_id", scanID).Int64("deleted_count", result.RowsAffected).Msg("Deleted browser events for scan")
	return nil
}

// DeleteBrowserEventsByWorkspaceID deletes all browser events for a workspace
func (d *DatabaseConnection) DeleteBrowserEventsByWorkspaceID(workspaceID uint) error {
	result := d.db.Where("workspace_id = ?", workspaceID).Delete(&BrowserEvent{})
	if result.Error != nil {
		log.Error().Err(result.Error).Uint("workspace_id", workspaceID).Msg("Failed to delete browser events by workspace ID")
		return result.Error
	}
	log.Debug().Uint("workspace_id", workspaceID).Int64("deleted_count", result.RowsAffected).Msg("Deleted browser events for workspace")
	return nil
}

// CountBrowserEventsByScanID returns the count of browser events for a scan
func (d *DatabaseConnection) CountBrowserEventsByScanID(scanID uint) (int64, error) {
	var count int64
	err := d.db.Model(&BrowserEvent{}).Where("scan_id = ?", scanID).Count(&count).Error
	return count, err
}

// GetBrowserEventTypeStats returns event counts grouped by event type for a scan
func (d *DatabaseConnection) GetBrowserEventTypeStats(scanID uint) (map[BrowserEventType]int64, error) {
	stats := make(map[BrowserEventType]int64)

	rows, err := d.db.Model(&BrowserEvent{}).
		Select("event_type, SUM(occurrence_count) as total_count").
		Where("scan_id = ?", scanID).
		Group("event_type").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var eventType BrowserEventType
		var count int64
		if err := rows.Scan(&eventType, &count); err != nil {
			continue
		}
		stats[eventType] = count
	}

	return stats, nil
}

// GetBrowserEventCategoryStats returns event counts grouped by category for a scan
func (d *DatabaseConnection) GetBrowserEventCategoryStats(scanID uint) (map[BrowserEventCategory]int64, error) {
	stats := make(map[BrowserEventCategory]int64)

	rows, err := d.db.Model(&BrowserEvent{}).
		Select("category, SUM(occurrence_count) as total_count").
		Where("scan_id = ?", scanID).
		Group("category").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var category BrowserEventCategory
		var count int64
		if err := rows.Scan(&category, &count); err != nil {
			continue
		}
		stats[category] = count
	}

	return stats, nil
}

// ScanStorageEventSummary provides aggregate info about storage usage across a scan
type ScanStorageEventSummary struct {
	ScanID                  uint
	HasLocalStorageEvents   bool
	HasSessionStorageEvents bool
	LocalStorageKeys        []string
	SessionStorageKeys      []string
	TotalStorageEvents      int64
}

// HasStorageEventsForScan returns true if any storage events were captured during crawl
func (d *DatabaseConnection) HasStorageEventsForScan(scanID uint) (bool, error) {
	var count int64
	err := d.db.Model(&BrowserEvent{}).
		Where("scan_id = ?", scanID).
		Where("event_type = ?", BrowserEventDOMStorage).
		Where("source = ?", SourceCrawler).
		Limit(1).
		Count(&count).Error

	if err != nil {
		log.Error().Err(err).Uint("scan_id", scanID).Msg("Failed to check for storage events")
		return false, err
	}

	return count > 0, nil
}

// GetStorageKeysFromCrawlEvents returns deduplicated storage keys observed during crawl
func (d *DatabaseConnection) GetStorageKeysFromCrawlEvents(scanID uint, storageType string) ([]string, error) {
	isLocalStorage := storageType == "localStorage"

	var events []BrowserEvent
	err := d.db.Model(&BrowserEvent{}).
		Where("scan_id = ?", scanID).
		Where("event_type = ?", BrowserEventDOMStorage).
		Where("source = ?", SourceCrawler).
		Find(&events).Error

	if err != nil {
		log.Error().Err(err).Uint("scan_id", scanID).Str("storage_type", storageType).Msg("Failed to get storage keys from crawl events")
		return nil, err
	}

	// Extract unique keys from event data
	keySet := make(map[string]bool)
	for _, event := range events {
		var data map[string]interface{}
		if err := json.Unmarshal(event.Data, &data); err != nil {
			continue
		}

		// Check if this event is for the requested storage type
		eventIsLocal, ok := data["isLocalStorage"].(bool)
		if !ok {
			continue
		}
		if eventIsLocal != isLocalStorage {
			continue
		}

		// Extract the key
		if key, ok := data["key"].(string); ok && key != "" {
			keySet[key] = true
		}
	}

	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}

	log.Debug().
		Uint("scan_id", scanID).
		Str("storage_type", storageType).
		Int("keys_found", len(keys)).
		Strs("keys", keys).
		Msg("Retrieved storage keys from crawl events")

	return keys, nil
}

// GetScanStorageEventSummary retrieves storage event summary for a scan
func (d *DatabaseConnection) GetScanStorageEventSummary(scanID uint) (*ScanStorageEventSummary, error) {
	summary := &ScanStorageEventSummary{
		ScanID:             scanID,
		LocalStorageKeys:   []string{},
		SessionStorageKeys: []string{},
	}

	// Get all storage events for this scan from the crawl phase
	var events []BrowserEvent
	err := d.db.Model(&BrowserEvent{}).
		Where("scan_id = ?", scanID).
		Where("event_type = ?", BrowserEventDOMStorage).
		Where("source = ?", SourceCrawler).
		Find(&events).Error

	if err != nil {
		log.Error().Err(err).Uint("scan_id", scanID).Msg("Failed to get storage event summary")
		return nil, err
	}

	summary.TotalStorageEvents = int64(len(events))

	// Process events to extract keys and determine storage types
	localKeySet := make(map[string]bool)
	sessionKeySet := make(map[string]bool)

	for _, event := range events {
		var data map[string]interface{}
		if err := json.Unmarshal(event.Data, &data); err != nil {
			continue
		}

		isLocal, ok := data["isLocalStorage"].(bool)
		if !ok {
			continue
		}

		key, _ := data["key"].(string)

		if isLocal {
			summary.HasLocalStorageEvents = true
			if key != "" {
				localKeySet[key] = true
			}
		} else {
			summary.HasSessionStorageEvents = true
			if key != "" {
				sessionKeySet[key] = true
			}
		}
	}

	// Convert sets to slices
	for key := range localKeySet {
		summary.LocalStorageKeys = append(summary.LocalStorageKeys, key)
	}
	for key := range sessionKeySet {
		summary.SessionStorageKeys = append(summary.SessionStorageKeys, key)
	}

	log.Debug().
		Uint("scan_id", scanID).
		Bool("has_local", summary.HasLocalStorageEvents).
		Bool("has_session", summary.HasSessionStorageEvents).
		Int("local_keys", len(summary.LocalStorageKeys)).
		Int("session_keys", len(summary.SessionStorageKeys)).
		Int64("total_events", summary.TotalStorageEvents).
		Msg("Generated storage event summary for scan")

	return summary, nil
}

// Formattable interface implementation for BrowserEvent

// TableHeaders returns the headers for table output
func (e BrowserEvent) TableHeaders() []string {
	return []string{"ID", "Event Type", "Category", "URL", "Description", "Occurrences", "Source", "Last Seen"}
}

// TableRow returns the row data for table output
func (e BrowserEvent) TableRow() []string {
	formattedURL := e.URL
	if len(e.URL) > 50 {
		formattedURL = e.URL[0:50] + "..."
	}
	formattedDesc := e.Description
	if len(e.Description) > 40 {
		formattedDesc = e.Description[0:40] + "..."
	}
	return []string{
		e.ID.String(),
		string(e.EventType),
		string(e.Category),
		formattedURL,
		formattedDesc,
		fmt.Sprintf("%d", e.OccurrenceCount),
		e.Source,
		e.LastSeenAt.Format("2006-01-02 15:04:05"),
	}
}

// String returns a plain text representation
func (e BrowserEvent) String() string {
	dataBytes, _ := json.Marshal(e.Data)
	scanID := ""
	if e.ScanID != nil {
		scanID = fmt.Sprintf("%d", *e.ScanID)
	}
	return fmt.Sprintf(
		"ID: %s\nEvent Type: %s\nCategory: %s\nURL: %s\nDescription: %s\nData: %s\nOccurrences: %d\nFirst Seen: %s\nLast Seen: %s\nSource: %s\nWorkspace ID: %d\nScan ID: %s",
		e.ID.String(), e.EventType, e.Category, e.URL, e.Description, string(dataBytes),
		e.OccurrenceCount, e.FirstSeenAt.Format(time.RFC3339), e.LastSeenAt.Format(time.RFC3339),
		e.Source, e.WorkspaceID, scanID,
	)
}

// Pretty returns a colorized representation
func (e BrowserEvent) Pretty() string {
	dataBytes, _ := json.MarshalIndent(e.Data, "", "  ")
	scanID := "N/A"
	if e.ScanID != nil {
		scanID = fmt.Sprintf("%d", *e.ScanID)
	}
	return fmt.Sprintf(
		"%sID:%s %s\n%sEvent Type:%s %s\n%sCategory:%s %s\n%sURL:%s %s\n%sDescription:%s %s\n%sData:%s %s\n%sOccurrences:%s %d\n%sFirst Seen:%s %s\n%sLast Seen:%s %s\n%sSource:%s %s\n%sWorkspace ID:%s %d\n%sScan ID:%s %s\n",
		lib.Blue, lib.ResetColor, e.ID.String(),
		lib.Blue, lib.ResetColor, e.EventType,
		lib.Blue, lib.ResetColor, e.Category,
		lib.Blue, lib.ResetColor, e.URL,
		lib.Blue, lib.ResetColor, e.Description,
		lib.Blue, lib.ResetColor, string(dataBytes),
		lib.Blue, lib.ResetColor, e.OccurrenceCount,
		lib.Blue, lib.ResetColor, e.FirstSeenAt.Format(time.RFC3339),
		lib.Blue, lib.ResetColor, e.LastSeenAt.Format(time.RFC3339),
		lib.Blue, lib.ResetColor, e.Source,
		lib.Blue, lib.ResetColor, e.WorkspaceID,
		lib.Blue, lib.ResetColor, scanID,
	)
}
