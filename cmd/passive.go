package cmd

import (
	"fmt"
	"os"

	"github.com/rs/zerolog/log"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/passive"
	"github.com/spf13/cobra"
)

var passiveHistoryID int

// passiveCmd represents the passive command
var passiveCmd = &cobra.Command{
	Use:   "passive",
	Short: "Passive scan against a history item",
	Run: func(cmd *cobra.Command, args []string) {
		if passiveHistoryID == 0 {
			fmt.Println("An ID needs to be provided")
			os.Exit(0)
		}
		history, err := db.Connection().GetHistory(uint(passiveHistoryID))
		if err != nil {
			log.Panic().Err(err).Msg("Could not find a issue with the provided ID")
			os.Exit(0)
		}
		passive.ScanHistoryItem(&history)

	},
}

func init() {
	rootCmd.AddCommand(passiveCmd)
	passiveCmd.Flags().IntVarP(&passiveHistoryID, "id", "i", 0, "History ID")
}
