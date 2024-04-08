package cmd

import (
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var filterIssueCodes []string
var filterTaskID uint
var filterTaskJobID uint

// getIssuesCmd represents the results command
var getIssuesCmd = &cobra.Command{
	Use:     "issues",
	Aliases: []string{"i", "issue", "vulnerabilities", "v", "vulns", "vuln"},
	Short:   "List detected issues",
	Run: func(cmd *cobra.Command, args []string) {
		issues, _, err := db.Connection.ListIssues(db.IssueFilter{
			Codes:       filterIssueCodes,
			WorkspaceID: uint(workspaceID),
			TaskID:      uint(filterTaskID),
			TaskJobID:   uint(filterTaskJobID),
		})
		if err != nil {
			log.Error().Err(err).Msg("Error received trying to get issues from db")
		}
		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			log.Error().Err(err).Msg("Error parsing format type")
			return
		}

		formattedOutput, err := lib.FormatOutput(issues, formatType)
		if err != nil {
			log.Error().Err(err).Msg("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
	},
}

func init() {
	getCmd.AddCommand(getIssuesCmd)

	getIssuesCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
	// getIssuesCmd.MarkFlagRequired("workspace")
	getIssuesCmd.Flags().UintVarP(&filterTaskID, "task", "t", 0, "Task ID")
	getIssuesCmd.Flags().UintVarP(&filterTaskJobID, "task-job", "j", 0, "Task Job ID")
	getIssuesCmd.Flags().StringSliceVarP(&filterIssueCodes, "code", "c", []string{}, "Filter by issue code. Can be added multiple times.")
}
