
package cmd

import (
	"fmt"
	"os"
	"github.com/pyneda/sukyan/db"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var describeHistoryID int

// historyCmd represents the history command
var describeHistoryCmd = &cobra.Command{
	Use:     "history",
	Aliases: []string{"h", "hist"},
	Short:   "Get details of a history record",
	Long:    `Get details of a history record.`,
	Run: func(cmd *cobra.Command, args []string) {
		if describeHistoryID == 0 {
			fmt.Println("An ID needs to be provided")
			os.Exit(0)
		}
		history, err := db.Connection.GetHistory(describeHistoryID)
		if err != nil {
			log.Panic().Err(err).Msg("Could not find a issue with the provided ID")
		}
		db.PrintHistory(history)
	},
}

func init() {
	describeCmd.AddCommand(describeHistoryCmd)
	describeHistoryCmd.Flags().IntVarP(&describeHistoryID, "id", "i", 0, "History ID")

}
