package stats

import (
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	workspaceID uint
	format      string
)

// WorkspaceStatsCmd represents the workspace stats command
var WorkspaceStatsCmd = &cobra.Command{
	Use:     "workspace",
	Aliases: []string{"w", "workspaces"},
	Short:   "Get workspace statistics",
	Long:    `Retrieve statistics for a specific workspace including counts of issues, history entries, JWTs, websocket connections, tasks, etc.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if workspaceID == 0 {
			return fmt.Errorf("workspace ID is required")
		}

		// Check if workspace exists
		workspaceExists, err := db.Connection().WorkspaceExists(workspaceID)
		if err != nil {
			return fmt.Errorf("error checking workspace existence: %v", err)
		}
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
			return fmt.Errorf("workspace with ID %d does not exist", workspaceID)
		}

		// Get workspace stats
		stats, err := db.Connection().GetWorkspaceStats(workspaceID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to retrieve workspace statistics")
			return fmt.Errorf("failed to retrieve workspace statistics: %v", err)
		}

		// Format and display output
		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			return fmt.Errorf("error parsing format type: %v", err)
		}

		formattedOutput, err := lib.FormatSingleOutput(stats, formatType)
		if err != nil {
			return fmt.Errorf("error formatting output: %v", err)
		}

		fmt.Println(formattedOutput)
		return nil
	},
}

func init() {
	WorkspaceStatsCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID (required)")
	WorkspaceStatsCmd.Flags().StringVarP(&format, "format", "f", "json", "Output format (json, yaml, table, text, pretty)")
	WorkspaceStatsCmd.MarkFlagRequired("workspace")
}
