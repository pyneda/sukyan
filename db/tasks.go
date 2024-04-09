package db

import (
	"fmt"
	"time"

	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
)

type Task struct {
	BaseModel
	Title               string            `json:"title"`
	Status              string            `gorm:"index" json:"status"`
	StartedAt           time.Time         `json:"started_at"`
	FinishedAt          time.Time         `json:"finished_at"`
	Workspace           Workspace         `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WorkspaceID         uint              `json:"workspace_id" gorm:"index" `
	Histories           []History         `gorm:"foreignKey:TaskID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	Issues              []Issue           `gorm:"foreignKey:TaskID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	Stats               TaskStats         `gorm:"-" json:"stats,omitempty"`
	PlaygroundSessionID *uint             `gorm:"index" json:"playground_session_id"`
	PlaygroundSession   PlaygroundSession `json:"-" gorm:"foreignKey:PlaygroundSessionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (t Task) TableHeaders() []string {
	return []string{"ID", "Title", "Status", "StartedAt", "FinishedAt", "WorkspaceID"}
}

func (t Task) TableRow() []string {
	return []string{
		fmt.Sprintf("%d", t.ID),
		t.Title,
		t.Status,
		t.StartedAt.Format(time.RFC3339),
		t.FinishedAt.Format(time.RFC3339),
		fmt.Sprintf("%d", t.WorkspaceID),
	}
}

// String provides a basic textual representation of the Task.
func (t Task) String() string {
	return fmt.Sprintf("ID: %d, Title: %s, Status: %s, StartedAt: %s, FinishedAt: %s, WorkspaceID: %d",
		t.ID, t.Title, t.Status, t.StartedAt.Format(time.RFC3339), t.FinishedAt.Format(time.RFC3339), t.WorkspaceID)
}

// Pretty provides a more formatted, user-friendly representation of the Task.
func (t Task) Pretty() string {
	return fmt.Sprintf(
		"%sID:%s %d\n%sTitle:%s %s\n%sStatus:%s %s\n%sStartedAt:%s %s\n%sFinishedAt:%s %s\n%sWorkspaceID:%s %d\n%sStats:%s\n  Requests: Crawler: %d, Scanner: %d\n  Issues: Unknown: %d, Info: %d, Low: %d, Medium: %d, High: %d, Critical: %d\n",
		lib.Blue, lib.ResetColor, t.ID,
		lib.Blue, lib.ResetColor, t.Title,
		lib.Blue, lib.ResetColor, t.Status,
		lib.Blue, lib.ResetColor, t.StartedAt.Format(time.RFC3339),
		lib.Blue, lib.ResetColor, t.FinishedAt.Format(time.RFC3339),
		lib.Blue, lib.ResetColor, t.WorkspaceID,
		lib.Blue, lib.ResetColor,
		t.Stats.Requests.Crawler, t.Stats.Requests.Scanner,
		t.Stats.Issues.Unknown, t.Stats.Issues.Info, t.Stats.Issues.Low, t.Stats.Issues.Medium, t.Stats.Issues.High, t.Stats.Issues.Critical)
}

type TaskFilter struct {
	Query               string     `json:"query" validate:"omitempty,dive,ascii"`
	Statuses            []string   `json:"statuses" validate:"omitempty,dive,oneof=crawling scanning nuclei running finished failed paused"`
	Pagination          Pagination `json:"pagination"`
	WorkspaceID         uint       `json:"workspace_id" validate:"omitempty,numeric"`
	FetchStats          bool       `json:"fetch_stats"`
	PlaygroundSessionID uint       `json:"playground_session_id"`
}

var (
	TaskStatusPending         string = "pending"
	TaskStatusCrawling        string = "crawling"
	TaskStatusScanning        string = "scanning"
	TaskStatusNuclei          string = "nuclei"
	TaskStatusRunning         string = "running"
	TaskStatusFinished        string = "finished"
	TaskStatusFailed          string = "failed"
	TaskStatusPaused          string = "paused"
	DefaultWorkspaceTaskTitle string = "Default task"
)

type TaskStats struct {
	Requests RequestsStats `json:"requests"`
	Issues   IssuesStats   `json:"issues"`
}

type RequestsStats struct {
	Crawler int64 `json:"crawler"`
	Scanner int64 `json:"scanner"`
}

type IssuesStats struct {
	Unknown  int64 `json:"unknown"`
	Info     int64 `json:"info"`
	Low      int64 `json:"low"`
	Medium   int64 `json:"medium"`
	High     int64 `json:"high"`
	Critical int64 `json:"critical"`
}

func (d *DatabaseConnection) NewTask(workspaceID uint, playgroundSessionID *uint, title, status string) (*Task, error) {
	task := &Task{
		WorkspaceID:         workspaceID,
		Status:              status,
		StartedAt:           time.Now(),
		Title:               title,
		PlaygroundSessionID: playgroundSessionID,
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

	if filter.PlaygroundSessionID > 0 {
		filterQuery["playground_session_id"] = filter.PlaygroundSessionID
	}

	query := d.db.Scopes(Paginate(&filter.Pagination)).Order("created_at desc")
	if len(filterQuery) > 0 {
		query = query.Where(filterQuery)
	}

	if filter.Query != "" {
		likeQuery := "%" + filter.Query + "%"
		query = query.Where("title LIKE ?", likeQuery)
	}

	err = query.Find(&items).Error
	if err != nil {
		return nil, 0, err
	}

	if len(filterQuery) > 0 {
		d.db.Model(&Task{}).Where(filterQuery).Count(&count)
	} else {
		d.db.Model(&Task{}).Count(&count)
	}

	if filter.FetchStats {
		for _, task := range items {
			historyCounts := map[string]int64{}
			rows, _ := d.db.Model(&History{}).Select("source, COUNT(*) as count").Where("task_id = ?", task.ID).Group("source").Rows()
			for rows.Next() {
				var source string
				var count int64
				rows.Scan(&source, &count)
				historyCounts[source] = count
			}
			rows.Close()

			issueCounts := map[severity]int64{}
			rows, _ = d.db.Model(&Issue{}).Select("severity, COUNT(*) as count").Where("task_id = ?", task.ID).Group("severity").Rows()

			for rows.Next() {
				var sev severity
				var count int64
				rows.Scan(&sev, &count)
				issueCounts[sev] = count
			}
			rows.Close()

			task.Stats = TaskStats{
				Requests: RequestsStats{
					Crawler: historyCounts["Crawler"],
					Scanner: historyCounts["Scanner"],
				},
				Issues: IssuesStats{
					Unknown:  issueCounts[Unknown],
					Info:     issueCounts[Info],
					Low:      issueCounts[Low],
					Medium:   issueCounts[Medium],
					High:     issueCounts[High],
					Critical: issueCounts[Critical],
				},
			}
		}
	}

	return items, count, nil
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
