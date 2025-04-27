package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"

	"github.com/spf13/cobra"
)

// historyCmd represents the history command
var describeHistoryCmd = &cobra.Command{
	Use:        "history [id]",
	Aliases:    []string{"h", "hist"},
	Short:      "Get details of a history record",
	Long:       `Get details of a history record.`,
	Args:       cobra.ExactArgs(1),
	ArgAliases: []string{"id"},
	Run: func(cmd *cobra.Command, args []string) {
		describeHistoryID, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid ID provided")
			os.Exit(0)
		}
		if describeHistoryID == 0 {
			fmt.Println("An ID needs to be provided")
			os.Exit(0)
		}
		history, err := db.Connection().GetHistory(uint(describeHistoryID))
		if err != nil {
			fmt.Println("Could not find a issue with the provided ID")
			os.Exit(0)
		}
		// db.PrintHistory(history)
		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			fmt.Println("Error parsing format type")
			os.Exit(0)
		}
		formattedOutput, err := lib.FormatSingleOutput(history, formatType)
		if err != nil {
			fmt.Println("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
	},
}

func init() {
	describeCmd.AddCommand(describeHistoryCmd)

}
