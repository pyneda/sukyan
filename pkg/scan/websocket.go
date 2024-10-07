package scan

import (
	"net/url"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/rs/zerolog/log"
)

func EvaluateWebSocketConnections(connections []db.WebSocketConnection, interactionsManager *integrations.InteractionsManager, payloadGenerators []*generation.PayloadGenerator, options HistoryItemScanOptions) {
	connectionsPerHost := make(map[string][]db.WebSocketConnection)
	cleartextConnectionsPerHost := make(map[string][]db.WebSocketConnection)
	for _, item := range connections {
		u, err := url.Parse(item.URL)
		if err != nil {
			log.Error().Err(err).Str("url", item.URL).Uint("connection", item.ID).Msg("Could not parse websocket connection url URL")
			continue
		}

		host := u.Host
		connectionsPerHost[host] = append(connectionsPerHost[host], item)
		if u.Scheme == "ws" {
			cleartextConnectionsPerHost[host] = append(cleartextConnectionsPerHost[host], item)
			db.CreateIssueFromWebSocketConnectionAndTemplate(&item, db.UnencryptedWebsocketConnectionCode, "", 100, "", &options.WorkspaceID, &options.TaskID, &options.TaskJobID)
		}
		ActiveScanWebSocketConnection(&item, interactionsManager, payloadGenerators, options)
	}

}

func ActiveScanWebSocketConnection(item *db.WebSocketConnection, interactionsManager *integrations.InteractionsManager, payloadGenerators []*generation.PayloadGenerator, options HistoryItemScanOptions) {
	log.Info().Uint("connection", item.ID).Msg("Active scanning websocket connection")
	for _, msg := range item.Messages {
		log.Debug().Msgf("Sending message %s", msg.PayloadData)
	}
}
