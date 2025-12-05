package get

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/spf13/cobra"
)

var (
	scanJobScanID       uint
	scanJobStatusFilter string
	scanJobTypeFilter   string
)

// getScanJobsCmd represents the get scan-jobs command
var getScanJobsCmd = &cobra.Command{
	Use:     "scan-jobs",
	Aliases: []string{"scanjobs", "sj"},
	Short:   "List scan jobs",
	RunE: func(cmd *cobra.Command, args []string) error {
		var statuses []db.ScanJobStatus
		var jobTypes []db.ScanJobType

		if scanJobStatusFilter != "" {
			for _, s := range strings.Split(scanJobStatusFilter, ",") {
				statuses = append(statuses, db.ScanJobStatus(strings.TrimSpace(s)))
			}
		}

		if scanJobTypeFilter != "" {
			for _, t := range strings.Split(scanJobTypeFilter, ",") {
				jobTypes = append(jobTypes, db.ScanJobType(strings.TrimSpace(t)))
			}
		}

		filters := db.ScanJobFilter{
			Pagination: db.Pagination{
				PageSize: pageSize,
				Page:     page,
			},
			ScanID:    scanJobScanID,
			Statuses:  statuses,
			JobTypes:  jobTypes,
			Query:     query,
			SortBy:    "id",
			SortOrder: "desc",
		}

		jobs, count, err := db.Connection().ListScanJobs(filters)
		if err != nil {
			return err
		}

		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			return err
		}

		if formatType == lib.Pretty || formatType == lib.Table {
			fmt.Printf("Total scan jobs: %d\n\n", count)
		}

		formattedOutput, err := lib.FormatOutput(jobs, formatType)
		if err != nil {
			return err
		}

		fmt.Println(formattedOutput)
		return nil
	},
}

func init() {
	GetCmd.AddCommand(getScanJobsCmd)
	getScanJobsCmd.Flags().UintVar(&scanJobScanID, "scan", 0, "Filter by scan ID")
	getScanJobsCmd.Flags().StringVar(&scanJobStatusFilter, "status", "", "Comma-separated list of statuses to filter (pending,claimed,running,completed,failed,cancelled)")
	getScanJobsCmd.Flags().StringVar(&scanJobTypeFilter, "type", "", "Comma-separated list of job types to filter (active_scan,websocket_scan,discovery,nuclei,crawl)")
}
