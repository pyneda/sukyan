package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
)

type TaskJobStatus string

var (
	TaskJobScheduled TaskJobStatus = "scheduled"
	TaskJobRunning   TaskJobStatus = "running"
	TaskJobFinished  TaskJobStatus = "finished"
	TaskJobFailed    TaskJobStatus = "failed"
)

type TaskJob struct {
	BaseModel
	Title                 string               `json:"title"`
	TaskID                uint                 `json:"task_id"`
	Task                  Task                 `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Status                TaskJobStatus        `gorm:"index" json:"status"`
	StartedAt             time.Time            `json:"started_at"`
	CompletedAt           time.Time            `json:"completed_at"`
	HistoryID             *uint                `json:"history_id"`
	History               History              `json:"history" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WebsocketConnectionID *uint                `json:"websocket_connection_id" gorm:"index;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	WebSocketConnection   *WebSocketConnection `json:"-" gorm:"foreignKey:WebsocketConnectionID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	URL                   string               `json:"url"`
	Method                string               `json:"method"`
	OriginalStatusCode    int                  `json:"original_status_code"`
}

type TaskJobFilter struct {
	Query       string     `json:"query" validate:"omitempty,dive,ascii"`
	Statuses    []string   `json:"statuses" validate:"omitempty,dive,oneof=scheduled running finished failed"`
	Titles      []string   `json:"titles" validate:"omitempty,dive,ascii"`
	Pagination  Pagination `json:"pagination"`
	TaskID      uint       `json:"task_id" validate:"omitempty,numeric"`
	WorkspaceID uint       `json:"workspace_id" validate:"omitempty,numeric"`
	StatusCodes []int      `json:"status_codes" validate:"omitempty,dive,numeric"`
	Methods     []string   `json:"methods" validate:"omitempty,dive,oneof=GET POST PUT DELETE PATCH HEAD OPTIONS TRACE"`
	SortBy      string     `json:"sort_by" validate:"omitempty,oneof=id history_method history_url history_status history_parameters_count title status started_at completed_at created_at updated_at"`
	SortOrder   string     `json:"sort_order" validate:"omitempty,oneof=asc desc"`
}

var TaskJobSortFieldMap = map[string]string{
	"id":                       "id",
	"history_method":           "histories.method",
	"history_url":              "histories.url",
	"history_status":           "histories.status_code",
	"history_parameters_count": "histories.parameters_count",
	"title":                    "title",
	"status":                   "status",
	"started_at":               "started_at",
	"completed_at":             "completed_at",
	"created_at":               "created_at",
	"updated_at":               "updated_at",
}

func (d *DatabaseConnection) ListTaskJobs(filter TaskJobFilter) (items []*TaskJob, count int64, err error) {
	query := d.db.Preload("History") // Eager load History

	// Basic filters
	if len(filter.Statuses) > 0 {
		query = query.Where("task_jobs.status IN ?", filter.Statuses)
	}
	if len(filter.Titles) > 0 {
		query = query.Where("task_jobs.title IN ?", filter.Titles)
	}

	if filter.Query != "" {
		query = query.Where("task_jobs.title LIKE ?", "%"+filter.Query+"%")
	}

	if filter.TaskID > 0 {
		query = query.Where("task_jobs.task_id = ?", filter.TaskID)
	}

	if filter.WorkspaceID > 0 {
		query = query.Joins("JOIN tasks ON tasks.id = task_jobs.task_id").Where("tasks.workspace_id = ?", filter.WorkspaceID)
	}

	// Filters related to History
	if len(filter.StatusCodes) > 0 {
		query = query.Where("histories.status_code IN ?", filter.StatusCodes)
	}
	if len(filter.Methods) > 0 {
		query = query.Where("histories.method IN ?", filter.Methods)
	}

	needsHistoryJoin := len(filter.StatusCodes) > 0 || len(filter.Methods) > 0 || strings.HasPrefix(filter.SortBy, "history_")

	if needsHistoryJoin {
		query = query.Joins("JOIN histories ON histories.id = task_jobs.history_id")
	}

	// Sorting

	if sortField, exists := TaskJobSortFieldMap[filter.SortBy]; exists {
		sortOrder := "asc"
		if filter.SortOrder == "desc" {
			sortOrder = "desc"
		}
		query = query.Order(sortField + " " + sortOrder)
	} else {
		query = query.Order("id desc")
	}

	// Pagination and final query
	err = query.Scopes(Paginate(&filter.Pagination)).Find(&items).Error
	d.db.Model(&TaskJob{}).Count(&count)

	log.Debug().Interface("filters", filter).Int("gathered", len(items)).Int("count", int(count)).Int("total_results", len(items)).Msg("Getting task job items")

	return items, count, err
}

func (d *DatabaseConnection) NewTaskJob(taskID uint, title string, status TaskJobStatus, originalHistory *History) (*TaskJob, error) {
	task := &TaskJob{
		TaskID:             taskID,
		Status:             status,
		Title:              title,
		StartedAt:          time.Now(),
		HistoryID:          &originalHistory.ID,
		URL:                originalHistory.URL,
		Method:             originalHistory.Method,
		OriginalStatusCode: originalHistory.StatusCode,
	}
	return d.CreateTaskJob(task)
}

func (d *DatabaseConnection) NewWebSocketTaskJob(taskID uint, title string, status TaskJobStatus, originalConnection *WebSocketConnection) (*TaskJob, error) {

	method := "GET?"
	if originalConnection.UpgradeRequest.Method != "" {
		method = originalConnection.UpgradeRequest.Method
	}
	task := &TaskJob{
		TaskID:                taskID,
		Status:                status,
		Title:                 title,
		StartedAt:             time.Now(),
		HistoryID:             originalConnection.UpgradeRequestID,
		WebsocketConnectionID: &originalConnection.ID,
		URL:                   originalConnection.URL,
		Method:                method,
		OriginalStatusCode:    originalConnection.StatusCode,
	}
	return d.CreateTaskJob(task)
}

func (d *DatabaseConnection) CreateTaskJob(item *TaskJob) (*TaskJob, error) {
	result := d.db.Create(&item)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("task-job", item).Msg("TaskJob creation failed")
	}
	return item, result.Error
}

func (d *DatabaseConnection) UpdateTaskJob(item *TaskJob) (*TaskJob, error) {
	result := d.db.Model(&TaskJob{}).Where("id = ?", item.ID).Updates(item)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("task-job", item).Msg("TaskJob update failed")
	}
	return item, result.Error
}

func (d *DatabaseConnection) GetTaskJobByID(id uint) (*TaskJob, error) {
	var item TaskJob
	err := d.db.Where("id = ?", id).First(&item).Error
	return &item, err
}

// TaskJobExists checks if a task job exists
func (d *DatabaseConnection) TaskJobExists(id uint) (bool, error) {
	var count int64
	err := d.db.Model(&TaskJob{}).Where("id = ?", id).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (d *DatabaseConnection) DeleteTaskJobs(filter TaskJobFilter) (int64, error) {
	query := d.db.Model(&TaskJob{})

	if len(filter.Statuses) > 0 {
		query = query.Where("task_jobs.status IN ?", filter.Statuses)
	}
	if len(filter.Titles) > 0 {
		query = query.Where("task_jobs.title IN ?", filter.Titles)
	}
	if filter.TaskID > 0 {
		query = query.Where("task_jobs.task_id = ?", filter.TaskID)
	}

	needsHistoryJoin := len(filter.StatusCodes) > 0 || len(filter.Methods) > 0

	if needsHistoryJoin {
		query = query.Joins("JOIN histories ON histories.id = task_jobs.history_id")
		if len(filter.StatusCodes) > 0 {
			query = query.Where("histories.status_code IN ?", filter.StatusCodes)
		}
		if len(filter.Methods) > 0 {
			query = query.Where("histories.method IN ?", filter.Methods)
		}
	}

	result := query.Delete(&TaskJob{})
	if result.Error != nil {
		log.Error().Err(result.Error).Msg("TaskJob deletion failed")
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

func (tj TaskJob) TableHeaders() []string {
	return []string{"ID", "Title", "Status", "Task ID", "URL", "Method", "Status Code", "Started At", "Completed At"}
}

func (tj TaskJob) TableRow() []string {
	formattedURL := tj.URL
	if len(tj.URL) > PrintMaxURLLength {
		formattedURL = tj.URL[0:PrintMaxURLLength] + "..."
	}
	return []string{
		fmt.Sprintf("%d", tj.ID),
		tj.Title,
		string(tj.Status),
		fmt.Sprintf("%d", tj.TaskID),
		formattedURL,
		tj.Method,
		fmt.Sprintf("%d", tj.OriginalStatusCode),
		tj.StartedAt.Format(time.RFC3339),
		tj.CompletedAt.Format(time.RFC3339),
	}
}

func (tj TaskJob) String() string {
	return fmt.Sprintf("ID: %d, Title: %s, Status: %s, Task ID: %d, URL: %s, Method: %s, Status Code: %d, Started At: %s, Completed At: %s",
		tj.ID, tj.Title, tj.Status, tj.TaskID, tj.URL, tj.Method, tj.OriginalStatusCode, tj.StartedAt.Format(time.RFC3339), tj.CompletedAt.Format(time.RFC3339))
}

func (tj TaskJob) Pretty() string {
	return fmt.Sprintf(
		"%sID:%s %d\n%sTitle:%s %s\n%sStatus:%s %s\n%sTask ID:%s %d\n%sURL:%s %s\n%sMethod:%s %s\n%sStatus Code:%s %d\n%sStarted At:%s %s\n%sCompleted At:%s %s\n",
		lib.Blue, lib.ResetColor, tj.ID,
		lib.Blue, lib.ResetColor, tj.Title,
		lib.Blue, lib.ResetColor, tj.Status,
		lib.Blue, lib.ResetColor, tj.TaskID,
		lib.Blue, lib.ResetColor, tj.URL,
		lib.Blue, lib.ResetColor, tj.Method,
		lib.Blue, lib.ResetColor, tj.OriginalStatusCode,
		lib.Blue, lib.ResetColor, tj.StartedAt.Format(time.RFC3339),
		lib.Blue, lib.ResetColor, tj.CompletedAt.Format(time.RFC3339),
	)
}
