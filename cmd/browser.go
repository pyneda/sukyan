package cmd

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/manual"
	"github.com/rs/zerolog/log"
	"os"

	"github.com/spf13/cobra"
)

var browserInitialURL string

// browserCmd represents the browser command
var browserCmd = &cobra.Command{
	Use:   "browser",
	Short: "Launch a browser that records all traffic",
	Run: func(cmd *cobra.Command, args []string) {
		workspaceExists, _ := db.Connection.WorkspaceExists(workspaceID)
		if !workspaceExists {
			log.Error().Uint("id", workspaceID).Msg("Workspace does not exist")
			workspaces, count, _ := db.Connection.ListWorkspaces(db.WorkspaceFilters{})
			if count == 0 {
				log.Info().Msg("No workspaces found, creating default")
				db.Connection.CreateDefaultWorkspace()
			} else {
				log.Info().Msg("Available workspaces:")
				for _, workspace := range workspaces {
					log.Info().Msgf("ID: %d, Code: %s, Title: %s", workspace.ID, workspace.Code, workspace.Title)
				}
			}
			os.Exit(1)
		}
		manual.LaunchUserBrowser(workspaceID, browserInitialURL)
	},
}

func init() {
	rootCmd.AddCommand(browserCmd)
	browserCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
	browserCmd.Flags().StringVarP(&browserInitialURL, "url", "u", "", "Initial URL to load")
}
