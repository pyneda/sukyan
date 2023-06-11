package integrations

import (
	"strings"
	"time"

	"github.com/projectdiscovery/interactsh/pkg/client"
	"github.com/projectdiscovery/interactsh/pkg/server"
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
	i.client, _ = client.New(client.DefaultOptions)
	i.client.StartPolling(i.PollingInterval, func(interaction *server.Interaction) {
		if i.GetAsnInfo {
			i.client.TryGetAsnInfo(interaction)
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
	i.client.StopPolling()
	i.client.Close()
}
