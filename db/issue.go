package db

import (
	"fmt"
	"sort"

	"github.com/pyneda/sukyan/lib"

	"github.com/rs/zerolog/log"
)

// Issue holds table for storing issues found
type Issue struct {
	BaseModel
	Code          string           `gorm:"index" json:"code"`
	Title         string           `gorm:"index" json:"title"`
	Description   string           `json:"description"`
	Details       string           `json:"details"`
	Remediation   string           `json:"remediation"`
	Cwe           int              `json:"cwe"`
	URL           string           `gorm:"index" json:"url"`
	StatusCode    int              `gorm:"index" json:"status_code"`
	HTTPMethod    string           `gorm:"index" json:"http_method"`
	Payload       string           `json:"payload"`
	Request       []byte           `json:"request"`
	Response      []byte           `json:"response"`
	FalsePositive bool             `gorm:"index" json:"false_positive"`
	Confidence    int              `gorm:"index" json:"confidence"`
	References    StringSlice      `json:"references"`
	Severity      severity         `gorm:"index,type:severity;default:'Info'" json:"severity"`
	CURLCommand   string           `json:"curl_command"`
	Note          string           `json:"note"`
	Workspace     Workspace        `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WorkspaceID   *uint            `json:"workspace_id" gorm:"index"`
	Interactions  []OOBInteraction `json:"interactions" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Requests      []History        `json:"requests" gorm:"many2many:issue_requests;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	TaskID        *uint            `json:"task_id" gorm:"index"`
	Task          Task             `json:"-" gorm:"foreignKey:TaskID"`
	TaskJobID     *uint            `json:"task_job_id" gorm:"index;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	TaskJob       TaskJob          `json:"-" gorm:"foreignKey:TaskJobID"`
}

func (i Issue) String() string {
	return fmt.Sprintf(
		"ID: %d\nCode: %s\nTitle: %s\nCWE: %d\nURL: %s\nStatus Code: %d\nHTTP Method: %s\nPayload: %s\nFalse Positive: %t\nConfidence: %d\nReferences: %v\nSeverity: %s\nCURL Command: %s\nNote: %s\nWorkspace ID: %v\nTask ID: %v\nDescription: %s\nDetails: %s\nRemediation: %s\nRequest: %s\nResponse: %s",
		i.ID, i.Code, i.Title, i.Cwe, i.URL, i.StatusCode, i.HTTPMethod, i.Payload, i.FalsePositive, i.Confidence, i.References, i.Severity, i.CURLCommand, i.Note, *i.WorkspaceID, *i.TaskID, i.Description, i.Details, i.Remediation, string(i.Request), string(i.Response),
	)
}

func (i Issue) Pretty() string {
	return fmt.Sprintf(
		"%sID:%s %d\n%sCode:%s %s\n%sTitle:%s %s\n%sCWE:%s %d\n%sURL:%s %s\n%sStatus Code:%s %d\n%sHTTP Method:%s %s\n%sPayload:%s %s\n%sFalse Positive:%s %t\n%sConfidence:%s %d\n%sReferences:%s %v\n%sSeverity:%s %s\n%sCURL Command:%s %s\n%sNote:%s %s\n%sWorkspace ID:%s %v\n%sTask ID:%s %v\n\n%sDescription:%s %s\n\n%sDetails:%s %s\n\n%sRemediation:%s %s\n\n%sRequest:%s %s\n\n%sResponse:%s %s\n",
		lib.Blue, lib.ResetColor, i.ID,
		lib.Blue, lib.ResetColor, i.Code,
		lib.Blue, lib.ResetColor, i.Title,
		lib.Blue, lib.ResetColor, i.Cwe,
		lib.Blue, lib.ResetColor, i.URL,
		lib.Blue, lib.ResetColor, i.StatusCode,
		lib.Blue, lib.ResetColor, i.HTTPMethod,
		lib.Blue, lib.ResetColor, i.Payload,
		lib.Blue, lib.ResetColor, i.FalsePositive,
		lib.Blue, lib.ResetColor, i.Confidence,
		lib.Blue, lib.ResetColor, i.References,
		lib.Blue, lib.ResetColor, i.Severity,
		lib.Blue, lib.ResetColor, i.CURLCommand,
		lib.Blue, lib.ResetColor, i.Note,
		lib.Blue, lib.ResetColor, *i.WorkspaceID,
		lib.Blue, lib.ResetColor, *i.TaskID,
		lib.Blue, lib.ResetColor, i.Description,
		lib.Blue, lib.ResetColor, i.Details,
		lib.Blue, lib.ResetColor, i.Remediation,
		lib.Blue, lib.ResetColor, string(i.Request),
		lib.Blue, lib.ResetColor, string(i.Response),
	)
}

type GroupedIssue struct {
	Title    string       `json:"title"`
	Code     string       `json:"code"`
	Count    int          `json:"count"`
	Severity string       `json:"severity"`
	Items    []*IssueItem `json:"items"`
}

type IssueItem struct {
	ID         uint   `json:"id"`
	URL        string `json:"url"`
	Confidence int    `json:"confidence"`
}

// AddInteraction adds an interaction to an issue in the database.
func (i Issue) AddInteraction(interaction OOBInteraction) error {
	return Connection.db.Model(&i).Association("Interactions").Append(&interaction)
}

// UpdateFalsePositive updates the FalsePositive attribute of an issue in the database.
func (i Issue) UpdateFalsePositive(value bool) error {
	i.FalsePositive = value
	return Connection.db.Model(&i).Update("false_positive", value).Error
}

func (i Issue) IsEmpty() bool {
	return i.ID == 0
}

// IssueFilter represents available issue filters
type IssueFilter struct {
	Codes       []string
	WorkspaceID uint
	TaskID      uint
	TaskJobID   uint
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

	if filter.TaskID != 0 {
		query = query.Where("task_id = ?", filter.TaskID)
	}

	if filter.TaskJobID != 0 {
		query = query.Where("task_job_id = ?", filter.TaskJobID)
	}

	result := query.Order(severityOrderQuery).Order("title ASC, created_at DESC").Find(&issues).Count(&count)

	if result.Error != nil {
		err = result.Error
	}

	return issues, count, err
}

func (d *DatabaseConnection) ListIssuesGrouped(filter IssueFilter) ([]*GroupedIssue, error) {
	var issues []Issue
	query := d.db.Model(&Issue{}).Select("id, url, confidence, title, code, severity")

	// Apply filters
	if len(filter.Codes) > 0 {
		query = query.Where("code IN ?", filter.Codes)
	}
	if filter.WorkspaceID != 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	}
	if filter.TaskID != 0 {
		query = query.Where("task_id = ?", filter.TaskID)
	}
	if filter.TaskJobID != 0 {
		query = query.Where("task_job_id = ?", filter.TaskJobID)
	}

	// Execute the query
	err := query.Find(&issues).Error
	if err != nil {
		return nil, err
	}

	// Post-process to build grouped structure
	issueMap := make(map[string]*GroupedIssue)

	for _, issue := range issues {
		// Create a composite key from Code, Severity, and Title
		key := issue.Code + "|" + issue.Severity.String() + "|" + issue.Title

		grouped, exists := issueMap[key]
		if !exists {
			grouped = &GroupedIssue{
				Title:    issue.Title,
				Code:     issue.Code,
				Severity: issue.Severity.String(),
				Items:    []*IssueItem{},
			}
			issueMap[key] = grouped
		}

		item := &IssueItem{
			ID:         issue.ID,
			URL:        issue.URL,
			Confidence: issue.Confidence,
		}
		grouped.Items = append(grouped.Items, item)
		grouped.Count = len(grouped.Items) // Update the count
	}

	var groupedIssues []*GroupedIssue
	for _, v := range issueMap {
		groupedIssues = append(groupedIssues, v)
	}

	sort.Slice(groupedIssues, func(i, j int) bool {
		return GetSeverityOrder(groupedIssues[i].Severity) < GetSeverityOrder(groupedIssues[j].Severity)
	})
	return groupedIssues, nil
}

// ListIssuesGrouped Lists grouped issues
// func (d *DatabaseConnection) ListIssuesGrouped(filter IssueFilter) (issues []*GroupedIssue, err error) {
// 	query := d.db.Model(&Issue{}).Select("title, severity, code, COUNT(*)").Group("title,severity,code")

// 	query = query.Order(severityOrderQuery).Order("title ASC")

// 	if len(filter.Codes) > 0 {
// 		query = query.Where("code IN ?", filter.Codes)
// 	}

// 	if filter.WorkspaceID != 0 {
// 		query = query.Where("workspace_id = ?", filter.WorkspaceID)
// 	}

// 	if filter.TaskID != 0 {
// 		query = query.Where("task_id = ?", filter.TaskID)
// 	}

// 	if filter.TaskJobID != 0 {
// 		query = query.Where("task_job_id = ?", filter.TaskJobID)
// 	}

// 	err = query.Find(&issues).Error
// 	return issues, err
// }

// CreateIssue saves an issue to the database
func (d *DatabaseConnection) CreateIssue(issue Issue) (Issue, error) {
	// result := d.db.Create(&issue)

	if issue.TaskID != nil && *issue.TaskID == 0 {
		issue.TaskID = nil
	}
	if issue.TaskJobID != nil && *issue.TaskJobID == 0 {
		issue.TaskJobID = nil
	}

	result := d.db.FirstOrCreate(&issue, issue)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("issue", issue).Msg("Failed to create web issue")
	}
	return issue, result.Error
}

// GetIssue get a single issue by ID
func (d *DatabaseConnection) GetIssue(id int, includeRelated bool) (issue Issue, err error) {
	query := d.db

	if includeRelated {
		query = query.Preload("Interactions").Preload("Requests")
	}

	err = query.First(&issue, id).Error
	return issue, err
}
