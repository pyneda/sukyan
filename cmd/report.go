package cmd

import (
	"bytes"
	"fmt"
	"os"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/report"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	reportTitle    string
	reportFormat   string
	reportOutput   string
	minConfidence  int
	maxRequestSize int
	taskID         uint
	scanID         uint
)

// reportCmd represents the report command
var reportCmd = &cobra.Command{
	Use:     "report",
	Aliases: []string{"r"},
	Short:   "Generates a report for a given workspace",
	Run: func(cmd *cobra.Command, args []string) {
		if workspaceID == 0 && taskID == 0 && scanID == 0 {
			fmt.Println("Please provide a workspace, task, or scan to generate a report")
			return
		}

		if scanID != 0 {
			scan, err := db.Connection().GetScanByID(scanID)
			if err != nil {
				fmt.Printf("Error fetching scan details: %v\n", err)
				return
			}
			if reportOutput == "" {
				reportOutput = fmt.Sprintf("scan-%d-report.%s", scan.ID, reportFormat)
			}
			if reportTitle == "" {
				reportTitle = fmt.Sprintf("Report for scan: %s", scan.Title)
			}
			// Use scan's workspace if not explicitly provided
			if workspaceID == 0 {
				workspaceID = scan.WorkspaceID
			}
		}

		if taskID != 0 {
			task, err := db.Connection().GetTaskByID(taskID, false)
			if err != nil {
				fmt.Printf("Error fetching task details: %v\n", err)
				return
			}
			if reportOutput == "" {
				reportOutput = fmt.Sprintf("%s-report.%s", lib.Slugify(task.Title), reportFormat)
			}
			if reportTitle == "" {
				reportTitle = fmt.Sprintf("Report for task: %s", task.Title)
			}

		}
		if workspaceID != 0 {
			workspace, err := db.Connection().GetWorkspaceByID(workspaceID)
			if err != nil {
				fmt.Printf("Error fetching workspace details: %v\n", err)
				return
			}
			if reportOutput == "" {
				reportOutput = fmt.Sprintf("%s-report.%s", lib.Slugify(workspace.Code), reportFormat)
			}
			if reportTitle == "" {
				reportTitle = fmt.Sprintf("Report for workspace: %s", workspace.Code)
			}
		}

		format, err := toReportFormat(reportFormat)
		if err != nil {
			fmt.Println(err)
			return
		}
		if reportTitle == "" {
			reportTitle = "Sukyan report"
		}
		if reportOutput == "" {

			reportOutput = fmt.Sprintf("%s.%s", lib.Slugify(reportTitle), reportFormat)
		}

		issues, _, err := db.Connection().ListIssues(db.IssueFilter{
			WorkspaceID:   workspaceID,
			TaskID:        taskID,
			ScanID:        scanID,
			MinConfidence: minConfidence,
		})

		if err != nil {
			fmt.Printf("There has been an error fetching issues to generate report: %v\n", err)
			return
		}

		options := report.ReportOptions{
			WorkspaceID:    workspaceID,
			Issues:         issues,
			Title:          reportTitle,
			Format:         format,
			TaskID:         taskID,
			ScanID:         scanID,
			MaxRequestSize: maxRequestSize,
		}

		var buf bytes.Buffer
		if err := report.GenerateReport(options, &buf); err != nil {
			log.Error().Err(err).Msg("Failed to generate report")
			fmt.Println("Failed to generate report")
			return
		}

		err = os.WriteFile(reportOutput, buf.Bytes(), os.ModePerm)
		if err != nil {
			fmt.Printf("Failed to write report to file: %v\n", err)
			return
		}

		fmt.Printf("Report generated and saved to %s\n", reportOutput)
	},
}

// Convert a string format to report.ReportFormat type
func toReportFormat(format string) (report.ReportFormat, error) {
	switch format {
	case string(report.ReportFormatHTML):
		return report.ReportFormatHTML, nil
	case string(report.ReportFormatJSON):
		return report.ReportFormatJSON, nil
	default:
		return "", fmt.Errorf("invalid format provided: %s", format)
	}
}

func init() {
	rootCmd.AddCommand(reportCmd)

	reportCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
	reportCmd.Flags().UintVarP(&taskID, "task", "t", 0, "Task ID")
	reportCmd.Flags().UintVarP(&scanID, "scan", "s", 0, "Scan ID (for orchestrator scans)")
	reportCmd.Flags().StringVarP(&reportTitle, "title", "T", "", "Report Title")
	reportCmd.Flags().StringVarP(&reportFormat, "format", "f", "html", "Report Format (html or json)")
	reportCmd.Flags().StringVarP(&reportOutput, "output", "o", "", "Output file path")
	reportCmd.Flags().IntVarP(&minConfidence, "min-confidence", "c", 0, "Minimum issue confidence level to include in the report")
	reportCmd.Flags().IntVar(&maxRequestSize, "max-request-size", 200*1024, "Maximum size (in bytes) for request/response content when using html report format. 0 means no limit")
}
