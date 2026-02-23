package scanctl

import (
	"fmt"
	"os"

	"github.com/pyneda/sukyan/db"
	"github.com/spf13/cobra"
)

var pauseAllScansCmd = &cobra.Command{
	Use:     "pause-all",
	Aliases: []string{"pa"},
	Short:   "Pause all active scans",
	Long:    `Pauses all currently running scans. They can be resumed later using the resume-all command.`,
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		paused, err := db.Connection().PauseAllScans()
		if err != nil {
			fmt.Printf("Failed to pause scans: %s\n", err)
			os.Exit(1)
		}

		if len(paused) == 0 {
			fmt.Println("No active scans to pause")
			return
		}

		fmt.Printf("Paused %d scan(s):\n", len(paused))
		for _, scan := range paused {
			fmt.Printf("  - Scan %d: %s (was %s)\n", scan.ID, scan.Title, scan.PreviousStatus)
		}
	},
}

func init() {
	ScanCtlCmd.AddCommand(pauseAllScansCmd)
}
