package scanctl

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var statsFormat string

// statsScanCmd represents the stats command
var statsScanCmd = &cobra.Command{
	Use:        "stats [scan-id]",
	Aliases:    []string{"s", "status"},
	Short:      "Show scan statistics",
	Long:       `Shows job statistics for a scan including pending, running, completed, and failed jobs.`,
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

		scan, err := db.Connection().GetScanByID(uint(scanID))
		if err != nil {
			fmt.Printf("Failed to get scan: %s\n", err)
			os.Exit(1)
		}

		stats, err := db.Connection().GetScanJobStats(uint(scanID))
		if err != nil {
			fmt.Printf("Failed to get scan stats: %s\n", err)
			os.Exit(1)
		}

		formatType, err := lib.ParseFormatType(statsFormat)
		if err != nil {
			fmt.Println("Error parsing format type")
			os.Exit(1)
		}

		if formatType == lib.JSON {
			output := map[string]interface{}{
				"scan":      scan,
				"job_stats": stats,
			}
			formatted, err := json.MarshalIndent(output, "", "  ")
			if err != nil {
				fmt.Println("Error formatting output")
				return
			}
			fmt.Println(string(formatted))
		} else if formatType == lib.YAML {
			output := map[string]interface{}{
				"scan":      scan,
				"job_stats": stats,
			}
			formatted, err := yaml.Marshal(output)
			if err != nil {
				fmt.Println("Error formatting output")
				return
			}
			fmt.Println(string(formatted))
		} else {
			// Pretty/table format
			fmt.Printf("Scan ID: %d\n", scan.ID)
			fmt.Printf("Title: %s\n", scan.Title)
			fmt.Printf("Status: %s\n", scan.Status)
			fmt.Printf("Phase: %s\n", scan.Phase)
			fmt.Printf("Progress: %.1f%%\n", scan.Progress())
			fmt.Println()
			fmt.Println("Job Statistics:")
			fmt.Printf("  Pending:   %d\n", stats[db.ScanJobStatusPending])
			fmt.Printf("  Claimed:   %d\n", stats[db.ScanJobStatusClaimed])
			fmt.Printf("  Running:   %d\n", stats[db.ScanJobStatusRunning])
			fmt.Printf("  Completed: %d\n", stats[db.ScanJobStatusCompleted])
			fmt.Printf("  Failed:    %d\n", stats[db.ScanJobStatusFailed])
			fmt.Printf("  Cancelled: %d\n", stats[db.ScanJobStatusCancelled])
		}
	},
}

func init() {
	ScanCtlCmd.AddCommand(statsScanCmd)
	statsScanCmd.Flags().StringVarP(&statsFormat, "format", "f", "pretty", "Output format (json, yaml, pretty)")
}
