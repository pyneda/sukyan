package db

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pyneda/sukyan/lib"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// History holds table for storing requests history found
type History struct {
	BaseModel
	StatusCode          int               `gorm:"index" json:"status_code"`
	URL                 string            `json:"url"`
	CleanURL            string            `gorm:"index" json:"clean_url"`
	Depth               int               `gorm:"index" json:"depth"`
	RawRequest          []byte            `json:"raw_request"`
	RawResponse         []byte            `json:"raw_response"`
	Method              string            `gorm:"index" json:"method"`
	Proto               string            `json:"proto" gorm:"index"`
	ParametersCount     int               `gorm:"index" json:"parameters_count"`
	Evaluated           bool              `gorm:"index" json:"evaluated"`
	Note                string            `json:"note"`
	Source              string            `gorm:"index" json:"source"`
	JsonWebTokens       []JsonWebToken    `gorm:"many2many:json_web_token_histories;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"json_web_tokens"`
	Workspace           Workspace         `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WorkspaceID         *uint             `json:"workspace_id" gorm:"index"`
	TaskID              *uint             `json:"task_id" gorm:"index" `
	Task                Task              `json:"-" gorm:"foreignKey:TaskID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ScanID              *uint             `json:"scan_id" gorm:"index"`
	Scan                *Scan             `json:"-" gorm:"foreignKey:ScanID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ScanJobID           *uint             `json:"scan_job_id" gorm:"index"`
	ScanJob             *ScanJob          `json:"-" gorm:"foreignKey:ScanJobID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	PlaygroundSessionID *uint             `json:"playground_session_id" gorm:"index" `
	PlaygroundSession   PlaygroundSession `json:"-" gorm:"foreignKey:PlaygroundSessionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ResponseBodySize    int               `gorm:"index" json:"response_body_size"`
	RequestBodySize     int               `gorm:"index" json:"request_body_size"`
	RequestContentType  string            `gorm:"index" json:"request_content_type"`
	ResponseContentType string            `gorm:"index" json:"response_content_type"`
	IsWebSocketUpgrade  bool              `json:"is_websocket_upgrade"`
}

func (h History) TaskTitle() string {
	return fmt.Sprintf("Active scan %s", h.URL)
}

func (h History) Logger() *zerolog.Logger {
	logger := log.With().Uint("history", h.ID).Str("method", h.Method).Str("url", h.URL).Logger()
	return &logger
}

func (h History) ResponseHash() string {
	body, _ := h.ResponseBody()
	return lib.HashBytes(body)
}

// RequestBody returns the request body extracted from RawRequest
func (h History) RequestBody() ([]byte, error) {
	_, body, err := lib.SplitHTTPMessage(h.RawRequest)
	return body, err
}

// ResponseBody returns the response body extracted from RawResponse
func (h History) ResponseBody() ([]byte, error) {
	_, body, err := lib.SplitHTTPMessage(h.RawResponse)
	return body, err
}

// RequestHeaders returns the request headers as a map
func (h History) RequestHeaders() (map[string][]string, error) {
	headers, _, err := lib.SplitHTTPMessage(h.RawRequest)
	if err != nil {
		return nil, err
	}
	return lib.ParseHTTPHeaders(headers)
}

// ResponseHeaders returns the response headers as a map
func (h History) ResponseHeaders() (map[string][]string, error) {
	headers, _, err := lib.SplitHTTPMessage(h.RawResponse)
	if err != nil {
		return nil, err
	}
	return lib.ParseHTTPHeaders(headers)
}

// RequestHeadersAsString returns the request headers as a string
func (h History) RequestHeadersAsString() (string, error) {
	headers, err := h.RequestHeaders()
	if err != nil {
		return "", err
	}
	return lib.FormatHeadersAsString(headers), nil
}

// ResponseHeadersAsString returns the response headers as a string
func (h History) ResponseHeadersAsString() (string, error) {
	headers, err := h.ResponseHeaders()
	if err != nil {
		return "", err
	}
	return lib.FormatHeadersAsString(headers), nil
}

// IsHTMLResponse checks if the response appears to be HTML content.
// This is useful for detecting soft 404s where servers return their homepage
// or default page for any requested path.
func (h History) IsHTMLResponse() bool {
	// Check Content-Type header first
	contentType := strings.ToLower(h.ResponseContentType)
	if strings.Contains(contentType, "text/html") {
		return true
	}

	// Also check body content for HTML markers
	body, err := h.ResponseBody()
	if err != nil || len(body) == 0 {
		return false
	}

	bodyStr := strings.TrimSpace(string(body))
	bodyLower := strings.ToLower(bodyStr)

	// Check for common HTML indicators at the start
	// Note: <?xml alone is NOT HTML - many config files (web.config, pom.xml) start with it
	// XHTML starts with <?xml but then has <!DOCTYPE html or <html xmlns
	htmlPrefixes := []string{
		"<!doctype html",
		"<html",
		"<head",
		"<body",
	}

	for _, prefix := range htmlPrefixes {
		if strings.HasPrefix(bodyLower, prefix) {
			return true
		}
	}

	// Handle XML documents - only treat as HTML if it's actually XHTML
	if strings.HasPrefix(bodyLower, "<?xml") {
		// Check if this is XHTML (has html element after xml declaration)
		if strings.Contains(bodyLower, "<html") ||
			strings.Contains(bodyLower, "<!doctype html") ||
			strings.Contains(bodyLower, "xhtml") {
			return true
		}
		// Regular XML config file, not HTML
		return false
	}

	// Check for HTML structure patterns anywhere in content
	htmlPatterns := []string{
		"<html",
		"</html>",
		"<head>",
		"</head>",
		"<body",
		"</body>",
		"<title>",
		"<meta charset",
		"<meta http-equiv",
		"<link rel=\"stylesheet\"",
		"<script src=",
	}

	htmlIndicators := 0
	for _, pattern := range htmlPatterns {
		if strings.Contains(bodyLower, pattern) {
			htmlIndicators++
			if htmlIndicators >= 3 {
				return true
			}
		}
	}

	return false
}

// IsJSONResponse checks if the response appears to be JSON content
func (h History) IsJSONResponse() bool {
	contentType := strings.ToLower(h.ResponseContentType)
	if strings.Contains(contentType, "application/json") {
		return true
	}

	body, err := h.ResponseBody()
	if err != nil || len(body) == 0 {
		return false
	}

	trimmed := strings.TrimSpace(string(body))
	return (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"))
}

// IsXMLResponse checks if the response appears to be XML content (not HTML/XHTML)
func (h History) IsXMLResponse() bool {
	contentType := strings.ToLower(h.ResponseContentType)
	if strings.Contains(contentType, "application/xml") ||
		strings.Contains(contentType, "text/xml") ||
		strings.Contains(contentType, "application/xrd+xml") {
		return true
	}

	body, err := h.ResponseBody()
	if err != nil || len(body) == 0 {
		return false
	}

	trimmed := strings.TrimSpace(string(body))
	trimmedLower := strings.ToLower(trimmed)
	// Make sure it's XML but not HTML/XHTML
	return strings.HasPrefix(trimmedLower, "<?xml") &&
		!strings.Contains(trimmedLower, "<!doctype html") &&
		!strings.Contains(trimmedLower, "<html")
}

// IsPlainTextResponse checks if the response is plain text
func (h History) IsPlainTextResponse() bool {
	contentType := strings.ToLower(h.ResponseContentType)
	return strings.Contains(contentType, "text/plain")
}

// For backward compatibility
func (h *History) GetResponseHeadersAsMap() (map[string][]string, error) {
	return h.ResponseHeaders()
}

// For backward compatibility
func (h *History) GetRequestHeadersAsMap() (map[string][]string, error) {
	return h.RequestHeaders()
}

// For backward compatibility
func (h *History) GetResponseHeadersAsString() (string, error) {
	return h.ResponseHeadersAsString()
}

func (h History) TableHeaders() []string {
	return []string{"ID", "URL", "Method", "Response Body Size", "Workspace", "Task", "Source"}
}

func (h History) TableRow() []string {
	formattedURL := h.URL
	if len(h.URL) > PrintMaxURLLength {
		formattedURL = h.URL[0:PrintMaxURLLength] + "..."
	}
	return []string{
		fmt.Sprintf("%d", h.ID),
		formattedURL,
		h.Method,
		fmt.Sprintf("%d", h.ResponseBodySize),
		formatUintPointer(h.WorkspaceID),
		formatUintPointer(h.TaskID),
		h.Source,
	}
}

func (h History) String() string {
	return fmt.Sprintf(
		"ID: %d\nWorkspace: %s\nTask: %s\nSource: %s\nURL: %s\nMethod: %s\nResponse Body Size: %d\nRequest:\n%s\nResponse:\n%s",
		h.ID, formatUintPointer(h.WorkspaceID), formatUintPointer(h.TaskID), h.Source, h.URL, h.Method, h.ResponseBodySize, string(h.RawRequest), string(h.RawResponse),
	)
}

func (h History) Pretty() string {
	return fmt.Sprintf(
		"%sID:%s %d\n%sWorkspace:%s %s\n%sTask:%s %s\n%sSource:%s %s\n%sURL:%s %s\n%sMethod:%s %s\n%sResponseBodySize:%s %d\n%sRequest:\n%s%s\n%sResponse:\n%s%s\n",
		lib.Blue, lib.ResetColor, h.ID,
		lib.Blue, lib.ResetColor, formatUintPointer(h.WorkspaceID),
		lib.Blue, lib.ResetColor, formatUintPointer(h.TaskID),
		lib.Blue, lib.ResetColor, h.Source,
		lib.Blue, lib.ResetColor, h.URL,
		lib.Blue, lib.ResetColor, h.Method,
		lib.Blue, lib.ResetColor, h.ResponseBodySize,
		lib.Blue, lib.ResetColor, string(h.RawRequest),
		lib.Blue, lib.ResetColor, string(h.RawResponse),
	)
}

// HistoryFilter represents available history filters
type HistoryFilter struct {
	Query                string     `json:"query" validate:"omitempty,ascii"`
	StatusCodes          []int      `json:"status_codes" validate:"omitempty,dive,gte=100,lte=599"`
	Methods              []string   `json:"methods" validate:"omitempty,dive,oneof=GET POST PUT DELETE PATCH HEAD OPTIONS TRACE"`
	ResponseContentTypes []string   `json:"response_content_types" validate:"omitempty,dive,ascii"`
	RequestContentTypes  []string   `json:"request_content_types" validate:"omitempty,dive,ascii"`
	Sources              []string   `json:"sources" validate:"omitempty,dive,ascii"`
	Pagination           Pagination `json:"pagination"`
	WorkspaceID          uint       `json:"workspace_id" validate:"omitempty,numeric"`
	ScanID               uint       `json:"scan_id" validate:"omitempty,numeric"`
	ScanJobID            uint       `json:"scan_job_id" validate:"omitempty,numeric"`
	SortBy               string     `json:"sort_by" validate:"omitempty,oneof=id created_at updated_at status_code request_body_size url response_body_size parameters_count method"` // Validate to be one of the listed fields
	SortOrder            string     `json:"sort_order" validate:"omitempty,oneof=asc desc"`                                                                                           // Validate to be either "asc" or "desc"
	TaskID               uint       `json:"task_id" validate:"omitempty,numeric"`
	IDs                  []uint     `json:"ids" validate:"omitempty,dive,numeric"`
	PlaygroundSessionID  uint       `json:"playground_session_id" validate:"omitempty,numeric"`
	CreatedAfter         *time.Time `json:"created_after,omitempty"`
	CreatedBefore        *time.Time `json:"created_before,omitempty"`
}

// ListHistory Lists history
func (d *DatabaseConnection) ListHistory(filter HistoryFilter) (items []*History, count int64, err error) {
	query := d.db.Model(&History{})

	if filter.Query != "" {
		likeQuery := "%" + filter.Query + "%"
		query = query.Where("url LIKE ? OR note LIKE ?", likeQuery, likeQuery)
	}

	if len(filter.StatusCodes) > 0 {
		query = query.Where("status_code IN ?", filter.StatusCodes)
	}
	if len(filter.Methods) > 0 {
		query = query.Where("method IN ?", filter.Methods)
	}
	if len(filter.Sources) > 0 {
		query = query.Where("source IN ?", filter.Sources)
	}
	if len(filter.ResponseContentTypes) > 0 {
		query = query.Where("response_content_type IN ?", filter.ResponseContentTypes)
	}
	if len(filter.RequestContentTypes) > 0 {
		query = query.Where("request_content_type IN ?", filter.RequestContentTypes)
	}
	if filter.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	}
	if filter.TaskID > 0 {
		query = query.Where("task_id = ?", filter.TaskID)
	}
	if filter.ScanID > 0 {
		query = query.Where("scan_id = ?", filter.ScanID)
	}
	if filter.ScanJobID > 0 {
		query = query.Where("scan_job_id = ?", filter.ScanJobID)
	}
	if len(filter.IDs) > 0 {
		query = query.Where("id IN ?", filter.IDs)
	}
	if filter.PlaygroundSessionID > 0 {
		query = query.Where("playground_session_id = ?", filter.PlaygroundSessionID)
	}

	if filter.CreatedAfter != nil {
		query = query.Where("created_at >= ?", *filter.CreatedAfter)
	}
	if filter.CreatedBefore != nil {
		query = query.Where("created_at <= ?", *filter.CreatedBefore)
	}

	if err := query.Count(&count).Error; err != nil {
		return nil, 0, err
	}

	validSortBy := map[string]bool{
		"id":                 true,
		"created_at":         true,
		"updated_at":         true,
		"status_code":        true,
		"request_body_size":  true,
		"url":                true,
		"response_body_size": true,
		"parameters_count":   true,
		"method":             true,
	}

	validSortOrder := map[string]bool{
		"asc":  true,
		"desc": true,
	}

	order := "id desc"
	if validSort, exists := validSortBy[filter.SortBy]; exists && validSort {
		if validOrder, exists := validSortOrder[filter.SortOrder]; exists && validOrder {
			order = filter.SortBy + " " + filter.SortOrder
		} else {
			order = filter.SortBy + " asc"
		}
	}

	err = query.Scopes(Paginate(&filter.Pagination)).Order(order).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}

	log.Debug().Interface("filters", filter).Int("gathered", len(items)).Int64("count", count).Msg("Getting history items")

	return items, count, err
}

// CreateHistory saves an history item to the database
func (d *DatabaseConnection) CreateHistory(record *History) (*History, error) {

	if record.TaskID != nil && *record.TaskID == 0 {
		record.TaskID = nil
	}
	if record.PlaygroundSessionID != nil && *record.PlaygroundSessionID == 0 {
		record.PlaygroundSessionID = nil
	}
	record.ID = 0
	enhanceHistoryItem(record)
	result := d.db.Create(&record)
	if result.Error != nil {
		if strings.Contains(result.Error.Error(), "invalid byte sequence for encoding \"UTF8\"") || strings.Contains(result.Error.Error(), "SQLSTATE 22021") {
			log.Warn().Str("url", record.URL).Str("method", record.Method).Str("source", record.Source).Msg("UTF-8 encoding error detected, sanitizing and retrying")

			record.URL = lib.SanitizeUTF8(record.URL)
			record.Note = lib.SanitizeUTF8(record.Note)
			record.RequestContentType = lib.SanitizeUTF8(record.RequestContentType)
			record.ResponseContentType = lib.SanitizeUTF8(record.ResponseContentType)
			record.CleanURL = lib.SanitizeUTF8(record.CleanURL)

			result = d.db.Create(&record)
			if result.Error != nil {
				log.Error().Err(result.Error).Str("url", record.URL).Str("method", record.Method).Str("source", record.Source).Msg("Failed to create web history record after UTF-8 sanitization")
			} else {
				log.Info().Str("url", record.URL).Str("method", record.Method).Str("source", record.Source).Msg("Successfully created web history record after UTF-8 sanitization")
			}
		} else {
			log.Error().Err(result.Error).Str("url", record.URL).Str("method", record.Method).Str("source", record.Source).Msg("Failed to create web history record")
		}
	}
	return record, result.Error
}

func (d *DatabaseConnection) UpdateHistory(record *History) (*History, error) {
	enhanceHistoryItem(record)
	result := d.db.Save(&record)
	if result.Error != nil {
		log.Error().Err(result.Error).Uint("history", record.ID).Msg("Failed to update web history record")
	}
	return record, result.Error
}

// GetHistory get a single history record by ID
func (d *DatabaseConnection) GetHistory(id uint) (history History, err error) {
	err = d.db.First(&history, id).Error
	return history, err
}

// GetHistory get a single history record by URL
func (d *DatabaseConnection) GetHistoryFromURL(urlString string) (history History, err error) {
	err = d.db.Where("url = ?", urlString).Order("created_at ASC").First(&history).Error
	return history, err
}

func (d *DatabaseConnection) GetHistoryByID(id uint) (*History, error) {
	var history History
	err := d.db.First(&history, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("record not found")
		}
		return nil, err
	}

	return &history, nil
}

type HistorySummary struct {
	ID              uint   `json:"id"`
	Depth           int    `json:"depth"`
	URL             string `json:"url"`
	StatusCode      int    `json:"status_code"`
	Method          string `json:"method"`
	ParametersCount int    `json:"parameters_count"`
}

func (d *DatabaseConnection) GetChildrenHistories(parent *History) ([]*HistorySummary, error) {
	var children []*HistorySummary

	query := d.db.Model(&History{}).
		Select("MIN(id) as id, url, depth, method, status_code, parameters_count").
		Where("depth >= ? AND depth <= ? AND url LIKE ?", parent.Depth, parent.Depth+1, parent.URL+"%").
		Group("url, depth, method, status_code, parameters_count").
		Order("url desc")

	if parent.WorkspaceID != nil {
		query = query.Where("workspace_id = ?", *parent.WorkspaceID)
	}

	err := query.Scan(&children).Error
	if err != nil {
		return nil, err
	}

	return children, nil
}

func (d *DatabaseConnection) GetRootHistoryNodes(workspaceID uint) ([]*HistorySummary, error) {
	var rootChildren []*HistorySummary
	query := d.db.Model(&History{}).
		Select("MIN(id) as id, url, depth").
		Where("depth = 0 AND url LIKE ?", "%/").
		Group("url, depth").
		Order("url desc")

	if workspaceID != 0 {
		query = query.Where("workspace_id = ?", workspaceID)
	}

	err := query.Scan(&rootChildren).Error
	if err != nil {
		return nil, err
	}

	return rootChildren, nil
}

// GetHistoriesByID retrieves a list of history records by their IDs
func (d *DatabaseConnection) GetHistoriesByID(ids []uint) ([]History, error) {
	var histories []History
	err := d.db.Where("id IN ?", ids).Find(&histories).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("records not found")
		}
		return nil, err
	}

	return histories, nil
}

func (d *DatabaseConnection) GetHistoriesByIDAndWorkspace(ids []uint, workspaceID uint) ([]History, error) {
	var histories []History
	err := d.db.Where("id IN ? AND workspace_id = ?", ids, workspaceID).Find(&histories).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("records not found")
		}
		return nil, err
	}

	return histories, nil
}

// HistoryExists checks if a history record exists
func (d *DatabaseConnection) HistoryExists(id uint) (bool, error) {
	var count int64
	err := d.db.Model(&History{}).Where("id = ?", id).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// HistoryDeletionFilter holds criteria for deleting history items
type HistoryDeletionFilter struct {
	StatusCodes          []int    `json:"status_codes"`
	Methods              []string `json:"methods"`
	ResponseContentTypes []string `json:"response_content_types"`
	RequestContentTypes  []string `json:"request_content_types"`
	Sources              []string `json:"sources"`
	WorkspaceID          uint     `json:"workspace_id"`
}

// DeleteHistory deletes history items based on the provided filter
func (d *DatabaseConnection) DeleteHistory(filter HistoryDeletionFilter) (deletedCount int64, err error) {
	filterQuery := make(map[string]interface{})

	if len(filter.StatusCodes) > 0 {
		filterQuery["status_code"] = filter.StatusCodes
	}
	if len(filter.Methods) > 0 {
		filterQuery["method"] = filter.Methods
	}
	if len(filter.Sources) > 0 {
		filterQuery["source"] = filter.Sources
	}
	if len(filter.ResponseContentTypes) > 0 {
		filterQuery["response_content_type"] = filter.ResponseContentTypes
	}
	if len(filter.RequestContentTypes) > 0 {
		filterQuery["request_content_type"] = filter.RequestContentTypes
	}
	if filter.WorkspaceID > 0 {
		filterQuery["workspace_id"] = filter.WorkspaceID
	}

	// Perform the deletion
	tx := d.db.Where(filterQuery).Delete(&History{})
	deletedCount = tx.RowsAffected

	if tx.Error != nil {
		err = tx.Error
		return 0, err
	}

	log.Info().Interface("filters", filter).Int64("deleted_count", deletedCount).Msg("Deleted history items")

	return deletedCount, nil
}
