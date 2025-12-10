package get

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"gopkg.in/yaml.v3"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var filterIssueCodes []string
var filterTaskID uint
var filterTaskJobID uint
var filterIssueScanID uint
var filterMinConfidence int

// getIssuesCmd represents the results command
var getIssuesCmd = &cobra.Command{
	Use:     "issues",
	Aliases: []string{"i", "issue", "vulnerabilities", "v", "vulns", "vuln"},
	Short:   "List detected issues",
	Run: func(cmd *cobra.Command, args []string) {
		issues, _, err := db.Connection().ListIssues(db.IssueFilter{
			Codes:         filterIssueCodes,
			WorkspaceID:   uint(workspaceID),
			ScanID:        filterIssueScanID,
			TaskID:        uint(filterTaskID),
			TaskJobID:     uint(filterTaskJobID),
			MinConfidence: filterMinConfidence,
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

var listIssueCodesCmd = &cobra.Command{
	Use:     "codes",
	Aliases: []string{"t", "c", "types", "issue-types"},
	Short:   "List unique detected issue types/codes",
	Run: func(cmd *cobra.Command, args []string) {
		issueTypes, err := db.Connection().ListUniqueIssueCodes(db.IssueFilter{
			WorkspaceID:   uint(workspaceID),
			ScanID:        filterIssueScanID,
			TaskID:        uint(filterTaskID),
			TaskJobID:     uint(filterTaskJobID),
			MinConfidence: filterMinConfidence,
		})
		if err != nil {
			log.Error().Err(err).Msg("Error received trying to get issue types from db")
			return
		}

		if len(issueTypes) == 0 {
			fmt.Println("No issue types found for the specified scope")
			return
		}

		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			log.Error().Err(err).Msg("Error parsing format type")
			return
		}

		formattedOutput, err := formatStringSlice(issueTypes, formatType)
		if err != nil {
			log.Error().Err(err).Msg("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
	},
}

func formatStringSlice(data []string, format lib.FormatType) (string, error) {
	switch format {
	case lib.Text, lib.Pretty:
		return strings.Join(data, "\n"), nil
	case lib.JSON:
		j, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return "", err
		}
		return string(j), nil
	case lib.YAML:
		y, err := yaml.Marshal(data)
		if err != nil {
			return "", err
		}
		return string(y), nil
	case lib.Table:
		var buffer bytes.Buffer
		table := tablewriter.NewWriter(&buffer)
		table.SetHeader([]string{"Issue Type"})
		for _, item := range data {
			table.Append([]string{item})
		}
		table.SetBorder(true)
		table.Render()
		return buffer.String(), nil
	default:
		return "", fmt.Errorf("unknown format: %v", format)
	}
}

func init() {
	GetCmd.AddCommand(getIssuesCmd)

	getIssuesCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
	getIssuesCmd.Flags().UintVar(&filterIssueScanID, "scan", 0, "Scan ID")
	getIssuesCmd.Flags().UintVarP(&filterTaskID, "task", "t", 0, "Task ID")
	getIssuesCmd.Flags().UintVarP(&filterTaskJobID, "task-job", "j", 0, "Task Job ID")
	getIssuesCmd.Flags().StringSliceVarP(&filterIssueCodes, "code", "c", []string{}, "Filter by issue code. Can be added multiple times.")
	getIssuesCmd.Flags().IntVarP(&filterMinConfidence, "min-confidence", "m", 0, "Minimum confidence level (0-100)")

	// issue types
	getIssuesCmd.AddCommand(listIssueCodesCmd)

	listIssueCodesCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
	listIssueCodesCmd.Flags().UintVar(&filterIssueScanID, "scan", 0, "Scan ID")
	listIssueCodesCmd.Flags().UintVarP(&filterTaskID, "task", "t", 0, "Task ID")
	listIssueCodesCmd.Flags().UintVarP(&filterTaskJobID, "task-job", "j", 0, "Task Job ID")
	listIssueCodesCmd.Flags().IntVarP(&filterMinConfidence, "min-confidence", "m", 0, "Minimum confidence level (0-100)")

}
