package describe

import (
	"fmt"
	"os"
	"strconv"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"

	"github.com/spf13/cobra"
)

// describeScanJobCmd represents the scan-job command
var describeScanJobCmd = &cobra.Command{
	Use:        "scan-job [id]",
	Aliases:    []string{"scanjob", "sj"},
	Short:      "Get details of a scan job",
	Long:       `Display detailed information about a scan job including its status, target, and execution details.`,
	Args:       cobra.ExactArgs(1),
	ArgAliases: []string{"id"},
	Run: func(cmd *cobra.Command, args []string) {
		jobID, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid ID provided")
			os.Exit(0)
		}
		if jobID == 0 {
			fmt.Println("An ID needs to be provided")
			os.Exit(0)
		}
		job, err := db.Connection().GetScanJobByID(uint(jobID))
		if err != nil {
			fmt.Println("Could not find a scan job with the provided ID")
			os.Exit(0)
		}
		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			fmt.Println("Error parsing format type")
			os.Exit(0)
		}
		formattedOutput, err := lib.FormatSingleOutput(job, formatType)
		if err != nil {
			fmt.Println("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
	},
}

func init() {
	DescribeCmd.AddCommand(describeScanJobCmd)
}
