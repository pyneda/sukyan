// Package api provides the REST API handlers for Sukyan.
package api

import (
	"bytes"
	"embed"
	"html/template"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/scan/manager"
	"github.com/spf13/viper"
)

//go:embed templates/*.html
var templateFS embed.FS

// DashboardTemplateData holds the data for rendering the dashboard template
type DashboardTemplateData struct {
	Title           string
	RefreshInterval int
	DashboardPath   string
}

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

	// Throughput metrics
	Throughput *ThroughputInfo `json:"throughput,omitempty"`

	// Job duration statistics
	DurationStats []JobDurationInfo `json:"duration_stats,omitempty"`

	// System stats
	SystemStats *db.SystemStats `json:"system_stats,omitempty"`
}

// ThroughputInfo holds job throughput metrics for the dashboard
type ThroughputInfo struct {
	LastMinute    int64   `json:"last_minute"`
	Last5Minutes  int64   `json:"last_5_minutes"`
	LastHour      int64   `json:"last_hour"`
	JobsPerMinute float64 `json:"jobs_per_minute"`
	SuccessRate   float64 `json:"success_rate"`
	QueueDepth    int64   `json:"queue_depth"`
	InFlight      int64   `json:"in_flight"`
}

// JobDurationInfo holds job duration statistics for a job type
type JobDurationInfo struct {
	JobType     string  `json:"job_type"`
	Count       int64   `json:"count"`
	AvgDuration float64 `json:"avg_duration_ms"`
	MinDuration float64 `json:"min_duration_ms"`
	MaxDuration float64 `json:"max_duration_ms"`
	P50Duration float64 `json:"p50_duration_ms"`
	P95Duration float64 `json:"p95_duration_ms"`
	P99Duration float64 `json:"p99_duration_ms"`
}

// WorkerNodesInfo holds information about distributed worker nodes
type WorkerNodesInfo struct {
	TotalNodes     int              `json:"total_nodes"`
	RunningNodes   int              `json:"running_nodes"`
	StoppedNodes   int              `json:"stopped_nodes"`
	TotalWorkers   int              `json:"total_workers"`
	ActiveWorkers  int              `json:"active_workers"`
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

	// Throughput metrics
	throughput, err := db.Connection().GetJobThroughputStats()
	if err == nil {
		stats.Throughput = &ThroughputInfo{
			LastMinute:    throughput.LastMinute,
			Last5Minutes:  throughput.Last5Minutes,
			LastHour:      throughput.LastHour,
			JobsPerMinute: throughput.JobsPerMinute,
			SuccessRate:   throughput.SuccessRate,
			QueueDepth:    throughput.QueueDepth,
			InFlight:      throughput.InFlight,
		}
	}

	// Job duration statistics
	durationStats, err := db.Connection().GetJobDurationStats()
	if err == nil {
		stats.DurationStats = make([]JobDurationInfo, 0, len(durationStats))
		for _, ds := range durationStats {
			stats.DurationStats = append(stats.DurationStats, JobDurationInfo{
				JobType:     ds.JobType,
				Count:       ds.Count,
				AvgDuration: ds.AvgDuration,
				MinDuration: ds.MinDuration,
				MaxDuration: ds.MaxDuration,
				P50Duration: ds.P50Duration,
				P95Duration: ds.P95Duration,
				P99Duration: ds.P99Duration,
			})
		}
	}

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
		TotalWorkers:   dbStats.TotalWorkers,
		ActiveWorkers:  dbStats.ActiveWorkers,
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

	tmpl, err := template.ParseFS(templateFS, "templates/dashboard.html")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load dashboard template: " + err.Error())
	}

	data := DashboardTemplateData{
		Title:           title,
		RefreshInterval: refreshInterval,
		DashboardPath:   dashboardPath,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to render dashboard: " + err.Error())
	}

	c.Set("Content-Type", "text/html")
	return c.SendString(buf.String())
}

// GetDashboardStatsHandler is a wrapper handler that retrieves the scan manager and calls GetDashboardStats
func GetDashboardStatsHandler(c *fiber.Ctx) error {
	return GetDashboardStats(c, GetScanManager())
}

// DashboardHTMLHandler is a wrapper handler that retrieves the scan manager and calls DashboardHTML
func DashboardHTMLHandler(c *fiber.Ctx) error {
	return DashboardHTML(c, GetScanManager())
}
