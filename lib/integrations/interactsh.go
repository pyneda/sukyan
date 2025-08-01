package integrations

import (
	"os"
	"strings"
	"time"

	"github.com/projectdiscovery/interactsh/pkg/client"
	"github.com/projectdiscovery/interactsh/pkg/server"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type InteractionDomain struct {
	ID  string
	URL string
}

type InteractionsManager struct {
	client                *client.Client
	GetAsnInfo            bool
	PollingInterval       time.Duration
	OnInteractionCallback func(interaction *server.Interaction)
}

func (i *InteractionsManager) Start() {
	options := client.DefaultOptions
	if viper.GetString("scan.oob.server_urls") != "" {
		options.ServerURL = viper.GetString("scan.oob.server_urls")
	}
	var err error
	i.client, err = client.New(options)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not create interactsh client")
		os.Exit(1)
	}
	i.client.StartPolling(i.PollingInterval, func(interaction *server.Interaction) {
		if i.GetAsnInfo {
			err := i.client.TryGetAsnInfo(interaction)
			if err != nil {
				log.Error().Err(err).Str("interaction_id", interaction.FullId).Msg("Error getting ASN info for interaction")
			}
		}
		i.OnInteractionCallback(interaction)
	})
}

func (i *InteractionsManager) GetIdentifierFromURL(url string) string {
	parts := strings.Split(url, ".")
	if len(parts) > 1 {
		return parts[0]
	}
	return ""
}

func (i *InteractionsManager) GetURL() InteractionDomain {
	url := i.client.URL()
	return InteractionDomain{
		ID:  i.GetIdentifierFromURL(url),
		URL: url,
	}
}

func (i *InteractionsManager) Stop() {
	err := i.client.StopPolling()
	if err != nil {
		log.Error().Err(err).Msg("Error stopping interactsh client polling")
	}
	err = i.client.Close()
	if err != nil {
		log.Error().Err(err).Msg("Error closing interactsh client")
	}
}
