package scanctl

import (
	"fmt"
	"os"
	"strconv"

	"github.com/pyneda/sukyan/db"
	"github.com/spf13/cobra"
)

// pauseScanCmd represents the pause command
var pauseScanCmd = &cobra.Command{
	Use:        "pause [scan-id]",
	Aliases:    []string{"p"},
	Short:      "Pause a running scan",
	Long:       `Pauses a running scan. The scan can be resumed later using the resume command.`,
	Args:       cobra.ExactArgs(1),
	ArgAliases: []string{"id"},
	Run: func(cmd *cobra.Command, args []string) {
		scanID, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid scan ID provided")
			os.Exit(1)
		}
		if scanID == 0 {
			fmt.Println("A valid scan ID needs to be provided")
			os.Exit(1)
		}

		scan, err := db.Connection().PauseScan(uint(scanID))
		if err != nil {
			fmt.Printf("Failed to pause scan: %s\n", err)
			os.Exit(1)
		}

		fmt.Printf("Scan %d paused successfully\n", scan.ID)
		fmt.Printf("  - Title: %s\n", scan.Title)
		fmt.Printf("  - Previous Status: %s\n", scan.PreviousStatus)
		fmt.Printf("  - Current Status: %s\n", scan.Status)
	},
}

func init() {
	ScanCtlCmd.AddCommand(pauseScanCmd)
}
