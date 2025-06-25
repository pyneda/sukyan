package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	fpValue bool
	fpShow  bool
)

// falsePositiveCmd represents the false-positive command
var falsePositiveCmd = &cobra.Command{
	Use:     "false-positive [ids...]",
	Aliases: []string{"fp"},
	Short:   "Set or unset issues as false positives",
	Long: `Set or unset one or multiple issues as false positives by their IDs.

Examples:
  sukyan fp 123                    # Set issue 123 as false positive
  sukyan fp 123 456 789            # Set multiple issues as false positives
  sukyan fp 123 --value=false      # Unset issue 123 as false positive
  sukyan fp 123 --show             # Show current false positive status of issue 123`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var issueIDs []int
		for _, arg := range args {
			id, err := strconv.Atoi(arg)
			if err != nil {
				fmt.Printf("Invalid issue ID: %s\n", arg)
				return
			}
			issueIDs = append(issueIDs, id)
		}

		if fpShow {
			showIssueStatuses(issueIDs)
			return
		}

		updateFalsePositiveStatus(issueIDs, fpValue)
	},
}

func showIssueStatuses(issueIDs []int) {
	fmt.Println("Current false positive status:")
	fmt.Println(strings.Repeat("-", 50))

	for _, id := range issueIDs {
		issue, err := db.Connection().GetIssue(id, false)
		if err != nil {
			fmt.Printf("Issue ID %d: Error - %v\n", id, err)
			continue
		}

		status := "false"
		if issue.FalsePositive {
			status = "true"
		}

		fmt.Printf("Issue ID %d: %s (Title: %s)\n", id, status, issue.Title)
	}
}

func updateFalsePositiveStatus(issueIDs []int, value bool) {
	action := "Setting"
	if !value {
		action = "Unsetting"
	}

	fmt.Printf("%s %d issue(s) as false positive: %v\n", action, len(issueIDs), value)
	fmt.Println(strings.Repeat("-", 50))

	successCount := 0
	errorCount := 0

	for _, id := range issueIDs {
		issue, err := db.Connection().GetIssue(id, false)
		if err != nil {
			fmt.Printf("❌ Issue ID %d: Failed to fetch - %v\n", id, err)
			errorCount++
			continue
		}

		err = issue.UpdateFalsePositive(value)
		if err != nil {
			fmt.Printf("❌ Issue ID %d: Failed to update - %v\n", id, err)
			log.Error().Int("id", id).Err(err).Msg("Failed to update issue false positive status")
			errorCount++
			continue
		}

		fmt.Printf("✅ Issue ID %d: Successfully updated (Title: %s)\n", id, issue.Title)
		successCount++
	}

	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("Summary: %d successful, %d failed\n", successCount, errorCount)

	if errorCount > 0 {
		fmt.Printf("Some operations failed. Please check the logs for more details.\n")
	}
}

func init() {
	rootCmd.AddCommand(falsePositiveCmd)

	falsePositiveCmd.Flags().BoolVar(&fpValue, "value", true, "Set false positive value (true or false)")
	falsePositiveCmd.Flags().BoolVar(&fpShow, "show", false, "Show current false positive status instead of updating")
}
