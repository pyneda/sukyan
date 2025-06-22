package stats

import (
	"github.com/spf13/cobra"
)

// StatsCmd represents the stats command
var StatsCmd = &cobra.Command{
	Use:     "stats",
	Aliases: []string{"stat", "statistics", "metrics"},
	Short:   "Statistics and metrics commands",
	Long:    `Retrieve various statistics and metrics from the application`,
}

func init() {
	// Add subcommands to stats command
	StatsCmd.AddCommand(WorkspaceStatsCmd)
}
