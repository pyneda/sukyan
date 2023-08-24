package db

import (
	"github.com/rs/zerolog/log"
)

// Issue holds table for storing issues found
type Issue struct {
	BaseModel
	Code          string      `gorm:"index" json:"code"`
	Title         string      `gorm:"index" json:"title"`
	Description   string      `json:"description"`
	Details       string      `json:"details"`
	Remediation   string      `json:"remediation"`
	Cwe           int         `json:"cwe"`
	URL           string      `gorm:"index" json:"url"`
	StatusCode    int         `gorm:"index" json:"status_code"`
	HTTPMethod    string      `gorm:"index" json:"http_method"`
	Payload       string      `json:"payload"`
	Request       []byte      `json:"request"`
	Response      []byte      `json:"response"`
	FalsePositive bool        `json:"false_positive"`
	Confidence    int         `json:"confidence"`
	References    StringSlice `json:"references"`
	Severity      severity    `gorm:"type:severity;default:'Info'" json:"severity"`
	CURLCommand   string      `json:"curl_command"`
	Note          string      `json:"note"`
	Workspace     Workspace   `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WorkspaceID   *uint       `json:"workspace_id"`
}

// IssueFilter represents available issue filters
type IssueFilter struct {
	Codes       []string
	WorkspaceID uint
}

// ListIssues Lists issues
func (d *DatabaseConnection) ListIssues(filter IssueFilter) (issues []*Issue, count int64, err error) {
	query := d.db

	if len(filter.Codes) > 0 {
		query = query.Where("code IN ?", filter.Codes)
	}

	if filter.WorkspaceID != 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	}

	result := query.Order("severity desc, created_at desc").Find(&issues).Count(&count)
	if result.Error != nil {
		err = result.Error
	}

	return issues, count, err
}

type GroupedIssue struct {
	Title    string `json:"title"`
	Code     string `json:"code"`
	Count    int    `json:"count"`
	Severity string `json:"severity"`
}

// ListIssues Lists issues
func (d *DatabaseConnection) ListIssuesGrouped(filter IssueFilter) (issues []*GroupedIssue, err error) {
	if len(filter.Codes) > 0 {
		err = d.db.Model(&Issue{}).Select("title, severity, code, COUNT(*)").Where("workspace_id = ?", filter.WorkspaceID).Where("code IN ?", filter.Codes).Group("title,severity,code").Find(&issues).Error
	} else {
		err = d.db.Model(&Issue{}).Select("title, severity, code, COUNT(*)").Where("workspace_id = ?", filter.WorkspaceID).Group("title,severity,code").Find(&issues).Error
	}
	return issues, err
}

// CreateIssue saves an issue to the database
func (d *DatabaseConnection) CreateIssue(issue Issue) (Issue, error) {
	// result := d.db.Create(&issue)
	result := d.db.FirstOrCreate(&issue, issue)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("issue", issue).Msg("Failed to create web issue")
	}
	return issue, result.Error
}

// GetIssue get a single issue by ID
func (d *DatabaseConnection) GetIssue(id int) (issue Issue, err error) {
	err = d.db.First(&issue, id).Error
	return issue, err
}
