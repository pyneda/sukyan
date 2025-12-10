package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// BrowserEventResponse represents a browser event for API responses
type BrowserEventResponse struct {
	ID          uuid.UUID `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	EventType   string    `json:"event_type"`
	Category    string    `json:"category"`
	URL         string    `json:"url"`
	Description string    `json:"description"`
	Data        any       `json:"data"`

	// Aggregation fields
	ContentHash     string    `json:"content_hash"`
	OccurrenceCount int       `json:"occurrence_count"`
	FirstSeenAt     time.Time `json:"first_seen_at"`
	LastSeenAt      time.Time `json:"last_seen_at"`

	// Context relationships
	WorkspaceID uint  `json:"workspace_id"`
	ScanID      *uint `json:"scan_id,omitempty"`
	ScanJobID   *uint `json:"scan_job_id,omitempty"`
	HistoryID   *uint `json:"history_id,omitempty"`
	TaskID      *uint `json:"task_id,omitempty"`

	// Source tracking
	Source string `json:"source"`
}

// BrowserEventListResponse represents a paginated list of browser events
type BrowserEventListResponse struct {
	Data  []BrowserEventResponse `json:"data"`
	Count int64                  `json:"count"`
}

// BrowserEventStatsResponse represents browser event statistics
type BrowserEventStatsResponse struct {
	TotalCount  int64            `json:"total_count"`
	ByEventType map[string]int64 `json:"by_event_type"`
	ByCategory  map[string]int64 `json:"by_category"`
}

// BrowserEventResponseFromDB creates a BrowserEventResponse from a db.BrowserEvent
func BrowserEventResponseFromDB(event *db.BrowserEvent) BrowserEventResponse {
	var data any
	if event.Data != nil {
		// Unmarshal the JSON data to a generic type for the response
		_ = json.Unmarshal(event.Data, &data)
	}

	return BrowserEventResponse{
		ID:              event.ID,
		CreatedAt:       event.CreatedAt,
		UpdatedAt:       event.UpdatedAt,
		EventType:       string(event.EventType),
		Category:        string(event.Category),
		URL:             event.URL,
		Description:     event.Description,
		Data:            data,
		ContentHash:     event.ContentHash,
		OccurrenceCount: event.OccurrenceCount,
		FirstSeenAt:     event.FirstSeenAt,
		LastSeenAt:      event.LastSeenAt,
		WorkspaceID:     event.WorkspaceID,
		ScanID:          event.ScanID,
		ScanJobID:       event.ScanJobID,
		HistoryID:       event.HistoryID,
		TaskID:          event.TaskID,
		Source:          event.Source,
	}
}

// BrowserEventResponsesFromDB creates a slice of BrowserEventResponse from a slice of db.BrowserEvent
func BrowserEventResponsesFromDB(events []db.BrowserEvent) []BrowserEventResponse {
	responses := make([]BrowserEventResponse, len(events))
	for i, event := range events {
		responses[i] = BrowserEventResponseFromDB(&event)
	}
	return responses
}

// GetValidBrowserEventTypes returns all valid browser event types
func GetValidBrowserEventTypes() []string {
	return []string{
		string(db.BrowserEventConsole),
		string(db.BrowserEventDialog),
		string(db.BrowserEventDOMStorage),
		string(db.BrowserEventSecurity),
		string(db.BrowserEventCertificate),
		string(db.BrowserEventAudit),
		string(db.BrowserEventIndexedDB),
		string(db.BrowserEventCacheStorage),
		string(db.BrowserEventBackgroundService),
		string(db.BrowserEventDatabase),
		string(db.BrowserEventNetworkAuth),
	}
}

// GetValidBrowserEventCategories returns all valid browser event categories
func GetValidBrowserEventCategories() []string {
	return []string{
		string(db.BrowserEventCategoryRuntime),
		string(db.BrowserEventCategoryStorage),
		string(db.BrowserEventCategorySecurity),
		string(db.BrowserEventCategoryNetwork),
		string(db.BrowserEventCategoryAudit),
	}
}

// @Summary Get browser events
// @Description Get browser events with optional pagination and filtering
// @Tags BrowserEvents
// @Produce json
// @Param page_size query integer false "Size of each page" default(50)
// @Param page query integer false "Page number" default(1)
// @Param workspace query int true "Workspace ID"
// @Param scan_id query int false "Scan ID"
// @Param scan_job_id query int false "Scan Job ID"
// @Param history_id query int false "History ID"
// @Param task query int false "Task ID"
// @Param event_types query string false "Comma-separated list of event types (console,dialog,dom_storage,security,certificate,audit,indexeddb,cache_storage,background_service,database,network_auth)"
// @Param categories query string false "Comma-separated list of categories (runtime,storage,security,network,audit)"
// @Param url query string false "Filter by URL (partial match)"
// @Param sources query string false "Comma-separated list of sources to filter by"
// @Param sort_by query string false "Sort by field (id,created_at,event_type,category,occurrence_count,last_seen_at,first_seen_at)" default(last_seen_at)
// @Param sort_order query string false "Sort order (asc,desc)" default(desc)
// @Success 200 {object} BrowserEventListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/browser-events [get]
func FindBrowserEvents(c *fiber.Ctx) error {
	unparsedPageSize := c.Query("page_size", "50")
	unparsedPage := c.Query("page", "1")
	unparsedEventTypes := c.Query("event_types")
	unparsedCategories := c.Query("categories")
	unparsedSources := c.Query("sources")
	urlFilter := c.Query("url")
	sortBy := c.Query("sort_by", "last_seen_at")
	sortOrder := c.Query("sort_order", "desc")

	pageSize, err := parseInt(unparsedPageSize)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing page size parameter query")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid page size parameter"})
	}

	page, err := parseInt(unparsedPage)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing page parameter query")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid page parameter"})
	}

	workspaceID, err := parseWorkspaceID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}

	scanID, err := parseScanID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid scan",
			"message": "The provided scan ID does not seem valid",
		})
	}

	scanJobID, err := parseScanJobID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid scan job",
			"message": "The provided scan job ID does not seem valid",
		})
	}

	historyID, err := parseHistoryID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid history",
			"message": "The provided history ID does not seem valid",
		})
	}

	taskID, err := parseTaskID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid task",
			"message": "The provided task ID does not seem valid",
		})
	}

	// Parse event types
	var eventTypes []db.BrowserEventType
	if unparsedEventTypes != "" {
		validTypes := GetValidBrowserEventTypes()
		for _, et := range strings.Split(unparsedEventTypes, ",") {
			et = strings.TrimSpace(et)
			found := false
			for _, valid := range validTypes {
				if et == valid {
					eventTypes = append(eventTypes, db.BrowserEventType(et))
					found = true
					break
				}
			}
			if !found {
				log.Warn().Str("event_type", et).Msg("Invalid event type provided")
			}
		}
	}

	// Parse categories
	var categories []db.BrowserEventCategory
	if unparsedCategories != "" {
		validCategories := GetValidBrowserEventCategories()
		for _, cat := range strings.Split(unparsedCategories, ",") {
			cat = strings.TrimSpace(cat)
			found := false
			for _, valid := range validCategories {
				if cat == valid {
					categories = append(categories, db.BrowserEventCategory(cat))
					found = true
					break
				}
			}
			if !found {
				log.Warn().Str("category", cat).Msg("Invalid category provided")
			}
		}
	}

	// Parse sources
	var sources []string
	if unparsedSources != "" {
		sources = parseCommaSeparatedStrings(unparsedSources)
	}

	// Build filter
	filter := db.BrowserEventFilter{
		Pagination: db.Pagination{
			Page:     page,
			PageSize: pageSize,
		},
		WorkspaceID: workspaceID,
		EventTypes:  eventTypes,
		Categories:  categories,
		URL:         urlFilter,
		Sources:     sources,
		SortBy:      sortBy,
		SortOrder:   sortOrder,
	}

	// Set optional uint filters
	if scanID > 0 {
		filter.ScanID = &scanID
	}
	if scanJobID > 0 {
		filter.ScanJobID = &scanJobID
	}
	if historyID > 0 {
		filter.HistoryID = &historyID
	}
	if taskID > 0 {
		filter.TaskID = &taskID
	}

	events, count, err := db.Connection().ListBrowserEvents(filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
	}

	response := BrowserEventListResponse{
		Data:  BrowserEventResponsesFromDB(events),
		Count: count,
	}
	return c.Status(http.StatusOK).JSON(response)
}

// @Summary Get browser event by ID
// @Description Get details of a specific browser event by its UUID
// @Tags BrowserEvents
// @Produce json
// @Param id path string true "Browser event UUID"
// @Success 200 {object} BrowserEventResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/browser-events/{id} [get]
func GetBrowserEventByID(c *fiber.Ctx) error {
	idParam := c.Params("id")

	eventID, err := uuid.Parse(idParam)
	if err != nil {
		log.Error().Err(err).Str("id", idParam).Msg("Error parsing browser event ID parameter")
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid browser event ID",
			Message: "The provided browser event ID is not a valid UUID",
		})
	}

	event, err := db.Connection().GetBrowserEvent(eventID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Browser event not found",
				Message: "No browser event found with the provided ID",
			})
		}
		log.Error().Err(err).Str("id", idParam).Msg("Error fetching browser event details")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Error fetching browser event details",
			Message: "An unexpected error occurred while fetching the browser event details. Please try again later.",
		})
	}

	return c.Status(http.StatusOK).JSON(BrowserEventResponseFromDB(event))
}

// @Summary Get browser event statistics
// @Description Get browser event statistics for a scan grouped by event type and category
// @Tags BrowserEvents
// @Produce json
// @Param scan_id query int true "Scan ID"
// @Success 200 {object} BrowserEventStatsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/browser-events/stats [get]
func GetBrowserEventStats(c *fiber.Ctx) error {
	scanID, err := parseScanID(c)
	if err != nil || scanID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid scan",
			"message": "A valid scan_id is required",
		})
	}

	typeStats, err := db.Connection().GetBrowserEventTypeStats(scanID)
	if err != nil {
		log.Error().Err(err).Uint("scan_id", scanID).Msg("Error fetching browser event type stats")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
	}

	categoryStats, err := db.Connection().GetBrowserEventCategoryStats(scanID)
	if err != nil {
		log.Error().Err(err).Uint("scan_id", scanID).Msg("Error fetching browser event category stats")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
	}

	totalCount, err := db.Connection().CountBrowserEventsByScanID(scanID)
	if err != nil {
		log.Error().Err(err).Uint("scan_id", scanID).Msg("Error counting browser events")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
	}

	// Convert typed maps to string-keyed maps for the response
	byEventType := make(map[string]int64, len(typeStats))
	for k, v := range typeStats {
		byEventType[string(k)] = v
	}

	byCategory := make(map[string]int64, len(categoryStats))
	for k, v := range categoryStats {
		byCategory[string(k)] = v
	}

	response := BrowserEventStatsResponse{
		TotalCount:  totalCount,
		ByEventType: byEventType,
		ByCategory:  byCategory,
	}
	return c.Status(http.StatusOK).JSON(response)
}

// parseHistoryID parses the history_id query parameter
func parseHistoryID(c *fiber.Ctx) (uint, error) {
	unparsed := c.Query("history_id")
	if unparsed == "" {
		return 0, nil
	}
	historyID64, err := parseUint(unparsed)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing history_id parameter query")
		return 0, err
	}
	return historyID64, nil
}
