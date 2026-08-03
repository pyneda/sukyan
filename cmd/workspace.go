package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/workspace"
	"github.com/spf13/cobra"
)

var (
	workspaceArchivePath string
	workspaceImportCode  string
	workspaceImportTitle string
)

var workspaceCmd = &cobra.Command{
	Use:     "workspace",
	Aliases: []string{"ws"},
	Short:   "📦 Move workspaces between deployments",
}

var workspaceExportCmd = &cobra.Command{
	Use:   "export [workspace id]",
	Short: "Export a workspace and all its data to a portable archive",
	Long: `Writes a compressed archive holding every row that belongs to the workspace:
history, issues, websocket traffic, API definitions, playground sessions and the
rest. The archive can be imported into another sukyan deployment.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		workspaceID, err := strconv.ParseUint(args[0], 10, 64)
		if err != nil || workspaceID == 0 {
			return fmt.Errorf("invalid workspace ID %q", args[0])
		}
		if workspaceArchivePath == "" {
			return fmt.Errorf("an output path is required (--output)")
		}

		file, err := os.Create(workspaceArchivePath)
		if err != nil {
			return fmt.Errorf("creating %s: %w", workspaceArchivePath, err)
		}
		defer file.Close()

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		result, err := workspace.Export(ctx, db.Connection(), uint(workspaceID), file, workspace.ExportOptions{
			Progress: func(table string, rows int64) {
				if rows > 0 {
					fmt.Printf("  %-40s %10d rows\n", table, rows)
				}
			},
		})
		if err != nil {
			return err
		}

		info, statErr := file.Stat()
		size := "unknown size"
		if statErr == nil {
			size = humanBytes(info.Size())
		}
		fmt.Printf("\nExported workspace %q (%d rows) to %s [%s] in %s\n",
			result.Workspace.Code, result.TotalRows, workspaceArchivePath, size,
			result.Duration.Round(time.Millisecond))
		return nil
	},
}

var workspaceImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import a workspace archive into this deployment",
	Long: `Recreates an exported workspace as a new workspace. Identifiers are rewritten so
the imported data cannot collide with, or reference, anything already stored.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if workspaceArchivePath == "" {
			return fmt.Errorf("an input path is required (--input)")
		}

		file, err := os.Open(workspaceArchivePath)
		if err != nil {
			return fmt.Errorf("opening %s: %w", workspaceArchivePath, err)
		}
		defer file.Close()

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		result, err := workspace.Import(ctx, db.Connection(), file, workspace.ImportOptions{
			Code:  workspaceImportCode,
			Title: workspaceImportTitle,
			Progress: func(table string, rows int64) {
				if rows > 0 {
					fmt.Printf("  %-40s %10d rows\n", table, rows)
				}
			},
		})
		if err != nil {
			return err
		}

		fmt.Printf("\nImported %q from %q as workspace %d (code %q), %d rows in %s\n",
			result.Source.Title, result.Source.Code, result.WorkspaceID, result.Code,
			result.TotalRows, result.Duration.Round(time.Millisecond))
		for _, skipped := range result.Skipped {
			fmt.Printf("  skipped %s: %s\n", skipped.Table, skipped.Reason)
		}
		return nil
	},
}

func humanBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

func init() {
	workspaceExportCmd.Flags().StringVarP(&workspaceArchivePath, "output", "o", "", "Path to write the archive to")
	workspaceImportCmd.Flags().StringVarP(&workspaceArchivePath, "input", "i", "", "Path of the archive to import")
	workspaceImportCmd.Flags().StringVar(&workspaceImportCode, "code", "", "Code for the imported workspace (defaults to the archived code)")
	workspaceImportCmd.Flags().StringVar(&workspaceImportTitle, "title", "", "Title for the imported workspace")

	workspaceCmd.AddCommand(workspaceExportCmd)
	workspaceCmd.AddCommand(workspaceImportCmd)
	rootCmd.AddCommand(workspaceCmd)
}
