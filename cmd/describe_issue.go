package cmd

import (
	"fmt"
	"strconv"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"

	"github.com/spf13/cobra"
)

// var describeIssueID int

// describeIssueCmd represents the issue command
var describeIssueCmd = &cobra.Command{
	Use:        "issue [id]",
	Aliases:    []string{"i"},
	Short:      "Get details of a detected issue",
	Long:       `List issue details.`,
	Args:       cobra.ExactArgs(1),
	ArgAliases: []string{"id"},
	Run: func(cmd *cobra.Command, args []string) {
		describeIssueID, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid ID provided")
			return
		}
		if describeIssueID == 0 {
			fmt.Println("An ID needs to be provided")
			return
		}
		issue, err := db.Connection.GetIssue(describeIssueID, true)
		if err != nil {
			fmt.Println("Could not find a issue with the provided ID")
			return
		}
		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			fmt.Println("Error parsing format type")
			return
		}
		formattedOutput, err := lib.FormatSingleOutput(issue, formatType)
		if err != nil {
			fmt.Println("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
	},
}

func init() {
	describeCmd.AddCommand(describeIssueCmd)

	// describeIssueCmd.Flags().IntVarP(&describeIssueID, "id", "i", 0, "Issue ID")
}
