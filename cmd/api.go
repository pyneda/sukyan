package cmd

import (
	"github.com/pyneda/sukyan/api"

	"github.com/spf13/cobra"
)

var apiEnableProxyServices bool

// apiCmd represents the api command
var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Starts the API server",
	Run: func(cmd *cobra.Command, args []string) {
		api.StartAPI(api.APIServerOptions{
			EnableProxyServices: apiEnableProxyServices,
		})
	},
}

func init() {
	apiCmd.Flags().BoolVar(
		&apiEnableProxyServices,
		"enable-proxy-services",
		false,
		"Enable proxy services management API endpoints",
	)
	rootCmd.AddCommand(apiCmd)
}
