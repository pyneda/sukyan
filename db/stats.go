package db

func GetDatabaseSize() (string, error) {
	var result string
	err := Connection.db.Raw("SELECT pg_size_pretty(pg_database_size(current_database()))").Scan(&result).Error
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

	issueCounts := map[string]int64{}
	rows, _ = d.db.Model(&Issue{}).Select("severity, COUNT(*) as count").
		Where("workspace_id = ?", workspaceID).
		Group("severity").Rows()
	for rows.Next() {
		var severity string
		var count int64
		rows.Scan(&severity, &count)
		issueCounts[severity] = count
	}
	rows.Close()

	stats.Issues = IssuesStats{
		Unknown:  issueCounts["unknown"],
		Info:     issueCounts["info"],
		Low:      issueCounts["low"],
		Medium:   issueCounts["medium"],
		High:     issueCounts["high"],
		Critical: issueCounts["critical"],
	}

	return stats, nil
}
