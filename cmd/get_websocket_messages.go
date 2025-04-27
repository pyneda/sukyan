package cmd

import (
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var filterWebSocketConnectionID uint

var listWebSocketMessagesCmd = &cobra.Command{
	Use:     "messages",
	Aliases: []string{"message", "msg", "websocket-messages", "websocket-message", "ws-messages", "ws-message"},
	Short:   "List WebSocket messages",
	Run: func(cmd *cobra.Command, args []string) {
		filters := db.WebSocketMessageFilter{
			Pagination: db.Pagination{
				PageSize: pageSize,
				Page:     page,
			},
			ConnectionID: filterWebSocketConnectionID,
		}

		messages, _, err := db.Connection().ListWebSocketMessages(filters)
		if err != nil {
			log.Error().Err(err).Msg("Error listing WebSocket messages")
			return
		}

		if len(messages) == 0 {
			fmt.Println("No WebSocket messages found")
			return
		}

		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			log.Error().Err(err).Msg("Error parsing format type")
			return
		}

		formattedOutput, err := lib.FormatOutput(messages, formatType)
		if err != nil {
			log.Error().Err(err).Msg("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
	},
}

func init() {
	getCmd.AddCommand(listWebSocketMessagesCmd)
	listWebSocketMessagesCmd.Flags().UintVarP(&filterWebSocketConnectionID, "connection-id", "c", 0, "Filter messages by WebSocket connection ID")
}
