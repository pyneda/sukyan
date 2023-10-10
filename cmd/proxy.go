package cmd

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/proxy"
	"github.com/rs/zerolog/log"
	"os"

	"github.com/spf13/cobra"
)

var proxyHost string
var proxyPort int
var proxyVerbose bool
var proxyIntercept bool

// proxyCmd represents the proxy command
var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Starts a proxy server",
	Long:  `Starts a proxy server that can be used to intercept and store requests and response to a specific workspace.`,
	Run: func(cmd *cobra.Command, args []string) {
		workspaceExists, _ := db.Connection.WorkspaceExists(workspaceID)
		if !workspaceExists {
			log.Error().Uint("id", workspaceID).Msg("Workspace does not exist")
			workspaces, count, _ := db.Connection.ListWorkspaces(db.WorkspaceFilters{})
			if count == 0 {
				log.Info().Msg("No workspaces found.")
			} else {
				log.Info().Msg("Available workspaces:")
				for _, workspace := range workspaces {
					log.Info().Msgf("ID: %d, Code: %s, Title: %s", workspace.ID, workspace.Code, workspace.Title)
				}
			}
			os.Exit(1)
		}
		proxy := proxy.Proxy{
			Host:                  proxyHost,
			Port:                  proxyPort,
			Verbose:               proxyVerbose,
			LogOutOfScopeRequests: true,
			WorkspaceID:           workspaceID,
			Intercept:             proxyIntercept,
		}
		proxy.Run()
	},
}

func init() {
	rootCmd.AddCommand(proxyCmd)
	proxyCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace to save requests to")
	proxyCmd.Flags().StringVarP(&proxyHost, "host", "H", "localhost", "Proxy host")
	proxyCmd.Flags().IntVarP(&proxyPort, "port", "p", 8008, "Proxy port")
	proxyCmd.Flags().BoolVarP(&proxyVerbose, "verbose", "v", false, "Verbose logging")
	proxyCmd.Flags().BoolVarP(&proxyIntercept, "intercept", "i", false, "Intercept requests")
}
