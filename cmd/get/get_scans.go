package get

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/spf13/cobra"
)

var (
	scanStatusFilter string
	scanWorkspaceID  uint
)

// getScansCmd represents the get scans command
var getScansCmd = &cobra.Command{
	Use:     "scans",
	Aliases: []string{"scan"},
	Short:   "List scans",
	RunE: func(cmd *cobra.Command, args []string) error {
		var statuses []db.ScanStatus

		if scanStatusFilter != "" {
			for _, s := range strings.Split(scanStatusFilter, ",") {
				statuses = append(statuses, db.ScanStatus(strings.TrimSpace(s)))
			}
		}

		filters := db.ScanFilter{
			Pagination: db.Pagination{
				PageSize: pageSize,
				Page:     page,
			},
			WorkspaceID: scanWorkspaceID,
			Statuses:    statuses,
			Query:       query,
			SortBy:      "id",
			SortOrder:   "desc",
		}

		scans, count, err := db.Connection().ListScans(filters)
		if err != nil {
			return err
		}

		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			return err
		}

		if formatType == lib.Pretty || formatType == lib.Table {
			fmt.Printf("Total scans: %d\n\n", count)
		}

		formattedOutput, err := lib.FormatOutput(scans, formatType)
		if err != nil {
			return err
		}

		fmt.Println(formattedOutput)
		return nil
	},
}

func init() {
	GetCmd.AddCommand(getScansCmd)
	getScansCmd.Flags().UintVarP(&scanWorkspaceID, "workspace", "w", 0, "Workspace ID")
	getScansCmd.Flags().StringVar(&scanStatusFilter, "status", "", "Comma-separated list of statuses to filter (pending,crawling,scanning,paused,completed,cancelled,failed)")
}
