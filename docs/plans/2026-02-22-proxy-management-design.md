# Proxy Service Management System

**Date:** 2026-02-22
**Status:** Approved
**Author:** Design session with user

## Overview

Design for a proxy service management system that allows users to create, configure, start/stop, and monitor HTTP/WebSocket proxy instances through the Sukyan UI. Each proxy service is workspace-scoped, persisted in the database, and can be managed independently.

## Background

Currently, Sukyan has a proxy implementation (`pkg/proxy/`) that runs via CLI (`sukyan proxy --workspace 1`). It's a pass-through logging proxy that captures all HTTP and WebSocket traffic to the database. However:

- No UI management - must use CLI
- One proxy per CLI invocation
- No persistence - proxy config not stored in database
- No ability to relate traffic to specific proxy instances
- Manual restart required after server restarts

This design addresses these limitations by creating a full proxy lifecycle management system.

## Goals

1. Allow users to create and configure multiple proxy services per workspace through the UI
2. Enable/disable proxies without CLI interaction
3. Persist proxy configuration and state in the database
4. Auto-restart enabled proxies when the API server restarts
5. Relate History and WebSocketConnection records to specific proxy services
6. Provide live traffic viewing and historical traffic analysis per proxy

## Non-Goals

- Intercept-and-hold proxy functionality (backend not implemented yet)
- Cross-machine proxy distribution
- Scope-based filtering (future enhancement)
- Custom CA certificates per proxy (future enhancement)

## Architecture

### Approach: In-Process Goroutine Manager with Persistent State

Each proxy runs as a goroutine within the API server process. The `ProxyManager` tracks running proxies in memory, while the database stores desired state. On API server startup, all proxies marked `enabled=true` are automatically restarted.

**Why this approach:**
- Simple implementation - leverages existing `pkg/proxy/` code
- Easy debugging - all in one process
- Survives restarts - auto-restart on server boot
- No external process management complexity
- Appropriate for the logging proxy use case

## Database Models

### New Model: ProxyService

Location: `db/proxy_service.go`

```go
type ProxyService struct {
    BaseUUIDModel

    // Workspace scoping
    WorkspaceID *uint     `json:"workspace_id" gorm:"index;not null"`
    Workspace   Workspace `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`

    // Basic config
    Name        string `json:"name" gorm:"not null"`
    Host        string `json:"host" gorm:"default:localhost"`
    Port        int    `json:"port" gorm:"not null;uniqueIndex"`  // Must be unique globally

    // Current proxy settings (from pkg/proxy.Proxy)
    Verbose               bool `json:"verbose" gorm:"default:true"`
    LogOutOfScopeRequests bool `json:"log_out_of_scope_requests" gorm:"default:true"`

    // State management
    Enabled bool `json:"enabled" gorm:"default:false;index"`  // Desired state
}
```

**Key decisions:**
- Uses `BaseUUIDModel` (new model standard)
- Port has `uniqueIndex` to prevent conflicts
- `Enabled` field represents desired state (DB) vs. runtime state (memory)

### Updates to Existing Models

**History** (`db/history.go`):
```go
type History struct {
    // ... existing fields
    ProxyServiceID *uuid.UUID    `json:"proxy_service_id" gorm:"type:uuid;index"`
    ProxyService   *ProxyService `json:"proxy_service,omitempty" gorm:"foreignKey:ProxyServiceID"`
}
```

**WebSocketConnection** (`db/websocket_connection.go`):
```go
type WebSocketConnection struct {
    // ... existing fields
    ProxyServiceID *uuid.UUID    `json:"proxy_service_id" gorm:"type:uuid;index"`
    ProxyService   *ProxyService `json:"proxy_service,omitempty" gorm:"foreignKey:ProxyServiceID"`
}
```

This allows filtering History and WebSocket traffic by proxy service.

## Backend Components

### ProxyManager

Location: `pkg/proxy/manager.go`

Central coordinator for all running proxies.

```go
type ProxyManager struct {
    mu       sync.RWMutex
    proxies  map[uuid.UUID]*RunningProxy  // proxyServiceID -> running instance
}

type RunningProxy struct {
    Service    *db.ProxyService
    CancelFunc context.CancelFunc
    StartedAt  time.Time
}
```

**Key operations:**

- `StartProxy(proxyServiceID uuid.UUID) error`
  - Loads ProxyService from DB
  - Creates context with cancel
  - Spawns goroutine running `proxy.Proxy.Run(ctx)`
  - Stores in `proxies` map
  - Updates DB: `enabled=true`

- `StopProxy(proxyServiceID uuid.UUID) error`
  - Calls CancelFunc to stop goroutine
  - Waits for graceful shutdown
  - Removes from `proxies` map
  - Updates DB: `enabled=false`

- `RestartProxy(proxyServiceID uuid.UUID) error`
  - StopProxy() then StartProxy()

- `GetStatus(proxyServiceID uuid.UUID) (*ProxyStatus, error)`
  - Returns: running state, uptime, request count, websocket count

- `StartAllEnabled() error`
  - Called on API server startup
  - Queries DB for `enabled=true` proxies
  - Attempts to start each one
  - Logs warnings for port conflicts

### Updates to pkg/proxy/proxy.go

Minimal changes to existing Proxy struct:

```go
type Proxy struct {
    Host                  string
    Port                  int
    Verbose               bool
    LogOutOfScopeRequests bool
    WorkspaceID           uint
    ProxyServiceID        uuid.UUID  // NEW: Track which service this is
    wsConnections         sync.Map
}
```

**Changes to `Run()` method:**
- Accept `context.Context` parameter for graceful shutdown
- Listen for context cancellation
- Pass `ProxyServiceID` when creating History/WebSocketConnection records

## API Endpoints

Location: `api/proxy_services.go`

### CRUD Operations

```
POST   /api/v1/workspaces/:workspaceId/proxy-services
```
Create new proxy service. Returns error if port already in use.

**Request:**
```json
{
  "name": "Testing Proxy",
  "host": "localhost",
  "port": 8008,
  "verbose": true,
  "log_out_of_scope_requests": true
}
```

**Response:**
```json
{
  "id": "uuid",
  "workspace_id": 1,
  "name": "Testing Proxy",
  "host": "localhost",
  "port": 8008,
  "verbose": true,
  "log_out_of_scope_requests": true,
  "enabled": false,
  "created_at": "2026-02-22T10:00:00Z"
}
```

---

```
GET    /api/v1/workspaces/:workspaceId/proxy-services
```
List all proxy services for workspace.

**Response:**
```json
{
  "data": [
    {
      "id": "uuid",
      "name": "Testing Proxy",
      "host": "localhost",
      "port": 8008,
      "enabled": true,
      "running": true,
      "uptime_seconds": 3600,
      "requests_count": 1523,
      "websockets_count": 12
    }
  ],
  "count": 1
}
```

---

```
GET    /api/v1/proxy-services/:id
```
Get proxy details + runtime status.

---

```
PATCH  /api/v1/proxy-services/:id
```
Update proxy configuration. If proxy is running, automatically restarts with new config.

---

```
DELETE /api/v1/proxy-services/:id
```
Delete proxy service. Stops proxy if running, then deletes record.

### Lifecycle Control

```
POST   /api/v1/proxy-services/:id/start
```
Start proxy. Sets `enabled=true`, spawns goroutine.

Returns 409 Conflict if port already in use.

---

```
POST   /api/v1/proxy-services/:id/stop
```
Stop proxy. Sets `enabled=false`, cancels goroutine.

---

```
POST   /api/v1/proxy-services/:id/restart
```
Restart proxy with current configuration.

### Live Streaming

```
WS     /api/v1/proxy-services/:id/live
```
WebSocket endpoint for real-time traffic streaming.

**Message format:**
```json
{
  "type": "http_request",
  "history_id": "uuid",
  "method": "GET",
  "url": "https://example.com/api/users",
  "status_code": 200,
  "timestamp": "2026-02-22T10:30:00Z"
}
```

### Extended Existing Endpoints

**History filtering:**
```
GET /api/v1/history?proxy_service_id=uuid
```

**WebSocket filtering:**
```
GET /api/v1/websockets?proxy_service_id=uuid
```

Update `api/history.go` and `api/websockets.go` to support `proxy_service_id` query parameter.

## Frontend UI

The exact visual design, component structure, and interaction patterns will be determined by the `frontend-design` skill. This section provides the high-level structure.

### Routes

**Proxy Services Management:**
```
/workspaces/:workspaceId/proxy-services
```

Main view with:
- Table/list of all proxy services in workspace
- Columns: Name, Host:Port, Status, Uptime, Request Count
- Actions: Start/Stop, Edit, View Traffic, Delete
- "Create New Proxy" button

**Proxy Details/Traffic View:**
```
/workspaces/:workspaceId/proxy-services/:proxyId
```

View with:
- Proxy info card (name, host:port, settings)
- Start/Stop/Restart controls
- Live traffic viewer (WebSocket streaming)
- Historical traffic (filtered History view)
- Search/filter capabilities
- Request/response inspector

### Sidebar Integration

Add "Proxies" link to workspace sidebar navigation.

### Key UI Features

- Real-time status updates (running/stopped, uptime)
- Live traffic streaming with auto-scroll
- Historical traffic search and filtering
- Clear error messages for port conflicts
- Loading states during start/stop transitions
- Confirmation dialogs for delete operations

## Data Flow

### Starting a Proxy

```
User clicks "Start" in UI
    ↓
POST /api/v1/proxy-services/{id}/start
    ↓
API handler updates DB: enabled=true
    ↓
ProxyManager.StartProxy(id)
    ↓
Spawns goroutine → proxy.Proxy.Run(ctx) with ProxyServiceID
    ↓
Stores RunningProxy in manager.proxies map
    ↓
Returns success + status to UI
```

### Traffic Flow Through Proxy

```
Client makes request → Proxy (localhost:8008)
    ↓
proxy.Proxy intercepts (existing goproxy logic)
    ↓
Creates History record with ProxyServiceID=uuid
    ↓
If WebSocket upgrade → Creates WebSocketConnection with ProxyServiceID=uuid
    ↓
Forwards request to target
    ↓
If live stream client connected → Broadcast event via WebSocket
```

### Viewing Traffic

```
User opens proxy details page
    ↓
Connects WebSocket to /api/v1/proxy-services/{id}/live
    ↓
Loads historical data: GET /api/v1/history?proxy_service_id=uuid
    ↓
New traffic → Real-time broadcast to connected clients
```

## Error Handling & Edge Cases

### Port Conflicts

**Problem:** User tries to create/start proxy on already-used port.

**Solutions:**
- Database: Port has `uniqueIndex` constraint - rejects duplicate ports at DB level
- Runtime: Before starting, attempt to bind - catch bind errors gracefully
- API: Return 409 Conflict with clear message: "Port 8008 is already in use by proxy 'Testing Proxy'"
- UI: Display error message with link to conflicting proxy

### Proxy Crashes

**Problem:** Proxy goroutine panics or stops unexpectedly.

**Solutions:**
- Wrap `proxy.Run()` in defer/recover to catch panics
- On crash:
  - Remove from `ProxyManager.proxies` map
  - Log error with full stack trace
  - Keep `enabled=true` in DB (user's desired state)
- UI: Show "Stopped (crashed)" status with "Restart" button
- Provide access to server logs for debugging

### Server Restart

**Problem:** API server restarts, all proxy goroutines die.

**Solutions:**
- On API server startup: Call `ProxyManager.StartAllEnabled()`
- Query DB for all `enabled=true` proxies
- Attempt to restart each one
- If port conflict during restart:
  - Log warning
  - Set `enabled=false` for that proxy
  - Send notification (if notification system exists)
- UI: Shows correct status after polling/reconnecting

### Concurrent Start/Stop Requests

**Problem:** User clicks Start/Stop rapidly or multiple clients issue commands.

**Solutions:**
- `ProxyManager` uses `sync.RWMutex` for all operations
- API returns 409 Conflict if proxy is already starting/stopping
- UI: Disable buttons during transitions, show loading spinner
- Optimistic UI updates with rollback on error

### Graceful Shutdown

**Problem:** Need to stop all proxies cleanly when API server shuts down.

**Solutions:**
- ProxyManager implements `Shutdown(ctx context.Context) error`
- Iterates through all running proxies
- Calls `CancelFunc` for each
- Waits for all goroutines to exit (with timeout)
- Called during API server graceful shutdown

## Migration Plan

### Database Migration

1. Create `ProxyService` model in `db/proxy_service.go`
2. Add `ProxyServiceID` fields to `History` and `WebSocketConnection`
3. Register models in `db/atlas/main.go`
4. Generate migration: `atlas migrate diff --env gorm`
5. Review generated SQL
6. Apply migration: `sukyan migrate`

**Migration considerations:**
- Existing History/WebSocketConnection records will have `proxy_service_id=NULL`
- This is acceptable - old traffic not associated with managed proxies

### Backend Implementation Order

1. Implement `ProxyService` database model and CRUD operations
2. Implement `ProxyManager` with lifecycle methods
3. Update `pkg/proxy/proxy.go` to accept context and ProxyServiceID
4. Implement API endpoints in `api/proxy_services.go`
5. Add proxy_service_id filtering to existing History/WebSocket endpoints
6. Implement WebSocket live streaming endpoint
7. Integrate ProxyManager startup into API server initialization

### Frontend Implementation

Defer to `frontend-design` skill for implementation plan.

### Testing Strategy

**Unit tests:**
- ProxyService model CRUD operations
- ProxyManager lifecycle methods (mock proxy.Run)
- Port conflict detection

**Integration tests:**
- Start/stop proxy via API
- Traffic recording with ProxyServiceID
- Server restart with auto-recovery
- Concurrent operation safety

**Manual testing:**
- Create multiple proxies on different ports
- Verify traffic filtering by proxy_service_id
- Test live streaming functionality
- Verify graceful error handling

## Future Enhancements

These are explicitly out of scope for this design but prepared for:

1. **Scope-based filtering** - Only log requests matching domain/path rules
2. **Intercept mode** - When backend supports request hold/forward/drop
3. **Custom CA certificates** - Per-proxy SSL certificates
4. **Authentication** - Require auth token to use specific proxies
5. **Rate limiting** - Per-proxy request rate controls
6. **Multi-host deployment** - Distribute proxies across multiple machines

The database schema uses UUID primary keys and includes space for extensibility without requiring migrations.

## Summary

This design provides a complete proxy lifecycle management system that:

- Persists proxy configuration in the database
- Provides UI-driven management (no CLI required)
- Associates traffic with specific proxy instances
- Auto-restarts on server boot
- Handles errors gracefully
- Leverages existing proxy code with minimal changes
- Prepares for future enhancements

The architecture is simple, reliable, and appropriate for the use case of logging proxies in a security testing tool.
