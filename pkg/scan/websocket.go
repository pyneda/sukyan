package scan

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/rs/zerolog/log"
)

func ActiveScanWebSocketConnection(item *db.WebSocketConnection, interactionsManager *integrations.InteractionsManager, payloadGenerators []*generation.PayloadGenerator, options WebSocketScanOptions, deduplicationManager *WebSocketDeduplicationManager) {
	log.Info().Uint("connection", item.ID).Msg("Active scanning websocket connection")
	scopedInsertionPoints := []string{}
	for _, t := range WebSocketInsertionPointTypes() {
		scopedInsertionPoints = append(scopedInsertionPoints, t.String())
	}
	log.Info().Uint("connection", item.ID).Strs("scopedInsertionPoints", scopedInsertionPoints).Msg("Using insertion points")
	messages := item.Messages
	if len(messages) == 0 {
		log.Warn().Uint("connection", item.ID).Msg("No messages to replay received, trying to fetch them")
		dbMessages, _, err := db.Connection().ListWebSocketMessages(db.WebSocketMessageFilter{
			ConnectionID: item.ID,
		})

		if err != nil {
			log.Error().Err(err).Uint("connection", item.ID).Msg("Error listing websocket messages messages")
			return
		}
		if len(dbMessages) == 0 {
			log.Warn().Uint("connection", item.ID).Msg("No messages to replay")
			// NOTE: Could consider just doing raw message fuzzing
			return
		}
		messages = dbMessages
	}

	log.Info().Uint("connection", item.ID).Int("messages", len(messages)).Msg("Found messages to replay")
	scanner := WebSocketScanner{
		InteractionsManager: interactionsManager,
		AvoidRepeatedIssues: true,
	}
	skippedMessages := 0
	scannedMessages := 0

	for i, msg := range messages {
		// Only scan messages that were sent from client to server
		if msg.Direction != db.MessageSent {
			continue
		}

		if deduplicationManager != nil && !deduplicationManager.ShouldScanMessage(item.ID, &msg) {
			skippedMessages++
			log.Debug().
				Uint("connection", item.ID).
				Uint("message", msg.ID).
				Int("index", i).
				Str("mode", options.Mode.String()).
				Msg("Skipping WebSocket message due to deduplication rules")
			continue
		}

		log.Info().
			Uint("connection", item.ID).
			Uint("message", msg.ID).
			Int("index", i).
			Str("payload", msg.PayloadData).
			Msg("Scanning WebSocket message")

		// Find insertion points for this message
		insertionPoints, err := GetWebSocketMessageInsertionPoints(&msg, scopedInsertionPoints)
		log.Info().Uint("connection", item.ID).Uint("message", msg.ID).Int("insertionPoints", len(insertionPoints)).Msg("Found insertion points for WebSocket message")
		if err != nil {
			log.Error().Err(err).Uint("connection", item.ID).Uint("message", msg.ID).Msg("Failed to get insertion points")
			continue
		}

		if len(insertionPoints) == 0 {
			log.Debug().Uint("connection", item.ID).Uint("message", msg.ID).Msg("No insertion points found for WebSocket message")
			continue
		}

		log.Info().
			Uint("connection", item.ID).
			Uint("message", msg.ID).
			Int("insertionPoints", len(insertionPoints)).
			Msg("Found insertion points for WebSocket message")

		if deduplicationManager != nil {
			deduplicationManager.MarkMessageAsScanned(item.ID, &msg)
		}
		scannedMessages++
		// Run scan for this message and its insertion points
		results := scanner.Run(item, messages, i, payloadGenerators, insertionPoints, options)

		totalVulnerabilities := 0
		for issueCode, scanResults := range results {
			log.Info().
				Uint("connection", item.ID).
				Str("issue", issueCode).
				Int("count", len(scanResults)).
				Msg("Found vulnerabilities")
			totalVulnerabilities += len(scanResults)
		}

		log.Info().
			Uint("connection", item.ID).
			Uint("message", msg.ID).
			Int("vulnerabilities", totalVulnerabilities).
			Msg("Completed scanning WebSocket message")
	}

	log.Info().
		Uint("connection", item.ID).
		Int("scanned_messages", scannedMessages).
		Int("skipped_messages", skippedMessages).
		Str("mode", options.Mode.String()).
		Msg("Completed active scanning WebSocket connection")
}
