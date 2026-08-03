package db

import (
	"time"

	"gorm.io/gorm"
)

// DeploymentFinding is one row of the deployment-wide findings feed. It carries
// only what the overview renders — the full Issue holds the raw request and
// response bodies, which would make a polled feed expensive for no benefit.
type DeploymentFinding struct {
	ID             uint      `json:"id"`
	Title          string    `json:"title"`
	Code           string    `json:"code"`
	Severity       string    `json:"severity"`
	URL            string    `json:"url"`
	CreatedAt      time.Time `json:"created_at"`
	WorkspaceID    uint      `json:"workspace_id"`
	WorkspaceTitle string    `json:"workspace_title"`
	WorkspaceCode  string    `json:"workspace_code"`
}

// DeploymentFindings is the newest findings plus the totals for the period.
//
// The two are scoped differently on purpose. `Total` and `Issues` cover the
// period starting at `Since`, so the caller can say "47 since your last visit".
// `Data` is always the newest rows regardless of `Since`, so a deployment that
// found nothing recently still shows what it last found instead of an empty
// panel — which is context, not noise, when the question is "did anything
// happen while I was away".
type DeploymentFindings struct {
	Data   []DeploymentFinding `json:"data"`
	Since  *time.Time          `json:"since,omitempty"`
	Total  int64               `json:"total"`
	Issues IssuesStats         `json:"issues"`
}

// GetDeploymentFindings lists the newest issues across every workspace and
// counts the ones recorded at or after `since`, which may be nil for all time.
//
// False positives and informational severities are excluded — the caller
// presents these as things that need attention.
func (d *DatabaseConnection) GetDeploymentFindings(since *time.Time, limit int) (*DeploymentFindings, error) {
	if limit < 1 {
		limit = 12
	}

	result := &DeploymentFindings{Since: since, Data: []DeploymentFinding{}}

	// Informational observations ("HTTP/2 in use", "Server header disclosure")
	// outnumber real findings several to one on a busy deployment and would
	// crowd everything actionable out of a feed this short.
	base := func() *gorm.DB {
		return d.db.Model(&Issue{}).
			Joins("join workspaces on workspaces.id = issues.workspace_id and workspaces.deleted_at is null").
			Where("issues.false_positive = ?", false).
			Where("issues.severity in ?", []severity{Critical, High, Medium, Low})
	}

	if err := base().
		Select("issues.id, issues.title, issues.code, issues.severity, issues.url, issues.created_at, " +
			"issues.workspace_id, workspaces.title as workspace_title, workspaces.code as workspace_code").
		Order("issues.created_at desc").
		Limit(limit).
		Scan(&result.Data).Error; err != nil {
		return nil, err
	}

	counts := base()
	if since != nil {
		counts = counts.Where("issues.created_at >= ?", *since)
	}

	type severityRow struct {
		Severity severity
		Count    int64
	}
	var rows []severityRow
	if err := counts.
		Select("issues.severity, count(*) as count").
		Group("issues.severity").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		result.Total += row.Count
		switch row.Severity {
		case Critical:
			result.Issues.Critical += row.Count
		case High:
			result.Issues.High += row.Count
		case Medium:
			result.Issues.Medium += row.Count
		case Low:
			result.Issues.Low += row.Count
		case Info:
			result.Issues.Info += row.Count
		default:
			result.Issues.Unknown += row.Count
		}
	}

	return result, nil
}
