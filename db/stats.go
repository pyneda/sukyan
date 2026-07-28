package db

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pyneda/sukyan/lib"
)

func GetDatabaseSize() (string, error) {
	var result string
	err := Connection().DB().Raw("SELECT pg_size_pretty(pg_database_size(current_database()))").Scan(&result).Error
	if err != nil {
		return "", err
	}
	return result, nil
}

type SystemStats struct {
	DatabaseSize string `json:"database_size"`
}

func (d *DatabaseConnection) GetSystemStats() (SystemStats, error) {
	databaseSize, err := GetDatabaseSize()
	if err != nil {
		return SystemStats{}, err
	}
	stats := SystemStats{
		DatabaseSize: databaseSize,
	}
	return stats, nil
}

type WorkspaceStats struct {
	IssuesCount               int64         `json:"issues_count"`
	JWTCount                  int64         `json:"jwt_count"`
	HistoryCount              int64         `json:"history_count"`
	WebsocketConnectionsCount int64         `json:"websocket_connections_count"`
	TasksCount                int64         `json:"tasks_count"`
	Requests                  RequestsStats `json:"requests"`
	Issues                    IssuesStats   `json:"issues"`
}

// TableHeaders returns the headers for the WorkspaceStats table
func (w WorkspaceStats) TableHeaders() []string {
	return []string{"Issues", "JWTs", "History", "WebSocket Connections", "Tasks", "Crawler Requests", "Scanner Requests", "Unknown Issues", "Info Issues", "Low Issues", "Medium Issues", "High Issues", "Critical Issues"}
}

// TableRow returns a row representation of WorkspaceStats for display in a table
func (w WorkspaceStats) TableRow() []string {
	return []string{
		fmt.Sprintf("%d", w.IssuesCount),
		fmt.Sprintf("%d", w.JWTCount),
		fmt.Sprintf("%d", w.HistoryCount),
		fmt.Sprintf("%d", w.WebsocketConnectionsCount),
		fmt.Sprintf("%d", w.TasksCount),
		fmt.Sprintf("%d", w.Requests.Crawler),
		fmt.Sprintf("%d", w.Requests.Scanner),
		fmt.Sprintf("%d", w.Issues.Unknown),
		fmt.Sprintf("%d", w.Issues.Info),
		fmt.Sprintf("%d", w.Issues.Low),
		fmt.Sprintf("%d", w.Issues.Medium),
		fmt.Sprintf("%d", w.Issues.High),
		fmt.Sprintf("%d", w.Issues.Critical),
	}
}

// String provides a basic textual representation of the WorkspaceStats
func (w WorkspaceStats) String() string {
	return fmt.Sprintf("Issues: %d, JWTs: %d, History: %d, WebSocket Connections: %d, Tasks: %d, Requests(Crawler: %d, Scanner: %d), Issues(Unknown: %d, Info: %d, Low: %d, Medium: %d, High: %d, Critical: %d)",
		w.IssuesCount, w.JWTCount, w.HistoryCount, w.WebsocketConnectionsCount, w.TasksCount,
		w.Requests.Crawler, w.Requests.Scanner,
		w.Issues.Unknown, w.Issues.Info, w.Issues.Low, w.Issues.Medium, w.Issues.High, w.Issues.Critical)
}

// Pretty provides a more formatted, user-friendly representation of the WorkspaceStats
func (w WorkspaceStats) Pretty() string {
	return fmt.Sprintf(
		"%sWorkspace Statistics:%s\n"+
			"  %sIssues:%s %d\n"+
			"  %sJWTs:%s %d\n"+
			"  %sHistory:%s %d\n"+
			"  %sWebSocket Connections:%s %d\n"+
			"  %sTasks:%s %d\n"+
			"\n%sRequests:%s\n"+
			"  %sCrawler:%s %d\n"+
			"  %sScanner:%s %d\n"+
			"\n%sIssues by Severity:%s\n"+
			"  %sUnknown:%s %d\n"+
			"  %sInfo:%s %d\n"+
			"  %sLow:%s %d\n"+
			"  %sMedium:%s %d\n"+
			"  %sHigh:%s %d\n"+
			"  %sCritical:%s %d\n",
		lib.Blue, lib.ResetColor,
		lib.Blue, lib.ResetColor, w.IssuesCount,
		lib.Blue, lib.ResetColor, w.JWTCount,
		lib.Blue, lib.ResetColor, w.HistoryCount,
		lib.Blue, lib.ResetColor, w.WebsocketConnectionsCount,
		lib.Blue, lib.ResetColor, w.TasksCount,
		lib.Blue, lib.ResetColor,
		lib.Blue, lib.ResetColor, w.Requests.Crawler,
		lib.Blue, lib.ResetColor, w.Requests.Scanner,
		lib.Blue, lib.ResetColor,
		lib.Blue, lib.ResetColor, w.Issues.Unknown,
		lib.Blue, lib.ResetColor, w.Issues.Info,
		lib.Blue, lib.ResetColor, w.Issues.Low,
		lib.Blue, lib.ResetColor, w.Issues.Medium,
		lib.Blue, lib.ResetColor, w.Issues.High,
		lib.Blue, lib.ResetColor, w.Issues.Critical)
}

func (d *DatabaseConnection) GetWorkspaceStats(workspaceID uint) (WorkspaceStats, error) {
	var stats WorkspaceStats

	if err := d.db.Model(&Issue{}).Where("workspace_id = ?", workspaceID).Count(&stats.IssuesCount).Error; err != nil {
		return stats, err
	}

	if err := d.db.Model(&JsonWebToken{}).Where("workspace_id = ?", workspaceID).Count(&stats.JWTCount).Error; err != nil {
		return stats, err
	}

	if err := d.db.Model(&History{}).Where("workspace_id = ?", workspaceID).Count(&stats.HistoryCount).Error; err != nil {
		return stats, err
	}

	if err := d.db.Model(&WebSocketConnection{}).Where("workspace_id = ?", workspaceID).Count(&stats.WebsocketConnectionsCount).Error; err != nil {
		return stats, err
	}

	if err := d.db.Model(&Task{}).Where("workspace_id = ?", workspaceID).Count(&stats.TasksCount).Error; err != nil {
		return stats, err
	}

	requestCounts := map[string]int64{}
	rows, _ := d.db.Model(&History{}).Select("source, COUNT(*) as count").
		Where("workspace_id = ?", workspaceID).
		Group("source").Rows()
	for rows.Next() {
		var source string
		var count int64
		rows.Scan(&source, &count)
		requestCounts[source] = count
	}
	rows.Close()

	stats.Requests = RequestsStats{
		Crawler: requestCounts["Crawler"],
		Scanner: requestCounts["Scanner"],
	}

	issueCounts := map[severity]int64{}
	rows, _ = d.db.Model(&Issue{}).Select("severity, COUNT(*) as count").
		Where("workspace_id = ?", workspaceID).
		Group("severity").Rows()
	for rows.Next() {
		var sev severity
		var count int64
		rows.Scan(&sev, &count)
		issueCounts[sev] = count
	}
	rows.Close()

	stats.Issues = IssuesStats{
		Unknown:  issueCounts[Unknown],
		Info:     issueCounts[Info],
		Low:      issueCounts[Low],
		Medium:   issueCounts[Medium],
		High:     issueCounts[High],
		Critical: issueCounts[Critical],
	}

	return stats, nil
}

// WorkspaceRollup is a per-workspace summary used by the global workspaces view.
type WorkspaceRollup struct {
	WorkspaceID     uint        `json:"workspace_id"`
	Code            string      `json:"code"`
	Title           string      `json:"title"`
	IssuesCount     int64       `json:"issues_count"`
	Issues          IssuesStats `json:"issues"`
	HistoryCount    int64       `json:"history_count"`
	ActiveScanCount int64       `json:"active_scan_count"`
	LastActivityAt  *time.Time  `json:"last_activity_at"`
}

// WorkspaceRollupFilter controls searching, sorting and pagination of rollups.
type WorkspaceRollupFilter struct {
	Query      string     `json:"query"`
	SortBy     string     `json:"sort_by"`
	SortOrder  string     `json:"sort_order"`
	Pagination Pagination `json:"pagination"`
}

// activeScanStatuses are the statuses that count as "currently running" for
// rollup purposes. Paused is included: the scan still occupies the workspace.
var activeScanStatuses = []ScanStatus{
	ScanStatusPending,
	ScanStatusCrawling,
	ScanStatusScanning,
	ScanStatusPaused,
}

// ListWorkspaceRollups returns one summary row per workspace.
//
// It deliberately does NOT reuse GetWorkspaceStats, which issues seven COUNT
// queries per workspace. At 200 workspaces that would be ~1400 queries per page
// load. This uses a fixed number of grouped queries regardless of workspace
// count, then merges in Go.
//
// Sorting happens after the merge because the sort columns are aggregates
// computed in separate queries, so the full workspace list is loaded before
// paginating. That is fine into the low thousands of workspaces. Past that,
// rewrite this as a single join with ORDER BY and LIMIT pushed into SQL; the
// change stays contained in this function.
func (d *DatabaseConnection) ListWorkspaceRollups(filter WorkspaceRollupFilter) ([]WorkspaceRollup, int64, error) {
	var workspaces []Workspace
	q := d.db.Model(&Workspace{})
	if filter.Query != "" {
		like := "%" + filter.Query + "%"
		q = q.Where("title ILIKE ? OR code ILIKE ?", like, like)
	}
	if err := q.Find(&workspaces).Error; err != nil {
		return nil, 0, err
	}

	rows := make([]WorkspaceRollup, 0, len(workspaces))
	index := make(map[uint]int, len(workspaces))
	for _, ws := range workspaces {
		index[ws.ID] = len(rows)
		rows = append(rows, WorkspaceRollup{
			WorkspaceID: ws.ID,
			Code:        ws.Code,
			Title:       ws.Title,
		})
	}

	if len(rows) == 0 {
		return rows, 0, nil
	}

	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.WorkspaceID)
	}

	// Issues grouped by workspace and severity.
	type issueGroup struct {
		WorkspaceID uint
		Severity    string
		Count       int64
	}
	var issueGroups []issueGroup
	if err := d.db.Model(&Issue{}).
		Select("workspace_id, severity, COUNT(*) as count").
		Where("workspace_id IN ?", ids).
		Group("workspace_id, severity").
		Scan(&issueGroups).Error; err != nil {
		return nil, 0, err
	}
	for _, g := range issueGroups {
		i, ok := index[g.WorkspaceID]
		if !ok {
			continue
		}
		rows[i].IssuesCount += g.Count
		switch severity(g.Severity) {
		case Critical:
			rows[i].Issues.Critical = g.Count
		case High:
			rows[i].Issues.High = g.Count
		case Medium:
			rows[i].Issues.Medium = g.Count
		case Low:
			rows[i].Issues.Low = g.Count
		case Info:
			rows[i].Issues.Info = g.Count
		default:
			rows[i].Issues.Unknown = g.Count
		}
	}

	// History counts.
	type countGroup struct {
		WorkspaceID uint
		Count       int64
	}
	var historyGroups []countGroup
	if err := d.db.Model(&History{}).
		Select("workspace_id, COUNT(*) as count").
		Where("workspace_id IN ?", ids).
		Group("workspace_id").
		Scan(&historyGroups).Error; err != nil {
		return nil, 0, err
	}
	for _, g := range historyGroups {
		if i, ok := index[g.WorkspaceID]; ok {
			rows[i].HistoryCount = g.Count
		}
	}

	// Active scan counts.
	var scanGroups []countGroup
	if err := d.db.Model(&Scan{}).
		Select("workspace_id, COUNT(*) as count").
		Where("workspace_id IN ? AND status IN ?", ids, activeScanStatuses).
		Group("workspace_id").
		Scan(&scanGroups).Error; err != nil {
		return nil, 0, err
	}
	for _, g := range scanGroups {
		if i, ok := index[g.WorkspaceID]; ok {
			rows[i].ActiveScanCount = g.Count
		}
	}

	// Last activity, taken as the most recent scan update in the workspace.
	type activityGroup struct {
		WorkspaceID uint
		LastAt      *time.Time
	}
	var activityGroups []activityGroup
	if err := d.db.Model(&Scan{}).
		Select("workspace_id, MAX(updated_at) as last_at").
		Where("workspace_id IN ?", ids).
		Group("workspace_id").
		Scan(&activityGroups).Error; err != nil {
		return nil, 0, err
	}
	for _, g := range activityGroups {
		if i, ok := index[g.WorkspaceID]; ok {
			rows[i].LastActivityAt = g.LastAt
		}
	}

	sortWorkspaceRollups(rows, filter.SortBy, filter.SortOrder)

	total := int64(len(rows))
	offset, limit := filter.Pagination.GetData()
	if offset >= len(rows) {
		return []WorkspaceRollup{}, total, nil
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[offset:end], total, nil
}

func sortWorkspaceRollups(rows []WorkspaceRollup, sortBy, sortOrder string) {
	desc := sortOrder != "asc"

	less := func(a, b WorkspaceRollup) bool {
		switch sortBy {
		case "critical":
			return a.Issues.Critical < b.Issues.Critical
		case "high":
			return a.Issues.High < b.Issues.High
		case "issues":
			return a.IssuesCount < b.IssuesCount
		case "active_scans":
			return a.ActiveScanCount < b.ActiveScanCount
		case "last_activity":
			switch {
			case a.LastActivityAt == nil && b.LastActivityAt == nil:
				return a.WorkspaceID < b.WorkspaceID
			case a.LastActivityAt == nil:
				return true
			case b.LastActivityAt == nil:
				return false
			default:
				return a.LastActivityAt.Before(*b.LastActivityAt)
			}
		case "title":
			return strings.ToLower(a.Title) < strings.ToLower(b.Title)
		default:
			return a.WorkspaceID < b.WorkspaceID
		}
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if desc {
			return less(rows[j], rows[i])
		}
		return less(rows[i], rows[j])
	})
}
