package scanctl

import (
	"fmt"
	"os"
	"strconv"

	"github.com/pyneda/sukyan/db"
	"github.com/spf13/cobra"
)

// cancelScanCmd represents the cancel command
var cancelScanCmd = &cobra.Command{
	Use:        "cancel [scan-id]",
	Aliases:    []string{"c", "stop"},
	Short:      "Cancel a scan",
	Long:       `Cancels a running or paused scan. All pending jobs will be cancelled.`,
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

		scan, err := db.Connection().CancelScan(uint(scanID))
		if err != nil {
			fmt.Printf("Failed to cancel scan: %s\n", err)
			os.Exit(1)
		}

		fmt.Printf("Scan %d cancelled successfully\n", scan.ID)
		fmt.Printf("  - Title: %s\n", scan.Title)
		fmt.Printf("  - Current Status: %s\n", scan.Status)
	},
}

func init() {
	ScanCtlCmd.AddCommand(cancelScanCmd)
}
