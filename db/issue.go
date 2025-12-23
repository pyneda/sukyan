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
	FalsePositive bool        `gorm:"index" json:"false_positive"`
	Confidence    int         `gorm:"index" json:"confidence"`
	References    StringSlice `json:"references"`
	Severity      severity    `gorm:"index,type:severity;default:'Info'" json:"severity"`
	CURLCommand   string      `json:"curl_command"`
	Note          string      `json:"note"`
	POC           string      `gorm:"column:poc" json:"poc"`
	POCType       string      `gorm:"column:poc_type" json:"poc_type"`
	Workspace     Workspace   `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WorkspaceID   *uint       `json:"workspace_id" gorm:"index"`
	// OriginalHistory   History          `json:"original_history" gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	// OriginalHistoryID *uint            `json:"original_history_id" gorm:"index"`
	Interactions          []OOBInteraction     `json:"interactions" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Requests              []History            `json:"requests" gorm:"many2many:issue_requests;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	TaskID                *uint                `json:"task_id" gorm:"index"`
	Task                  Task                 `json:"-" gorm:"foreignKey:TaskID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	TaskJobID             *uint                `json:"task_job_id" gorm:"index;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	TaskJob               TaskJob              `json:"-" gorm:"foreignKey:TaskJobID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	ScanID                *uint                `json:"scan_id" gorm:"index"`
	Scan                  *Scan                `json:"-" gorm:"foreignKey:ScanID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	ScanJobID             *uint                `json:"scan_job_id" gorm:"index"`
	ScanJob               *ScanJob             `json:"-" gorm:"foreignKey:ScanJobID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	WebsocketConnectionID *uint                `json:"websocket_connection_id" gorm:"index;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	WebSocketConnection   *WebSocketConnection `json:"-" gorm:"foreignKey:WebsocketConnectionID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
}

func (i Issue) TableHeaders() []string {
	return []string{"ID", "Title", "Code", "Severity", "Confidence", "False Positive", "URL", "Status Code", "HTTP Method", "Description"}
}

func (i Issue) TableRow() []string {
	formattedURL := i.URL
	if len(i.URL) > PrintMaxURLLength {
		formattedURL = i.URL[0:PrintMaxURLLength] + "..."
	}
	formattedDescription := i.Description
	if len(i.Description) > PrintMaxDescriptionLength {
		formattedDescription = i.Description[0:PrintMaxDescriptionLength] + "..."
	}
	return []string{
		fmt.Sprintf("%d", i.ID),
		i.Title,
		i.Code,
		i.Severity.String(),
		fmt.Sprintf("%d", i.Confidence),
		fmt.Sprintf("%t", i.FalsePositive),
		formattedURL,
		fmt.Sprintf("%d", i.StatusCode),
		i.HTTPMethod,
		formattedDescription,
	}
}

func (i Issue) String() string {
	workspaceID := "<nil>"
	if i.WorkspaceID != nil {
		workspaceID = fmt.Sprintf("%d", *i.WorkspaceID)
	}
	scanID := "<nil>"
	if i.ScanID != nil {
		scanID = fmt.Sprintf("%d", *i.ScanID)
	}

	return fmt.Sprintf(
		"ID: %d\nCode: %s\nTitle: %s\nCWE: %d\nURL: %s\nStatus Code: %d\nHTTP Method: %s\nPayload: %s\nFalse Positive: %t\nConfidence: %d\nReferences: %v\nSeverity: %s\nCURL Command: %s\nNote: %s\nPOC Type: %s\nWorkspace ID: %s\nScan ID: %s\nDescription: %s\nDetails: %s\nRemediation: %s\nPOC: %s\nRequest: %s\nResponse: %s",
		i.ID, i.Code, i.Title, i.Cwe, i.URL, i.StatusCode, i.HTTPMethod, i.Payload, i.FalsePositive, i.Confidence, i.References, i.Severity, i.CURLCommand, i.Note, i.POCType, workspaceID, scanID, i.Description, i.Details, i.Remediation, i.POC, string(i.Request), string(i.Response),
	)
}

func (i Issue) Pretty() string {
	workspaceID := "<nil>"
	if i.WorkspaceID != nil {
		workspaceID = fmt.Sprintf("%d", *i.WorkspaceID)
	}
	scanID := "<nil>"
	if i.ScanID != nil {
		scanID = fmt.Sprintf("%d", *i.ScanID)
	}

	return fmt.Sprintf(
		"%sID:%s %d\n%sCode:%s %s\n%sTitle:%s %s\n%sCWE:%s %d\n%sURL:%s %s\n%sStatus Code:%s %d\n%sHTTP Method:%s %s\n%sPayload:%s %s\n%sFalse Positive:%s %t\n%sConfidence:%s %d\n%sReferences:%s %v\n%sSeverity:%s %s\n%sCURL Command:%s %s\n%sNote:%s %s\n%sPOC Type:%s %s\n%sWorkspace ID:%s %s\n%sScan ID:%s %s\n\n%sDescription:%s %s\n\n%sDetails:%s %s\n\n%sRemediation:%s %s\n\n%sPOC:%s %s\n\n%sRequest:%s %s\n\n%sResponse:%s %s\n",
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
		lib.Blue, lib.ResetColor, i.POCType,
		lib.Blue, lib.ResetColor, workspaceID,
		lib.Blue, lib.ResetColor, scanID,
		lib.Blue, lib.ResetColor, i.Description,
		lib.Blue, lib.ResetColor, i.Details,
		lib.Blue, lib.ResetColor, i.Remediation,
		lib.Blue, lib.ResetColor, i.POC,
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
	ID            uint   `json:"id"`
	URL           string `json:"url"`
	Confidence    int    `json:"confidence"`
	FalsePositive bool   `json:"false_positive"`
}

// AddInteraction adds an interaction to an issue in the database.
func (i Issue) AddInteraction(interaction OOBInteraction) error {
	return Connection().DB().Model(&i).Association("Interactions").Append(&interaction)
}

// UpdateFalsePositive updates the FalsePositive attribute of an issue in the database.
func (i Issue) UpdateFalsePositive(value bool) error {
	i.FalsePositive = value
	return Connection().DB().Model(&i).Update("false_positive", value).Error
}

func (i Issue) IsEmpty() bool {
	return i.ID == 0
}

// IssueFilter represents available issue filters
type IssueFilter struct {
	Codes         []string
	WorkspaceID   uint
	TaskID        uint
	TaskJobID     uint
	ScanID        uint
	ScanJobID     uint
	URL           string
	MinConfidence int
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

	if filter.URL != "" {
		query = query.Where("url = ?", filter.URL)
	}
	if filter.TaskID != 0 {
		query = query.Where("task_id = ?", filter.TaskID)
	}

	if filter.TaskJobID != 0 {
		query = query.Where("task_job_id = ?", filter.TaskJobID)
	}

	if filter.ScanID != 0 {
		query = query.Where("scan_id = ?", filter.ScanID)
	}

	if filter.ScanJobID != 0 {
		query = query.Where("scan_job_id = ?", filter.ScanJobID)
	}

	if filter.MinConfidence > 0 {
		query = query.Where("confidence >= ?", filter.MinConfidence)
	}

	result := query.Order(severityOrderQuery).Order("title ASC, created_at DESC").Find(&issues).Count(&count)

	if result.Error != nil {
		err = result.Error
	}

	return issues, count, err
}

func (d *DatabaseConnection) ListIssuesGrouped(filter IssueFilter) ([]*GroupedIssue, error) {
	var issues []Issue
	query := d.db.Model(&Issue{}).Select("id, url, confidence, title, code, severity, false_positive")

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
	if filter.ScanID != 0 {
		query = query.Where("scan_id = ?", filter.ScanID)
	}

	if filter.ScanJobID != 0 {
		query = query.Where("scan_job_id = ?", filter.ScanJobID)
	}

	if filter.MinConfidence > 0 {
		query = query.Where("confidence >= ?", filter.MinConfidence)
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
			ID:            issue.ID,
			URL:           issue.URL,
			Confidence:    issue.Confidence,
			FalsePositive: issue.FalsePositive,
		}
		grouped.Items = append(grouped.Items, item)
		grouped.Count = len(grouped.Items) // Update the count
	}

	var groupedIssues []*GroupedIssue
	for _, v := range issueMap {
		groupedIssues = append(groupedIssues, v)
	}

	sort.Slice(groupedIssues, func(i, j int) bool {
		severityOrderI := GetSeverityOrder(groupedIssues[i].Severity)
		severityOrderJ := GetSeverityOrder(groupedIssues[j].Severity)

		if severityOrderI != severityOrderJ {
			// Sort by severity first
			return severityOrderI < severityOrderJ
		}

		return groupedIssues[i].Title < groupedIssues[j].Title
	})
	return groupedIssues, nil
}

func (d *DatabaseConnection) ListUniqueIssueCodes(filter IssueFilter) ([]string, error) {
	var codes []string
	query := d.db.Model(&Issue{}).Distinct("code")

	if filter.WorkspaceID != 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	}
	if filter.TaskID != 0 {
		query = query.Where("task_id = ?", filter.TaskID)
	}
	if filter.TaskJobID != 0 {
		query = query.Where("task_job_id = ?", filter.TaskJobID)
	}
	if filter.MinConfidence > 0 {
		query = query.Where("confidence >= ?", filter.MinConfidence)
	}

	err := query.Pluck("code", &codes).Error
	return codes, err
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
	// Handle foreign key constraints: set pointers to nil if they point to 0
	// This is needed because the new scan engine (V2) uses ScanID/ScanJobID instead of TaskID/TaskJobID
	// When these IDs are 0, passing a pointer to 0 violates the foreign key constraint
	if issue.TaskID != nil && *issue.TaskID == 0 {
		issue.TaskID = nil
	}
	if issue.TaskJobID != nil && *issue.TaskJobID == 0 {
		issue.TaskJobID = nil
	}
	if issue.ScanID != nil && *issue.ScanID == 0 {
		issue.ScanID = nil
	}
	if issue.ScanJobID != nil && *issue.ScanJobID == 0 {
		issue.ScanJobID = nil
	}

	var existingIssue Issue
	query := d.db.Where("code = ? AND title = ? AND details = ? AND url = ? AND status_code = ? AND http_method = ? AND payload = ? AND confidence = ? AND severity = ? AND workspace_id = ? AND task_id = ? AND task_job_id = ? AND scan_id = ? AND websocket_connection_id = ?",
		issue.Code,
		issue.Title,
		issue.Details,
		issue.URL,
		issue.StatusCode,
		issue.HTTPMethod,
		issue.Payload,
		issue.Confidence,
		issue.Severity,
		issue.WorkspaceID,
		issue.TaskID,
		issue.TaskJobID,
		issue.ScanID,
		issue.WebsocketConnectionID,
	)

	result := query.First(&existingIssue)

	if result.Error == nil {
		return existingIssue, nil
	}

	result = d.db.Create(&issue)
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
