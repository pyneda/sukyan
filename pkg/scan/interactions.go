package scan

import (
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"github.com/projectdiscovery/interactsh/pkg/server"

)

func SaveInteractionCallback(interaction *server.Interaction) {
	log.Info().Str("protocol", interaction.Protocol).Str("full_id", interaction.FullId).Str("remote_address", interaction.RemoteAddress).Msg("Got interaction")
	// By now just saving it, but should also try to match it with some OOBTest
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
	db.Connection.CreateInteraction(interactionToSave)

}
