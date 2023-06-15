package db

import (
	"encoding/json"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// History holds table for storing requests history found
type History struct {
	// Similar schema: https://github.com/gilcrest/httplog
	gorm.Model
	StatusCode           int
	URL                  string
	RequestHeaders       datatypes.JSON
	RequestContentLength int64
	ResponseHeaders      datatypes.JSON
	ResponseBody         string
	ResponseBodySize     int
	RawRequest           string `json:"raw_request"`
	RawResponse          string `json:"raw_response"`
	Method               string
	ContentType          string
	Evaluated            bool
	Note                 string
	Source               string
	// ResponseContentLength int64
	//ResponseTimestamp
	//RequestTimestamp
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
		default:
			log.Warn().Interface("value", v).Msg("value not a []string")

		}
	}

	return stringMap, nil
}

func (h *History) getCreateQueryData() (History, History) {
	conditions := History{
		URL:              h.URL,
		StatusCode:       h.StatusCode,
		Method:           h.Method,
		ContentType:      h.ContentType,
		ResponseBodySize: h.ResponseBodySize,
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
	StatusCodes  []int
	Methods      []string
	ContentTypes []string
	Sources      []string
	Pagination   Pagination
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

	if len(filter.ContentTypes) > 0 {
		filterQuery["content_type"] = filter.ContentTypes
	}
	// Perform the query
	if filterQuery != nil && len(filterQuery) > 0 {
		// err = d.db.Scopes(Paginate(&filter.Pagination)).Where(filterQuery).Find(&items).Count(&count).Error
		err = d.db.Scopes(Paginate(&filter.Pagination)).Where(filterQuery).Order("created_at desc").Find(&items).Error
		d.db.Model(&History{}).Where(filterQuery).Count(&count)

	} else {
		// err = d.db.Scopes(Paginate(&filter.Pagination)).Find(&items).Count(&count).Error
		err = d.db.Scopes(Paginate(&filter.Pagination)).Order("created_at desc").Find(&items).Error
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
