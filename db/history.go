package db

import (
	"encoding/json"
	"net/http"
	"sukyan/pkg/http_utils"

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
	Method               string
	ContentType          string
	Evaluated            bool
	Note                 string
	// ResponseContentLength int64
	//ResponseTimestamp
	//RequestTimestamp
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

	log.Info().Interface("filters", filter).Int("gathered", len(items)).Int("count", int(count)).Msg("Getting history items")

	return items, count, err
}

// CreateHistory saves an history item to the database
func (d *DatabaseConnection) CreateHistory(record History) (History, error) {
	conditions, attrs := record.getCreateQueryData()

	result := d.db.Where(conditions).Attrs(attrs).FirstOrCreate(&record)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("history", record).Msg("Failed to create web history record")
	}
	return record, result.Error
}

func (d *DatabaseConnection) CreateHistoryFromHttpResponse(response *http.Response, bodyData http_utils.ResponseBodyData) (History, error) {
	requestHeaders, err := json.Marshal(response.Request.Header)
	if err != nil {
		log.Error().Err(err).Msg("Error converting request headers to json")
	}
	responseHeaders, err := json.Marshal(response.Header)
	if err != nil {
		log.Error().Err(err).Msg("Error converting response headers to json")
	}
	// body, bodySize, err := http_utils.ReadResponseBodyData(response)
	// if err != nil {
	// 	log.Error().Err(err).Msg("Error reading response body data")
	// }
	record := History{
		URL:            response.Request.URL.String(),
		StatusCode:     response.StatusCode,
		RequestHeaders: datatypes.JSON(requestHeaders),
		// RequestContentLength int64
		ResponseHeaders:  datatypes.JSON(responseHeaders),
		ResponseBody:     bodyData.Content,
		ResponseBodySize: bodyData.Size,
		Method:           response.Request.Method,
		ContentType:      response.Header.Get("Content-Type"),
		Evaluated:        false,
		// Note                 string
	}
	return d.CreateHistory(record)
}

// Old
// func (d *DatabaseConnection) CreateHistory(record History) (History, error) {
// 	result := d.db.Create(&record)
// 	if result.Error != nil {
// 		log.Error().Err(result.Error).Interface("history", record).Msg("Failed to create web history record")
// 	}
// 	return record, result.Error
// }

// GetHistory get a single history record by ID
func (d *DatabaseConnection) GetHistory(id int) (history History, err error) {
	err = d.db.First(&history, id).Error
	return history, err
}
