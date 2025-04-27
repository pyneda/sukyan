package cmd

import (
	"os"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/manual"
	"github.com/rs/zerolog/log"

	"github.com/spf13/cobra"
)

var browserInitialURL string
var sessionTitle string

// browserCmd represents the browser command
var browserCmd = &cobra.Command{
	Use:   "browser",
	Short: "Launch a browser that records all traffic",
	Run: func(cmd *cobra.Command, args []string) {
		workspaceExists, _ := db.Connection().WorkspaceExists(workspaceID)
		if !workspaceExists {
			log.Error().Uint("id", workspaceID).Msg("Workspace does not exist")
			workspaces, count, _ := db.Connection().ListWorkspaces(db.WorkspaceFilters{})
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
		task, err := db.Connection().NewTask(workspaceID, nil, sessionTitle, db.TaskStatusRunning, db.TaskTypeBrowser)
		if err != nil {
			log.Error().Err(err).Msg("Could not create task")
			os.Exit(1)
		}
		manual.LaunchUserBrowser(workspaceID, browserInitialURL, task.ID)
	},
}

func init() {
	rootCmd.AddCommand(browserCmd)
	browserCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
	browserCmd.Flags().StringVarP(&browserInitialURL, "url", "u", "", "Initial URL to load")
	browserCmd.Flags().StringVarP(&sessionTitle, "title", "t", "Browser session", "Session title")
}
