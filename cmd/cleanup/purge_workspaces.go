package cleanup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/spf13/cobra"
)

var purgeWorkspacesCmd = &cobra.Command{
	Use:   "purge-workspaces",
	Short: "🗑️  Finish deleting workspaces whose purge was interrupted",
	Long: `Deleting a large workspace removes its rows in batches. If that is interrupted --
the process is stopped, the machine restarts -- the workspace stays hidden but its
rows remain. This command finds those workspaces and finishes removing them.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		connection := db.Connection()
		if connection == nil {
			return fmt.Errorf("failed to connect to database")
		}

		pending, err := connection.WorkspacesPendingPurge()
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			fmt.Println("No workspaces are pending purge.")
			return nil
		}

		fmt.Printf("Found %d workspace(s) pending purge.\n", len(pending))

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		var totalRows int64
		for _, workspaceID := range pending {
			result, err := connection.DeleteWorkspaceCascade(ctx, workspaceID, db.WorkspaceDeleteOptions{
				Progress: func(table string, deleted int64) {
					fmt.Printf("\r  workspace %-6d %-24s %10d rows", workspaceID, table, deleted)
				},
			})
			totalRows += result.TotalRows
			fmt.Println()

			if err != nil {
				if errors.Is(err, context.Canceled) {
					fmt.Printf("Interrupted. Re-run to continue; %d rows removed so far.\n", totalRows)
					return nil
				}
				return fmt.Errorf("purging workspace %d: %w", workspaceID, err)
			}
			fmt.Printf("  workspace %d purged: %d rows in %s\n", workspaceID, result.TotalRows, result.Duration.Round(time.Millisecond))
		}

		fmt.Printf("\nPurged %d workspace(s), %d rows total.\n", len(pending), totalRows)
		return nil
	},
}

func init() {
	CleanupCmd.AddCommand(purgeWorkspacesCmd)
}
