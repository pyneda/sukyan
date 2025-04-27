package cmd

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/spf13/cobra"
)

var (
	statusFilter        string
	playgroundSessionID uint
)

// getTasksCmd represents the get tasks command
var getTasksCmd = &cobra.Command{
	Use:     "tasks",
	Aliases: []string{"task", "t"},
	Short:   "List tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		statuses := []string{}

		if statusFilter != "" {
			statuses = strings.Split(statusFilter, ",")
		}
		filters := db.TaskFilter{
			Pagination: db.Pagination{
				PageSize: pageSize,
				Page:     page,
			},
			FetchStats: true,

			WorkspaceID:         workspaceID,
			Statuses:            statuses,
			Query:               query,
			PlaygroundSessionID: playgroundSessionID,
		}

		tasks, _, err := db.Connection().ListTasks(filters)
		if err != nil {
			return err
		}

		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			return err
		}

		formattedOutput, err := lib.FormatOutput(tasks, formatType)
		if err != nil {
			return err
		}

		fmt.Println(formattedOutput)
		return nil
	},
}

func init() {
	getCmd.AddCommand(getTasksCmd)
	getTasksCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
	getTasksCmd.Flags().StringVar(&statusFilter, "status", "", "Comma-separated list of statuses to filter")
	getTasksCmd.Flags().UintVar(&playgroundSessionID, "playground-session", 0, "Playground session ID to filter by")
	getTasksCmd.PersistentFlags().StringVarP(&query, "query", "q", "", "Search query")

}
