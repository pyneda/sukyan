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
	WorkerCount         int       `json:"worker_count"`       // Workers in this process (deprecated, use local_worker_count)
	LocalWorkerCount    int       `json:"local_worker_count"` // Workers in this process
	TotalWorkerCount    int       `json:"total_worker_count"` // Workers across all nodes
	NodeID              string    `json:"node_id"`

	// Orchestrator config
	OrchestratorConfig *OrchestratorConfigInfo `json:"orchestrator_config,omitempty"`

	// Worker nodes
	WorkerNodes *WorkerNodesInfo `json:"worker_nodes,omitempty"`

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

// WorkerNodesInfo holds information about distributed worker nodes
type WorkerNodesInfo struct {
	TotalNodes     int              `json:"total_nodes"`
	RunningNodes   int              `json:"running_nodes"`
	StoppedNodes   int              `json:"stopped_nodes"`
	TotalClaimed   int64            `json:"total_claimed"`
	TotalCompleted int64            `json:"total_completed"`
	TotalFailed    int64            `json:"total_failed"`
	Nodes          []WorkerNodeInfo `json:"nodes"`
}

// WorkerNodeInfo holds information about a single worker node
type WorkerNodeInfo struct {
	ID            string           `json:"id"`
	Hostname      string           `json:"hostname"`
	WorkerCount   int              `json:"worker_count"`
	Status        string           `json:"status"`
	IsStale       bool             `json:"is_stale"`
	StartedAt     time.Time        `json:"started_at"`
	LastSeenAt    time.Time        `json:"last_seen_at"`
	JobsClaimed   int              `json:"jobs_claimed"`
	JobsCompleted int              `json:"jobs_completed"`
	JobsFailed    int              `json:"jobs_failed"`
	Version       string           `json:"version"`
	RunningJobs   []RunningJobInfo `json:"running_jobs,omitempty"`
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
		stats.LocalWorkerCount = scanManager.WorkerCount()
		stats.NodeID = scanManager.NodeID()

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

	// Get worker nodes info
	stats.WorkerNodes = getWorkerNodesInfo()

	// Calculate total workers across all active nodes
	if stats.WorkerNodes != nil {
		for _, node := range stats.WorkerNodes.Nodes {
			if node.Status == "running" && !node.IsStale {
				stats.TotalWorkerCount += node.WorkerCount
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

// getWorkerNodesInfo retrieves information about all registered worker nodes
func getWorkerNodesInfo() *WorkerNodesInfo {
	heartbeatThreshold := 2 * time.Minute

	// Get all worker nodes
	nodes, err := db.Connection().GetAllWorkerNodes()
	if err != nil {
		return nil
	}

	// Get aggregate stats
	dbStats, err := db.Connection().GetWorkerNodeStats()
	if err != nil {
		return nil
	}

	info := &WorkerNodesInfo{
		TotalNodes:     dbStats.TotalNodes,
		RunningNodes:   dbStats.RunningNodes,
		StoppedNodes:   dbStats.StoppedNodes,
		TotalClaimed:   dbStats.TotalClaimed,
		TotalCompleted: dbStats.TotalCompleted,
		TotalFailed:    dbStats.TotalFailed,
		Nodes:          make([]WorkerNodeInfo, 0, len(nodes)),
	}

	for _, node := range nodes {
		isStale := node.Status == db.WorkerNodeStatusRunning && time.Since(node.LastSeenAt) > heartbeatThreshold
		nodeInfo := WorkerNodeInfo{
			ID:            node.ID,
			Hostname:      node.Hostname,
			WorkerCount:   node.WorkerCount,
			Status:        string(node.Status),
			IsStale:       isStale,
			StartedAt:     node.StartedAt,
			LastSeenAt:    node.LastSeenAt,
			JobsClaimed:   node.JobsClaimed,
			JobsCompleted: node.JobsCompleted,
			JobsFailed:    node.JobsFailed,
			Version:       node.Version,
		}

		// Get running jobs for this worker node
		if node.Status == db.WorkerNodeStatusRunning && !isStale {
			nodeInfo.RunningJobs = getRunningJobsForWorker(node.ID)
		}

		info.Nodes = append(info.Nodes, nodeInfo)
	}

	return info
}

// getRunningJobsForWorker retrieves currently running jobs for a specific worker
func getRunningJobsForWorker(workerID string) []RunningJobInfo {
	var jobs []db.ScanJob
	// Match worker_id pattern (worker IDs are prefixed with node ID)
	err := db.Connection().DB().
		Where("worker_id LIKE ? AND status IN ?", workerID+"%", []db.ScanJobStatus{db.ScanJobStatusClaimed, db.ScanJobStatusRunning}).
		Order("started_at ASC").
		Limit(10). // Limit per worker
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

	dashboardPath := viper.GetString("api.dashboard.path")
	if dashboardPath == "" {
		dashboardPath = "/dashboard"
	}

	html := getDashboardTemplate(title, refreshInterval, dashboardPath)
	c.Set("Content-Type", "text/html")
	return c.SendString(html)
}

func getDashboardTemplate(title string, refreshInterval int, dashboardPath string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s</title>
    <style>
        :root {
            --bg-primary: #000000;
            --bg-secondary: #0a0a0a;
            --bg-tertiary: #111111;
            --bg-elevated: #171717;
            --border-subtle: rgba(255,255,255,0.08);
            --border-default: rgba(255,255,255,0.12);
            --text-primary: #ededed;
            --text-secondary: #a1a1a1;
            --text-tertiary: #666666;
            --accent-blue: #0070f3;
            --accent-green: #00d68f;
            --accent-yellow: #f5a623;
            --accent-red: #ee0000;
            --accent-purple: #8b5cf6;
            --radius-sm: 6px;
            --radius-md: 10px;
            --radius-lg: 14px;
            --shadow-sm: 0 1px 2px rgba(0,0,0,0.4);
            --shadow-md: 0 4px 12px rgba(0,0,0,0.5);
            --shadow-lg: 0 8px 30px rgba(0,0,0,0.6);
        }
        
        * { margin: 0; padding: 0; box-sizing: border-box; }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Inter', 'Segoe UI', Roboto, sans-serif;
            background: var(--bg-primary);
            color: var(--text-primary);
            padding: 24px 32px;
            min-height: 100vh;
            line-height: 1.5;
            -webkit-font-smoothing: antialiased;
            -moz-osx-font-smoothing: grayscale;
        }
        
        .header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 32px;
            padding-bottom: 24px;
            border-bottom: 1px solid var(--border-subtle);
        }
        
        h1 {
            font-size: 1.5rem;
            font-weight: 600;
            color: var(--text-primary);
            letter-spacing: -0.02em;
        }
        
        .status-indicator {
            display: flex;
            align-items: center;
            gap: 16px;
        }
        
        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%%;
            background: var(--text-tertiary);
            transition: all 0.3s ease;
        }
        
        .status-dot.active {
            background: var(--accent-green);
            box-shadow: 0 0 12px rgba(0,214,143,0.5);
        }
        
        .status-dot.inactive {
            background: var(--accent-red);
            box-shadow: 0 0 12px rgba(238,0,0,0.4);
        }
        
        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
            gap: 16px;
            margin-bottom: 24px;
        }
        
        .card {
            background: var(--bg-secondary);
            border-radius: var(--radius-lg);
            padding: 20px;
            border: 1px solid var(--border-subtle);
            transition: border-color 0.2s ease, box-shadow 0.2s ease;
        }
        
        .card:hover {
            border-color: var(--border-default);
            box-shadow: var(--shadow-sm);
        }
        
        .card h2 {
            font-size: 0.75rem;
            font-weight: 500;
            color: var(--text-tertiary);
            margin-bottom: 16px;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }
        
        .card h3 {
            font-size: 0.8rem;
            font-weight: 500;
            color: var(--text-tertiary);
            margin: 16px 0 12px 0;
            border-top: 1px solid var(--border-subtle);
            padding-top: 16px;
        }
        
        .stat-grid {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 16px;
        }
        
        /* Tab styles */
        .tabs-container {
            background: var(--bg-secondary);
            border-radius: var(--radius-lg);
            border: 1px solid var(--border-subtle);
            margin-bottom: 24px;
            overflow: hidden;
        }
        
        .tab-header {
            display: flex;
            background: var(--bg-tertiary);
            border-bottom: 1px solid var(--border-subtle);
            padding: 4px;
            gap: 4px;
        }
        
        .tab-btn {
            flex: 1;
            padding: 10px 20px;
            background: transparent;
            border: none;
            border-radius: var(--radius-md);
            color: var(--text-secondary);
            font-size: 0.875rem;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.15s ease;
        }
        
        .tab-btn:hover {
            color: var(--text-primary);
            background: rgba(255,255,255,0.05);
        }
        
        .tab-btn.active {
            color: var(--text-primary);
            background: var(--bg-secondary);
            box-shadow: var(--shadow-sm);
        }
        
        .tab-content {
            display: none;
            padding: 24px;
        }
        
        .tab-content.active {
            display: block;
        }
        
        .stat {
            text-align: center;
            padding: 8px 0;
        }
        
        .stat-value {
            font-size: 1.75rem;
            font-weight: 600;
            color: var(--text-primary);
            letter-spacing: -0.02em;
            font-variant-numeric: tabular-nums;
        }
        
        .stat-label {
            font-size: 0.75rem;
            color: var(--text-tertiary);
            margin-top: 4px;
            font-weight: 500;
        }
        
        .stat-value.pending { color: var(--accent-yellow); }
        .stat-value.running { color: var(--accent-blue); }
        .stat-value.completed { color: var(--accent-green); }
        .stat-value.failed { color: var(--accent-red); }
        
        .scan-list {
            display: flex;
            flex-direction: column;
            gap: 12px;
        }
        
        .scan-item {
            background: var(--bg-tertiary);
            border-radius: var(--radius-md);
            padding: 16px;
            border: 1px solid var(--border-subtle);
            transition: all 0.2s ease;
        }
        
        .scan-item:hover {
            border-color: var(--border-default);
        }
        
        .scan-item.paused { border-left: 3px solid var(--accent-yellow); }
        .scan-item.completed { border-left: 3px solid var(--accent-green); }
        .scan-item:not(.paused):not(.completed) { border-left: 3px solid var(--accent-blue); }
        
        .scan-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 12px;
        }
        
        .scan-title {
            font-weight: 500;
            font-size: 0.9rem;
            cursor: pointer;
            transition: color 0.15s ease;
        }
        
        .scan-title:hover { color: var(--accent-blue); }
        
        .scan-id {
            color: var(--text-tertiary);
            font-size: 0.8rem;
            margin-left: 8px;
            font-variant-numeric: tabular-nums;
        }
        
        .scan-phase {
            background: var(--bg-elevated);
            padding: 4px 10px;
            border-radius: 20px;
            font-size: 0.7rem;
            color: var(--text-secondary);
            font-weight: 500;
            text-transform: uppercase;
            letter-spacing: 0.03em;
        }
        
        .progress-bar {
            background: var(--bg-elevated);
            border-radius: 4px;
            height: 4px;
            margin: 12px 0;
            overflow: hidden;
        }
        
        .progress-fill {
            background: linear-gradient(90deg, var(--accent-blue), var(--accent-green));
            height: 100%%;
            border-radius: 4px;
            transition: width 0.5s cubic-bezier(0.4, 0, 0.2, 1);
        }
        
        .scan-stats {
            display: flex;
            gap: 16px;
            font-size: 0.75rem;
            color: var(--text-tertiary);
            flex-wrap: wrap;
        }
        
        .scan-stats span {
            display: flex;
            align-items: center;
            gap: 4px;
            font-variant-numeric: tabular-nums;
        }
        
        .empty-state {
            text-align: center;
            padding: 48px 24px;
            color: var(--text-tertiary);
            font-size: 0.875rem;
        }
        
        .config-list {
            display: flex;
            flex-wrap: wrap;
            gap: 8px;
        }
        
        .config-item {
            background: var(--bg-elevated);
            padding: 6px 12px;
            border-radius: 20px;
            font-size: 0.75rem;
            font-weight: 500;
            color: var(--text-secondary);
            border: 1px solid var(--border-subtle);
        }
        
        .config-item.enabled {
            background: rgba(0,214,143,0.1);
            color: var(--accent-green);
            border-color: rgba(0,214,143,0.2);
        }
        
        .config-item.disabled {
            background: rgba(238,0,0,0.1);
            color: var(--accent-red);
            border-color: rgba(238,0,0,0.2);
        }
        
        .timestamp {
            color: var(--text-tertiary);
            font-size: 0.8rem;
            font-variant-numeric: tabular-nums;
        }
        
        @keyframes pulse {
            0%%, 100%% { opacity: 1; }
            50%% { opacity: 0.4; }
        }
        
        .loading { animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite; }
        
        /* Job type breakdown styles */
        .job-type-breakdown { margin-top: 16px; }
        
        .job-type-row {
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 10px 12px;
            background: var(--bg-tertiary);
            border-radius: var(--radius-sm);
            margin-bottom: 6px;
            font-size: 0.8rem;
            border: 1px solid var(--border-subtle);
        }
        
        .job-type-name {
            font-weight: 500;
            min-width: 120px;
            color: var(--text-secondary);
        }
        
        .job-type-stats {
            display: flex;
            gap: 16px;
        }
        
        .job-type-stats span {
            display: flex;
            align-items: center;
            gap: 4px;
            font-variant-numeric: tabular-nums;
        }
        
        .job-type-stats .pending { color: var(--accent-yellow); }
        .job-type-stats .running { color: var(--accent-blue); }
        .job-type-stats .completed { color: var(--accent-green); }
        .job-type-stats .failed { color: var(--accent-red); }
        
        /* Running jobs table */
        .running-jobs { margin-top: 12px; }
        
        .running-job {
            display: grid;
            grid-template-columns: 60px 1fr 80px 100px;
            padding: 10px 12px;
            background: var(--bg-tertiary);
            border-radius: var(--radius-sm);
            margin-bottom: 4px;
            font-size: 0.75rem;
            gap: 12px;
            align-items: center;
            border: 1px solid var(--border-subtle);
        }
        
        .running-job .job-id { color: var(--text-tertiary); font-variant-numeric: tabular-nums; }
        .running-job .job-url {
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
            color: var(--text-secondary);
        }
        .running-job .job-type { color: var(--accent-blue); font-weight: 500; }
        .running-job .job-elapsed { color: var(--accent-yellow); text-align: right; font-variant-numeric: tabular-nums; }
        
        /* Failed jobs */
        .failed-jobs { margin-top: 12px; }
        
        .failed-job {
            padding: 10px 12px;
            background: rgba(238,0,0,0.05);
            border-radius: var(--radius-sm);
            margin-bottom: 6px;
            font-size: 0.75rem;
            border: 1px solid rgba(238,0,0,0.15);
        }
        
        .failed-job-header { display: flex; justify-content: space-between; margin-bottom: 4px; }
        .failed-job .job-url { color: var(--text-secondary); }
        .failed-job .job-error { color: var(--accent-red); font-size: 0.7rem; margin-top: 6px; opacity: 0.9; }
        
        /* Collapsible sections */
        .collapsible-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            cursor: pointer;
            padding: 6px 0;
        }
        
        .collapsible-header:hover { color: var(--accent-blue); }
        .collapsible-content { display: none; }
        .collapsible-content.show { display: block; }
        .toggle-icon { transition: transform 0.2s ease; }
        .toggle-icon.expanded { transform: rotate(90deg); }
        
        /* Global stats by type */
        .global-type-stats { margin-top: 16px; }
        
        .type-stat-bar {
            display: flex;
            align-items: center;
            margin-bottom: 10px;
            font-size: 0.8rem;
        }
        
        .type-stat-bar .type-name {
            min-width: 100px;
            color: var(--text-secondary);
            font-weight: 500;
        }
        
        .type-stat-bar .bar-container {
            flex: 1;
            height: 6px;
            background: var(--bg-elevated);
            border-radius: 3px;
            overflow: hidden;
            margin: 0 12px;
            display: flex;
        }
        
        .type-stat-bar .bar-segment {
            height: 100%%;
            transition: width 0.4s cubic-bezier(0.4, 0, 0.2, 1);
        }
        
        .type-stat-bar .bar-segment.completed { background: var(--accent-green); }
        .type-stat-bar .bar-segment.running { background: var(--accent-blue); }
        .type-stat-bar .bar-segment.pending { background: var(--accent-yellow); }
        .type-stat-bar .bar-segment.failed { background: var(--accent-red); }
        .type-stat-bar .type-count { min-width: 50px; text-align: right; color: var(--text-tertiary); font-variant-numeric: tabular-nums; }
        
        /* Worker nodes styles */
        .worker-summary {
            display: flex;
            gap: 16px;
            margin-bottom: 16px;
            font-size: 0.85rem;
            padding-bottom: 12px;
            border-bottom: 1px solid var(--border-subtle);
        }
        
        .worker-stat strong { color: var(--accent-blue); }
        .worker-list { display: flex; flex-direction: column; gap: 10px; }
        
        .worker-node {
            background: var(--bg-tertiary);
            border-radius: var(--radius-md);
            padding: 14px;
            border: 1px solid var(--border-subtle);
            transition: all 0.2s ease;
        }
        
        .worker-node:hover { border-color: var(--border-default); }
        .worker-node.stopped { opacity: 0.6; }
        .worker-node.stale { border-color: rgba(238,0,0,0.3); }
        .worker-node.running { border-left: 3px solid var(--accent-green); }
        
        .worker-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; }
        .worker-id { font-weight: 500; font-size: 0.85rem; }
        
        .worker-status {
            padding: 3px 10px;
            border-radius: 20px;
            font-size: 0.65rem;
            font-weight: 600;
            text-transform: uppercase;
            letter-spacing: 0.03em;
        }
        
        .worker-status.running { background: rgba(0,214,143,0.15); color: var(--accent-green); }
        .worker-status.stopped { background: var(--bg-elevated); color: var(--text-tertiary); }
        .worker-status.stale { background: rgba(238,0,0,0.15); color: var(--accent-red); }
        
        .worker-details { display: flex; gap: 16px; font-size: 0.75rem; color: var(--text-tertiary); flex-wrap: wrap; }
        .current-node { background: rgba(0,112,243,0.08); border-color: rgba(0,112,243,0.3); }
        
        /* Worker summary card (compact) */
        .worker-stats-compact { display: flex; gap: 20px; flex-wrap: wrap; }
        .worker-stats-compact .stat { text-align: center; flex: 1; min-width: 70px; }
        
        /* Full worker details in tab */
        .worker-node-full {
            background: var(--bg-tertiary);
            border-radius: var(--radius-md);
            padding: 20px;
            margin-bottom: 16px;
            border: 1px solid var(--border-subtle);
            transition: all 0.2s ease;
        }
        
        .worker-node-full:hover { border-color: var(--border-default); }
        .worker-node-full.stopped { opacity: 0.6; }
        .worker-node-full.stale { border-color: rgba(238,0,0,0.3); }
        .worker-node-full.running { border-left: 3px solid var(--accent-green); }
        
        .worker-node-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
        .worker-node-id { font-weight: 600; font-size: 0.95rem; letter-spacing: -0.01em; }
        .worker-node-hostname { color: var(--text-tertiary); font-size: 0.8rem; margin-left: 10px; }
        
        .worker-node-stats {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(100px, 1fr));
            gap: 16px;
            margin-top: 16px;
            padding-top: 16px;
            border-top: 1px solid var(--border-subtle);
        }
        
        .worker-node-stat { text-align: center; }
        .worker-node-stat-value { font-size: 1.25rem; font-weight: 600; color: var(--text-primary); font-variant-numeric: tabular-nums; }
        .worker-node-stat-label { font-size: 0.7rem; color: var(--text-tertiary); margin-top: 4px; font-weight: 500; }
        
        .worker-meta {
            display: flex;
            gap: 16px;
            font-size: 0.8rem;
            color: var(--text-tertiary);
            flex-wrap: wrap;
            margin-top: 12px;
        }
        
        .worker-meta span { display: flex; align-items: center; gap: 6px; }
        
        /* Worker running jobs in full view */
        .worker-running-jobs {
            margin-top: 16px;
            padding-top: 16px;
            border-top: 1px solid var(--border-subtle);
        }
        
        .worker-running-jobs.empty { opacity: 0.6; }
        
        .worker-jobs-header {
            font-size: 0.85rem;
            font-weight: 500;
            color: var(--text-secondary);
            margin-bottom: 12px;
        }
        
        .worker-jobs-list { display: flex; flex-direction: column; gap: 8px; }
        
        .worker-job-item {
            background: var(--bg-secondary);
            border-radius: var(--radius-sm);
            padding: 12px 14px;
            border: 1px solid var(--border-subtle);
            transition: all 0.15s ease;
        }
        
        .worker-job-item:hover { border-color: var(--border-default); }
        
        .worker-job-main {
            display: flex;
            align-items: center;
            gap: 10px;
            margin-bottom: 8px;
        }
        
        .worker-job-type {
            background: rgba(0,112,243,0.15);
            padding: 3px 10px;
            border-radius: 20px;
            font-size: 0.65rem;
            color: var(--accent-blue);
            font-weight: 600;
            text-transform: uppercase;
            letter-spacing: 0.02em;
        }
        
        .worker-job-url {
            flex: 1;
            font-size: 0.8rem;
            color: var(--text-secondary);
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }
        
        .worker-job-meta {
            display: flex;
            gap: 16px;
            font-size: 0.7rem;
            color: var(--text-tertiary);
        }
        
        .worker-job-id { color: var(--text-tertiary); font-variant-numeric: tabular-nums; }
        
        .worker-job-method {
            background: var(--bg-elevated);
            padding: 2px 8px;
            border-radius: 4px;
            font-weight: 600;
            font-size: 0.65rem;
        }
        
        .worker-job-elapsed { color: var(--accent-yellow); font-variant-numeric: tabular-nums; }
        .worker-job-worker { color: var(--text-tertiary); }
        
        /* Section headers in tabs */
        .section-header {
            font-size: 1.1rem;
            font-weight: 600;
            color: var(--text-primary);
            margin-bottom: 20px;
            padding-bottom: 12px;
            border-bottom: 1px solid var(--border-subtle);
            letter-spacing: -0.01em;
        }
        
        .sub-section { margin-bottom: 32px; }
        
        .sub-section h3 {
            font-size: 0.75rem;
            font-weight: 500;
            color: var(--text-tertiary);
            margin-bottom: 16px;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }
        
        /* Scrollbar styling */
        ::-webkit-scrollbar { width: 8px; height: 8px; }
        ::-webkit-scrollbar-track { background: var(--bg-primary); }
        ::-webkit-scrollbar-thumb { background: var(--bg-elevated); border-radius: 4px; }
        ::-webkit-scrollbar-thumb:hover { background: var(--text-tertiary); }
    </style>
</head>
<body>
    <div class="header">
        <h1>%s</h1>
        <div class="status-indicator">
            <span class="timestamp" id="last-update">—</span>
            <div class="status-dot" id="manager-status" title="Scan Manager"></div>
            <div class="status-dot" id="orchestrator-status" title="Orchestrator"></div>
        </div>
    </div>

    <!-- First Row: Metrics Cards -->
    <div class="grid">
        <div class="card">
            <h2>System Status</h2>
            <div class="stat-grid">
                <div class="stat">
                    <div class="stat-value" id="worker-count">—</div>
                    <div class="stat-label">Total Workers</div>
                </div>
                <div class="stat">
                    <div class="stat-value" id="local-worker-count">—</div>
                    <div class="stat-label">Local Workers</div>
                </div>
                <div class="stat">
                    <div class="stat-value" id="active-scans-count">—</div>
                    <div class="stat-label">Active Scans</div>
                </div>
                <div class="stat">
                    <div class="stat-value pending" id="paused-scans-count">—</div>
                    <div class="stat-label">Paused Scans</div>
                </div>
                <div class="stat">
                    <div class="stat-value" id="db-size">—</div>
                    <div class="stat-label">Database Size</div>
                </div>
            </div>
        </div>

        <div class="card">
            <h2>Global Queue</h2>
            <div class="stat-grid">
                <div class="stat">
                    <div class="stat-value pending" id="global-pending">—</div>
                    <div class="stat-label">Pending</div>
                </div>
                <div class="stat">
                    <div class="stat-value running" id="global-running">—</div>
                    <div class="stat-label">Running</div>
                </div>
                <div class="stat">
                    <div class="stat-value completed" id="global-completed">—</div>
                    <div class="stat-label">Completed</div>
                </div>
                <div class="stat">
                    <div class="stat-value failed" id="global-failed">—</div>
                    <div class="stat-label">Failed</div>
                </div>
            </div>
            <div class="global-type-stats" id="global-type-stats"></div>
        </div>

        <div class="card">
            <h2>Orchestrator</h2>
            <div class="config-list" id="orchestrator-config">
                <div class="config-item loading">—</div>
            </div>
        </div>

        <div class="card">
            <h2>Worker Nodes</h2>
            <div class="worker-info">
                <div style="font-size: 0.75rem; color: var(--text-tertiary); margin-bottom: 12px;">
                    Current Node: <span id="current-node-id" style="color: var(--accent-blue); font-weight: 500;">—</span>
                </div>
            </div>
            <div class="worker-stats-compact" id="worker-stats-compact">
                <div class="stat">
                    <div class="stat-value" id="running-nodes">—</div>
                    <div class="stat-label">Running</div>
                </div>
                <div class="stat">
                    <div class="stat-value" id="stopped-nodes">—</div>
                    <div class="stat-label">Stopped</div>
                </div>
                <div class="stat">
                    <div class="stat-value completed" id="total-jobs-completed">—</div>
                    <div class="stat-label">Completed</div>
                </div>
                <div class="stat">
                    <div class="stat-value failed" id="total-jobs-failed">—</div>
                    <div class="stat-label">Failed</div>
                </div>
            </div>
        </div>
    </div>

    <!-- Tab-based Content -->
    <div class="tabs-container">
        <div class="tab-header">
            <button class="tab-btn" onclick="switchTab('scans')">Scans</button>
            <button class="tab-btn active" onclick="switchTab('workers')">Workers</button>
        </div>
        
        <!-- Scans Tab -->
        <div id="tab-scans" class="tab-content">
            <div class="sub-section">
                <h3>Active Scans</h3>
                <div class="scan-list" id="active-scans">
                    <div class="empty-state loading">Loading...</div>
                </div>
            </div>
            
            <div class="grid" style="margin-bottom: 0;">
                <div class="sub-section">
                    <h3>Paused Scans</h3>
                    <div class="scan-list" id="paused-scans">
                        <div class="empty-state">No paused scans</div>
                    </div>
                </div>

                <div class="sub-section">
                    <h3>Recent Completed</h3>
                    <div class="scan-list" id="recent-scans">
                        <div class="empty-state">No recent scans</div>
                    </div>
                </div>
            </div>
        </div>
        
        <!-- Workers Tab -->
        <div id="tab-workers" class="tab-content active">
            <div class="section-header">Worker Nodes</div>
            <div id="worker-nodes-full">
                <div class="empty-state loading">Loading...</div>
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
        
        function switchTab(tabName) {
            // Update tab buttons
            document.querySelectorAll('.tab-btn').forEach(btn => btn.classList.remove('active'));
            document.querySelector(`+"`"+`.tab-btn[onclick="switchTab('${tabName}')"]`+"`"+`).classList.add('active');
            
            // Update tab content
            document.querySelectorAll('.tab-content').forEach(content => content.classList.remove('active'));
            document.getElementById(`+"`"+`tab-${tabName}`+"`"+`).classList.add('active');
        }

        function toggleScanDetails(scanId) {
            if (expandedScans.has(scanId)) {
                expandedScans.delete(scanId);
            } else {
                expandedScans.add(scanId);
            }
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
            if (!jobs || jobs.length === 0) return '<div style="color: var(--text-tertiary); font-size: 0.85rem; padding: 10px;">No jobs currently running</div>';
            return `+"`"+`
                <div class="running-jobs">
                    ${jobs.slice(0, 10).map(j => `+"`"+`
                        <div class="running-job">
                            <span class="job-id">#${j.job_id}</span>
                            <span class="job-url" title="${j.url || ''}">${truncateUrl(j.url)}</span>
                            <span class="job-type">${formatJobType(j.job_type)}</span>
                            <span class="job-elapsed">${j.elapsed_time || '—'}</span>
                        </div>
                    `+"`"+`).join('')}
                    ${jobs.length > 10 ? `+"`"+`<div style="color: var(--text-tertiary); font-size: 0.8rem; text-align: center; padding: 5px;">... and ${jobs.length - 10} more</div>`+"`"+` : ''}
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
                                <span style="color: var(--text-tertiary);">${formatJobType(j.job_type)}</span>
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
                        <span>${progress}%%</span>
                        <span>${scan.duration || '—'}</span>
                        <span>${scan.completed_jobs}/${scan.total_jobs} jobs</span>
                        <span style="color: var(--accent-red);">${scan.failed_jobs} failed</span>
                        <span style="color: var(--accent-blue);">${scan.running_jobs} running</span>
                        <span style="color: var(--accent-yellow);">${scan.pending_jobs} pending</span>
                    </div>
                    ${hasDetails ? `+"`"+`
                        <div id="scan-details-${scan.id}" class="collapsible-content ${isExpanded ? 'show' : ''}">
                            ${scan.job_stats_by_type?.length > 0 ? `+"`"+`
                                <h3>Jobs by Type</h3>
                                ${renderJobTypeBreakdown(scan.job_stats_by_type)}
                            `+"`"+` : ''}
                            ${scan.running_job_details?.length > 0 ? `+"`"+`
                                <h3>Currently Running (${scan.running_job_details.length})</h3>
                                ${renderRunningJobs(scan.running_job_details)}
                            `+"`"+` : ''}
                            ${scan.recent_failed_jobs?.length > 0 ? `+"`"+`
                                <h3>Recent Failures</h3>
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

            document.getElementById('worker-count').textContent = data.total_worker_count || data.worker_count || 0;
            document.getElementById('local-worker-count').textContent = data.local_worker_count || data.worker_count || 0;
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

            // Render worker nodes
            renderWorkerNodesCompact(data);
            renderWorkerNodesFull(data);
        }

        function renderWorkerNodesCompact(data) {
            if (!data.worker_nodes) return;
            
            const wn = data.worker_nodes;
            document.getElementById('running-nodes').textContent = wn.running_nodes || 0;
            document.getElementById('stopped-nodes').textContent = wn.stopped_nodes || 0;
            document.getElementById('total-jobs-completed').textContent = formatNumber(wn.total_completed || 0);
            document.getElementById('total-jobs-failed').textContent = formatNumber(wn.total_failed || 0);

            // Update current node indicator
            const nodeId = document.getElementById('current-node-id');
            if (nodeId && data.node_id) {
                nodeId.textContent = data.node_id;
            }
        }

        function renderWorkerNodesFull(data) {
            const container = document.getElementById('worker-nodes-full');
            if (!container) return;
            
            if (!data.worker_nodes || data.worker_nodes.nodes.length === 0) {
                container.innerHTML = '<div class="empty-state">No worker nodes registered</div>';
                return;
            }

            const wn = data.worker_nodes;
            let html = `+"`"+`
                <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 15px; margin-bottom: 25px; padding: 20px; background: var(--bg-secondary); border-radius: 12px; border: 1px solid var(--border-subtle);">
                    <div class="stat">
                        <div class="stat-value">${wn.total_nodes}</div>
                        <div class="stat-label">Total Nodes</div>
                    </div>
                    <div class="stat">
                        <div class="stat-value completed">${wn.running_nodes}</div>
                        <div class="stat-label">Running</div>
                    </div>
                    <div class="stat">
                        <div class="stat-value" style="color: var(--text-tertiary);">${wn.stopped_nodes}</div>
                        <div class="stat-label">Stopped</div>
                    </div>
                    <div class="stat">
                        <div class="stat-value running">${formatNumber(wn.total_claimed)}</div>
                        <div class="stat-label">Jobs Claimed</div>
                    </div>
                    <div class="stat">
                        <div class="stat-value completed">${formatNumber(wn.total_completed)}</div>
                        <div class="stat-label">Jobs Completed</div>
                    </div>
                    <div class="stat">
                        <div class="stat-value failed">${formatNumber(wn.total_failed)}</div>
                        <div class="stat-label">Jobs Failed</div>
                    </div>
                </div>
            `+"`"+`;

            for (const node of wn.nodes) {
                const statusClass = node.is_stale ? 'stale' : node.status;
                const statusText = node.is_stale ? 'STALE' : node.status.toUpperCase();
                const lastSeen = new Date(node.last_seen_at).toLocaleTimeString();
                const startedAt = new Date(node.started_at).toLocaleString();
                const isCurrentNode = data.node_id === node.id;
                const runningJobsCount = node.running_jobs?.length || 0;
                
                html += `+"`"+`
                    <div class="worker-node-full ${statusClass} ${isCurrentNode ? 'current-node' : ''}">
                        <div class="worker-node-header">
                            <div>
                                <span class="worker-node-id">${node.id}</span>
                                ${isCurrentNode ? '<span style="color: var(--accent-blue); font-size: 0.75rem; margin-left: 8px;">(current)</span>' : ''}
                                <span class="worker-node-hostname">${node.hostname || '-'}</span>
                            </div>
                            <span class="worker-status ${statusClass}">${statusText}</span>
                        </div>
                        <div class="worker-meta">
                            <span>Workers: ${node.worker_count}</span>
                            <span>Started: ${startedAt}</span>
                            <span>Last seen: ${lastSeen}</span>
                            ${node.version ? `+"`"+`<span>v${node.version}</span>`+"`"+` : ''}
                        </div>
                        <div class="worker-node-stats">
                            <div class="worker-node-stat">
                                <div class="worker-node-stat-value running">${runningJobsCount}</div>
                                <div class="worker-node-stat-label">Active Jobs</div>
                            </div>
                            <div class="worker-node-stat">
                                <div class="worker-node-stat-value" style="color: var(--accent-blue);">${node.jobs_claimed}</div>
                                <div class="worker-node-stat-label">Total Claimed</div>
                            </div>
                            <div class="worker-node-stat">
                                <div class="worker-node-stat-value completed">${node.jobs_completed}</div>
                                <div class="worker-node-stat-label">Completed</div>
                            </div>
                            <div class="worker-node-stat">
                                <div class="worker-node-stat-value failed">${node.jobs_failed}</div>
                                <div class="worker-node-stat-label">Failed</div>
                            </div>
                            <div class="worker-node-stat">
                                <div class="worker-node-stat-value" style="color: ${node.jobs_claimed > 0 ? 'var(--accent-green)' : 'var(--text-tertiary)'};">
                                    ${node.jobs_claimed > 0 ? ((node.jobs_completed / node.jobs_claimed) * 100).toFixed(1) : 0}%%
                                </div>
                                <div class="worker-node-stat-label">Success Rate</div>
                            </div>
                        </div>
                        ${runningJobsCount > 0 ? `+"`"+`
                            <div class="worker-running-jobs">
                                <div class="worker-jobs-header">Currently Processing (${runningJobsCount})</div>
                                <div class="worker-jobs-list">
                                    ${node.running_jobs.map(job => `+"`"+`
                                        <div class="worker-job-item">
                                            <div class="worker-job-main">
                                                <span class="worker-job-type">${formatJobType(job.job_type)}</span>
                                                <span class="worker-job-url" title="${job.url || ''}">${truncateUrl(job.url, 80)}</span>
                                            </div>
                                            <div class="worker-job-meta">
                                                <span class="worker-job-id">#${job.job_id}</span>
                                                ${job.method ? `+"`"+`<span class="worker-job-method">${job.method}</span>`+"`"+` : ''}
                                                <span class="worker-job-elapsed">${job.elapsed_time || '—'}</span>
                                                <span class="worker-job-worker" title="${job.worker_id}">${job.worker_id?.split('-').pop() || '—'}</span>
                                            </div>
                                        </div>
                                    `+"`"+`).join('')}
                                </div>
                            </div>
                        `+"`"+` : `+"`"+`
                            <div class="worker-running-jobs empty">
                                <div class="worker-jobs-header" style="color: var(--text-tertiary);">No active jobs</div>
                            </div>
                        `+"`"+`}
                    </div>
                `+"`"+`;
            }
            container.innerHTML = html;
        }

        async function fetchStats() {
            try {
                const response = await fetch('%s/stats');
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
</html>`, title, title, refreshInterval, dashboardPath)
}

// GetDashboardStatsHandler is a wrapper handler that retrieves the scan manager and calls GetDashboardStats
func GetDashboardStatsHandler(c *fiber.Ctx) error {
	return GetDashboardStats(c, GetScanManager())
}

// DashboardHTMLHandler is a wrapper handler that retrieves the scan manager and calls DashboardHTML
func DashboardHTMLHandler(c *fiber.Ctx) error {
	return DashboardHTML(c, GetScanManager())
}
