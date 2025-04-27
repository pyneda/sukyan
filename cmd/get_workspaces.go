package cmd

import (
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/spf13/cobra"
)

var query string

// getWorkspacesCmd represents the get workspaces command
var getWorkspacesCmd = &cobra.Command{
	Use:     "workspaces",
	Aliases: []string{"workspace", "w"},
	Short:   "List workspaces",
	RunE: func(cmd *cobra.Command, args []string) error {
		filters := db.WorkspaceFilters{
			Pagination: db.Pagination{
				PageSize: pageSize,
				Page:     page,
			},
		}
		if query != "" {
			filters.Query = query
		}

		items, _, err := db.Connection().ListWorkspaces(filters)
		if err != nil {
			return err
		}

		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			return err
		}

		formattedOutput, err := lib.FormatOutput(items, formatType)

		if err != nil {
			return err
		}

		fmt.Println(formattedOutput)
		return nil
	},
}

func init() {
	getCmd.AddCommand(getWorkspacesCmd)
	getWorkspacesCmd.PersistentFlags().StringVarP(&query, "query", "q", "", "Search query")
}
