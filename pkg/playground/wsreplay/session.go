package wsreplay

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pyneda/sukyan/pkg/playground/stream"
	"github.com/rs/zerolog/log"
)

// Persister is the abstraction the engine uses to record connections + frames to the DB.
// In tests it's a fake; in production it's wired to db.Connection() (Task 11).
//
// Callers (and the engine itself) MUST pass canonical values for the string-typed
// arguments — the DB layer relies on exact-match queries:
//   - direction: "sent" or "received"
//   - source:    "playground"
//
// Non-canonical values (case differences, typos) will be persisted as-is and
// silently break recovery sweeps and history filters.
type Persister interface {
	CreateConnection(url string, headers []HeaderSpec, statusCode int, source string, playgroundSessionID *uint) (uint, error)
	RecordMessage(connID uint, opcode int, content string, direction string) (uint, error)
	CloseConnection(connID uint) error
}

// PersistedMessage is the read-back shape of a recorded frame, used by tests and consumers.
type PersistedMessage struct {
	ID           uint
	ConnectionID uint
	Opcode       int
	Content      string
	Direction    string
}

// Frame is the in-memory representation passed to runs and the broadcaster.
type Frame struct {
	MessageID uint
	Opcode    int
	Content   string
	Direction string // "sent" | "received"
	Ts        time.Time
}

// SessionConfig parameterizes DialSession.
type SessionConfig struct {
	TargetURL           string
	Headers             []HeaderSpec
	PlaygroundSessionID *uint
	Instance            Instance
	Persister           Persister
	Events              *stream.Broadcaster
	// ConnectTimeout bounds the dial+upgrade handshake. Defaults to 10s if zero.
	ConnectTimeout time.Duration
	// SendTimeout bounds the time a single Send waits for the writer to ack. Defaults to 5s if zero.
	SendTimeout time.Duration
	BufferSize  int // received-frames channel buffer; default 1000
	// Source tags WebSocketConnection.Source on persistence. Defaults to
	// "playground" if empty (back-compat with existing call sites). The
	// dialer's source-injection accommodation: today it was hard-coded.
	Source string
	// TLSConfig is passed to the gorilla Dialer's TLSClientConfig. nil means
	// use Go's default (which respects system roots and rejects invalid certs).
	TLSConfig *tls.Config
}

// Session owns one upstream WS connection and its IO goroutines.
type Session struct {
	cfg       SessionConfig
	conn      *websocket.Conn
	connID    uint
	state     atomic.Value // SessionState
	closeOnce sync.Once
	closed    chan struct{}
	frames    chan Frame // received frames consumed by NextFrame
	sendCh    chan sendReq
	wg        sync.WaitGroup
	// readErrCh carries the error returned by the last readLoop ReadMessage call.
	// It is a buffered channel of size 1; readLoop sends at most once.
	// NextFrame selects on it so callers see the real gorilla close error rather
	// than a generic "timeout" when the peer sends a close frame.
	readErrCh chan error
}

type sendReq struct {
	opcode  int
	content string
	done    chan error
	cancel  chan struct{}
}

// DialSession opens an upstream WS connection, registers it with the persister,
// and starts reader and writer goroutines. Caller must Close() when done.
func DialSession(ctx context.Context, cfg SessionConfig) (*Session, error) {
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 1000
	}
	if cfg.ConnectTimeout <= 0 {
		cfg.ConnectTimeout = 10 * time.Second
	}
	if cfg.SendTimeout <= 0 {
		cfg.SendTimeout = 5 * time.Second
	}
	hdr := http.Header{}
	// TODO(v2): Sec-WebSocket-Protocol must be passed via dialer.Subprotocols, not headers,
	// for proper subprotocol negotiation. For now, custom Sec-WebSocket-Protocol values are
	// passed as raw headers and may be ignored by the server.
	for _, h := range cfg.Headers {
		if h.Enabled {
			hdr.Add(h.Key, h.Value)
		}
	}
	dialer := websocket.Dialer{
		HandshakeTimeout:  cfg.ConnectTimeout,
		TLSClientConfig:   cfg.TLSConfig,
		EnableCompression: false, // v1: per-message-deflate disabled; see WS fuzzer spec §3
	}
	dialCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	conn, resp, err := dialer.DialContext(dialCtx, cfg.TargetURL, hdr)
	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
	}
	source := cfg.Source
	if source == "" {
		source = "playground"
	}
	connID, perr := cfg.Persister.CreateConnection(cfg.TargetURL, cfg.Headers, statusCode, source, cfg.PlaygroundSessionID)
	if perr != nil {
		if conn != nil {
			conn.Close()
		}
		return nil, fmt.Errorf("persist connection: %w", perr)
	}
	if err != nil {
		_ = cfg.Persister.CloseConnection(connID)
		return nil, fmt.Errorf("dial: %w", err)
	}
	s := &Session{
		cfg:       cfg,
		conn:      conn,
		connID:    connID,
		closed:    make(chan struct{}),
		frames:    make(chan Frame, cfg.BufferSize),
		sendCh:    make(chan sendReq),
		readErrCh: make(chan error, 1),
	}
	s.state.Store(StateConnected)
	s.publish("state_changed", map[string]any{"from": StateConnecting, "to": StateConnected})
	s.wg.Add(2)
	go s.readLoop()
	go s.writeLoop()
	return s, nil
}

// State returns the current lifecycle state.
func (s *Session) State() SessionState { return s.state.Load().(SessionState) }

// ConnectionID returns the persister-assigned websocket_connections.id.
func (s *Session) ConnectionID() uint { return s.connID }

// Instance returns the session's instance descriptor (interactive vs run).
func (s *Session) Instance() Instance { return s.cfg.Instance }

// Send queues an outgoing frame. Errors if the session is not in StateConnected
// or if the send doesn't complete within SendTimeout.
func (s *Session) Send(opcode int, content string) error {
	if s.State() != StateConnected {
		return errors.New("not connected")
	}
	req := sendReq{opcode: opcode, content: content, done: make(chan error, 1), cancel: make(chan struct{})}
	select {
	case s.sendCh <- req:
	case <-s.closed:
		return errors.New("session closed")
	}
	select {
	case err := <-req.done:
		return err
	case <-time.After(s.cfg.SendTimeout):
		close(req.cancel)
		return errors.New("send timeout")
	}
}

// NextFrame blocks until the next received frame, the timeout fires, or the session closes.
// Used by the run walker (Task 9).
func (s *Session) NextFrame(timeout time.Duration) (Frame, error) {
	select {
	case f := <-s.frames:
		return f, nil
	case err := <-s.readErrCh:
		// Re-queue so subsequent NextFrame calls see the same error.
		select {
		case s.readErrCh <- err:
		default:
		}
		return Frame{}, err
	case <-time.After(timeout):
		return Frame{}, errors.New("timeout")
	case <-s.closed:
		return Frame{}, errors.New("closed")
	}
}

// Close shuts down the upstream socket and joins the IO goroutines.
// Safe to call multiple times.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.state.Store(StateClosing)
		s.publish("state_changed", map[string]any{"to": StateClosing})
		// State must be StateClosing BEFORE closing the conn so readLoop sees the
		// graceful close and doesn't transition to StateErrored.
		_ = s.conn.Close()
		close(s.closed)
		s.wg.Wait()
		_ = s.cfg.Persister.CloseConnection(s.connID)
		s.state.Store(StateDisconnected)
		s.publish("state_changed", map[string]any{"to": StateDisconnected})
	})
}

func (s *Session) readLoop() {
	defer s.wg.Done()
	for {
		mt, msg, err := s.conn.ReadMessage()
		if err != nil {
			if s.State() == StateConnected {
				s.state.Store(StateErrored)
				s.publish("state_changed", map[string]any{"from": StateConnected, "to": StateErrored, "error": err.Error()})
			}
			// Non-blocking send: NextFrame may race to select on readErrCh.
			// Buffer size 1 ensures this never blocks.
			select {
			case s.readErrCh <- err:
			default:
			}
			return
		}
		content := string(msg)
		mid, perr := s.cfg.Persister.RecordMessage(s.connID, mt, content, "received")
		if perr != nil {
			s.recordPersistError("received", mt, perr)
		} else {
			s.publish("frame_received", map[string]any{"message_id": mid, "opcode": mt, "content": content, "ts": time.Now()})
		}
		f := Frame{MessageID: mid, Opcode: mt, Content: content, Direction: "received", Ts: time.Now()}
		// Detect buffer saturation once-per-burst.
		if len(s.frames) == cap(s.frames) {
			s.publish("frames_buffer_full", map[string]any{"capacity": cap(s.frames)})
		}
		select {
		case s.frames <- f:
		case <-s.closed:
			return
		}
	}
}

func (s *Session) writeLoop() {
	defer s.wg.Done()
	for {
		select {
		case req := <-s.sendCh:
			select {
			case <-req.cancel:
				continue
			default:
			}
			err := s.conn.WriteMessage(req.opcode, []byte(req.content))
			if err == nil {
				if mid, perr := s.cfg.Persister.RecordMessage(s.connID, req.opcode, req.content, "sent"); perr != nil {
					s.recordPersistError("sent", req.opcode, perr)
				} else {
					s.publish("frame_sent", map[string]any{"message_id": mid, "opcode": req.opcode, "content": req.content, "ts": time.Now()})
				}
			} else {
				s.handleWriteError(err)
			}
			req.done <- err
		case <-s.closed:
			return
		}
	}
}

// handleWriteError is called by writeLoop on a non-nil WriteMessage error.
// It transitions to StateErrored (if still connected) and forces the upstream
// connection closed so readLoop wakes up and exits cleanly.
func (s *Session) handleWriteError(err error) {
	if s.State() != StateConnected {
		return
	}
	s.state.Store(StateErrored)
	s.publish("state_changed", map[string]any{"from": StateConnected, "to": StateErrored, "error": err.Error()})
	_ = s.conn.Close()
}

// recordPersistError surfaces a persister failure so operators see it in logs
// and the UI can render a degraded-state badge. The frame still flows in-memory.
func (s *Session) recordPersistError(direction string, opcode int, err error) {
	log.Warn().
		Err(err).
		Uint("connection_id", s.connID).
		Str("direction", direction).
		Int("opcode", opcode).
		Msg("ws playground frame persist failed")
	s.publish("persist_error", map[string]any{
		"direction": direction,
		"opcode":    opcode,
		"error":     err.Error(),
	})
}

func (s *Session) publish(evType string, data map[string]any) {
	if s.cfg.Events == nil {
		return
	}
	raw, _ := json.Marshal(data)
	s.cfg.Events.Publish(&Event{Type: evType, Instance: s.cfg.Instance, Data: raw, Ts: time.Now()})
}
