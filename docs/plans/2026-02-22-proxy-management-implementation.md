# Proxy Service Management System Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a complete proxy lifecycle management system with UI-driven control, traffic filtering, and live streaming.

**Architecture:** Database-backed proxy configuration with in-process goroutine manager, workspace-scoped proxies, auto-restart on server boot, REST API + WebSocket live streaming, React UI with real-time updates.

**Tech Stack:** Go (Fiber, GORM, Atlas), TypeScript (React, TanStack Router, TanStack Query), WebSocket for live streaming.

---

## Phase 1: Backend - Database Models

### Task 1: Create ProxyService Model

**Files:**
- Create: `sukyan/db/proxy_service.go`
- Test: `sukyan/db/proxy_service_test.go`

**Step 1: Write the failing test**

Create `sukyan/db/proxy_service_test.go`:

```go
package db_test

import (
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateProxyService(t *testing.T) {
	workspace := createTestWorkspace(t)

	proxyService := &db.ProxyService{
		WorkspaceID:           &workspace.ID,
		Name:                  "Test Proxy",
		Host:                  "localhost",
		Port:                  9001,
		Verbose:               true,
		LogOutOfScopeRequests: true,
		Enabled:               false,
	}

	created, err := db.Connection().CreateProxyService(proxyService)
	require.NoError(t, err)
	assert.NotNil(t, created.ID)
	assert.Equal(t, "Test Proxy", created.Name)
	assert.Equal(t, 9001, created.Port)
}

func TestProxyServicePortUniqueness(t *testing.T) {
	workspace := createTestWorkspace(t)

	proxy1 := &db.ProxyService{
		WorkspaceID: &workspace.ID,
		Name:        "Proxy 1",
		Port:        9002,
	}
	_, err := db.Connection().CreateProxyService(proxy1)
	require.NoError(t, err)

	// Try to create another proxy with same port
	proxy2 := &db.ProxyService{
		WorkspaceID: &workspace.ID,
		Name:        "Proxy 2",
		Port:        9002,
	}
	_, err = db.Connection().CreateProxyService(proxy2)
	assert.Error(t, err) // Should fail due to unique constraint
}

func createTestWorkspace(t *testing.T) *db.Workspace {
	workspace := &db.Workspace{
		Code:  "test-proxy-" + generateRandomString(8),
		Title: "Test Workspace",
	}
	created, err := db.Connection().CreateWorkspace(workspace)
	require.NoError(t, err)
	return created
}

func generateRandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[i%len(letters)]
	}
	return string(b)
}
```

**Step 2: Run test to verify it fails**

```bash
cd sukyan
go test ./db -run TestCreateProxyService -v
```

Expected: FAIL with "undefined: db.ProxyService"

**Step 3: Write minimal implementation**

Create `sukyan/db/proxy_service.go`:

```go
package db

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// ProxyService represents a managed proxy instance
type ProxyService struct {
	BaseUUIDModel

	// Workspace scoping
	WorkspaceID *uint     `json:"workspace_id" gorm:"index;not null"`
	Workspace   Workspace `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`

	// Basic config
	Name string `json:"name" gorm:"not null"`
	Host string `json:"host" gorm:"default:localhost"`
	Port int    `json:"port" gorm:"not null;uniqueIndex"`

	// Current proxy settings
	Verbose               bool `json:"verbose" gorm:"default:true"`
	LogOutOfScopeRequests bool `json:"log_out_of_scope_requests" gorm:"default:true"`

	// State management
	Enabled bool `json:"enabled" gorm:"default:false;index"`
}

// TableHeaders returns the table headers for ProxyService
func (p ProxyService) TableHeaders() []string {
	return []string{"ID", "Name", "Host", "Port", "Enabled"}
}

// TableRow returns the table row for ProxyService
func (p ProxyService) TableRow() []string {
	return []string{
		p.ID.String(),
		p.Name,
		p.Host,
		fmt.Sprintf("%d", p.Port),
		fmt.Sprintf("%t", p.Enabled),
	}
}

// CreateProxyService creates a new proxy service
func (conn *DatabaseConnection) CreateProxyService(proxyService *ProxyService) (*ProxyService, error) {
	result := conn.db.Create(proxyService)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("proxy_service", proxyService).Msg("ProxyService creation failed")
		return nil, result.Error
	}
	return proxyService, nil
}

// GetProxyServiceByID retrieves a proxy service by ID
func (conn *DatabaseConnection) GetProxyServiceByID(id uuid.UUID) (*ProxyService, error) {
	var proxyService ProxyService
	if err := conn.db.Where("id = ?", id).First(&proxyService).Error; err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Unable to fetch proxy service by ID")
		return nil, err
	}
	return &proxyService, nil
}

// GetProxyServiceByPort retrieves a proxy service by port
func (conn *DatabaseConnection) GetProxyServiceByPort(port int) (*ProxyService, error) {
	var proxyService ProxyService
	if err := conn.db.Where("port = ?", port).First(&proxyService).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		log.Error().Err(err).Int("port", port).Msg("Unable to fetch proxy service by port")
		return nil, err
	}
	return &proxyService, nil
}

// ListProxyServicesByWorkspace lists all proxy services for a workspace
func (conn *DatabaseConnection) ListProxyServicesByWorkspace(workspaceID uint) ([]*ProxyService, error) {
	var proxyServices []*ProxyService
	if err := conn.db.Where("workspace_id = ?", workspaceID).Find(&proxyServices).Error; err != nil {
		log.Error().Err(err).Uint("workspace_id", workspaceID).Msg("Unable to list proxy services")
		return nil, err
	}
	return proxyServices, nil
}

// ListEnabledProxyServices lists all enabled proxy services
func (conn *DatabaseConnection) ListEnabledProxyServices() ([]*ProxyService, error) {
	var proxyServices []*ProxyService
	if err := conn.db.Where("enabled = ?", true).Find(&proxyServices).Error; err != nil {
		log.Error().Err(err).Msg("Unable to list enabled proxy services")
		return nil, err
	}
	return proxyServices, nil
}

// UpdateProxyService updates a proxy service
func (conn *DatabaseConnection) UpdateProxyService(id uuid.UUID, updates *ProxyService) error {
	var proxyService ProxyService
	if err := conn.db.Where("id = ?", id).First(&proxyService).Error; err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Unable to fetch proxy service for update")
		return err
	}

	// Update fields
	if updates.Name != "" {
		proxyService.Name = updates.Name
	}
	if updates.Host != "" {
		proxyService.Host = updates.Host
	}
	if updates.Port != 0 {
		proxyService.Port = updates.Port
	}
	proxyService.Verbose = updates.Verbose
	proxyService.LogOutOfScopeRequests = updates.LogOutOfScopeRequests
	proxyService.Enabled = updates.Enabled

	if err := conn.db.Save(&proxyService).Error; err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Unable to update proxy service")
		return err
	}
	return nil
}

// DeleteProxyService deletes a proxy service
func (conn *DatabaseConnection) DeleteProxyService(id uuid.UUID) error {
	if err := conn.db.Where("id = ?", id).Delete(&ProxyService{}).Error; err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Unable to delete proxy service")
		return err
	}
	return nil
}

// SetProxyServiceEnabled sets the enabled status of a proxy service
func (conn *DatabaseConnection) SetProxyServiceEnabled(id uuid.UUID, enabled bool) error {
	if err := conn.db.Model(&ProxyService{}).Where("id = ?", id).Update("enabled", enabled).Error; err != nil {
		log.Error().Err(err).Str("id", id.String()).Bool("enabled", enabled).Msg("Unable to update proxy service enabled status")
		return err
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

```bash
cd sukyan
go test ./db -run TestCreateProxyService -v
go test ./db -run TestProxyServicePortUniqueness -v
```

Expected: PASS

**Step 5: Commit**

```bash
cd sukyan
git add db/proxy_service.go db/proxy_service_test.go
git commit -m "feat(db): add ProxyService model with CRUD operations

- UUID-based model with workspace scoping
- Port uniqueness constraint
- Enabled field for state management
- Full CRUD operations and filtering

🤖 Generated with Claude Code"
```

---

### Task 2: Update History Model with ProxyServiceID

**Files:**
- Modify: `sukyan/db/history.go`
- Test: `sukyan/db/history_test.go`

**Step 1: Write the failing test**

Add to `sukyan/db/history_test.go`:

```go
func TestHistoryWithProxyService(t *testing.T) {
	workspace := createTestWorkspace(t)
	proxyService := &db.ProxyService{
		WorkspaceID: &workspace.ID,
		Name:        "Test Proxy",
		Port:        9003,
	}
	created, err := db.Connection().CreateProxyService(proxyService)
	require.NoError(t, err)

	history := &db.History{
		WorkspaceID:    &workspace.ID,
		ProxyServiceID: &created.ID,
		URL:            "https://example.com",
		Method:         "GET",
		StatusCode:     200,
	}

	err = db.Connection().CreateHistory(history)
	require.NoError(t, err)
	assert.Equal(t, created.ID, *history.ProxyServiceID)

	// Fetch with preload
	fetched, err := db.Connection().GetHistoryByID(history.ID, true)
	require.NoError(t, err)
	assert.NotNil(t, fetched.ProxyService)
	assert.Equal(t, "Test Proxy", fetched.ProxyService.Name)
}
```

**Step 2: Run test to verify it fails**

```bash
cd sukyan
go test ./db -run TestHistoryWithProxyService -v
```

Expected: FAIL with "undefined: ProxyServiceID"

**Step 3: Add ProxyServiceID to History model**

In `sukyan/db/history.go`, add to the History struct:

```go
type History struct {
	// ... existing fields ...

	ProxyServiceID *uuid.UUID    `json:"proxy_service_id" gorm:"type:uuid;index"`
	ProxyService   *ProxyService `json:"proxy_service,omitempty" gorm:"foreignKey:ProxyServiceID"`
}
```

Update `GetHistoryByID` to preload ProxyService:

```go
func (conn *DatabaseConnection) GetHistoryByID(id uint, preload bool) (*History, error) {
	var history History
	query := conn.db
	if preload {
		query = query.Preload("Task").Preload("ProxyService")
	}
	if err := query.Where("id = ?", id).First(&history).Error; err != nil {
		return nil, err
	}
	return &history, nil
}
```

**Step 4: Run test to verify it passes**

```bash
cd sukyan
go test ./db -run TestHistoryWithProxyService -v
```

Expected: PASS

**Step 5: Commit**

```bash
cd sukyan
git add db/history.go db/history_test.go
git commit -m "feat(db): add ProxyServiceID to History model

- Add optional foreign key to ProxyService
- Support preloading proxy service with history
- Enable traffic filtering by proxy

🤖 Generated with Claude Code"
```

---

### Task 3: Update WebSocketConnection Model with ProxyServiceID

**Files:**
- Modify: `sukyan/db/websocket_connection.go`
- Test: `sukyan/db/websocket_connection_test.go`

**Step 1: Write the failing test**

Add to `sukyan/db/websocket_connection_test.go`:

```go
func TestWebSocketConnectionWithProxyService(t *testing.T) {
	workspace := createTestWorkspace(t)
	proxyService := &db.ProxyService{
		WorkspaceID: &workspace.ID,
		Name:        "WS Test Proxy",
		Port:        9004,
	}
	created, err := db.Connection().CreateProxyService(proxyService)
	require.NoError(t, err)

	wsConn := &db.WebSocketConnection{
		URL:            "wss://example.com/ws",
		WorkspaceID:    &workspace.ID,
		ProxyServiceID: &created.ID,
		Source:         db.SourceProxy,
	}

	err = db.Connection().CreateWebSocketConnection(wsConn)
	require.NoError(t, err)
	assert.Equal(t, created.ID, *wsConn.ProxyServiceID)

	// Fetch with preload
	fetched, err := db.Connection().GetWebSocketConnectionByID(wsConn.ID)
	require.NoError(t, err)
	assert.NotNil(t, fetched.ProxyService)
	assert.Equal(t, "WS Test Proxy", fetched.ProxyService.Name)
}
```

**Step 2: Run test to verify it fails**

```bash
cd sukyan
go test ./db -run TestWebSocketConnectionWithProxyService -v
```

Expected: FAIL

**Step 3: Add ProxyServiceID to WebSocketConnection model**

In `sukyan/db/websocket_connection.go`, add to the WebSocketConnection struct:

```go
type WebSocketConnection struct {
	// ... existing fields ...

	ProxyServiceID *uuid.UUID    `json:"proxy_service_id" gorm:"type:uuid;index"`
	ProxyService   *ProxyService `json:"proxy_service,omitempty" gorm:"foreignKey:ProxyServiceID"`
}
```

Update query methods to preload ProxyService where appropriate.

**Step 4: Run test to verify it passes**

```bash
cd sukyan
go test ./db -run TestWebSocketConnectionWithProxyService -v
```

Expected: PASS

**Step 5: Commit**

```bash
cd sukyan
git add db/websocket_connection.go db/websocket_connection_test.go
git commit -m "feat(db): add ProxyServiceID to WebSocketConnection

- Add optional foreign key to ProxyService
- Enable WebSocket traffic filtering by proxy

🤖 Generated with Claude Code"
```

---

### Task 4: Register Models and Generate Migration

**Files:**
- Modify: `sukyan/db/atlas/main.go`

**Step 1: Register ProxyService in Atlas**

In `sukyan/db/atlas/main.go`, add to the model list:

```go
&db.ProxyService{},
```

**Step 2: Generate migration**

```bash
cd sukyan
atlas migrate diff --env gorm
```

Expected: New migration file created in `db/migrations/`

**Step 3: Review generated migration**

```bash
cd sukyan
cat db/migrations/$(ls -t db/migrations | head -1)
```

Verify:
- `proxy_services` table created with UUID primary key
- Port has unique index
- `histories` table has `proxy_service_id` column
- `websocket_connections` table has `proxy_service_id` column

**Step 4: Apply migration**

```bash
cd sukyan
go run main.go migrate
```

Expected: Migration applied successfully

**Step 5: Commit**

```bash
cd sukyan
git add db/atlas/main.go db/migrations/
git commit -m "feat(db): add ProxyService migration

- Create proxy_services table with UUID PK
- Add proxy_service_id to histories
- Add proxy_service_id to websocket_connections
- Port uniqueness constraint

🤖 Generated with Claude Code"
```

---

## Phase 2: Backend - Proxy Manager

### Task 5: Create ProxyManager Core

**Files:**
- Create: `sukyan/pkg/proxy/manager.go`
- Test: `sukyan/pkg/proxy/manager_test.go`

**Step 1: Write the failing test**

Create `sukyan/pkg/proxy/manager_test.go`:

```go
package proxy_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyManagerStartStop(t *testing.T) {
	manager := proxy.NewProxyManager()

	// Create test workspace and proxy service
	workspace := createTestWorkspace(t)
	proxyService := &db.ProxyService{
		WorkspaceID: &workspace.ID,
		Name:        "Manager Test",
		Host:        "localhost",
		Port:        9100,
		Enabled:     false,
	}
	created, err := db.Connection().CreateProxyService(proxyService)
	require.NoError(t, err)

	// Start proxy
	err = manager.StartProxy(context.Background(), created.ID)
	require.NoError(t, err)

	// Verify it's running
	status, err := manager.GetStatus(created.ID)
	require.NoError(t, err)
	assert.True(t, status.Running)
	assert.NotZero(t, status.StartedAt)

	// Stop proxy
	err = manager.StopProxy(created.ID)
	require.NoError(t, err)

	// Verify it's stopped
	status, err = manager.GetStatus(created.ID)
	require.NoError(t, err)
	assert.False(t, status.Running)
}

func TestProxyManagerPortConflict(t *testing.T) {
	manager := proxy.NewProxyManager()

	workspace := createTestWorkspace(t)

	proxy1 := &db.ProxyService{
		WorkspaceID: &workspace.ID,
		Name:        "Proxy 1",
		Port:        9101,
	}
	created1, err := db.Connection().CreateProxyService(proxy1)
	require.NoError(t, err)

	// Start first proxy
	err = manager.StartProxy(context.Background(), created1.ID)
	require.NoError(t, err)

	proxy2 := &db.ProxyService{
		WorkspaceID: &workspace.ID,
		Name:        "Proxy 2",
		Port:        9101, // Same port
	}
	created2, err := db.Connection().CreateProxyService(proxy2)
	// DB allows creation (different IDs)
	require.NoError(t, err)

	// Try to start second proxy on same port
	err = manager.StartProxy(context.Background(), created2.ID)
	assert.Error(t, err) // Should fail - port in use

	// Cleanup
	manager.StopProxy(created1.ID)
}

func createTestWorkspace(t *testing.T) *db.Workspace {
	workspace := &db.Workspace{
		Code:  "test-mgr-" + uuid.New().String()[:8],
		Title: "Test Workspace",
	}
	created, err := db.Connection().CreateWorkspace(workspace)
	require.NoError(t, err)
	return created
}
```

**Step 2: Run test to verify it fails**

```bash
cd sukyan
go test ./pkg/proxy -run TestProxyManager -v
```

Expected: FAIL with "undefined: proxy.NewProxyManager"

**Step 3: Write minimal implementation**

Create `sukyan/pkg/proxy/manager.go`:

```go
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

// ProxyManager manages the lifecycle of all proxy instances
type ProxyManager struct {
	mu      sync.RWMutex
	proxies map[uuid.UUID]*RunningProxy
}

// RunningProxy represents a running proxy instance
type RunningProxy struct {
	Service    *db.ProxyService
	CancelFunc context.CancelFunc
	StartedAt  time.Time
}

// ProxyStatus represents the runtime status of a proxy
type ProxyStatus struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Enabled        bool      `json:"enabled"`
	Running        bool      `json:"running"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	UptimeSeconds  int64     `json:"uptime_seconds"`
	RequestsCount  int64     `json:"requests_count"`
	WebSocketCount int64     `json:"websocket_count"`
}

// NewProxyManager creates a new proxy manager
func NewProxyManager() *ProxyManager {
	return &ProxyManager{
		proxies: make(map[uuid.UUID]*RunningProxy),
	}
}

// StartProxy starts a proxy service
func (pm *ProxyManager) StartProxy(ctx context.Context, proxyServiceID uuid.UUID) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if already running
	if _, exists := pm.proxies[proxyServiceID]; exists {
		return fmt.Errorf("proxy service %s is already running", proxyServiceID)
	}

	// Load proxy service from database
	proxyService, err := db.Connection().GetProxyServiceByID(proxyServiceID)
	if err != nil {
		return fmt.Errorf("failed to load proxy service: %w", err)
	}

	// Check if port is available
	if err := pm.checkPortAvailable(proxyService.Port); err != nil {
		return fmt.Errorf("port %d is not available: %w", proxyService.Port, err)
	}

	// Create cancellable context
	proxyCtx, cancel := context.WithCancel(ctx)

	// Create proxy instance
	proxy := &Proxy{
		Host:                  proxyService.Host,
		Port:                  proxyService.Port,
		Verbose:               proxyService.Verbose,
		LogOutOfScopeRequests: proxyService.LogOutOfScopeRequests,
		WorkspaceID:           *proxyService.WorkspaceID,
		ProxyServiceID:        proxyService.ID,
	}

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

		err := proxy.RunWithContext(proxyCtx)
		if err != nil && err != context.Canceled {
			log.Error().Err(err).Str("proxy_id", proxyServiceID.String()).Msg("Proxy stopped with error")
		}

		pm.mu.Lock()
		delete(pm.proxies, proxyServiceID)
		pm.mu.Unlock()
	}()

	// Store running proxy
	pm.proxies[proxyServiceID] = &RunningProxy{
		Service:    proxyService,
		CancelFunc: cancel,
		StartedAt:  time.Now(),
	}

	// Update database enabled status
	if err := db.Connection().SetProxyServiceEnabled(proxyServiceID, true); err != nil {
		log.Warn().Err(err).Str("proxy_id", proxyServiceID.String()).Msg("Failed to update enabled status")
	}

	log.Info().Str("proxy_id", proxyServiceID.String()).Str("name", proxyService.Name).Int("port", proxyService.Port).Msg("Proxy started")
	return nil
}

// StopProxy stops a running proxy service
func (pm *ProxyManager) StopProxy(proxyServiceID uuid.UUID) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	running, exists := pm.proxies[proxyServiceID]
	if !exists {
		return fmt.Errorf("proxy service %s is not running", proxyServiceID)
	}

	// Cancel context to stop proxy
	running.CancelFunc()

	// Remove from map
	delete(pm.proxies, proxyServiceID)

	// Update database enabled status
	if err := db.Connection().SetProxyServiceEnabled(proxyServiceID, false); err != nil {
		log.Warn().Err(err).Str("proxy_id", proxyServiceID.String()).Msg("Failed to update enabled status")
	}

	log.Info().Str("proxy_id", proxyServiceID.String()).Msg("Proxy stopped")
	return nil
}

// RestartProxy restarts a proxy service
func (pm *ProxyManager) RestartProxy(ctx context.Context, proxyServiceID uuid.UUID) error {
	if err := pm.StopProxy(proxyServiceID); err != nil {
		// If not running, that's ok, we'll start it anyway
		log.Debug().Err(err).Str("proxy_id", proxyServiceID.String()).Msg("Proxy not running, starting fresh")
	}

	// Give it a moment to fully stop
	time.Sleep(100 * time.Millisecond)

	return pm.StartProxy(ctx, proxyServiceID)
}

// GetStatus returns the status of a proxy service
func (pm *ProxyManager) GetStatus(proxyServiceID uuid.UUID) (*ProxyStatus, error) {
	pm.mu.RLock()
	running, isRunning := pm.proxies[proxyServiceID]
	pm.mu.RUnlock()

	// Load from database to get config
	proxyService, err := db.Connection().GetProxyServiceByID(proxyServiceID)
	if err != nil {
		return nil, fmt.Errorf("failed to load proxy service: %w", err)
	}

	status := &ProxyStatus{
		ID:      proxyService.ID,
		Name:    proxyService.Name,
		Enabled: proxyService.Enabled,
		Running: isRunning,
	}

	if isRunning {
		status.StartedAt = running.StartedAt
		status.UptimeSeconds = int64(time.Since(running.StartedAt).Seconds())

		// Get traffic stats
		// TODO: Implement count queries for History and WebSocketConnection
	}

	return status, nil
}

// StartAllEnabled starts all enabled proxy services (called on server startup)
func (pm *ProxyManager) StartAllEnabled(ctx context.Context) error {
	proxies, err := db.Connection().ListEnabledProxyServices()
	if err != nil {
		return fmt.Errorf("failed to list enabled proxies: %w", err)
	}

	log.Info().Int("count", len(proxies)).Msg("Starting enabled proxy services")

	for _, proxyService := range proxies {
		if err := pm.StartProxy(ctx, proxyService.ID); err != nil {
			log.Error().Err(err).Str("proxy_id", proxyService.ID.String()).Str("name", proxyService.Name).Msg("Failed to start proxy, disabling")
			// Disable in database on failure
			db.Connection().SetProxyServiceEnabled(proxyService.ID, false)
		}
	}

	return nil
}

// Shutdown stops all running proxies
func (pm *ProxyManager) Shutdown(ctx context.Context) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	log.Info().Int("count", len(pm.proxies)).Msg("Shutting down all proxies")

	for id, running := range pm.proxies {
		running.CancelFunc()
		log.Info().Str("proxy_id", id.String()).Msg("Proxy stopped")
	}

	pm.proxies = make(map[uuid.UUID]*RunningProxy)
	return nil
}

// checkPortAvailable checks if a port is available
func (pm *ProxyManager) checkPortAvailable(port int) error {
	// Check if any running proxy is using this port
	for _, running := range pm.proxies {
		if running.Service.Port == port {
			return fmt.Errorf("port already in use by proxy %s", running.Service.Name)
		}
	}

	// Try to bind to port
	addr := fmt.Sprintf("localhost:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	listener.Close()

	return nil
}
```

**Step 4: Run test to verify it passes**

```bash
cd sukyan
go test ./pkg/proxy -run TestProxyManager -v
```

Expected: PASS

**Step 5: Commit**

```bash
cd sukyan
git add pkg/proxy/manager.go pkg/proxy/manager_test.go
git commit -m "feat(proxy): add ProxyManager for lifecycle control

- Start/stop/restart proxy goroutines
- Port conflict detection
- Auto-restart enabled proxies on startup
- Graceful shutdown support

🤖 Generated with Claude Code"
```

---

### Task 6: Update Proxy to Support Context and ProxyServiceID

**Files:**
- Modify: `sukyan/pkg/proxy/proxy.go`

**Step 1: Add ProxyServiceID field to Proxy struct**

In `sukyan/pkg/proxy/proxy.go`, update the Proxy struct:

```go
type Proxy struct {
	Host                  string
	Port                  int
	Verbose               bool
	LogOutOfScopeRequests bool
	WorkspaceID           uint
	ProxyServiceID        uuid.UUID  // NEW
	wsConnections         sync.Map
}
```

**Step 2: Add RunWithContext method**

Add new method that accepts context:

```go
func (p *Proxy) RunWithContext(ctx context.Context) error {
	err := p.SetCA()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to set CA")
		return err
	}
	listenAddress := fmt.Sprintf("%s:%d", p.Host, p.Port)
	log.Info().Str("address", listenAddress).Uint("workspace", p.WorkspaceID).Str("proxy_service_id", p.ProxyServiceID.String()).Msg("Proxy starting up")

	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = p.Verbose

	// ... rest of existing setup from Run() method ...

	// Create server
	server := &http.Server{
		Addr:    listenAddress,
		Handler: proxy,
	}

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		log.Info().Str("proxy_service_id", p.ProxyServiceID.String()).Msg("Shutting down proxy")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("Proxy shutdown error")
			return err
		}
		return nil
	case err := <-serverErr:
		return err
	}
}
```

**Step 3: Update history creation to include ProxyServiceID**

In the OnResponse handler where History is created, update the options:

```go
options := http_utils.HistoryCreationOptions{
	Source:              db.SourceProxy,
	WorkspaceID:         p.WorkspaceID,
	ProxyServiceID:      &p.ProxyServiceID,  // NEW
	TaskID:              0,
	CreateNewBodyStream: true,
	IsWebSocketUpgrade:  isWebSocketUpgrade,
}
```

**Step 4: Update WebSocketConnection creation**

In `createWebSocketConnection`, add ProxyServiceID:

```go
connection := &db.WebSocketConnection{
	URL:              resp.Request.URL.String(),
	RequestHeaders:   datatypes.JSON(requestHeaders),
	ResponseHeaders:  datatypes.JSON(responseHeaders),
	StatusCode:       resp.StatusCode,
	StatusText:       resp.Status,
	WorkspaceID:      &p.WorkspaceID,
	ProxyServiceID:   &p.ProxyServiceID,  // NEW
	Source:           db.SourceProxy,
	UpgradeRequestID: &history.ID,
}
```

**Step 5: Update http_utils.HistoryCreationOptions**

In `sukyan/pkg/http_utils/history.go`, add ProxyServiceID field:

```go
type HistoryCreationOptions struct {
	// ... existing fields ...
	ProxyServiceID      *uuid.UUID
}
```

Update the history creation to set the field:

```go
if options.ProxyServiceID != nil {
	history.ProxyServiceID = options.ProxyServiceID
}
```

**Step 6: Keep existing Run() method for CLI compatibility**

Add a wrapper that uses RunWithContext:

```go
func (p *Proxy) Run() {
	ctx := context.Background()
	if err := p.RunWithContext(ctx); err != nil {
		log.Fatal().Err(err).Msg("Proxy failed")
	}
}
```

**Step 7: Commit**

```bash
cd sukyan
git add pkg/proxy/proxy.go pkg/http_utils/history.go
git commit -m "feat(proxy): add context support and ProxyServiceID tracking

- RunWithContext for graceful shutdown
- Track ProxyServiceID in History and WebSocketConnection
- Maintain backward compatibility with CLI Run()

🤖 Generated with Claude Code"
```

---

## Phase 3: Backend - API Endpoints

### Task 7: Create Proxy Services API Endpoints

**Files:**
- Create: `sukyan/api/proxy_services.go`
- Test: `sukyan/api/proxy_services_test.go` (optional, can test manually)

**Step 1: Create API handlers**

Create `sukyan/api/proxy_services.go`:

```go
package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/proxy"
	"github.com/rs/zerolog/log"
)

// ProxyServiceCreateInput defines input for creating a proxy service
type ProxyServiceCreateInput struct {
	Name                  string `json:"name" validate:"required"`
	Host                  string `json:"host" validate:"required"`
	Port                  int    `json:"port" validate:"required,min=1,max=65535"`
	Verbose               bool   `json:"verbose"`
	LogOutOfScopeRequests bool   `json:"log_out_of_scope_requests"`
}

// ProxyServiceUpdateInput defines input for updating a proxy service
type ProxyServiceUpdateInput struct {
	Name                  string `json:"name"`
	Host                  string `json:"host"`
	Port                  int    `json:"port" validate:"omitempty,min=1,max=65535"`
	Verbose               bool   `json:"verbose"`
	LogOutOfScopeRequests bool   `json:"log_out_of_scope_requests"`
}

// CreateProxyService creates a new proxy service
func (s *Server) CreateProxyService(c *fiber.Ctx) error {
	workspaceID, err := getUintParam(c, "workspaceId")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid workspace ID"})
	}

	var input ProxyServiceCreateInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Check workspace exists
	exists, _ := db.Connection().WorkspaceExists(workspaceID)
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Workspace not found"})
	}

	// Check if port is already in use
	existing, err := db.Connection().GetProxyServiceByPort(input.Port)
	if err == nil && existing != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error":          "Port already in use",
			"conflicting_id": existing.ID,
			"conflicting_name": existing.Name,
		})
	}

	proxyService := &db.ProxyService{
		WorkspaceID:           &workspaceID,
		Name:                  input.Name,
		Host:                  input.Host,
		Port:                  input.Port,
		Verbose:               input.Verbose,
		LogOutOfScopeRequests: input.LogOutOfScopeRequests,
		Enabled:               false,
	}

	created, err := db.Connection().CreateProxyService(proxyService)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create proxy service")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create proxy service"})
	}

	return c.Status(fiber.StatusCreated).JSON(created)
}

// ListProxyServices lists all proxy services for a workspace
func (s *Server) ListProxyServices(c *fiber.Ctx) error {
	workspaceID, err := getUintParam(c, "workspaceId")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid workspace ID"})
	}

	proxies, err := db.Connection().ListProxyServicesByWorkspace(workspaceID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list proxy services")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to list proxy services"})
	}

	// Enrich with runtime status
	var enriched []fiber.Map
	for _, p := range proxies {
		status, _ := s.proxyManager.GetStatus(p.ID)
		item := fiber.Map{
			"id":                        p.ID,
			"workspace_id":              p.WorkspaceID,
			"name":                      p.Name,
			"host":                      p.Host,
			"port":                      p.Port,
			"verbose":                   p.Verbose,
			"log_out_of_scope_requests": p.LogOutOfScopeRequests,
			"enabled":                   p.Enabled,
			"created_at":                p.CreatedAt,
			"updated_at":                p.UpdatedAt,
		}
		if status != nil {
			item["running"] = status.Running
			item["started_at"] = status.StartedAt
			item["uptime_seconds"] = status.UptimeSeconds
		} else {
			item["running"] = false
		}
		enriched = append(enriched, item)
	}

	return c.JSON(fiber.Map{
		"data":  enriched,
		"count": len(enriched),
	})
}

// GetProxyService retrieves a proxy service by ID
func (s *Server) GetProxyService(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid proxy service ID"})
	}

	proxyService, err := db.Connection().GetProxyServiceByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Proxy service not found"})
	}

	status, _ := s.proxyManager.GetStatus(id)
	response := fiber.Map{
		"id":                        proxyService.ID,
		"workspace_id":              proxyService.WorkspaceID,
		"name":                      proxyService.Name,
		"host":                      proxyService.Host,
		"port":                      proxyService.Port,
		"verbose":                   proxyService.Verbose,
		"log_out_of_scope_requests": proxyService.LogOutOfScopeRequests,
		"enabled":                   proxyService.Enabled,
		"created_at":                proxyService.CreatedAt,
		"updated_at":                proxyService.UpdatedAt,
	}
	if status != nil {
		response["running"] = status.Running
		response["started_at"] = status.StartedAt
		response["uptime_seconds"] = status.UptimeSeconds
	} else {
		response["running"] = false
	}

	return c.JSON(response)
}

// UpdateProxyService updates a proxy service
func (s *Server) UpdateProxyService(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid proxy service ID"})
	}

	var input ProxyServiceUpdateInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Check if proxy exists
	existing, err := db.Connection().GetProxyServiceByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Proxy service not found"})
	}

	// If port is changing, check availability
	if input.Port != 0 && input.Port != existing.Port {
		portProxy, err := db.Connection().GetProxyServiceByPort(input.Port)
		if err == nil && portProxy != nil && portProxy.ID != id {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error":            "Port already in use",
				"conflicting_id":   portProxy.ID,
				"conflicting_name": portProxy.Name,
			})
		}
	}

	// Update proxy service
	updates := &db.ProxyService{
		Name:                  input.Name,
		Host:                  input.Host,
		Port:                  input.Port,
		Verbose:               input.Verbose,
		LogOutOfScopeRequests: input.LogOutOfScopeRequests,
	}

	if err := db.Connection().UpdateProxyService(id, updates); err != nil {
		log.Error().Err(err).Msg("Failed to update proxy service")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update proxy service"})
	}

	// If proxy is running, restart it with new config
	status, _ := s.proxyManager.GetStatus(id)
	if status != nil && status.Running {
		if err := s.proxyManager.RestartProxy(c.Context(), id); err != nil {
			log.Error().Err(err).Msg("Failed to restart proxy with new config")
		}
	}

	return c.JSON(fiber.Map{"message": "Proxy service updated"})
}

// DeleteProxyService deletes a proxy service
func (s *Server) DeleteProxyService(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid proxy service ID"})
	}

	// Check if exists
	_, err = db.Connection().GetProxyServiceByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Proxy service not found"})
	}

	// Stop if running
	status, _ := s.proxyManager.GetStatus(id)
	if status != nil && status.Running {
		if err := s.proxyManager.StopProxy(id); err != nil {
			log.Warn().Err(err).Msg("Failed to stop proxy before deletion")
		}
	}

	// Delete from database
	if err := db.Connection().DeleteProxyService(id); err != nil {
		log.Error().Err(err).Msg("Failed to delete proxy service")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete proxy service"})
	}

	return c.JSON(fiber.Map{"message": "Proxy service deleted"})
}

// StartProxyService starts a proxy service
func (s *Server) StartProxyService(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid proxy service ID"})
	}

	if err := s.proxyManager.StartProxy(c.Context(), id); err != nil {
		log.Error().Err(err).Msg("Failed to start proxy service")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	status, _ := s.proxyManager.GetStatus(id)
	return c.JSON(status)
}

// StopProxyService stops a proxy service
func (s *Server) StopProxyService(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid proxy service ID"})
	}

	if err := s.proxyManager.StopProxy(id); err != nil {
		log.Error().Err(err).Msg("Failed to stop proxy service")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	status, _ := s.proxyManager.GetStatus(id)
	return c.JSON(status)
}

// RestartProxyService restarts a proxy service
func (s *Server) RestartProxyService(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid proxy service ID"})
	}

	if err := s.proxyManager.RestartProxy(c.Context(), id); err != nil {
		log.Error().Err(err).Msg("Failed to restart proxy service")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	status, _ := s.proxyManager.GetStatus(id)
	return c.JSON(status)
}

func getUintParam(c *fiber.Ctx, param string) (uint, error) {
	val, err := c.ParamsInt(param)
	if err != nil {
		return 0, err
	}
	return uint(val), nil
}
```

**Step 2: Add ProxyManager to Server struct**

In `sukyan/api/server.go`, add ProxyManager field:

```go
type Server struct {
	// ... existing fields ...
	proxyManager *proxy.ProxyManager
}
```

Initialize in server creation:

```go
func NewServer() *Server {
	// ... existing code ...

	server := &Server{
		// ... existing fields ...
		proxyManager: proxy.NewProxyManager(),
	}

	// ... existing code ...

	return server
}
```

**Step 3: Register routes**

In `sukyan/api/server.go`, add route registration:

```go
func (server *Server) setupRoutes() {
	// ... existing routes ...

	// Proxy services routes
	apiV1.Post("/workspaces/:workspaceId/proxy-services", server.CreateProxyService)
	apiV1.Get("/workspaces/:workspaceId/proxy-services", server.ListProxyServices)
	apiV1.Get("/proxy-services/:id", server.GetProxyService)
	apiV1.Patch("/proxy-services/:id", server.UpdateProxyService)
	apiV1.Delete("/proxy-services/:id", server.DeleteProxyService)

	// Lifecycle routes
	apiV1.Post("/proxy-services/:id/start", server.StartProxyService)
	apiV1.Post("/proxy-services/:id/stop", server.StopProxyService)
	apiV1.Post("/proxy-services/:id/restart", server.RestartProxyService)
}
```

**Step 4: Start enabled proxies on server startup**

In server initialization, after database connection:

```go
func (server *Server) Start() error {
	// ... existing startup code ...

	// Start all enabled proxy services
	if err := server.proxyManager.StartAllEnabled(context.Background()); err != nil {
		log.Warn().Err(err).Msg("Failed to start some enabled proxies")
	}

	// ... rest of startup code ...
}
```

**Step 5: Shutdown proxies on server shutdown**

Add graceful shutdown:

```go
func (server *Server) Shutdown(ctx context.Context) error {
	log.Info().Msg("Shutting down server")

	// Shutdown all proxies
	if err := server.proxyManager.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Error shutting down proxies")
	}

	// ... existing shutdown code ...
}
```

**Step 6: Test manually**

```bash
cd sukyan
go run main.go api

# In another terminal:
# Create proxy
curl -X POST http://localhost:8080/api/v1/workspaces/1/proxy-services \
  -H "Content-Type: application/json" \
  -d '{"name":"Test Proxy","host":"localhost","port":9200,"verbose":true}'

# List proxies
curl http://localhost:8080/api/v1/workspaces/1/proxy-services

# Start proxy
curl -X POST http://localhost:8080/api/v1/proxy-services/{id}/start

# Check status
curl http://localhost:8080/api/v1/proxy-services/{id}
```

**Step 7: Commit**

```bash
cd sukyan
git add api/proxy_services.go api/server.go
git commit -m "feat(api): add proxy service management endpoints

- CRUD operations for proxy services
- Start/stop/restart lifecycle control
- Port conflict detection
- Auto-start enabled proxies on server boot
- Graceful shutdown support

🤖 Generated with Claude Code"
```

---

### Task 8: Add ProxyServiceID Filtering to History API

**Files:**
- Modify: `sukyan/api/history.go`

**Step 1: Add proxy_service_id to HistoryFilter**

In the filter struct used for history queries, add:

```go
type HistoryFilter struct {
	// ... existing fields ...
	ProxyServiceID *uuid.UUID `json:"proxy_service_id"`
}
```

**Step 2: Apply filter in query**

In the list history handler, add filtering:

```go
if filter.ProxyServiceID != nil {
	query = query.Where("proxy_service_id = ?", filter.ProxyServiceID)
}
```

**Step 3: Parse from query params**

```go
if proxyServiceIDStr := c.Query("proxy_service_id"); proxyServiceIDStr != "" {
	proxyServiceID, err := uuid.Parse(proxyServiceIDStr)
	if err == nil {
		filter.ProxyServiceID = &proxyServiceID
	}
}
```

**Step 4: Test**

```bash
# Get history for specific proxy
curl "http://localhost:8080/api/v1/history?proxy_service_id={uuid}"
```

**Step 5: Commit**

```bash
cd sukyan
git add api/history.go
git commit -m "feat(api): add proxy_service_id filter to history API

- Filter history by proxy service UUID
- Enables traffic viewing per proxy

🤖 Generated with Claude Code"
```

---

### Task 9: Add ProxyServiceID Filtering to WebSocket API

**Files:**
- Modify: `sukyan/api/websockets.go`

**Step 1: Add proxy_service_id to WebSocketFilter**

Similar to history, add filter field and query logic.

**Step 2: Commit**

```bash
cd sukyan
git add api/websockets.go
git commit -m "feat(api): add proxy_service_id filter to WebSocket API

- Filter WebSocket connections by proxy service
- Enables WS traffic viewing per proxy

🤖 Generated with Claude Code"
```

---

## Phase 4: Frontend Implementation

**Note:** The frontend implementation details will be determined by the `frontend-design` skill. This section provides high-level guidance.

### Frontend Tasks Overview

1. **Create types** - TypeScript types for ProxyService, ProxyStatus
2. **Create API client** - TanStack Query hooks for all proxy endpoints
3. **Create proxy list page** - Table with start/stop controls
4. **Create proxy details page** - Traffic viewer with live streaming
5. **Add sidebar navigation** - Link to proxy management
6. **Integrate with existing components** - Reuse History/WebSocket viewers with filtering

### Execution

Use the `frontend-design` skill to implement the UI:

```
I need a UI for the proxy management system. The design is in docs/plans/2026-02-22-proxy-management-design.md. Please create:

1. Proxy services list page at /workspaces/:workspaceId/proxy-services
2. Proxy details/traffic viewer at /workspaces/:workspaceId/proxy-services/:proxyId
3. Sidebar navigation integration
4. Real-time status updates and live traffic streaming
```

---

## Testing Plan

### Backend Tests

**Unit tests:**
- ✅ ProxyService model CRUD
- ✅ ProxyManager lifecycle
- Port conflict detection
- Context cancellation

**Integration tests:**
- API endpoint smoke tests
- Traffic recording with ProxyServiceID
- Filter queries

### Frontend Tests

Defer to frontend-design skill.

### Manual Testing Checklist

- [ ] Create multiple proxies on different ports
- [ ] Start/stop proxies via UI
- [ ] Verify port conflict errors
- [ ] Check traffic filtering by proxy_service_id
- [ ] Test server restart with auto-recovery
- [ ] Verify graceful shutdown
- [ ] Test live streaming WebSocket
- [ ] Test concurrent operations

---

## Deployment

1. Apply database migration: `sukyan migrate`
2. Restart API server: existing proxies will auto-start if enabled=true
3. Frontend: Deploy new UI build

---

## Future Enhancements

As noted in the design doc, these are out of scope but prepared for:

1. Scope-based filtering (domain/path rules)
2. Intercept mode backend implementation
3. Custom CA certificates per proxy
4. Authentication for proxy usage
5. Rate limiting controls
6. Multi-host deployment

---

## Summary

This implementation plan provides:

- Complete backend implementation with database models, proxy manager, and API
- TDD approach for critical components
- Bite-sized tasks (2-5 minutes each)
- Exact file paths and complete code
- Testing guidance
- Frontend integration points

Execute using `superpowers:executing-plans` or `superpowers:subagent-driven-development` skill.
