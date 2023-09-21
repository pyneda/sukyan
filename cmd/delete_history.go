package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/spf13/cobra"
)

var deleteHistoryCmd = &cobra.Command{
	Use:     "history",
	Aliases: []string{"h", "histories"},
	Short:   "Delete history items based on the provided filter",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Deleting history items with the following filters:")
		// fmt.Printf("Status Codes: %s\n", strings.Join(deleteHistoryFilterStatusCodes, ","))
		fmt.Printf("Methods: %s\n", strings.Join(deleteHistoryFilterMethods, ","))
		fmt.Printf("Response Content Types: %s\n", strings.Join(deleteHistoryFilterResponseContentTypes, ","))
		fmt.Printf("Request Content Types: %s\n", strings.Join(deleteHistoryFilterRequestContentTypes, ","))
		fmt.Printf("Sources: %s\n", strings.Join(deleteHistoryFilterSources, ","))
		fmt.Printf("Workspace ID: %d\n", workspaceID)

		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Are you sure you want to proceed with deletion? (yes/no): ")
		confirmation, _ := reader.ReadString('\n')
		confirmation = strings.TrimSpace(confirmation)

		if confirmation == "yes" {
			filter := db.HistoryDeletionFilter{
				StatusCodes:          deleteHistoryFilterStatusCodes,
				Methods:              deleteHistoryFilterMethods,
				ResponseContentTypes: deleteHistoryFilterResponseContentTypes,
				RequestContentTypes:  deleteHistoryFilterRequestContentTypes,
				Sources:              deleteHistoryFilterSources,
				WorkspaceID:          workspaceID,
			}
			deletedCount, err := db.Connection.DeleteHistory(filter)
			if err != nil {
				fmt.Printf("Error during deletion: %s\n", err)
			} else {
				fmt.Printf("Successfully deleted %d history items.\n", deletedCount)
			}
		} else {
			fmt.Println("Deletion aborted.")
		}
	},
}

var (
	deleteHistoryFilterStatusCodes          []int
	deleteHistoryFilterMethods              []string
	deleteHistoryFilterResponseContentTypes []string
	deleteHistoryFilterRequestContentTypes  []string
	deleteHistoryFilterSources              []string
)

func init() {
	deleteCmd.AddCommand(deleteHistoryCmd)
	deleteHistoryCmd.Flags().IntSliceVarP(&deleteHistoryFilterStatusCodes, "status-codes", "s", nil, "Status codes for filtering")
	deleteHistoryCmd.Flags().StringSliceVarP(&deleteHistoryFilterMethods, "methods", "m", nil, "HTTP methods for filtering")
	deleteHistoryCmd.Flags().StringSliceVarP(&deleteHistoryFilterResponseContentTypes, "response-content-types", "r", nil, "Response content types for filtering")
	deleteHistoryCmd.Flags().StringSliceVarP(&deleteHistoryFilterRequestContentTypes, "request-content-types", "q", nil, "Request content types for filtering")
	deleteHistoryCmd.Flags().StringSliceVarP(&deleteHistoryFilterSources, "sources", "c", nil, "Sources for filtering")
	deleteHistoryCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID for filtering")
}
