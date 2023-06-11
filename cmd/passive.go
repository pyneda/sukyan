package cmd

import (
	"fmt"
	"github.com/rs/zerolog/log"
	"os"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/passive"
	"github.com/spf13/cobra"
)

var passiveHistoryID int

// passiveCmd represents the passive command
var passiveCmd = &cobra.Command{
	Use:   "passive",
	Short: "Passive scan against a history item",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		if passiveHistoryID == 0 {
			fmt.Println("An ID needs to be provided")
			os.Exit(0)
		}
		history, err := db.Connection.GetHistory(passiveHistoryID)
		if err != nil {
			log.Panic().Err(err).Msg("Could not find a issue with the provided ID")
			os.Exit(0)
		}
		passive.ScanHistoryItem(history)

	},
}

func init() {
	rootCmd.AddCommand(passiveCmd)
	passiveCmd.Flags().IntVarP(&passiveHistoryID, "id", "i", 0, "History ID")
}
