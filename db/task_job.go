package db

import (
	"github.com/rs/zerolog/log"
	"strings"
	"time"
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
	Title       string        `json:"title"`
	TaskID      uint          `json:"task_id"`
	Task        Task          `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Status      TaskJobStatus `gorm:"index" json:"status"`
	StartedAt   time.Time     `json:"started_at"`
	CompletedAt time.Time     `json:"completed_at"`
	HistoryID   uint          `json:"history_id"`
	History     History       `json:"history" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

type TaskJobFilter struct {
	Statuses    []string   `json:"statuses" validate:"omitempty,dive,oneof=scheduled running finished failed"`
	Titles      []string   `json:"titles" validate:"omitempty,dive,ascii"`
	Pagination  Pagination `json:"pagination"`
	TaskID      uint       `json:"task_id" validate:"omitempty,numeric"`
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
		query = query.Where("status IN ?", filter.Statuses)
	}
	if len(filter.Titles) > 0 {
		query = query.Where("title IN ?", filter.Titles)
	}
	if filter.TaskID != 0 {
		query = query.Where("task_id = ?", filter.TaskID)
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

	log.Info().Interface("filters", filter).Int("gathered", len(items)).Int("count", int(count)).Int("total_results", len(items)).Msg("Getting task job items")

	return items, count, err
}

// func (d *DatabaseConnection) ListTaskJobs(filter TaskJobFilter) (items []*TaskJob, count int64, err error) {
// 	filterQuery := make(map[string]interface{})

// 	if len(filter.Statuses) > 0 {
// 		filterQuery["status"] = filter.Statuses
// 	}

// 	if len(filter.Titles) > 0 {
// 		filterQuery["title"] = filter.Titles
// 	}

// 	if filter.TaskID != 0 {
// 		filterQuery["task_id"] = filter.TaskID
// 	}

// 	query := d.db.Preload("History") // Eager load History

// 	if filterQuery != nil && len(filterQuery) > 0 {
// 		err = query.Scopes(Paginate(&filter.Pagination)).Where(filterQuery).Order("created_at desc").Find(&items).Error
// 		d.db.Model(&TaskJob{}).Where(filterQuery).Count(&count)
// 	} else {
// 		err = query.Scopes(Paginate(&filter.Pagination)).Order("created_at desc").Find(&items).Error
// 		d.db.Model(&TaskJob{}).Count(&count)
// 	}

// 	log.Info().Interface("filters", filter).Int("gathered", len(items)).Int("count", int(count)).Int("total_results", len(items)).Msg("Getting task job items")

// 	return items, count, err
// }

func (d *DatabaseConnection) NewTaskJob(taskID uint, title string, status TaskJobStatus, historyID uint) (*TaskJob, error) {
	task := &TaskJob{
		TaskID:    taskID,
		Status:    status,
		Title:     title,
		StartedAt: time.Now(),
		HistoryID: historyID,
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
