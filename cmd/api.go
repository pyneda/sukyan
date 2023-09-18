package cmd

import (
	"github.com/pyneda/sukyan/api"

	"github.com/spf13/cobra"
)

// apiCmd represents the api command
var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Starts the API server",
	Run: func(cmd *cobra.Command, args []string) {

		api.StartAPI()

	},
}

func init() {
	rootCmd.AddCommand(apiCmd)
}
