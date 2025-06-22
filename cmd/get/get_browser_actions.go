package get

import (
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/spf13/cobra"
)

var (
	scopeFilter string
)

var getBrowserActionsCmd = &cobra.Command{
	Use:     "browser-actions",
	Aliases: []string{"browser-action", "ba"},
	Short:   "List browser actions",
	RunE: func(cmd *cobra.Command, args []string) error {
		filters := db.StoredBrowserActionsFilter{
			Pagination: db.Pagination{
				PageSize: pageSize,
				Page:     page,
			},
		}
		if query != "" {
			filters.Query = query
		}
		if scopeFilter != "" {
			filters.Scope = db.BrowserActionScope(scopeFilter)
		}
		if workspaceID != 0 {
			filters.WorkspaceID = &workspaceID
		}

		items, _, err := db.Connection().ListStoredBrowserActions(filters)
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
	GetCmd.AddCommand(getBrowserActionsCmd)
	getBrowserActionsCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
	getBrowserActionsCmd.Flags().StringVar(&scopeFilter, "scope", "S", "Scope filter (global or workspace)")
	getBrowserActionsCmd.PersistentFlags().StringVarP(&query, "query", "q", "", "Search query")
}
