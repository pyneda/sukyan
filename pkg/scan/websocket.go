package scan

import (
	"net/url"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
)

func EvaluateWebSocketConnections(connections []db.WebSocketConnection, interactionsManager *integrations.InteractionsManager, payloadGenerators []*generation.PayloadGenerator, options options.HistoryItemScanOptions) {
	connectionsPerHost := make(map[string][]db.WebSocketConnection)
	cleartextConnectionsPerHost := make(map[string][]db.WebSocketConnection)
	for _, item := range connections {
		u, err := url.Parse(item.URL)
		if err != nil {
			log.Error().Err(err).Str("url", item.URL).Uint("connection", item.ID).Msg("Could not parse websocket connection url URL")
			continue
		}

		connectionOptions := options
		taskJob, err := db.Connection().NewWebSocketTaskJob(
			options.TaskID,
			item.TaskTitle(),
			db.TaskJobRunning,
			item.ID,
		)
		if err != nil {
			log.Error().Err(err).Str("url", item.URL).Uint("connection", item.ID).Msg("Could not create task job for websocket connection")
			continue
		}

		connectionOptions.TaskJobID = taskJob.ID

		host := u.Host
		connectionsPerHost[host] = append(connectionsPerHost[host], item)
		if u.Scheme == "ws" {
			cleartextConnectionsPerHost[host] = append(cleartextConnectionsPerHost[host], item)
			db.CreateIssueFromWebSocketConnectionAndTemplate(&item, db.UnencryptedWebsocketConnectionCode, "", 100, "", &connectionOptions.WorkspaceID, &connectionOptions.TaskID, &connectionOptions.TaskJobID)
		}
		ActiveScanWebSocketConnection(&item, interactionsManager, payloadGenerators, connectionOptions)
		taskJob.Status = db.TaskJobFinished
		taskJob.CompletedAt = time.Now()
		db.Connection().UpdateTaskJob(taskJob)
	}
	log.Info().Msg("Completed active scanning websocket connections")

}

func ActiveScanWebSocketConnection(item *db.WebSocketConnection, interactionsManager *integrations.InteractionsManager, payloadGenerators []*generation.PayloadGenerator, options options.HistoryItemScanOptions) {
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
		Concurrency:         6,
		InteractionsManager: interactionsManager,
		AvoidRepeatedIssues: true,
		WorkspaceID:         options.WorkspaceID,
		Mode:                options.Mode,
		ObservationWindow:   5 * time.Second,
	}

	wsOptions := WebSocketScanOptions{
		WorkspaceID:       options.WorkspaceID,
		TaskID:            options.TaskID,
		TaskJobID:         options.TaskJobID,
		Mode:              options.Mode,
		FingerprintTags:   options.FingerprintTags,
		ReplayMessages:    true, // Default to replaying messages to maintain state
		ObservationWindow: 5 * time.Second,
	}

	for i, msg := range messages {
		// Only scan messages that were sent from client to server
		if msg.Direction != db.MessageSent {
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

		// Run scan for this message and its insertion points
		results := scanner.Run(item, messages, i, payloadGenerators, insertionPoints, wsOptions)

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

	log.Info().Uint("connection", item.ID).Msg("Completed active scanning WebSocket connection")
}
