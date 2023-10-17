package scan

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/rs/zerolog/log"
)

func ActiveScanWebSocketConnection(item *db.WebSocketConnection, interactionsManager *integrations.InteractionsManager, payloadGenerators []*generation.PayloadGenerator, options HistoryItemScanOptions) {
	for _, msg := range item.Messages {
		log.Debug().Msgf("Sending message %s", msg.PayloadData)
	}
}
