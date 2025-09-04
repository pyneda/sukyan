package cleanup

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/spf13/cobra"
)

// Trim history specific flags
var (
	maxBodySize        int
	cleanupWorkspaceID uint
	sources            []string
	includeWebSockets  bool
	batchSize          int
	dryRun             bool
	workers            int
	maxRecords         int64
	yesFlag            bool
)

var trimHistoryCmd = &cobra.Command{
	Use:   "trim-history",
	Short: "✂️  Trim oversized HTTP request/response bodies",
	Long: `Trim HTTP request and response bodies to reduce database storage usage.

This command processes history records and trims request/response bodies that exceed 
the specified maximum size. Records associated with security issues or out-of-band 
tests are automatically excluded to preserve all scan results.`,
	RunE: runTrimHistory,
}

func init() {
	// Trim history flags
	trimHistoryCmd.Flags().IntVar(&maxBodySize, "max-body-size", 10240, "Maximum body size to keep in bytes (e.g., 1024 for 1KB, 10240 for 10KB)")
	trimHistoryCmd.Flags().UintVar(&cleanupWorkspaceID, "workspace-id", 0, "Process only records from specific workspace ID (0 = all workspaces)")
	trimHistoryCmd.Flags().StringSliceVar(&sources, "sources", nil, "Filter by specific sources (e.g., crawler,scanner,proxy)")
	trimHistoryCmd.Flags().BoolVar(&includeWebSockets, "include-websockets", false, "Include WebSocket upgrade requests in cleanup")
	trimHistoryCmd.Flags().IntVar(&batchSize, "batch-size", 1000, "Number of records to process per batch (1-10000)")
	trimHistoryCmd.Flags().IntVar(&workers, "workers", 4, "Number of concurrent workers (1-20)")
	trimHistoryCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without modifying the database")
	trimHistoryCmd.Flags().Int64Var(&maxRecords, "max-records", 0, "Maximum number of records to trim (0 = no limit)")
	trimHistoryCmd.Flags().BoolVar(&yesFlag, "yes", false, "Skip confirmation prompt and proceed automatically")

	trimHistoryCmd.MarkFlagRequired("max-body-size")
}

func runTrimHistory(cmd *cobra.Command, args []string) error {
	if maxBodySize < 0 {
		return fmt.Errorf("❌ max-body-size must be non-negative, got: %d", maxBodySize)
	}
	if maxBodySize == 0 {
		fmt.Println("⚠️  Warning: max-body-size is 0, all bodies will be completely removed")
	}
	if batchSize < 1 || batchSize > 10000 {
		return fmt.Errorf("❌ batch-size must be between 1 and 10000, got: %d", batchSize)
	}
	if workers < 1 || workers > 20 {
		return fmt.Errorf("❌ workers must be between 1 and 20, got: %d", workers)
	}
	if maxRecords < 0 {
		return fmt.Errorf("❌ max-records must be non-negative, got: %d", maxRecords)
	}

	connection := db.Connection()
	if connection == nil {
		return fmt.Errorf("❌ Failed to connect to database")
	}

	options := lib.HistoryCleanupOptions{
		MaxBodySize:       maxBodySize,
		Sources:           sources,
		IncludeWebSockets: includeWebSockets,
		BatchSize:         batchSize,
		DryRun:            dryRun,
		Workers:           workers,
		MaxRecords:        maxRecords,
	}

	if cleanupWorkspaceID > 0 {
		options.WorkspaceID = &cleanupWorkspaceID
	}

	fmt.Println("🧹 History Cleanup Configuration")
	fmt.Println("================================")
	fmt.Printf("📏 Max body size:     %s\n", formatBytes(maxBodySize))
	if options.WorkspaceID != nil {
		fmt.Printf("🏢 Workspace:         ID %d\n", *options.WorkspaceID)
	} else {
		fmt.Printf("🏢 Workspace:         All workspaces\n")
	}
	if len(sources) > 0 {
		fmt.Printf("📂 Sources:           %s\n", strings.Join(sources, ", "))
	} else {
		fmt.Printf("📂 Sources:           All sources\n")
	}
	fmt.Printf("🔌 Include WebSockets: %v\n", includeWebSockets)
	fmt.Printf("📦 Batch size:        %d records\n", batchSize)
	fmt.Printf("⚡ Workers:           %d concurrent\n", workers)
	if maxRecords > 0 {
		fmt.Printf("🎯 Max records:       %s records\n", formatNumber(maxRecords))
	} else {
		fmt.Printf("🎯 Max records:       No limit\n")
	}
	if dryRun {
		fmt.Printf("🔍 Mode:              Dry run (no changes will be made)\n")
	} else {
		fmt.Printf("⚡ Mode:              Live cleanup (changes will be applied)\n")
	}
	fmt.Println()

	if dryRun {
		fmt.Println("🔍 Running dry run analysis...")
	} else {
		if !yesFlag {
			fmt.Println("\n⚠️  This will permanently modify your database!")
			fmt.Println("💡 Consider running with --dry-run first to preview changes.")
			fmt.Print("Continue? (y/N): ")

			var response string
			fmt.Scanln(&response)
			response = strings.ToLower(strings.TrimSpace(response))

			if response != "y" && response != "yes" {
				fmt.Println("❌ Operation cancelled by user")
				return nil
			}
		} else {
			fmt.Println("\n✅ Auto-confirmed with --yes flag")
		}

		fmt.Println("⚡ Starting cleanup process...")
	}

	result, err := lib.TrimHistoryBodies(connection.DB(), options)
	if err != nil {
		fmt.Printf("\n❌ Cleanup failed: %v\n", err)
		return fmt.Errorf("cleanup failed: %w", err)
	}

	fmt.Println()
	if result.DryRun {
		fmt.Println("📊 Dry Run Analysis Results")
		fmt.Println("===========================")
		fmt.Printf("📋 Records analyzed:  %s\n", formatNumber(result.ProcessedCount))
		fmt.Printf("✂️  Would be trimmed:   %s records\n", formatNumber(result.TrimmedCount))
		if result.ProcessedCount > 0 {
			trimPercentage := float64(result.TrimmedCount) / float64(result.ProcessedCount) * 100
			fmt.Printf("📈 Trim rate:         %.1f%%\n", trimPercentage)
		}
		fmt.Printf("💾 Potential savings: %s\n", formatBytes(int(result.BytesSaved)))
		fmt.Println("\n💡 Run without --dry-run to apply these changes")
	} else {
		fmt.Println("✅ Cleanup Completed Successfully")
		fmt.Println("=================================")
		fmt.Printf("📋 Records processed: %s\n", formatNumber(result.ProcessedCount))
		fmt.Printf("✂️  Records trimmed:   %s\n", formatNumber(result.TrimmedCount))
		fmt.Printf("💾 Space saved:       %s\n", formatBytes(int(result.BytesSaved)))
		if result.ProcessedCount > 0 {
			trimPercentage := float64(result.TrimmedCount) / float64(result.ProcessedCount) * 100
			fmt.Printf("📈 Trim rate:         %.1f%%\n", trimPercentage)
		}

		if result.TrimmedCount > 0 {
			fmt.Println("\n🎉 Database cleanup completed!")
		} else {
			fmt.Println("\n✨ No records matched for trimming")
		}
	}

	return nil
}
