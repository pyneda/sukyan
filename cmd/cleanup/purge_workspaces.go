package cleanup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/spf13/cobra"
)

var purgeWorkspacesYes bool

var purgeWorkspacesCmd = &cobra.Command{
	Use:   "purge-workspaces",
	Short: "🗑️  Finish deleting workspaces whose purge was interrupted",
	Long: `Deleting a large workspace removes its rows in batches. If that is interrupted --
the process is stopped, the machine restarts -- the workspace stays hidden but its
rows remain. This command finds those workspaces and finishes removing them.`,
	SilenceUsage: true,
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

		// Anything soft-deleted lands here, not only interrupted purges, so the
		// operator sees exactly what is about to go before it goes.
		fmt.Printf("The following %d workspace(s) are marked as deleted and will be permanently removed:\n\n", len(pending))
		for _, workspace := range pending {
			fmt.Printf("  %-10d %-40s %s\n", workspace.ID, workspace.Code, workspace.Title)
		}

		if !purgeWorkspacesYes {
			fmt.Print("\nThis cannot be undone. Proceed? (yes/no): ")
			reader := bufio.NewReader(os.Stdin)
			confirmation, _ := reader.ReadString('\n')
			if strings.TrimSpace(confirmation) != "yes" {
				fmt.Println("Aborted.")
				return nil
			}
		}
		fmt.Println()

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		var totalRows int64
		for _, workspace := range pending {
			workspaceID := workspace.ID
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
	purgeWorkspacesCmd.Flags().BoolVarP(&purgeWorkspacesYes, "yes", "y", false, "Skip the confirmation prompt")
	CleanupCmd.AddCommand(purgeWorkspacesCmd)
}
