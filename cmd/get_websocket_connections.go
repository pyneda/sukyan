package cmd

import (
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	taskID uint
)

// getWebSocketConnectionsCmd represents the get WebSocket connections command
var getWebSocketConnectionsCmd = &cobra.Command{
	Use:   "websockets",
	Short: "List WebSocket connections",
	Run: func(cmd *cobra.Command, args []string) {
		for _, source := range filterHistorySources {
			if !db.IsValidSource(source) {
				fmt.Printf("Invalid source received: %s\n\n", source)
				fmt.Println("Valid sources are:")
				for _, s := range db.Sources {
					fmt.Printf("  - %s\n", s)
				}
				return
			}
		}

		filters := db.WebSocketConnectionFilter{
			Pagination: db.Pagination{
				PageSize: pageSize,
				Page:     page,
			},
			WorkspaceID: workspaceID,
			TaskID:      taskID,
			Sources:     filterHistorySources,
		}

		connections, _, err := db.Connection.ListWebSocketConnections(filters)
		if err != nil {
			return
		}

		if len(connections) == 0 {
			fmt.Println("No WebSocket connections found")
			return
		}

		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			log.Error().Err(err).Msg("Error parsing format type")
			return
		}

		formattedOutput, err := lib.FormatOutput(connections, formatType)
		if err != nil {
			log.Error().Err(err).Msg("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
	},
}

func init() {
	getCmd.AddCommand(getWebSocketConnectionsCmd)
	getWebSocketConnectionsCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
	getWebSocketConnectionsCmd.Flags().UintVar(&taskID, "task", 0, "Task ID")
	getWebSocketConnectionsCmd.Flags().StringSliceVarP(&filterHistorySources, "source", "S", []string{}, "Filter by source. Can be added multiple times.")
	getWebSocketConnectionsCmd.PersistentFlags().StringVarP(&query, "query", "q", "", "Search query")
}
