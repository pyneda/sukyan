package db

import (
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Issue holds table for storing issues found
type Issue struct {
	gorm.Model
	Code           string `gorm:"index" json:"code"`
	Title          string `gorm:"index"`
	Description    string
	Details				 string
	Remediation    string
	Cwe            int
	URL            string `gorm:"index"`
	StatusCode     int
	HTTPMethod     string
	Payload        string
	Request        string
	Response       string
	AdditionalInfo datatypes.JSON
	FalsePositive  bool
	Confidence     int
	// enums seem to fail - review later
	// Severity string `json:"severity" gorm:"type:ENUM('Info', 'Low', 'Medium', 'High', 'Critical');default:'Info'"`
	Severity string `json:"severity" gorm:"index; default:'Unknown'"`
	Note     string
}

// IssueFilter represents available issue filters
type IssueFilter struct {
	Codes []string
}

// ListIssues Lists issues
func (d *DatabaseConnection) ListIssues(filter IssueFilter) (issues []*Issue, count int64, err error) {
	if len(filter.Codes) > 0 {
		result := d.db.Where("code IN ?", filter.Codes).Order("created_at desc").Find(&issues).Count(&count)
		if result.Error != nil {
			err = result.Error
		}
	} else {
		result := d.db.Order("created_at desc").Find(&issues).Count(&count)
		if result.Error != nil {
			err = result.Error
		}
	}
	return issues, count, err
}

// ListIssues Lists issues
func (d *DatabaseConnection) ListIssuesGrouped(filter IssueFilter) (issues []*Issue, err error) {
	if len(filter.Codes) > 0 {
		err = d.db.Select("title,code,url").Where("code IN ?", filter.Codes).Group("title").Find(&issues).Error
	} else {
		err = d.db.Select("title,code,url").Group("title").Find(&issues).Error
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
