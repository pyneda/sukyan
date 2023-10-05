package db

import (
	"github.com/rs/zerolog/log"
	"time"
)

type Task struct {
	BaseModel
	Title       string    `json:"title"`
	Status      string    `gorm:"index" json:"status"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
	Workspace   Workspace `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WorkspaceID uint      `json:"workspace_id"`
	Histories   []History `gorm:"foreignKey:TaskID" json:"-"`
	Issues      []Issue   `gorm:"foreignKey:TaskID" json:"-"`
	Stats       TaskStats `gorm:"-" json:"stats,omitempty"`
}

type TaskFilter struct {
	Statuses    []string
	Pagination  Pagination
	WorkspaceID uint
	FetchStats  bool
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

	query := d.db.Debug().Scopes(Paginate(&filter.Pagination)).Order("created_at desc")
	if len(filterQuery) > 0 {
		query = query.Where(filterQuery)
	}

	err = query.Find(&items).Error
	if err != nil {
		return nil, 0, err
	}

	if len(filterQuery) > 0 {
		d.db.Debug().Model(&Task{}).Where(filterQuery).Count(&count)
	} else {
		d.db.Debug().Model(&Task{}).Count(&count)
	}

	if filter.FetchStats {
		for _, task := range items {
			var crawlerCount, scannerCount int64
			d.db.Debug().Model(&History{}).Where("task_id = ? AND source = ?", task.ID, "Crawler").Count(&crawlerCount)
			d.db.Debug().Model(&History{}).Where("task_id = ? AND source = ?", task.ID, "Scanner").Count(&scannerCount)

			var issueCounts map[severity]int64 = make(map[severity]int64)
			for _, sev := range []severity{Unknown, Info, Low, Medium, High, Critical} {
				var count int64
				d.db.Debug().Model(&Issue{}).Where("task_id = ? AND severity = ?", task.ID, sev).Count(&count)
				issueCounts[sev] = count
			}

			task.Stats = TaskStats{
				Requests: RequestsStats{
					Crawler: crawlerCount,
					Scanner: scannerCount,
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

func calculateStatsForTask(task *Task) TaskStats {
	crawlerCount := countHistoriesBySource(task.Histories, "Crawler")
	scannerCount := countHistoriesBySource(task.Histories, "Scanner")

	issueCounts := make(map[severity]int64)
	for _, sev := range []severity{Unknown, Info, Low, Medium, High, Critical} {
		issueCounts[sev] = countIssuesBySeverity(task.Issues, sev)
	}

	return TaskStats{
		Requests: RequestsStats{
			Crawler: crawlerCount,
			Scanner: scannerCount,
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

func countHistoriesBySource(histories []History, source string) int64 {
	count := 0
	for _, history := range histories {
		if history.Source == source {
			count++
		}
	}
	return int64(count)
}

func countIssuesBySeverity(issues []Issue, severityType severity) int64 {
	count := 0
	for _, issue := range issues {
		if issue.Severity == severityType {
			count++
		}
	}
	return int64(count)
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
