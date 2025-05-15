package scan

import (
	"math"
	"math/rand/v2"
	"time"

	"github.com/projectdiscovery/interactsh/pkg/server"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

func SaveInteractionCallback(interaction *server.Interaction) {
	log.Info().Str("protocol", interaction.Protocol).Str("full_id", interaction.FullId).Str("remote_address", interaction.RemoteAddress).Msg("Got interaction")

	interactionToSave := db.OOBInteraction{
		Protocol:      interaction.Protocol,
		FullID:        interaction.FullId,
		UniqueID:      interaction.UniqueID,
		QType:         interaction.QType,
		RawRequest:    interaction.RawRequest,
		RawResponse:   interaction.RawResponse,
		RemoteAddress: interaction.RemoteAddress,
		Timestamp:     interaction.Timestamp,
	}

	dbInteraction, err := db.Connection().CreateInteraction(&interactionToSave)
	if err != nil {
		log.Error().Err(err).Msg("Failed to save interaction")
		return
	}

	go attemptMatchWithRetry(dbInteraction.ID)
}

func attemptMatchWithRetry(interactionID uint) {
	maxRetries := 5
	baseDelay := 2 * time.Second
	maxDelay := 30 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Calculate exponential backoff with jitter
		delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))
		if delay > maxDelay {
			delay = maxDelay
		}
		// Add some jitter to prevent thundering herd
		jitter := time.Duration(rand.Float64() * float64(delay) * 0.2)
		delay += jitter

		select {
		case <-time.After(delay):
			currentInteraction, err := db.Connection().GetInteraction(interactionID)
			if err != nil {
				log.Error().Err(err).Uint("interaction_id", interactionID).Msg("Failed to fetch interaction")
				continue
			}

			if currentInteraction.OOBTestID != nil {
				log.Debug().Uint("interaction_id", interactionID).Msg("Interaction already matched, skipping")
				return
			}

			oobTest, err := db.Connection().MatchInteractionWithOOBTest(*currentInteraction)
			if err == nil || oobTest.ID != 0 {
				log.Info().
					Uint("interaction_id", interactionID).
					Uint("oob_test_id", oobTest.ID).
					Str("oob_test_name", oobTest.TestName).
					Int("attempt", attempt+1).
					Msg("Successfully matched interaction with OOBTest")
				return
			}

			log.Warn().
				Err(err).
				Uint("interaction_id", interactionID).
				Int("attempt", attempt+1).
				Str("next_delay", delay.String()).
				Msg("Failed to match interaction with OOBTest, retrying")
		}
	}

	log.Error().
		Uint("interaction_id", interactionID).
		Msg("Failed to match interaction with OOBTest after all retries")
}
