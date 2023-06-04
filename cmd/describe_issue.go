package cmd

import (
	"fmt"
	"github.com/pyneda/sukyan/db"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var describeIssueID int

// describeIssueCmd represents the issue command
var describeIssueCmd = &cobra.Command{
	Use:     "issue",
	Aliases: []string{"i"},
	Short:   "Get details of a detected issue",
	Long:    `List issue details.`,
	Run: func(cmd *cobra.Command, args []string) {
		if describeIssueID == 0 {
			fmt.Println("An ID needs to be provided")
			os.Exit(0)
		}
		issue, err := db.Connection.GetIssue(describeIssueID)
		if err != nil {
			log.Panic().Err(err).Msg("Could not find a issue with the provided ID")
		}
		db.PrintIssue(issue)
		// log.Info().Interface("issue", issue).Msg("Issue")
	},
}

func init() {
	describeCmd.AddCommand(describeIssueCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// issueCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// issueCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	describeIssueCmd.Flags().IntVarP(&describeIssueID, "id", "i", 0, "Issue ID")
}
