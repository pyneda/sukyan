package get

import (
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var filterStatusCodes []int
var filterContentTypes []string
var filterMethods []string
var filterPageSize int
var filterPage int
var filterHistorySources []string
var filterQuery string
var filterScanID uint
var filterHistoryTaskJobID uint

// historyCmd represents the history command
var historyCmd = &cobra.Command{
	Use:     "history",
	Aliases: []string{"hist", "h", "requests"},
	Short:   "List HTTP history records stored in database",
	Long:    `Allows to filter and display HTTP history records stored in database`,
	Run: func(cmd *cobra.Command, args []string) {
		for _, source := range filterHistorySources {
			if !db.IsValidSource(source) {
				fmt.Printf("Invalid source received: %s\n\n", source)
				fmt.Println("Valid sources are:")
				for _, s := range db.Sources {
					fmt.Printf("  - %s\n", s)
				}
				return
			}
		}
		items, _, err := db.Connection().ListHistory(db.HistoryFilter{
			StatusCodes:          filterStatusCodes,
			ResponseContentTypes: filterContentTypes,
			Methods:              filterMethods,
			WorkspaceID:          uint(workspaceID),
			ScanID:               filterScanID,
			TaskID:               filterTaskID,
			Sources:              filterHistorySources,
			Query:                filterQuery,
			Pagination: db.Pagination{
				Page:     filterPage,
				PageSize: filterPageSize,
			},
		})
		if err != nil {
			log.Error().Err(err).Msg("Error received trying to get issues from db")
			return
		}
		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			log.Error().Err(err).Msg("Error parsing format type")
			return
		}

		formattedOutput, err := lib.FormatOutput(items, formatType)
		if err != nil {
			log.Error().Err(err).Msg("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
	},
}

func init() {
	GetCmd.AddCommand(historyCmd)

	historyCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
	historyCmd.Flags().UintVar(&filterScanID, "scan", 0, "Scan ID")
	historyCmd.Flags().UintVarP(&filterTaskID, "task", "t", 0, "Task ID")
	historyCmd.Flags().StringVarP(&filterQuery, "query", "q", "", "Filter by query")
	historyCmd.Flags().StringSliceVarP(&filterHistorySources, "source", "S", []string{}, "Filter by source. Can be added multiple times.")
	historyCmd.Flags().IntSliceVarP(&filterStatusCodes, "status", "s", []int{}, "Filter by status code. Can be added multiple times.")
	historyCmd.Flags().StringSliceVarP(&filterContentTypes, "content-type", "c", []string{}, "Filter by content types. Can be added multiple times.")
	historyCmd.Flags().StringSliceVarP(&filterMethods, "method", "m", []string{}, "Filter by HTTP method. Can be added multiple times.")
	historyCmd.Flags().IntVarP(&filterPage, "page", "p", 1, "Page to get data from")
	historyCmd.Flags().IntVar(&filterPageSize, "page-size", 50, "Page size")
}
