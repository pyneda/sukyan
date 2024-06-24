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
			continue
		}

		host := u.Host
		connectionsPerHost[host] = append(connectionsPerHost[host], item)
		if u.Scheme == "ws" {
			cleartextConnectionsPerHost[host] = append(cleartextConnectionsPerHost[host], item)
		}

		ActiveScanWebSocketConnection(&item, interactionsManager, payloadGenerators, options)
	}

	// for host, items := range cleartextConnectionsPerHost {

	// }

}

func ActiveScanWebSocketConnection(item *db.WebSocketConnection, interactionsManager *integrations.InteractionsManager, payloadGenerators []*generation.PayloadGenerator, options HistoryItemScanOptions) {
	for _, msg := range item.Messages {
		log.Debug().Msgf("Sending message %s", msg.PayloadData)
	}
}
