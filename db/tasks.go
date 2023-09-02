package db

import (
	"github.com/rs/zerolog/log"
	"time"
)

type Task struct {
	BaseModel
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
	Workspace   Workspace `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WorkspaceID uint      `json:"workspace_id"`
}

type TaskFilter struct {
	Statuses    []string
	Pagination  Pagination
	WorkspaceID uint
}

var (
	TaskStatusCrawling        string = "crawling"
	TaskStatusScanning        string = "scanning"
	TaskStatusNuclei          string = "nuclei"
	TaskStatusRunning         string = "running"
	TaskStatusFinished        string = "finished"
	TaskStatusFailed          string = "failed"
	TaskStatusPaused          string = "paused"
	DefaultWorkspaceTaskTitle string = "Default task"
)

func (d *DatabaseConnection) NewTask(workspaceID uint, title, status string) (*Task, error) {
	task := &Task{
		WorkspaceID: workspaceID,
		Status:      status,
		StartedAt:   time.Now(),
		Title:       title,
	}
	return d.CreateTask(task)
}

func (d *DatabaseConnection) SetTaskStatus(id uint, status string) error {
	task, err := d.GetTaskByID(id)
	if err != nil {
		return err
	}
	task.Status = status
	task.FinishedAt = time.Now()
	_, err = d.UpdateTask(id, task)
	return err
}

func (d *DatabaseConnection) CreateTask(task *Task) (*Task, error) {
	result := d.db.Create(&task)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("task", task).Msg("Task creation failed")
	}
	return task, result.Error
}

func (d *DatabaseConnection) ListTasks(filter TaskFilter) (items []*Task, count int64, err error) {
	filterQuery := make(map[string]interface{})

	if len(filter.Statuses) > 0 {
		filterQuery["status"] = filter.Statuses
	}

	if filter.WorkspaceID > 0 {
		filterQuery["workspace_id"] = filter.WorkspaceID
	}

	if filterQuery != nil && len(filterQuery) > 0 {
		err = d.db.Scopes(Paginate(&filter.Pagination)).Where(filterQuery).Order("created_at desc").Find(&items).Error
		d.db.Model(&Task{}).Where(filterQuery).Count(&count)

	} else {
		err = d.db.Scopes(Paginate(&filter.Pagination)).Order("created_at desc").Find(&items).Error
		d.db.Model(&Task{}).Count(&count)
	}

	log.Info().Interface("filters", filter).Int("gathered", len(items)).Int("count", int(count)).Int("total_results", len(items)).Msg("Getting task items")

	return items, count, err
}

func (d *DatabaseConnection) UpdateTask(id uint, task *Task) (*Task, error) {
	result := d.db.Model(&Task{}).Where("id = ?", id).Updates(task)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("task", task).Msg("Task update failed")
	}
	return task, result.Error
}

func (d *DatabaseConnection) GetTaskByID(id uint) (*Task, error) {
	var task Task
	if err := d.db.Where("id = ?", id).First(&task).Error; err != nil {
		log.Error().Err(err).Interface("id", id).Msg("Unable to fetch task by ID")
		return nil, err
	}
	return &task, nil
}

func (d *DatabaseConnection) DeleteTask(id uint) error {
	if err := d.db.Delete(&Task{}, id).Error; err != nil {
		log.Error().Err(err).Interface("id", id).Msg("Error deleting task")
		return err
	}
	return nil
}

// TaskExists checks if a workspace exists
func (d *DatabaseConnection) TaskExists(id uint) (bool, error) {
	var count int64
	err := d.db.Model(&Task{}).Where("id = ?", id).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (d *DatabaseConnection) GetOrCreateDefaultWorkspaceTask(workspaceID uint) (*Task, error) {
	task := &Task{
		WorkspaceID: workspaceID,
		Title:       DefaultWorkspaceTaskTitle,
		Status:      TaskStatusScanning,
		StartedAt:   time.Now(),
	}
	result := d.db.Model(&Task{}).Where("workspace_id = ? AND title = ?", workspaceID, DefaultWorkspaceTaskTitle).FirstOrCreate(task)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("workspace_id", workspaceID).Msg("Unable to create default workspace task")
	}
	return task, result.Error
}
