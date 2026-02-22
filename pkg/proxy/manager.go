package proxy

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// ProxyManager manages the lifecycle of multiple proxy instances
type ProxyManager struct {
	mu      sync.RWMutex
	proxies map[uuid.UUID]*RunningProxy
}

// RunningProxy represents a running proxy instance
type RunningProxy struct {
	ID          uuid.UUID
	Service     *db.ProxyService
	StartedAt   time.Time
	CancelFunc  context.CancelFunc
	Host        string
	Port        int
	WorkspaceID uint
}

// ProxyStatus represents the runtime status of a proxy
type ProxyStatus struct {
	ID        uuid.UUID
	Running   bool
	StartedAt time.Time
	Host      string
	Port      int
}

// NewProxyManager creates a new ProxyManager
func NewProxyManager() *ProxyManager {
	return &ProxyManager{
		proxies: make(map[uuid.UUID]*RunningProxy),
	}
}

// StartProxy starts a proxy instance by ID
func (pm *ProxyManager) StartProxy(ctx context.Context, proxyServiceID uuid.UUID) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if already running
	if _, exists := pm.proxies[proxyServiceID]; exists {
		return fmt.Errorf("proxy %s is already running", proxyServiceID)
	}

	// Fetch proxy service from database
	service, err := db.Connection().GetProxyServiceByID(proxyServiceID)
	if err != nil {
		return fmt.Errorf("failed to fetch proxy service: %w", err)
	}

	// Check port availability
	if err := pm.checkPortAvailable(service.Port, proxyServiceID); err != nil {
		return err
	}

	// Create cancellable context for this proxy
	proxyCtx, cancel := context.WithCancel(ctx)

	// Create running proxy entry
	running := &RunningProxy{
		ID:          service.ID,
		Service:     service,
		StartedAt:   time.Now(),
		CancelFunc:  cancel,
		Host:        service.Host,
		Port:        service.Port,
		WorkspaceID: *service.WorkspaceID,
	}

	// Store in map
	pm.proxies[proxyServiceID] = running

	// Start proxy in goroutine
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Interface("panic", r).Str("proxy_id", proxyServiceID.String()).Msg("Proxy panic recovered")
				pm.mu.Lock()
				delete(pm.proxies, proxyServiceID)
				pm.mu.Unlock()
			}
		}()

		log.Info().
			Str("proxy_id", proxyServiceID.String()).
			Str("address", fmt.Sprintf("%s:%d", service.Host, service.Port)).
			Uint("workspace_id", *service.WorkspaceID).
			Msg("Starting proxy instance")

		// STUB: Wait for context cancellation since RunWithContext doesn't exist yet (Task 6)
		// TODO: Replace with actual proxy.RunWithContext(proxyCtx) in Task 6
		<-proxyCtx.Done()

		log.Info().
			Str("proxy_id", proxyServiceID.String()).
			Msg("Proxy instance stopped")

		pm.mu.Lock()
		delete(pm.proxies, proxyServiceID)
		pm.mu.Unlock()
	}()

	log.Info().
		Str("proxy_id", proxyServiceID.String()).
		Str("address", fmt.Sprintf("%s:%d", service.Host, service.Port)).
		Msg("Proxy started successfully")

	return nil
}

// StopProxy stops a running proxy instance
func (pm *ProxyManager) StopProxy(proxyServiceID uuid.UUID) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	running, exists := pm.proxies[proxyServiceID]
	if !exists {
		return fmt.Errorf("proxy %s is not running", proxyServiceID)
	}

	log.Info().
		Str("proxy_id", proxyServiceID.String()).
		Msg("Stopping proxy instance")

	// Cancel the context to stop the proxy
	running.CancelFunc()

	// Remove from map (goroutine will also remove it, but we do it here for immediate state update)
	delete(pm.proxies, proxyServiceID)

	log.Info().
		Str("proxy_id", proxyServiceID.String()).
		Msg("Proxy stopped successfully")

	return nil
}

// RestartProxy restarts a running proxy instance
func (pm *ProxyManager) RestartProxy(ctx context.Context, proxyServiceID uuid.UUID) error {
	log.Info().
		Str("proxy_id", proxyServiceID.String()).
		Msg("Restarting proxy instance")

	// Stop if running
	pm.mu.RLock()
	_, exists := pm.proxies[proxyServiceID]
	pm.mu.RUnlock()

	if exists {
		if err := pm.StopProxy(proxyServiceID); err != nil {
			return fmt.Errorf("failed to stop proxy during restart: %w", err)
		}
		// Give it a moment to fully stop
		time.Sleep(100 * time.Millisecond)
	}

	// Start again
	if err := pm.StartProxy(ctx, proxyServiceID); err != nil {
		return fmt.Errorf("failed to start proxy during restart: %w", err)
	}

	return nil
}

// GetStatus returns the runtime status of a proxy
func (pm *ProxyManager) GetStatus(proxyServiceID uuid.UUID) (*ProxyStatus, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	running, exists := pm.proxies[proxyServiceID]
	if !exists {
		// Not running - fetch from database to get basic info
		service, err := db.Connection().GetProxyServiceByID(proxyServiceID)
		if err != nil {
			return nil, fmt.Errorf("proxy not running and not found in database: %w", err)
		}
		return &ProxyStatus{
			ID:      service.ID,
			Running: false,
			Host:    service.Host,
			Port:    service.Port,
		}, nil
	}

	return &ProxyStatus{
		ID:        running.ID,
		Running:   true,
		StartedAt: running.StartedAt,
		Host:      running.Host,
		Port:      running.Port,
	}, nil
}

// StartAllEnabled starts all enabled proxy services (called on server startup)
func (pm *ProxyManager) StartAllEnabled(ctx context.Context) error {
	services, err := db.Connection().ListEnabledProxyServices()
	if err != nil {
		return fmt.Errorf("failed to list enabled proxies: %w", err)
	}

	log.Info().Int("count", len(services)).Msg("Starting enabled proxy services")

	var startErrors []error
	for _, service := range services {
		if err := pm.StartProxy(ctx, service.ID); err != nil {
			log.Error().
				Err(err).
				Str("proxy_id", service.ID.String()).
				Str("name", service.Name).
				Msg("Failed to start enabled proxy")
			startErrors = append(startErrors, fmt.Errorf("proxy %s (%s): %w", service.ID, service.Name, err))
		}
	}

	if len(startErrors) > 0 {
		return fmt.Errorf("failed to start %d proxy(ies): %v", len(startErrors), startErrors)
	}

	return nil
}

// Shutdown gracefully stops all running proxies
func (pm *ProxyManager) Shutdown() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	log.Info().Int("count", len(pm.proxies)).Msg("Shutting down all running proxies")

	var shutdownErrors []error
	for id, running := range pm.proxies {
		log.Info().
			Str("proxy_id", id.String()).
			Msg("Stopping proxy during shutdown")

		running.CancelFunc()
		delete(pm.proxies, id)
	}

	// Give proxies a moment to clean up
	time.Sleep(100 * time.Millisecond)

	if len(shutdownErrors) > 0 {
		return fmt.Errorf("shutdown errors: %v", shutdownErrors)
	}

	log.Info().Msg("All proxies shut down successfully")
	return nil
}

// checkPortAvailable checks if a port is available for use
// Returns error if port is in use by another proxy in the manager
func (pm *ProxyManager) checkPortAvailable(port int, excludeID uuid.UUID) error {
	// Check if any running proxy is using this port
	for id, running := range pm.proxies {
		if id != excludeID && running.Port == port {
			return fmt.Errorf("port %d is already in use by proxy %s", port, id)
		}
	}

	// Attempt to bind to the port to verify it's available at OS level
	// This is a quick check - actual binding happens in the proxy server
	listenAddr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("port %d is not available: %w", port, err)
	}
	listener.Close()

	return nil
}
