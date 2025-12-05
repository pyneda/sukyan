package describe

import (
	"fmt"
	"os"
	"strconv"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"

	"github.com/spf13/cobra"
)

// describeScanCmd represents the scan command
var describeScanCmd = &cobra.Command{
	Use:        "scan [id]",
	Aliases:    []string{"s"},
	Short:      "Get details of a scan",
	Long:       `Display detailed information about a scan including its status, progress, and configuration.`,
	Args:       cobra.ExactArgs(1),
	ArgAliases: []string{"id"},
	Run: func(cmd *cobra.Command, args []string) {
		scanID, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid ID provided")
			os.Exit(0)
		}
		if scanID == 0 {
			fmt.Println("An ID needs to be provided")
			os.Exit(0)
		}
		scan, err := db.Connection().GetScanByID(uint(scanID))
		if err != nil {
			fmt.Println("Could not find a scan with the provided ID")
			os.Exit(0)
		}
		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			fmt.Println("Error parsing format type")
			os.Exit(0)
		}
		formattedOutput, err := lib.FormatSingleOutput(scan, formatType)
		if err != nil {
			fmt.Println("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
	},
}

func init() {
	DescribeCmd.AddCommand(describeScanCmd)
}
