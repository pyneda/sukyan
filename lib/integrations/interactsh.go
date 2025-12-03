package integrations

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/projectdiscovery/interactsh/pkg/client"
	"github.com/projectdiscovery/interactsh/pkg/options"
	"github.com/projectdiscovery/interactsh/pkg/server"
	"github.com/projectdiscovery/interactsh/pkg/storage"
	fileutil "github.com/projectdiscovery/utils/file"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type InteractionDomain struct {
	ID  string
	URL string
}

type InteractionsManager struct {
	client                *client.Client
	GetAsnInfo            bool
	PollingInterval       time.Duration
	KeepAliveInterval     time.Duration
	SessionFile           string
	OnInteractionCallback func(interaction *server.Interaction)
	OnEvictionCallback    func()
	mu                    sync.Mutex
}

func (i *InteractionsManager) Start() {
	i.mu.Lock()
	defer i.mu.Unlock()

	clientOptions := client.DefaultOptions
	if viper.GetString("scan.oob.server_urls") != "" {
		clientOptions.ServerURL = viper.GetString("scan.oob.server_urls")
	}

	// Set keep-alive interval to prevent correlation ID eviction
	if i.KeepAliveInterval > 0 {
		clientOptions.KeepAliveInterval = i.KeepAliveInterval
	} else {
		clientOptions.KeepAliveInterval = time.Minute // default to 1 minute
	}

	// Try to load existing session to resume previous correlation ID
	if i.SessionFile != "" && fileutil.FileExists(i.SessionFile) {
		sessionInfo, err := i.loadSession()
		if err != nil {
			log.Warn().Err(err).Str("file", i.SessionFile).Msg("Could not load interactsh session, will create new one")
		} else {
			clientOptions.SessionInfo = sessionInfo
			log.Info().Str("correlation_id", sessionInfo.CorrelationID).Msg("Loaded existing interactsh session")
		}
	}

	var err error
	i.client, err = client.New(clientOptions)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not create interactsh client")
		os.Exit(1)
	}

	// Save session for future restarts
	if i.SessionFile != "" {
		if err := i.client.SaveSessionTo(i.SessionFile); err != nil {
			log.Error().Err(err).Str("file", i.SessionFile).Msg("Could not save interactsh session")
		} else {
			log.Info().Str("file", i.SessionFile).Msg("Saved interactsh session")
		}
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

	// Start a goroutine to monitor for eviction errors and trigger callback
	go i.monitorEviction()
}

func (i *InteractionsManager) loadSession() (*options.SessionInfo, error) {
	data, err := os.ReadFile(i.SessionFile)
	if err != nil {
		return nil, err
	}
	var sessionInfo options.SessionInfo
	if err := yaml.Unmarshal(data, &sessionInfo); err != nil {
		return nil, err
	}
	return &sessionInfo, nil
}

func (i *InteractionsManager) monitorEviction() {
	// The interactsh client logs eviction errors but continues polling.
	// We periodically check if the client is still valid by attempting to get a URL.
	// If the client state is Closed or we detect issues, we trigger the callback.
	ticker := time.NewTicker(i.PollingInterval * 2)
	defer ticker.Stop()

	for range ticker.C {
		i.mu.Lock()
		if i.client == nil {
			i.mu.Unlock()
			return
		}
		// Check if client URL returns empty (indicates closed/evicted state)
		url := i.client.URL()
		i.mu.Unlock()

		if url == "" && i.OnEvictionCallback != nil {
			log.Warn().Msg("Detected interactsh client eviction or closure, triggering callback")
			i.OnEvictionCallback()
			return
		}
	}
}

// Restart stops the current client and starts a fresh one with a new correlation ID.
// This is useful for recovering from eviction errors.
func (i *InteractionsManager) Restart() {
	log.Info().Msg("Restarting interactsh client")

	// Stop the current client
	i.Stop()

	// Delete session file to force fresh registration
	if i.SessionFile != "" && fileutil.FileExists(i.SessionFile) {
		if err := os.Remove(i.SessionFile); err != nil {
			log.Error().Err(err).Str("file", i.SessionFile).Msg("Could not delete interactsh session file")
		}
	}

	// Start fresh
	i.Start()
}

// IsEvicted checks if the correlation ID has been evicted by attempting to get a URL
func (i *InteractionsManager) IsEvicted() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.client == nil {
		return true
	}
	return i.client.URL() == ""
}

// ErrCorrelationIdNotFound is exposed for callers to check against
var ErrCorrelationIdNotFound = storage.ErrCorrelationIdNotFound

func (i *InteractionsManager) GetIdentifierFromURL(url string) string {
	parts := strings.Split(url, ".")
	if len(parts) > 1 {
		return parts[0]
	}
	return ""
}

func (i *InteractionsManager) GetURL() InteractionDomain {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.client == nil {
		return InteractionDomain{}
	}
	url := i.client.URL()
	return InteractionDomain{
		ID:  i.GetIdentifierFromURL(url),
		URL: url,
	}
}

func (i *InteractionsManager) Stop() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.client == nil {
		return
	}
	err := i.client.StopPolling()
	if err != nil {
		log.Error().Err(err).Msg("Error stopping interactsh client polling")
	}
	err = i.client.Close()
	if err != nil {
		log.Error().Err(err).Msg("Error closing interactsh client")
	}
	i.client = nil
}
