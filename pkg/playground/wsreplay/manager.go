package wsreplay

import (
	"context"
	"sync"
	"time"

	"github.com/pyneda/sukyan/pkg/playground/stream"
)

// defaultIdleTimeout is how long an interactive session may have zero
// subscribers before the manager auto-closes it to free upstream resources.
const defaultIdleTimeout = 30 * time.Second

// Manager is the in-process registry of active playground WS sessions.
//
// Layout:
//   - At most one interactive Session per playground_ws_session_id.
//   - Any number of concurrent run Sessions per playground_ws_session_id, keyed by run_id.
//   - One Broadcaster per playground_ws_session_id, shared across all subscribers
//     (interactive socket events + every active run's events fan out the same way).
//
// The Manager is intentionally single-process. Horizontal scaling is out of scope
// for v1; the design spec documents this.
type Manager struct {
	mu           sync.Mutex
	interactive  map[uint]*Session                    // session_id -> session
	runs         map[uint]map[uint]*Session           // session_id -> run_id -> session
	broadcasters map[uint]*stream.Broadcaster         // session_id -> broadcaster
	runCancels   map[uint]map[uint]context.CancelFunc // session_id -> run_id -> cancel
	persister    Persister
	idleTimeout  time.Duration
}

// NewManager constructs an empty Manager. The persister is held to be passed
// to DBPersister-using callers; the Manager itself does not invoke it.
func NewManager(p Persister) *Manager {
	return &Manager{
		interactive:  make(map[uint]*Session),
		runs:         make(map[uint]map[uint]*Session),
		broadcasters: make(map[uint]*stream.Broadcaster),
		runCancels:   make(map[uint]map[uint]context.CancelFunc),
		persister:    p,
		idleTimeout:  defaultIdleTimeout,
	}
}

// SetIdleTimeout sets how long an interactive session may have zero subscribers
// before the manager auto-closes it. Setting a new timeout does not retro-apply
// to already-running watchdogs; only watchdogs spawned after this call observe
// the new value.
func (m *Manager) SetIdleTimeout(d time.Duration) {
	m.mu.Lock()
	m.idleTimeout = d
	m.mu.Unlock()
}

// currentIdleTimeout returns the current idle timeout under lock.
func (m *Manager) currentIdleTimeout() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.idleTimeout
}

// tickInterval picks a watchdog tick interval that's small enough to react
// before the idle timeout elapses, but bounded so prod-default 30s timeouts
// don't busy-tick.
func tickInterval(idle time.Duration) time.Duration {
	t := idle / 4
	if t < 25*time.Millisecond {
		t = 25 * time.Millisecond
	}
	if t > time.Second {
		t = time.Second
	}
	return t
}

// BroadcasterFor returns the broadcaster for the given playground_ws_session_id,
// lazy-creating it on first request. Multiple callers (REST handlers, control-WS
// handler) share the same broadcaster.
func (m *Manager) BroadcasterFor(sessionID uint) *stream.Broadcaster {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.broadcasters[sessionID]; ok {
		return b
	}
	b := stream.NewBroadcaster(64, 1000)
	m.broadcasters[sessionID] = b
	return b
}

// CloseBroadcaster closes and removes the broadcaster for sessionID.
// Use when a playground session is deleted or otherwise permanently torn down.
// Safe when no broadcaster exists (no-op).
func (m *Manager) CloseBroadcaster(sessionID uint) {
	m.mu.Lock()
	b := m.broadcasters[sessionID]
	delete(m.broadcasters, sessionID)
	m.mu.Unlock()
	if b != nil {
		b.Close()
	}
}

// OpenInteractive opens (or returns) the interactive socket for sessionID.
// If a connected session already exists it is returned as-is.
// If a stale (non-connected) session exists, it is replaced.
func (m *Manager) OpenInteractive(ctx context.Context, sessionID uint, cfg SessionConfig) (*Session, error) {
	m.mu.Lock()
	// Re-use only if the existing session is connected. Errored/Closing/Disconnected
	// sessions are stale and replaced; for v1 DialSession is synchronous so we never
	// observe StateConnecting here.
	if existing := m.interactive[sessionID]; existing != nil && existing.State() == StateConnected {
		m.mu.Unlock()
		return existing, nil
	}
	m.mu.Unlock()

	// Default the Persister from the manager's singleton if the caller didn't set one.
	if cfg.Persister == nil {
		cfg.Persister = m.persister
	}

	sess, err := DialSession(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Re-check after re-locking. If another goroutine raced and registered a
	// connected session in the meantime, close ours and return the winner.
	m.mu.Lock()
	// Re-use only if the existing session is connected. Errored/Closing/Disconnected
	// sessions are stale and replaced; for v1 DialSession is synchronous so we never
	// observe StateConnecting here.
	if existing := m.interactive[sessionID]; existing != nil && existing.State() == StateConnected {
		m.mu.Unlock()
		sess.Close()
		return existing, nil
	}
	m.interactive[sessionID] = sess
	m.mu.Unlock()
	go m.watchInteractiveIdle(sessionID, sess)
	return sess, nil
}

// watchInteractiveIdle auto-closes the interactive session for sessionID once
// its broadcaster has had zero subscribers for at least idleTimeout. The
// watchdog exits when the registered session is replaced, the session
// transitions out of StateConnected, or it triggers the close itself.
func (m *Manager) watchInteractiveIdle(sessionID uint, sess *Session) {
	idle := m.currentIdleTimeout()
	if idle <= 0 {
		return
	}
	b := m.BroadcasterFor(sessionID)
	tick := time.NewTicker(tickInterval(idle))
	defer tick.Stop()

	var idleStart time.Time
	for range tick.C {
		// Bail if the registry no longer points at our session, or we've
		// transitioned out of Connected (e.g., upstream closed the socket).
		if m.GetInteractive(sessionID) != sess || sess.State() != StateConnected {
			return
		}
		if b.SubscriberCount() == 0 {
			if idleStart.IsZero() {
				idleStart = time.Now()
				continue
			}
			if time.Since(idleStart) >= idle {
				m.CloseInteractive(sessionID)
				return
			}
			continue
		}
		// Subscribers came back; reset the idle window.
		idleStart = time.Time{}
	}
}

// GetInteractive returns the current interactive session for sessionID, or nil.
func (m *Manager) GetInteractive(sessionID uint) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.interactive[sessionID]
}

// CloseInteractive closes and unregisters the interactive session for sessionID.
// Safe to call when no interactive session exists.
func (m *Manager) CloseInteractive(sessionID uint) {
	m.mu.Lock()
	sess := m.interactive[sessionID]
	delete(m.interactive, sessionID)
	m.mu.Unlock()
	if sess != nil {
		sess.Close()
	}
}

// RegisterRun records a run-instance Session under (sessionID, runID).
func (m *Manager) RegisterRun(sessionID, runID uint, sess *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runs[sessionID] == nil {
		m.runs[sessionID] = make(map[uint]*Session)
	}
	m.runs[sessionID][runID] = sess
}

// UnregisterRun removes a run-instance Session from the registry.
// Caller is responsible for Close()ing the session.
func (m *Manager) UnregisterRun(sessionID, runID uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rs, ok := m.runs[sessionID]; ok {
		delete(rs, runID)
		if len(rs) == 0 {
			delete(m.runs, sessionID)
		}
	}
	if rs, ok := m.runCancels[sessionID]; ok {
		delete(rs, runID)
		if len(rs) == 0 {
			delete(m.runCancels, sessionID)
		}
	}
}

// GetRun returns a run-instance Session if registered, else nil.
func (m *Manager) GetRun(sessionID, runID uint) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rs := m.runs[sessionID]; rs != nil {
		return rs[runID]
	}
	return nil
}

// RegisterRunCancel records the cancel function for a run, so CancelRun can
// signal context cancellation to the running walker. Caller is expected to
// call this immediately after constructing a cancellable context for the run.
func (m *Manager) RegisterRunCancel(sessionID, runID uint, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runCancels[sessionID] == nil {
		m.runCancels[sessionID] = make(map[uint]context.CancelFunc)
	}
	m.runCancels[sessionID][runID] = cancel
}

// CancelRun cancels the registered context for a run. Also closes the
// session if one is registered, since a context-only cancel cannot interrupt
// an in-flight NextFrame; closing the socket is the documented escape hatch
// (see WalkScript's cancellation contract).
func (m *Manager) CancelRun(sessionID, runID uint) {
	m.mu.Lock()
	cancel := m.runCancels[sessionID][runID]
	if rs, ok := m.runCancels[sessionID]; ok {
		delete(rs, runID)
		if len(rs) == 0 {
			delete(m.runCancels, sessionID)
		}
	}
	sess := m.runs[sessionID][runID]
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if sess != nil {
		sess.Close()
	}
}
