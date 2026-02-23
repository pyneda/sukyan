package scanctl

import (
	"fmt"
	"os"

	"github.com/pyneda/sukyan/db"
	"github.com/spf13/cobra"
)

var resumeAllScansCmd = &cobra.Command{
	Use:     "resume-all",
	Aliases: []string{"ra"},
	Short:   "Resume all paused scans",
	Long:    `Resumes all paused scans. Each scan will continue from where it left off.`,
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		resumed, err := db.Connection().ResumeAllScans()
		if err != nil {
			fmt.Printf("Failed to resume scans: %s\n", err)
			os.Exit(1)
		}

		if len(resumed) == 0 {
			fmt.Println("No paused scans to resume")
			return
		}

		fmt.Printf("Resumed %d scan(s):\n", len(resumed))
		for _, scan := range resumed {
			fmt.Printf("  - Scan %d: %s (status: %s)\n", scan.ID, scan.Title, scan.Status)
		}
	},
}

func init() {
	ScanCtlCmd.AddCommand(resumeAllScansCmd)
}
