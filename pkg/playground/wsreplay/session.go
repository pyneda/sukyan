package wsreplay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Persister is the abstraction the engine uses to record connections + frames to the DB.
// In tests it's a fake; in production it's wired to db.Connection() (Task 11).
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
	Events              *Broadcaster
	ConnectTimeout      time.Duration
	SendTimeout         time.Duration
	BufferSize          int // received-frames channel buffer; default 1000
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
}

type sendReq struct {
	opcode  int
	content string
	done    chan error
}

// DialSession opens an upstream WS connection, registers it with the persister,
// and starts reader and writer goroutines. Caller must Close() when done.
func DialSession(ctx context.Context, cfg SessionConfig) (*Session, error) {
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 1000
	}
	hdr := http.Header{}
	for _, h := range cfg.Headers {
		if h.Enabled {
			hdr.Add(h.Key, h.Value)
		}
	}
	dialer := websocket.Dialer{HandshakeTimeout: cfg.ConnectTimeout}
	dialCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	conn, resp, err := dialer.DialContext(dialCtx, cfg.TargetURL, hdr)
	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
	}
	connID, perr := cfg.Persister.CreateConnection(cfg.TargetURL, cfg.Headers, statusCode, "playground", cfg.PlaygroundSessionID)
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
		cfg:    cfg,
		conn:   conn,
		connID: connID,
		closed: make(chan struct{}),
		frames: make(chan Frame, cfg.BufferSize),
		sendCh: make(chan sendReq),
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

// Send queues an outgoing frame. Errors if the session is not in StateConnected
// or if the send doesn't complete within SendTimeout.
func (s *Session) Send(opcode int, content string) error {
	if s.State() != StateConnected {
		return errors.New("not connected")
	}
	req := sendReq{opcode: opcode, content: content, done: make(chan error, 1)}
	select {
	case s.sendCh <- req:
	case <-s.closed:
		return errors.New("session closed")
	}
	select {
	case err := <-req.done:
		return err
	case <-time.After(s.cfg.SendTimeout):
		return errors.New("send timeout")
	}
}

// NextFrame blocks until the next received frame, the timeout fires, or the session closes.
// Used by the run walker (Task 9).
func (s *Session) NextFrame(timeout time.Duration) (Frame, error) {
	select {
	case f := <-s.frames:
		return f, nil
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
				s.publish("state_changed", map[string]any{"to": StateErrored, "error": err.Error()})
			}
			return
		}
		content := string(msg)
		mid, _ := s.cfg.Persister.RecordMessage(s.connID, mt, content, "received")
		f := Frame{MessageID: mid, Opcode: mt, Content: content, Direction: "received", Ts: time.Now()}
		s.publish("frame_received", map[string]any{"message_id": mid, "opcode": mt, "content": content, "ts": f.Ts})
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
			err := s.conn.WriteMessage(req.opcode, []byte(req.content))
			if err == nil {
				mid, _ := s.cfg.Persister.RecordMessage(s.connID, req.opcode, req.content, "sent")
				s.publish("frame_sent", map[string]any{"message_id": mid, "opcode": req.opcode, "content": req.content, "ts": time.Now()})
			}
			req.done <- err
		case <-s.closed:
			return
		}
	}
}

func (s *Session) publish(evType string, data map[string]any) {
	if s.cfg.Events == nil {
		return
	}
	raw, _ := json.Marshal(data)
	s.cfg.Events.Publish(Event{Type: evType, Instance: s.cfg.Instance, Data: raw, Ts: time.Now()})
}
