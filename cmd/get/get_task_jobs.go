package get

import (
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	taskJobStatusFilter      []string
	taskJobTitleFilter       []string
	taskJobStatusCodesFilter []int
	taskJobMethodsFilter     []string
	taskJobTaskIDFilter      uint
	taskJobSortBy            string
	taskJobSortOrder         string
)

var getTaskJobsCmd = &cobra.Command{
	Use:     "task-jobs",
	Aliases: []string{"task-job", "tj", "jobs"},
	Short:   "List task jobs",
	Long:    "List task jobs with optional filtering by status, title, task ID, status codes, and HTTP methods",
	Run: func(cmd *cobra.Command, args []string) {
		filters := db.TaskJobFilter{
			Pagination: db.Pagination{
				PageSize: pageSize,
				Page:     page,
			},
			Statuses:    taskJobStatusFilter,
			Titles:      taskJobTitleFilter,
			StatusCodes: taskJobStatusCodesFilter,
			Methods:     taskJobMethodsFilter,
			TaskID:      taskJobTaskIDFilter,
			WorkspaceID: workspaceID,
			Query:       query,
			SortBy:      taskJobSortBy,
			SortOrder:   taskJobSortOrder,
		}

		taskJobs, _, err := db.Connection().ListTaskJobs(filters)
		if err != nil {
			log.Error().Err(err).Msg("Error retrieving task jobs from database")
			return
		}

		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			log.Error().Err(err).Msg("Error parsing format type")
			return
		}

		formattedOutput, err := lib.FormatOutput(taskJobs, formatType)
		if err != nil {
			log.Error().Err(err).Msg("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
	},
}

func init() {
	GetCmd.AddCommand(getTaskJobsCmd)
	getTaskJobsCmd.Flags().StringSliceVar(&taskJobStatusFilter, "status", []string{}, "Filter by status (scheduled, running, finished, failed). Can be specified multiple times.")
	getTaskJobsCmd.Flags().StringSliceVar(&taskJobTitleFilter, "title", []string{}, "Filter by title. Can be specified multiple times.")
	getTaskJobsCmd.Flags().IntSliceVar(&taskJobStatusCodesFilter, "status-codes", []int{}, "Filter by HTTP status codes. Can be specified multiple times.")
	getTaskJobsCmd.Flags().StringSliceVar(&taskJobMethodsFilter, "methods", []string{}, "Filter by HTTP methods (GET, POST, PUT, DELETE, etc.). Can be specified multiple times.")
	getTaskJobsCmd.Flags().UintVar(&taskJobTaskIDFilter, "task", 0, "Filter by task ID")
	getTaskJobsCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
	getTaskJobsCmd.Flags().StringVar(&taskJobSortBy, "sort-by", "id", "Sort by field (id, history_method, history_url, history_status, history_parameters_count, title, status, started_at, completed_at, created_at, updated_at)")
	getTaskJobsCmd.Flags().StringVar(&taskJobSortOrder, "sort-order", "desc", "Sort order (asc, desc)")
	getTaskJobsCmd.PersistentFlags().StringVarP(&query, "query", "q", "", "Search query")
}
