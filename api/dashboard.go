// Package api provides the REST API handlers for Sukyan.
package api

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/scan/manager"
	"github.com/spf13/viper"
)

// DashboardStats represents the comprehensive dashboard statistics
type DashboardStats struct {
	// System info
	Timestamp           time.Time `json:"timestamp"`
	RefreshInterval     int       `json:"refresh_interval"`
	ManagerRunning      bool      `json:"manager_running"`
	OrchestratorRunning bool      `json:"orchestrator_running"`
	WorkerCount         int       `json:"worker_count"`

	// Orchestrator config
	OrchestratorConfig *OrchestratorConfigInfo `json:"orchestrator_config,omitempty"`

	// Active scans
	ActiveScans []ScanInfo `json:"active_scans"`

	// Paused scans
	PausedScans []ScanInfo `json:"paused_scans"`

	// Recent completed scans (last 10)
	RecentScans []ScanInfo `json:"recent_scans"`

	// Global queue stats (aggregated across all scans)
	GlobalQueueStats *GlobalQueueStatsInfo `json:"global_queue_stats"`

	// System stats
	SystemStats *db.SystemStats `json:"system_stats,omitempty"`
}

// OrchestratorConfigInfo holds orchestrator configuration for display
type OrchestratorConfigInfo struct {
	PollInterval      string `json:"poll_interval"`
	PhaseTimeout      string `json:"phase_timeout"`
	EnableFingerprint bool   `json:"enable_fingerprint"`
	EnableDiscovery   bool   `json:"enable_discovery"`
	EnableNuclei      bool   `json:"enable_nuclei"`
	EnableWebSocket   bool   `json:"enable_websocket"`
}

// ScanInfo represents scan information for the dashboard
type ScanInfo struct {
	ID                 uint       `json:"id"`
	Title              string     `json:"title"`
	Status             string     `json:"status"`
	Phase              string     `json:"phase"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	Duration           string     `json:"duration,omitempty"`
	TotalJobs          int        `json:"total_jobs"`
	PendingJobs        int        `json:"pending_jobs"`
	RunningJobs        int        `json:"running_jobs"`
	CompletedJobs      int        `json:"completed_jobs"`
	FailedJobs         int        `json:"failed_jobs"`
	ProgressPercentage float64    `json:"progress_percentage"`

	// Granular job statistics by type
	JobStatsByType []JobTypeStats `json:"job_stats_by_type,omitempty"`

	// Currently running jobs
	RunningJobDetails []RunningJobInfo `json:"running_job_details,omitempty"`

	// Recent failed jobs
	RecentFailedJobs []FailedJobInfo `json:"recent_failed_jobs,omitempty"`
}

// JobTypeStats holds job statistics grouped by job type
type JobTypeStats struct {
	JobType   string `json:"job_type"`
	Pending   int64  `json:"pending"`
	Claimed   int64  `json:"claimed"`
	Running   int64  `json:"running"`
	Completed int64  `json:"completed"`
	Failed    int64  `json:"failed"`
	Total     int64  `json:"total"`
}

// RunningJobInfo holds information about a currently running job
type RunningJobInfo struct {
	JobID       uint       `json:"job_id"`
	JobType     string     `json:"job_type"`
	URL         string     `json:"url"`
	Method      string     `json:"method"`
	TargetHost  string     `json:"target_host"`
	WorkerID    string     `json:"worker_id"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	ElapsedTime string     `json:"elapsed_time,omitempty"`
}

// FailedJobInfo holds information about a failed job
type FailedJobInfo struct {
	JobID        uint       `json:"job_id"`
	JobType      string     `json:"job_type"`
	URL          string     `json:"url"`
	Method       string     `json:"method"`
	ErrorType    string     `json:"error_type,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	FailedAt     *time.Time `json:"failed_at,omitempty"`
	Attempts     int        `json:"attempts"`
}

// GlobalQueueStatsInfo holds aggregated queue statistics
type GlobalQueueStatsInfo struct {
	TotalPending   int `json:"total_pending"`
	TotalClaimed   int `json:"total_claimed"`
	TotalRunning   int `json:"total_running"`
	TotalCompleted int `json:"total_completed"`
	TotalFailed    int `json:"total_failed"`
	TotalCancelled int `json:"total_cancelled"`
	TotalJobs      int `json:"total_jobs"`

	// Aggregated stats by job type across all scans
	StatsByType []JobTypeStats `json:"stats_by_type,omitempty"`
}

// GetDashboardStats returns comprehensive dashboard statistics
// @Summary Get dashboard statistics
// @Description Returns real-time statistics for monitoring scan queue and orchestrator
// @Tags Dashboard
// @Accept json
// @Produce json
// @Success 200 {object} DashboardStats
// @Router /api/v1/dashboard/stats [get]
func GetDashboardStats(c *fiber.Ctx, scanManager *manager.ScanManager) error {
	stats := DashboardStats{
		Timestamp:       time.Now(),
		RefreshInterval: viper.GetInt("api.dashboard.refresh_interval"),
	}

	// Manager status
	if scanManager != nil {
		stats.ManagerRunning = scanManager.IsStarted()
		stats.WorkerCount = scanManager.WorkerCount()

		// Orchestrator status
		orch := scanManager.GetOrchestrator()
		if orch != nil {
			stats.OrchestratorRunning = orch.IsRunning()
			cfg := orch.GetConfig()
			stats.OrchestratorConfig = &OrchestratorConfigInfo{
				PollInterval:      cfg.PollInterval.String(),
				PhaseTimeout:      cfg.PhaseTimeout.String(),
				EnableFingerprint: cfg.EnableFingerprint,
				EnableDiscovery:   cfg.EnableDiscovery,
				EnableNuclei:      cfg.EnableNuclei,
				EnableWebSocket:   cfg.EnableWebSocket,
			}
		}
	}

	// Get active scans
	activeScans, err := db.Connection().GetActiveScans()
	if err == nil {
		stats.ActiveScans = make([]ScanInfo, 0, len(activeScans))
		for _, scan := range activeScans {
			info := buildScanInfo(scan)
			stats.ActiveScans = append(stats.ActiveScans, info)
		}
	}

	// Get paused scans
	pausedScans, err := db.Connection().GetPausedScans()
	if err == nil {
		stats.PausedScans = make([]ScanInfo, 0, len(pausedScans))
		for _, scan := range pausedScans {
			info := buildScanInfo(scan)
			stats.PausedScans = append(stats.PausedScans, info)
		}
	}

	// Get recent completed scans
	recentScans, _, err := db.Connection().ListScans(db.ScanFilter{
		Statuses: []db.ScanStatus{db.ScanStatusCompleted},
		Pagination: db.Pagination{
			PageSize: 10,
			Page:     1,
		},
	})
	if err == nil {
		stats.RecentScans = make([]ScanInfo, 0, len(recentScans))
		for _, scan := range recentScans {
			info := buildScanInfo(scan)
			stats.RecentScans = append(stats.RecentScans, info)
		}
	}

	// Global queue stats
	stats.GlobalQueueStats = calculateGlobalQueueStats(stats.ActiveScans, stats.PausedScans)

	// System stats
	sysStats, err := db.Connection().GetSystemStats()
	if err == nil {
		stats.SystemStats = &sysStats
	}

	return c.JSON(stats)
}

func buildScanInfo(scan *db.Scan) ScanInfo {
	info := ScanInfo{
		ID:            scan.ID,
		Title:         scan.Title,
		Status:        string(scan.Status),
		Phase:         string(scan.Phase),
		StartedAt:     scan.StartedAt,
		CompletedAt:   scan.CompletedAt,
		TotalJobs:     scan.TotalJobsCount,
		PendingJobs:   scan.PendingJobsCount,
		RunningJobs:   scan.RunningJobsCount,
		CompletedJobs: scan.CompletedJobsCount,
		FailedJobs:    scan.FailedJobsCount,
	}

	// Calculate duration
	if scan.StartedAt != nil {
		endTime := time.Now()
		if scan.CompletedAt != nil {
			endTime = *scan.CompletedAt
		}
		info.Duration = endTime.Sub(*scan.StartedAt).Round(time.Second).String()
	}

	// Calculate progress percentage
	if info.TotalJobs > 0 {
		info.ProgressPercentage = float64(info.CompletedJobs+info.FailedJobs) / float64(info.TotalJobs) * 100
	}

	// Get granular job statistics by type
	info.JobStatsByType = getJobStatsByType(scan.ID)

	// Get currently running job details (for active scans)
	if scan.IsActive() {
		info.RunningJobDetails = getRunningJobDetails(scan.ID)
	}

	// Get recent failed jobs
	info.RecentFailedJobs = getRecentFailedJobs(scan.ID)

	return info
}

// getJobStatsByType retrieves job statistics grouped by job type for a scan
func getJobStatsByType(scanID uint) []JobTypeStats {
	var results []struct {
		JobType string
		Status  string
		Count   int64
	}

	err := db.Connection().DB().Model(&db.ScanJob{}).
		Select("job_type, status, COUNT(*) as count").
		Where("scan_id = ?", scanID).
		Group("job_type, status").
		Scan(&results).Error

	if err != nil {
		return nil
	}

	// Aggregate by job type
	typeMap := make(map[string]*JobTypeStats)
	for _, r := range results {
		if _, ok := typeMap[r.JobType]; !ok {
			typeMap[r.JobType] = &JobTypeStats{JobType: r.JobType}
		}
		stats := typeMap[r.JobType]
		switch db.ScanJobStatus(r.Status) {
		case db.ScanJobStatusPending:
			stats.Pending = r.Count
		case db.ScanJobStatusClaimed:
			stats.Claimed = r.Count
		case db.ScanJobStatusRunning:
			stats.Running = r.Count
		case db.ScanJobStatusCompleted:
			stats.Completed = r.Count
		case db.ScanJobStatusFailed:
			stats.Failed = r.Count
		}
		stats.Total += r.Count
	}

	// Convert map to slice
	result := make([]JobTypeStats, 0, len(typeMap))
	for _, stats := range typeMap {
		result = append(result, *stats)
	}

	return result
}

// getRunningJobDetails retrieves details of currently running jobs for a scan
func getRunningJobDetails(scanID uint) []RunningJobInfo {
	var jobs []db.ScanJob
	err := db.Connection().DB().
		Where("scan_id = ? AND status IN ?", scanID, []db.ScanJobStatus{db.ScanJobStatusClaimed, db.ScanJobStatusRunning}).
		Order("started_at ASC").
		Limit(20). // Limit to avoid too much data
		Find(&jobs).Error

	if err != nil {
		return nil
	}

	result := make([]RunningJobInfo, 0, len(jobs))
	for _, job := range jobs {
		info := RunningJobInfo{
			JobID:      job.ID,
			JobType:    string(job.JobType),
			URL:        job.URL,
			Method:     job.Method,
			TargetHost: job.TargetHost,
			WorkerID:   job.WorkerID,
			StartedAt:  job.StartedAt,
		}
		// Calculate elapsed time
		if job.StartedAt != nil {
			info.ElapsedTime = time.Since(*job.StartedAt).Round(time.Second).String()
		} else if job.ClaimedAt != nil {
			info.ElapsedTime = time.Since(*job.ClaimedAt).Round(time.Second).String()
		}
		result = append(result, info)
	}

	return result
}

// getRecentFailedJobs retrieves recently failed jobs for a scan
func getRecentFailedJobs(scanID uint) []FailedJobInfo {
	var jobs []db.ScanJob
	err := db.Connection().DB().
		Where("scan_id = ? AND status = ?", scanID, db.ScanJobStatusFailed).
		Order("completed_at DESC").
		Limit(10). // Last 10 failed jobs
		Find(&jobs).Error

	if err != nil {
		return nil
	}

	result := make([]FailedJobInfo, 0, len(jobs))
	for _, job := range jobs {
		info := FailedJobInfo{
			JobID:    job.ID,
			JobType:  string(job.JobType),
			URL:      job.URL,
			Method:   job.Method,
			FailedAt: job.CompletedAt,
			Attempts: job.Attempts,
		}
		if job.ErrorType != nil {
			info.ErrorType = *job.ErrorType
		}
		if job.ErrorMessage != nil {
			info.ErrorMessage = *job.ErrorMessage
		}
		result = append(result, info)
	}

	return result
}

func calculateGlobalQueueStats(activeScans, pausedScans []ScanInfo) *GlobalQueueStatsInfo {
	stats := &GlobalQueueStatsInfo{}

	allScans := append(activeScans, pausedScans...)
	for _, scan := range allScans {
		stats.TotalPending += scan.PendingJobs
		stats.TotalRunning += scan.RunningJobs
		stats.TotalCompleted += scan.CompletedJobs
		stats.TotalFailed += scan.FailedJobs
		stats.TotalJobs += scan.TotalJobs
	}

	// Aggregate stats by type from all scans
	stats.StatsByType = getGlobalJobStatsByType()

	return stats
}

// getGlobalJobStatsByType retrieves aggregated job statistics by type across all active/paused scans
func getGlobalJobStatsByType() []JobTypeStats {
	var results []struct {
		JobType string
		Status  string
		Count   int64
	}

	// Get stats for jobs belonging to active or paused scans
	err := db.Connection().DB().Model(&db.ScanJob{}).
		Select("scan_jobs.job_type, scan_jobs.status, COUNT(*) as count").
		Joins("JOIN scans ON scan_jobs.scan_id = scans.id").
		Where("scans.status IN ?", []db.ScanStatus{db.ScanStatusCrawling, db.ScanStatusScanning, db.ScanStatusPaused}).
		Group("scan_jobs.job_type, scan_jobs.status").
		Scan(&results).Error

	if err != nil {
		return nil
	}

	// Aggregate by job type
	typeMap := make(map[string]*JobTypeStats)
	for _, r := range results {
		if _, ok := typeMap[r.JobType]; !ok {
			typeMap[r.JobType] = &JobTypeStats{JobType: r.JobType}
		}
		stats := typeMap[r.JobType]
		switch db.ScanJobStatus(r.Status) {
		case db.ScanJobStatusPending:
			stats.Pending = r.Count
		case db.ScanJobStatusClaimed:
			stats.Claimed = r.Count
		case db.ScanJobStatusRunning:
			stats.Running = r.Count
		case db.ScanJobStatusCompleted:
			stats.Completed = r.Count
		case db.ScanJobStatusFailed:
			stats.Failed = r.Count
		}
		stats.Total += r.Count
	}

	// Convert map to slice
	result := make([]JobTypeStats, 0, len(typeMap))
	for _, stats := range typeMap {
		result = append(result, *stats)
	}

	return result
}

// DashboardHTML serves the HTML dashboard page
// @Summary Get dashboard HTML page
// @Description Returns an HTML page with real-time dashboard
// @Tags Dashboard
// @Produce html
// @Router /dashboard [get]
func DashboardHTML(c *fiber.Ctx, scanManager *manager.ScanManager) error {
	refreshInterval := viper.GetInt("api.dashboard.refresh_interval")
	if refreshInterval < 1 {
		refreshInterval = 5
	}

	title := viper.GetString("api.dashboard.title")
	if title == "" {
		title = "Sukyan Scan Dashboard"
	}

	html := getDashboardTemplate(title, refreshInterval)
	c.Set("Content-Type", "text/html")
	return c.SendString(html)
}

func getDashboardTemplate(title string, refreshInterval int) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
            background: #0f1419; color: #e7e9ea; padding: 20px; min-height: 100vh;
        }
        .header {
            display: flex; justify-content: space-between; align-items: center;
            margin-bottom: 30px; padding-bottom: 20px; border-bottom: 1px solid #2f3336;
        }
        h1 { font-size: 1.8rem; color: #1d9bf0; }
        .status-indicator { display: flex; align-items: center; gap: 10px; }
        .status-dot {
            width: 12px; height: 12px; border-radius: 50%%; background: #71767b;
        }
        .status-dot.active { background: #00ba7c; box-shadow: 0 0 10px #00ba7c; }
        .status-dot.inactive { background: #f4212e; }
        .grid {
            display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 20px; margin-bottom: 30px;
        }
        .card {
            background: #16181c; border-radius: 16px; padding: 20px; border: 1px solid #2f3336;
        }
        .card h2 {
            font-size: 1.1rem; color: #71767b; margin-bottom: 15px;
            text-transform: uppercase; letter-spacing: 0.5px;
        }
        .card h3 {
            font-size: 0.95rem; color: #71767b; margin: 15px 0 10px 0;
            border-top: 1px solid #2f3336; padding-top: 15px;
        }
        .stat-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 15px; }
        .stat { text-align: center; }
        .stat-value { font-size: 2rem; font-weight: bold; color: #1d9bf0; }
        .stat-label { font-size: 0.85rem; color: #71767b; margin-top: 5px; }
        .stat-value.pending { color: #ffd400; }
        .stat-value.running { color: #1d9bf0; }
        .stat-value.completed { color: #00ba7c; }
        .stat-value.failed { color: #f4212e; }
        .scan-list { display: flex; flex-direction: column; gap: 15px; }
        .scan-item {
            background: #1e2125; border-radius: 12px; padding: 15px;
            border-left: 4px solid #1d9bf0;
        }
        .scan-item.paused { border-left-color: #ffd400; }
        .scan-item.completed { border-left-color: #00ba7c; }
        .scan-item.expanded { padding-bottom: 5px; }
        .scan-header {
            display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px;
        }
        .scan-title { font-weight: bold; font-size: 1rem; cursor: pointer; }
        .scan-title:hover { color: #1d9bf0; }
        .scan-id { color: #71767b; font-size: 0.85rem; }
        .scan-phase {
            background: #2f3336; padding: 4px 10px; border-radius: 12px;
            font-size: 0.8rem; color: #e7e9ea;
        }
        .progress-bar {
            background: #2f3336; border-radius: 10px; height: 8px;
            margin: 10px 0; overflow: hidden;
        }
        .progress-fill {
            background: linear-gradient(90deg, #1d9bf0, #00ba7c);
            height: 100%%; border-radius: 10px; transition: width 0.5s ease;
        }
        .scan-stats { display: flex; gap: 20px; font-size: 0.85rem; color: #71767b; flex-wrap: wrap; }
        .scan-stats span { display: flex; align-items: center; gap: 5px; }
        .empty-state { text-align: center; padding: 40px; color: #71767b; }
        .config-list { display: flex; flex-wrap: wrap; gap: 10px; }
        .config-item { background: #2f3336; padding: 6px 12px; border-radius: 8px; font-size: 0.85rem; }
        .config-item.enabled { background: #003d21; color: #00ba7c; }
        .config-item.disabled { background: #3d0d0d; color: #f4212e; }
        .timestamp { color: #71767b; font-size: 0.85rem; }
        @keyframes pulse { 0%%, 100%% { opacity: 1; } 50%% { opacity: 0.5; } }
        .loading { animation: pulse 1.5s infinite; }
        
        /* Job type breakdown styles */
        .job-type-breakdown { margin-top: 15px; }
        .job-type-row {
            display: flex; align-items: center; justify-content: space-between;
            padding: 8px 12px; background: #16181c; border-radius: 8px; margin-bottom: 6px;
            font-size: 0.85rem;
        }
        .job-type-name { 
            font-weight: 500; min-width: 120px;
            text-transform: capitalize;
        }
        .job-type-stats { display: flex; gap: 12px; }
        .job-type-stats span { display: flex; align-items: center; gap: 4px; }
        .job-type-stats .pending { color: #ffd400; }
        .job-type-stats .running { color: #1d9bf0; }
        .job-type-stats .completed { color: #00ba7c; }
        .job-type-stats .failed { color: #f4212e; }
        
        /* Running jobs table */
        .running-jobs { margin-top: 10px; }
        .running-job {
            display: grid; grid-template-columns: 60px 1fr 80px 100px;
            padding: 8px 10px; background: #16181c; border-radius: 6px;
            margin-bottom: 4px; font-size: 0.8rem; gap: 10px; align-items: center;
        }
        .running-job .job-id { color: #71767b; }
        .running-job .job-url { 
            overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
            color: #e7e9ea;
        }
        .running-job .job-type { color: #1d9bf0; text-transform: capitalize; }
        .running-job .job-elapsed { color: #ffd400; text-align: right; }
        
        /* Failed jobs */
        .failed-jobs { margin-top: 10px; }
        .failed-job {
            padding: 8px 10px; background: #2a1515; border-radius: 6px;
            margin-bottom: 4px; font-size: 0.8rem; border-left: 3px solid #f4212e;
        }
        .failed-job-header { display: flex; justify-content: space-between; margin-bottom: 4px; }
        .failed-job .job-url { color: #e7e9ea; }
        .failed-job .job-error { color: #f4212e; font-size: 0.75rem; margin-top: 4px; }
        
        /* Collapsible sections */
        .collapsible-header {
            display: flex; justify-content: space-between; align-items: center;
            cursor: pointer; padding: 5px 0;
        }
        .collapsible-header:hover { color: #1d9bf0; }
        .collapsible-content { display: none; }
        .collapsible-content.show { display: block; }
        .toggle-icon { transition: transform 0.2s; }
        .toggle-icon.expanded { transform: rotate(90deg); }
        
        /* Global stats by type */
        .global-type-stats { margin-top: 15px; }
        .type-stat-bar {
            display: flex; align-items: center; margin-bottom: 8px; font-size: 0.85rem;
        }
        .type-stat-bar .type-name { min-width: 100px; text-transform: capitalize; }
        .type-stat-bar .bar-container {
            flex: 1; height: 20px; background: #2f3336; border-radius: 4px;
            overflow: hidden; margin: 0 10px; display: flex;
        }
        .type-stat-bar .bar-segment {
            height: 100%%; transition: width 0.3s ease;
        }
        .type-stat-bar .bar-segment.completed { background: #00ba7c; }
        .type-stat-bar .bar-segment.running { background: #1d9bf0; }
        .type-stat-bar .bar-segment.pending { background: #ffd400; }
        .type-stat-bar .bar-segment.failed { background: #f4212e; }
        .type-stat-bar .type-count { min-width: 60px; text-align: right; color: #71767b; }
    </style>
</head>
<body>
    <div class="header">
        <h1>🔍 %s</h1>
        <div class="status-indicator">
            <span class="timestamp" id="last-update">Loading...</span>
            <div class="status-dot" id="manager-status" title="Scan Manager"></div>
            <div class="status-dot" id="orchestrator-status" title="Orchestrator"></div>
        </div>
    </div>

    <div class="grid">
        <div class="card">
            <h2>System Status</h2>
            <div class="stat-grid">
                <div class="stat">
                    <div class="stat-value" id="worker-count">-</div>
                    <div class="stat-label">Workers</div>
                </div>
                <div class="stat">
                    <div class="stat-value" id="active-scans-count">-</div>
                    <div class="stat-label">Active Scans</div>
                </div>
                <div class="stat">
                    <div class="stat-value pending" id="paused-scans-count">-</div>
                    <div class="stat-label">Paused Scans</div>
                </div>
                <div class="stat">
                    <div class="stat-value" id="db-size">-</div>
                    <div class="stat-label">Database Size</div>
                </div>
            </div>
        </div>

        <div class="card">
            <h2>Global Queue</h2>
            <div class="stat-grid">
                <div class="stat">
                    <div class="stat-value pending" id="global-pending">-</div>
                    <div class="stat-label">Pending</div>
                </div>
                <div class="stat">
                    <div class="stat-value running" id="global-running">-</div>
                    <div class="stat-label">Running</div>
                </div>
                <div class="stat">
                    <div class="stat-value completed" id="global-completed">-</div>
                    <div class="stat-label">Completed</div>
                </div>
                <div class="stat">
                    <div class="stat-value failed" id="global-failed">-</div>
                    <div class="stat-label">Failed</div>
                </div>
            </div>
            <div class="global-type-stats" id="global-type-stats"></div>
        </div>

        <div class="card">
            <h2>Orchestrator Config</h2>
            <div class="config-list" id="orchestrator-config">
                <div class="config-item">Loading...</div>
            </div>
        </div>
    </div>

    <div class="grid">
        <div class="card" style="grid-column: span 2;">
            <h2>Active Scans</h2>
            <div class="scan-list" id="active-scans">
                <div class="empty-state loading">Loading scans...</div>
            </div>
        </div>
    </div>

    <div class="grid">
        <div class="card">
            <h2>Paused Scans</h2>
            <div class="scan-list" id="paused-scans">
                <div class="empty-state">No paused scans</div>
            </div>
        </div>

        <div class="card">
            <h2>Recent Completed</h2>
            <div class="scan-list" id="recent-scans">
                <div class="empty-state">No recent scans</div>
            </div>
        </div>
    </div>

    <script>
        const REFRESH_INTERVAL = %d * 1000;
        const expandedScans = new Set();
        
        function formatNumber(num) {
            if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M';
            if (num >= 1000) return (num / 1000).toFixed(1) + 'K';
            return num.toString();
        }
        
        function formatJobType(type) {
            return type.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase());
        }
        
        function truncateUrl(url, maxLen = 60) {
            if (!url || url.length <= maxLen) return url || '-';
            return url.substring(0, maxLen) + '...';
        }

        function toggleScanDetails(scanId) {
            if (expandedScans.has(scanId)) {
                expandedScans.delete(scanId);
            } else {
                expandedScans.add(scanId);
            }
            // Re-render will happen on next update, or we can force it
            const content = document.getElementById('scan-details-' + scanId);
            const icon = document.getElementById('toggle-icon-' + scanId);
            if (content) {
                content.classList.toggle('show');
            }
            if (icon) {
                icon.classList.toggle('expanded');
            }
        }
        
        function renderJobTypeBreakdown(stats) {
            if (!stats || stats.length === 0) return '';
            return `+"`"+`
                <div class="job-type-breakdown">
                    ${stats.map(s => `+"`"+`
                        <div class="job-type-row">
                            <span class="job-type-name">${formatJobType(s.job_type)}</span>
                            <div class="job-type-stats">
                                <span class="pending">⏳ ${s.pending || 0}</span>
                                <span class="running">▶️ ${s.running || 0}</span>
                                <span class="completed">✅ ${s.completed || 0}</span>
                                <span class="failed">❌ ${s.failed || 0}</span>
                            </div>
                        </div>
                    `+"`"+`).join('')}
                </div>
            `+"`"+`;
        }
        
        function renderRunningJobs(jobs) {
            if (!jobs || jobs.length === 0) return '<div style="color: #71767b; font-size: 0.85rem; padding: 10px;">No jobs currently running</div>';
            return `+"`"+`
                <div class="running-jobs">
                    ${jobs.slice(0, 10).map(j => `+"`"+`
                        <div class="running-job">
                            <span class="job-id">#${j.job_id}</span>
                            <span class="job-url" title="${j.url || ''}">${truncateUrl(j.url)}</span>
                            <span class="job-type">${formatJobType(j.job_type)}</span>
                            <span class="job-elapsed">${j.elapsed_time || '-'}</span>
                        </div>
                    `+"`"+`).join('')}
                    ${jobs.length > 10 ? `+"`"+`<div style="color: #71767b; font-size: 0.8rem; text-align: center; padding: 5px;">... and ${jobs.length - 10} more</div>`+"`"+` : ''}
                </div>
            `+"`"+`;
        }
        
        function renderFailedJobs(jobs) {
            if (!jobs || jobs.length === 0) return '';
            return `+"`"+`
                <div class="failed-jobs">
                    ${jobs.slice(0, 5).map(j => `+"`"+`
                        <div class="failed-job">
                            <div class="failed-job-header">
                                <span class="job-url" title="${j.url || ''}">${truncateUrl(j.url, 50)}</span>
                                <span style="color: #71767b;">${formatJobType(j.job_type)}</span>
                            </div>
                            ${j.error_message ? `+"`"+`<div class="job-error">${j.error_type || 'Error'}: ${truncateUrl(j.error_message, 80)}</div>`+"`"+` : ''}
                        </div>
                    `+"`"+`).join('')}
                </div>
            `+"`"+`;
        }

        function renderScanItem(scan, type) {
            const progress = scan.progress_percentage.toFixed(1);
            const statusClass = type === 'paused' ? 'paused' : (type === 'completed' ? 'completed' : '');
            const isExpanded = expandedScans.has(scan.id);
            const hasDetails = type === 'active' && (scan.job_stats_by_type?.length > 0 || scan.running_job_details?.length > 0);
            
            return `+"`"+`
                <div class="scan-item ${statusClass} ${isExpanded ? 'expanded' : ''}">
                    <div class="scan-header">
                        <div>
                            <span class="scan-title" ${hasDetails ? `+"`"+`onclick="toggleScanDetails(${scan.id})"`+"`"+` : ''}>
                                ${hasDetails ? `+"`"+`<span id="toggle-icon-${scan.id}" class="toggle-icon ${isExpanded ? 'expanded' : ''}">▶</span>`+"`"+` : ''}
                                ${scan.title || 'Untitled Scan'}
                            </span>
                            <span class="scan-id">#${scan.id}</span>
                        </div>
                        <span class="scan-phase">${scan.phase || scan.status}</span>
                    </div>
                    <div class="progress-bar">
                        <div class="progress-fill" style="width: ${progress}%%"></div>
                    </div>
                    <div class="scan-stats">
                        <span>📊 ${progress}%%</span>
                        <span>⏱️ ${scan.duration || '-'}</span>
                        <span>✅ ${scan.completed_jobs}/${scan.total_jobs} jobs</span>
                        <span>❌ ${scan.failed_jobs} failed</span>
                        <span>▶️ ${scan.running_jobs} running</span>
                        <span>⏳ ${scan.pending_jobs} pending</span>
                    </div>
                    ${hasDetails ? `+"`"+`
                        <div id="scan-details-${scan.id}" class="collapsible-content ${isExpanded ? 'show' : ''}">
                            ${scan.job_stats_by_type?.length > 0 ? `+"`"+`
                                <h3>📋 Jobs by Type</h3>
                                ${renderJobTypeBreakdown(scan.job_stats_by_type)}
                            `+"`"+` : ''}
                            ${scan.running_job_details?.length > 0 ? `+"`"+`
                                <h3>▶️ Currently Running (${scan.running_job_details.length})</h3>
                                ${renderRunningJobs(scan.running_job_details)}
                            `+"`"+` : ''}
                            ${scan.recent_failed_jobs?.length > 0 ? `+"`"+`
                                <h3>❌ Recent Failures</h3>
                                ${renderFailedJobs(scan.recent_failed_jobs)}
                            `+"`"+` : ''}
                        </div>
                    `+"`"+` : ''}
                </div>
            `+"`"+`;
        }
        
        function renderGlobalTypeStats(stats) {
            if (!stats || stats.length === 0) return '';
            
            return stats.map(s => {
                const total = s.total || 1;
                const completedPct = ((s.completed || 0) / total * 100).toFixed(1);
                const runningPct = ((s.running || 0) / total * 100).toFixed(1);
                const pendingPct = ((s.pending || 0) / total * 100).toFixed(1);
                const failedPct = ((s.failed || 0) / total * 100).toFixed(1);
                
                return `+"`"+`
                    <div class="type-stat-bar">
                        <span class="type-name">${formatJobType(s.job_type)}</span>
                        <div class="bar-container">
                            <div class="bar-segment completed" style="width: ${completedPct}%%" title="Completed: ${s.completed || 0}"></div>
                            <div class="bar-segment running" style="width: ${runningPct}%%" title="Running: ${s.running || 0}"></div>
                            <div class="bar-segment pending" style="width: ${pendingPct}%%" title="Pending: ${s.pending || 0}"></div>
                            <div class="bar-segment failed" style="width: ${failedPct}%%" title="Failed: ${s.failed || 0}"></div>
                        </div>
                        <span class="type-count">${formatNumber(s.total)}</span>
                    </div>
                `+"`"+`;
            }).join('');
        }

        function updateDashboard(data) {
            document.getElementById('last-update').textContent = 
                'Updated: ' + new Date(data.timestamp).toLocaleTimeString();

            document.getElementById('manager-status').className = 
                'status-dot ' + (data.manager_running ? 'active' : 'inactive');
            document.getElementById('orchestrator-status').className = 
                'status-dot ' + (data.orchestrator_running ? 'active' : 'inactive');

            document.getElementById('worker-count').textContent = data.worker_count || 0;
            document.getElementById('active-scans-count').textContent = data.active_scans?.length || 0;
            document.getElementById('paused-scans-count').textContent = data.paused_scans?.length || 0;
            
            if (data.system_stats) {
                document.getElementById('db-size').textContent = data.system_stats.database_size || '-';
            }

            if (data.global_queue_stats) {
                document.getElementById('global-pending').textContent = 
                    formatNumber(data.global_queue_stats.total_pending);
                document.getElementById('global-running').textContent = 
                    formatNumber(data.global_queue_stats.total_running);
                document.getElementById('global-completed').textContent = 
                    formatNumber(data.global_queue_stats.total_completed);
                document.getElementById('global-failed').textContent = 
                    formatNumber(data.global_queue_stats.total_failed);
                
                // Render global type stats
                document.getElementById('global-type-stats').innerHTML = 
                    renderGlobalTypeStats(data.global_queue_stats.stats_by_type);
            }

            if (data.orchestrator_config) {
                const cfg = data.orchestrator_config;
                document.getElementById('orchestrator-config').innerHTML = `+"`"+`
                    <div class="config-item">Poll: ${cfg.poll_interval}</div>
                    <div class="config-item">Timeout: ${cfg.phase_timeout}</div>
                    <div class="config-item ${cfg.enable_fingerprint ? 'enabled' : 'disabled'}">Fingerprint</div>
                    <div class="config-item ${cfg.enable_discovery ? 'enabled' : 'disabled'}">Discovery</div>
                    <div class="config-item ${cfg.enable_nuclei ? 'enabled' : 'disabled'}">Nuclei</div>
                    <div class="config-item ${cfg.enable_websocket ? 'enabled' : 'disabled'}">WebSocket</div>
                `+"`"+`;
            }

            const activeContainer = document.getElementById('active-scans');
            if (data.active_scans && data.active_scans.length > 0) {
                activeContainer.innerHTML = data.active_scans.map(s => renderScanItem(s, 'active')).join('');
            } else {
                activeContainer.innerHTML = '<div class="empty-state">No active scans</div>';
            }

            const pausedContainer = document.getElementById('paused-scans');
            if (data.paused_scans && data.paused_scans.length > 0) {
                pausedContainer.innerHTML = data.paused_scans.map(s => renderScanItem(s, 'paused')).join('');
            } else {
                pausedContainer.innerHTML = '<div class="empty-state">No paused scans</div>';
            }

            const recentContainer = document.getElementById('recent-scans');
            if (data.recent_scans && data.recent_scans.length > 0) {
                recentContainer.innerHTML = data.recent_scans.map(s => renderScanItem(s, 'completed')).join('');
            } else {
                recentContainer.innerHTML = '<div class="empty-state">No recent scans</div>';
            }
        }

        async function fetchStats() {
            try {
                const response = await fetch('./stats');
                const data = await response.json();
                updateDashboard(data);
            } catch (error) {
                console.error('Failed to fetch stats:', error);
                document.getElementById('last-update').textContent = 'Error fetching data';
            }
        }

        fetchStats();
        setInterval(fetchStats, REFRESH_INTERVAL);
    </script>
</body>
</html>`, title, title, refreshInterval)
}

// GetDashboardStatsHandler is a wrapper handler that retrieves the scan manager and calls GetDashboardStats
func GetDashboardStatsHandler(c *fiber.Ctx) error {
	return GetDashboardStats(c, GetScanManager())
}

// DashboardHTMLHandler is a wrapper handler that retrieves the scan manager and calls DashboardHTML
func DashboardHTMLHandler(c *fiber.Ctx) error {
	return DashboardHTML(c, GetScanManager())
}
