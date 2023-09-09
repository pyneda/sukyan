package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"strings"
)

// History holds table for storing requests history found
type History struct {
	// Similar schema: https://github.com/gilcrest/httplog
	BaseModel
	StatusCode           int            `gorm:"index" json:"status_code"`
	URL                  string         `gorm:"index" json:"url"`
	Depth                int            `gorm:"index" json:"depth"`
	RequestHeaders       datatypes.JSON `json:"request_headers" swaggertype:"array`
	RequestBody          []byte         `json:"request_body"`
	RequestBodySize      int            `gorm:"index" json:"request_body_size"`
	RequestContentLength int64          `json:"request_content_length"`
	ResponseHeaders      datatypes.JSON `json:"response_headers" swaggertype:"array"`
	ResponseBody         []byte         `json:"response_body"`
	RequestContentType   string         `gorm:"index" json:"request_content_type"`
	ResponseBodySize     int            `gorm:"index" json:"response_body_size"`
	ResponseContentType  string         `gorm:"index" json:"response_content_type"`
	RawRequest           []byte         `json:"raw_request"`
	RawResponse          []byte         `json:"raw_response"`
	Method               string         `gorm:"index" json:"method"`
	ParametersCount      int            `gorm:"index" json:"parameters_count"`
	Evaluated            bool           `gorm:"index" json:"evaluated"`
	Note                 string         `json:"note"`
	Source               string         `gorm:"index" json:"source"`
	JsonWebTokens        []JsonWebToken `gorm:"many2many:json_web_token_histories" json:"json_web_tokens"`
	Workspace            Workspace      `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WorkspaceID          *uint          `json:"workspace_id" gorm:"index"`
}

func (h *History) GetResponseHeadersAsMap() (map[string][]string, error) {
	intermediateMap := make(map[string]interface{})
	err := json.Unmarshal([]byte(h.ResponseHeaders), &intermediateMap)
	if err != nil {
		return nil, err
	}

	stringMap := make(map[string][]string)
	for key, value := range intermediateMap {
		switch v := value.(type) {
		case []interface{}:
			for _, item := range v {
				switch itemStr := item.(type) {
				case string:
					stringMap[key] = append(stringMap[key], itemStr)
				default:
					log.Warn().Interface("value", itemStr).Msg("value not a string")
				}
			}
		case string:
			stringMap[key] = append(stringMap[key], v)
		default:
			log.Warn().Interface("value", v).Msg("value not a []string")

		}
	}

	return stringMap, nil
}

func (h *History) GetRequestHeadersAsMap() (map[string][]string, error) {
	intermediateMap := make(map[string]interface{})
	err := json.Unmarshal([]byte(h.RequestHeaders), &intermediateMap)
	if err != nil {
		return nil, err
	}

	stringMap := make(map[string][]string)
	for key, value := range intermediateMap {
		switch v := value.(type) {
		case []interface{}:
			for _, item := range v {
				switch itemStr := item.(type) {
				case string:
					stringMap[key] = append(stringMap[key], itemStr)
				default:
					log.Warn().Interface("value", itemStr).Msg("value not a string")
				}
			}
		case string:
			stringMap[key] = append(stringMap[key], v)
		default:
			log.Warn().Interface("value", v).Msg("value not a []string")

		}
	}

	return stringMap, nil
}

func (h *History) GetResponseHeadersAsString() (string, error) {
	headersMap, err := h.GetResponseHeadersAsMap()
	if err != nil {
		log.Error().Err(err).Uint("history", h.ID).Msg("Error getting response headers as map")
		return "", err
	}
	headers := make([]string, 0, len(headersMap))
	for name, values := range headersMap {
		for _, value := range values {
			headers = append(headers, fmt.Sprintf("%s: %s", name, value))
		}
	}

	return strings.Join(headers, "\n"), nil
}

func (h *History) getCreateQueryData() (History, History) {
	conditions := History{
		URL:                 h.URL,
		StatusCode:          h.StatusCode,
		Method:              h.Method,
		ResponseContentType: h.ResponseContentType,
		ResponseBodySize:    h.ResponseBodySize,
	}
	attrs := History{
		RequestHeaders:       h.RequestHeaders,
		RequestContentLength: h.RequestContentLength,
		ResponseHeaders:      h.ResponseHeaders,
		ResponseBody:         h.ResponseBody,
		Evaluated:            h.Evaluated,
		Note:                 h.Note,
	}
	return conditions, attrs
}

// HistoryFilter represents available history filters
type HistoryFilter struct {
	StatusCodes          []int    `json:"status_codes" validate:"omitempty,dive,numeric"`
	Methods              []string `json:"methods" validate:"omitempty,dive,oneof=GET POST PUT DELETE PATCH HEAD OPTIONS TRACE"`
	ResponseContentTypes []string `json:"response_content_types" validate:"omitempty,dive,ascii"`
	RequestContentTypes  []string `json:"request_content_types" validate:"omitempty,dive,ascii"`
	Sources              []string `json:"sources" validate:"omitempty,dive,ascii"`
	Pagination           Pagination
	WorkspaceID          uint   `json:"workspace_id" validate:"omitempty,numeric"`
	SortBy               string `json:"sort_by" validate:"omitempty,oneof=id created_at updated_at status_code request_body_size url response_body_size parameters_count method"` // Validate to be one of the listed fields
	SortOrder            string `json:"sort_order" validate:"omitempty,oneof=asc desc"`                                                                                           // Validate to be either "asc" or "desc"
}

// ListHistory Lists history
func (d *DatabaseConnection) ListHistory(filter HistoryFilter) (items []*History, count int64, err error) {
	//err = d.db.Find(&items).Error
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

	// Perform the query
	if filterQuery != nil && len(filterQuery) > 0 {
		err = d.db.Scopes(Paginate(&filter.Pagination)).Where(filterQuery).Order(order).Find(&items).Error
		d.db.Model(&History{}).Where(filterQuery).Count(&count)
	} else {
		err = d.db.Scopes(Paginate(&filter.Pagination)).Order(order).Find(&items).Error
		d.db.Model(&History{}).Count(&count)
	}

	// Add pagination: https://gorm.io/docs/scopes.html#pagination

	log.Info().Interface("filters", filter).Int("gathered", len(items)).Int("count", int(count)).Int("total_results", len(items)).Msg("Getting history items")

	return items, count, err
}

// CreateHistory saves an history item to the database
func (d *DatabaseConnection) CreateHistory(record *History) (*History, error) {
	// conditions, attrs := record.getCreateQueryData()

	// result := d.db.Where(conditions).Attrs(attrs).FirstOrCreate(&record)
	record.ID = 0
	enhanceHistoryItem(record)
	result := d.db.Create(&record)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("history", record).Msg("Failed to create web history record")
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
	// err = d.db.First(&history, id).Error
	err = d.db.Where("url = ?", urlString).Order("created_at ASC").First(&history).Error
	return history, err
}

func (d *DatabaseConnection) GetHistoryByID(id uint) (*History, error) {
	var history History
	err := d.db.First(&history, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// you can handle 'record not found' error here if you want to
			return nil, errors.New("record not found")
		}
		// other type of error
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
	// Query database for histories that have the same base URL and a depth equal to or greater than the parent
	err := d.db.Model(&History{}).
		Select("MIN(id) as id, url, depth, method, status_code, parameters_count").
		Where("depth >= ? AND depth <= ? AND url LIKE ?", parent.Depth, parent.Depth+1, parent.URL+"%").
		Group("url, depth, method, status_code, parameters_count").
		Scan(&children).Error
	if err != nil {
		return nil, err
	}

	return children, nil
}

func (d *DatabaseConnection) GetRootHistoryNodes() ([]*HistorySummary, error) {
	var rootChildren []*HistorySummary
	err := d.db.Model(&History{}).
		Select("MIN(id) as id, url, depth, method, status_code, parameters_count").
		Where("depth = 0 AND url LIKE ?", "%/").
		Group("url, depth, method, status_code, parameters_count").
		Order("url, parameters_count desc").
		Scan(&rootChildren).Error
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

// HistoryExists checks if a history record exists
func (d *DatabaseConnection) HistoryExists(id uint) (bool, error) {
	var count int64
	err := d.db.Model(&History{}).Where("id = ?", id).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
