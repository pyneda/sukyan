package cmd

import (
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// getDatabaseSizeCmd represents the getDatabaseSize command
var getDatabaseSizeCmd = &cobra.Command{
	Use:     "db_size",
	Short:   "Get the database size",
	Aliases: []string{"db-size", "db_s", "dbs"},
	Run: func(cmd *cobra.Command, args []string) {
		dbSize, err := db.GetDatabaseSize()
		if err != nil {
			log.Error().Err(err).Msg("Failed to get database size")
			return
		}
		fmt.Printf("Database size: %s\n", dbSize)
	},
}

func init() {
	utilsCmd.AddCommand(getDatabaseSizeCmd)
}
