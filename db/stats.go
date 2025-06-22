package db

import (
	"fmt"

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
