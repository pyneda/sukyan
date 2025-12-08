package scanctl

import (
	"fmt"
	"os"
	"strconv"

	"github.com/pyneda/sukyan/db"
	"github.com/spf13/cobra"
)

// resumeScanCmd represents the resume command
var resumeScanCmd = &cobra.Command{
	Use:        "resume [scan-id]",
	Aliases:    []string{"r"},
	Short:      "Resume a paused scan",
	Long:       `Resumes a paused scan. The scan will continue from where it left off.`,
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

		scan, err := db.Connection().ResumeScan(uint(scanID))
		if err != nil {
			fmt.Printf("Failed to resume scan: %s\n", err)
			os.Exit(1)
		}

		fmt.Printf("Scan %d resumed successfully\n", scan.ID)
		fmt.Printf("  - Title: %s\n", scan.Title)
		fmt.Printf("  - Current Status: %s\n", scan.Status)
	},
}

func init() {
	ScanCtlCmd.AddCommand(resumeScanCmd)
}
