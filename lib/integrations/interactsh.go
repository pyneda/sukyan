package integrations

import (
	"time"

	"github.com/projectdiscovery/interactsh/pkg/client"
	"github.com/projectdiscovery/interactsh/pkg/server"
)

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

func (i *InteractionsManager) GetURL() string {
	return i.client.URL()
}

func (i *InteractionsManager) Stop() {
	i.client.StopPolling()
	i.client.Close()
}
