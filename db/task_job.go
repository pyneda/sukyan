package db

import (
	"github.com/rs/zerolog/log"
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
	Task        Task          `json:"-" gorm:"foreignKey:TaskID"`
	Status      TaskJobStatus `json:"status"`
	StartedAt   time.Time     `json:"started_at"`
	CompletedAt time.Time     `json:"completed_at"`
	HistoryID   uint          `json:"history_id"`
	History     History       `json:"history" gorm:"foreignKey:HistoryID"`
}

type TaskJobFilter struct {
	Statuses    []string
	Titles      []string
	CompletedAt *time.Time
	Pagination  Pagination
	TaskID      uint
}

func (d *DatabaseConnection) ListTaskJobs(filter TaskJobFilter) (items []*TaskJob, count int64, err error) {
	filterQuery := make(map[string]interface{})

	if len(filter.Statuses) > 0 {
		filterQuery["status"] = filter.Statuses
	}

	if len(filter.Titles) > 0 {
		filterQuery["title"] = filter.Titles
	}

	if filter.TaskID != 0 {
		filterQuery["task_id"] = filter.TaskID
	}

	query := d.db.Preload("History") // Eager load History

	if filterQuery != nil && len(filterQuery) > 0 {
		err = query.Scopes(Paginate(&filter.Pagination)).Where(filterQuery).Order("created_at desc").Find(&items).Error
		d.db.Model(&TaskJob{}).Where(filterQuery).Count(&count)
	} else {
		err = query.Scopes(Paginate(&filter.Pagination)).Order("created_at desc").Find(&items).Error
		d.db.Model(&TaskJob{}).Count(&count)
	}

	log.Info().Interface("filters", filter).Int("gathered", len(items)).Int("count", int(count)).Int("total_results", len(items)).Msg("Getting task job items")

	return items, count, err
}

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
