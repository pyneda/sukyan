package get

import (
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/spf13/cobra"
)

var userQuery string

var getUsersCmd = &cobra.Command{
	Use:     "users",
	Aliases: []string{"user"},
	Short:   "List users",
	RunE: func(cmd *cobra.Command, args []string) error {
		items, _, _, err := db.Connection().ListUsers(db.UserListFilter{
			Query: userQuery,
			Pagination: db.Pagination{
				PageSize: pageSize,
				Page:     page,
			},
		})
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
	GetCmd.AddCommand(getUsersCmd)
	getUsersCmd.PersistentFlags().StringVarP(&userQuery, "query", "q", "", "Search query")
}
