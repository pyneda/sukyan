package cleanup

import (
	"github.com/spf13/cobra"
)

// CleanupCmd is the main cleanup command
var CleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "🧹 Clean up and optimize database storage",
	Long:  `Utilities to help clean up and optimize database storage by trimming large request/response bodies.`,
}

func init() {
	CleanupCmd.AddCommand(trimHistoryCmd)
	CleanupCmd.AddCommand(vacuumDbCmd)
}
