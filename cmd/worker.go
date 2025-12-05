package cmd

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan"
	"github.com/pyneda/sukyan/pkg/scan/manager"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	workerCount       int
	workerNodeID      string
	workerNodePrefix  string
	disableInteractsh bool
)

// workerCmd represents the worker command
var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Manage standalone scan workers",
	Long: `Run standalone scan workers that connect to the database and process scan jobs.

This is useful for:
- Scaling scan capacity on the same machine
- Distributing scan workers across multiple machines
- Running workers separately from the API server

Workers automatically register themselves and maintain heartbeats for monitoring.
Multiple worker processes can run simultaneously, competing for jobs via the queue.`,
}

// workerStartCmd represents the worker start command
var workerStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start standalone scan workers",
	Long: `Start one or more standalone scan workers that process scan jobs from the queue.

Examples:
  # Start with default settings (5 workers)
  sukyan worker start

  # Start 10 workers with a custom node ID
  sukyan worker start --workers 10 --id "worker-node-2"

  # Start workers with a custom prefix
  sukyan worker start --workers 5 --prefix "scanner"`,
	Run: runWorkerStart,
}

// workerStatusCmd represents the worker status command
var workerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of all registered worker nodes",
	Run:   runWorkerStatus,
}

// workerCleanupCmd represents the worker cleanup command
var workerCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Cleanup stale workers and reset their claimed jobs",
	Long: `Identifies worker nodes that haven't sent a heartbeat within the threshold,
marks them as stopped, and resets any jobs they had claimed back to pending status.

This is useful for:
- Recovering from crashed worker processes
- Resetting stuck jobs after network issues
- Manual cleanup of abandoned workers`,
	Run: runWorkerCleanup,
}

var (
	pruneAge string
)

// workerPruneCmd represents the worker prune command
var workerPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Delete old stopped worker nodes from the database",
	Long: `Removes worker node records that have been stopped for longer than the specified age.
This cleans up historical worker entries that accumulate over time.

By default, removes workers stopped for more than 24 hours.

Examples:
  # Prune workers stopped for more than 24 hours (default)
  sukyan worker prune

  # Prune workers stopped for more than 1 hour
  sukyan worker prune --age 1h

  # Prune workers stopped for more than 7 days
  sukyan worker prune --age 168h`,
	Run: runWorkerPrune,
}

func init() {
	rootCmd.AddCommand(workerCmd)
	workerCmd.AddCommand(workerStartCmd)
	workerCmd.AddCommand(workerStatusCmd)
	workerCmd.AddCommand(workerCleanupCmd)
	workerCmd.AddCommand(workerPruneCmd)

	// Flags for worker start
	workerStartCmd.Flags().IntVarP(&workerCount, "workers", "w", 5, "Number of workers to start")
	workerStartCmd.Flags().StringVar(&workerNodeID, "id", "", "Custom node ID (auto-generated if not set)")
	workerStartCmd.Flags().StringVar(&workerNodePrefix, "prefix", "worker", "Prefix for auto-generated node ID")
	workerStartCmd.Flags().BoolVar(&disableInteractsh, "no-interactsh", false, "Disable interactsh (OOB testing)")

	// Flags for worker prune
	workerPruneCmd.Flags().StringVar(&pruneAge, "age", "24h", "Delete workers stopped for longer than this duration")
}

func runWorkerStart(cmd *cobra.Command, args []string) {
	logger := log.With().Str("component", "worker-cli").Logger()

	logger.Info().
		Int("workers", workerCount).
		Str("node_id", workerNodeID).
		Str("prefix", workerNodePrefix).
		Msg("Starting standalone workers")

	// Initialize database connection (done lazily on first Connection() call)
	_ = db.Connection()
	logger.Info().Msg("Database connected")

	// Load payload generators
	generators, err := generation.LoadGenerators(viper.GetString("generators.directory"))
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load generators")
		os.Exit(1)
	}
	logger.Info().Int("count", len(generators)).Msg("Loaded payload generators")

	// Initialize interactions manager (optional)
	var interactionsManager *integrations.InteractionsManager
	if !disableInteractsh {
		oobPollingInterval := time.Duration(viper.GetInt("scan.oob.poll_interval"))
		oobKeepAliveInterval := time.Duration(viper.GetInt("scan.oob.keep_alive_interval"))
		oobSessionFile := viper.GetString("scan.oob.session_file")

		interactionsManager = &integrations.InteractionsManager{
			GetAsnInfo:            false,
			PollingInterval:       oobPollingInterval * time.Second,
			KeepAliveInterval:     oobKeepAliveInterval * time.Second,
			SessionFile:           oobSessionFile,
			OnInteractionCallback: scan.SaveInteractionCallback,
		}
		interactionsManager.OnEvictionCallback = func() {
			logger.Warn().Msg("Interactsh correlation ID evicted, restarting client")
			interactionsManager.Restart()
		}
		interactionsManager.Start()
		logger.Info().Msg("Interactsh manager started")
	} else {
		logger.Warn().Msg("Interactsh disabled - OOB testing will not work")
	}

	// Create scan manager config
	cfg := manager.DefaultConfig()
	cfg.WorkerCount = workerCount
	if workerNodeID != "" {
		cfg.WorkerIDPrefix = workerNodeID
	} else {
		cfg.WorkerIDPrefix = workerNodePrefix
	}

	// Create and start scan manager
	sm := manager.New(cfg, db.Connection(), interactionsManager, generators)
	if err := sm.Start(); err != nil {
		logger.Fatal().Err(err).Msg("Failed to start scan manager")
	}

	nodeID := sm.NodeID()
	logger.Info().
		Str("node_id", nodeID).
		Int("workers", workerCount).
		Msg("Workers started successfully")

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	logger.Info().Msg("Press Ctrl+C to stop workers")

	sig := <-sigCh
	logger.Info().Str("signal", sig.String()).Msg("Received shutdown signal")

	// Graceful shutdown
	logger.Info().Msg("Shutting down workers...")
	sm.Stop()

	if interactionsManager != nil {
		interactionsManager.Stop()
	}

	logger.Info().Msg("Workers stopped successfully")
}

func runWorkerStatus(cmd *cobra.Command, args []string) {
	logger := log.With().Str("component", "worker-cli").Logger()

	// Initialize database connection (done lazily on first Connection() call)
	_ = db.Connection()

	// Get all worker nodes
	nodes, err := db.Connection().GetAllWorkerNodes()
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to get worker nodes")
	}

	if len(nodes) == 0 {
		logger.Info().Msg("No worker nodes registered")
		return
	}

	// Get stats
	stats, err := db.Connection().GetWorkerNodeStats()
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to get worker stats")
	}

	// Print summary
	logger.Info().
		Int("total", stats.TotalNodes).
		Int("running", stats.RunningNodes).
		Int("stopped", stats.StoppedNodes).
		Int64("jobs_claimed", stats.TotalClaimed).
		Int64("jobs_completed", stats.TotalCompleted).
		Int64("jobs_failed", stats.TotalFailed).
		Msg("Worker node summary")

	// Print each node
	heartbeatThreshold := 2 * time.Minute
	for _, node := range nodes {
		isStale := time.Since(node.LastSeenAt) > heartbeatThreshold
		staleIndicator := ""
		if isStale && node.Status == db.WorkerNodeStatusRunning {
			staleIndicator = " (STALE)"
		}

		logger.Info().
			Str("id", node.ID).
			Str("hostname", node.Hostname).
			Str("status", string(node.Status)+staleIndicator).
			Int("workers", node.WorkerCount).
			Time("started_at", node.StartedAt).
			Time("last_seen", node.LastSeenAt).
			Int("claimed", node.JobsClaimed).
			Int("completed", node.JobsCompleted).
			Int("failed", node.JobsFailed).
			Str("version", node.Version).
			Msg("Worker node")
	}
}

func runWorkerCleanup(cmd *cobra.Command, args []string) {
	logger := log.With().Str("component", "worker-cli").Logger()

	// Initialize database connection
	_ = db.Connection()

	heartbeatThreshold := 2 * time.Minute

	logger.Info().
		Dur("threshold", heartbeatThreshold).
		Msg("Cleaning up stale workers")

	resetCount, err := db.Connection().ResetJobsFromStaleWorkers(heartbeatThreshold)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to cleanup stale workers")
	}

	logger.Info().
		Int64("jobs_reset", resetCount).
		Msg("Stale workers cleaned up successfully")
}

func runWorkerPrune(cmd *cobra.Command, args []string) {
	logger := log.With().Str("component", "worker-cli").Logger()

	// Parse age duration
	age, err := time.ParseDuration(pruneAge)
	if err != nil {
		logger.Fatal().Err(err).Str("age", pruneAge).Msg("Invalid age duration format")
	}

	// Initialize database connection
	_ = db.Connection()

	logger.Info().
		Dur("age", age).
		Msg("Pruning old stopped workers")

	deletedCount, err := db.Connection().DeleteOldWorkerNodes(age)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to prune old workers")
	}

	logger.Info().
		Int64("deleted", deletedCount).
		Msg("Old worker nodes pruned successfully")
}
