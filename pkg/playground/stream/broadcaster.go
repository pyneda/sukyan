// Package stream provides a generic event broadcaster used by playground
// streaming domains (wsreplay control stream, fuzz run stream). Events flow
// through a single Sequenced interface so the broadcaster stays free of any
// per-domain concerns.
package stream

import (
	"sync"
	"sync/atomic"
)

// Sequenced is implemented by any event published through a Broadcaster. The
// broadcaster stamps each event with a monotonically increasing seq before
// fan-out; subscribers use seq for the since-cursor replay on reconnect.
type Sequenced interface {
	GetSeq() int64
	SetSeq(int64)
}

// Broadcaster fans out events to N subscribers with cursor-based replay.
// Replay history is bounded by historySize. Slow subscribers (full channel)
// are dropped+closed.
//
// Ordering guarantees: replay events delivered to a new subscriber are sent
// before any live events that subscriber will receive on the same channel,
// as long as the subscriber drains in receive order. Live events published
// concurrently with Subscribe queue behind the in-flight replay sends because
// they share the same channel.
//
// Subscribe replays history asynchronously in a goroutine so that history
// larger than bufferPerSub does not deadlock the caller (the alternative —
// sending under the broadcaster lock — would block when the buffer fills).
//
// Publish-after-Close is a silent no-op: the broadcaster is dead and no
// subscribers exist.
type Broadcaster struct {
	mu           sync.Mutex
	subs         map[*subscriber]struct{}
	history      []Sequenced
	historySize  int
	bufferPerSub int
	seq          atomic.Int64
}

type subscriber struct {
	ch     chan Sequenced
	done   chan struct{}
	closed bool
}

// NewBroadcaster creates a broadcaster with the given per-subscriber channel
// buffer size and replay history size. Non-positive values fall back to
// defaults (64 and 1000 respectively).
func NewBroadcaster(bufferPerSub, historySize int) *Broadcaster {
	if bufferPerSub <= 0 {
		bufferPerSub = 64
	}
	if historySize <= 0 {
		historySize = 1000
	}
	return &Broadcaster{
		subs:         make(map[*subscriber]struct{}),
		bufferPerSub: bufferPerSub,
		historySize:  historySize,
	}
}

// Subscribe registers a receiver. Events with seq > since are replayed first.
// Returns the channel and the latest seq at the time of subscribe (caller may
// use as snapshot baseline).
//
// Replay is performed asynchronously to avoid blocking under the broadcaster
// lock when the historical event count exceeds bufferPerSub. The replay
// goroutine exits early if the subscriber is closed via Unsubscribe or Close.
func (b *Broadcaster) Subscribe(since int64) (<-chan Sequenced, int64) {
	s := &subscriber{
		ch:   make(chan Sequenced, b.bufferPerSub),
		done: make(chan struct{}),
	}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	var replay []Sequenced
	for _, ev := range b.history {
		if ev.GetSeq() > since {
			replay = append(replay, ev)
		}
	}
	last := b.seq.Load()
	b.mu.Unlock()

	go func() {
		for _, ev := range replay {
			select {
			case s.ch <- ev:
			case <-s.done:
				return
			}
		}
	}()

	return s.ch, last
}

// Publish stamps the event with a fresh seq and fans it out. Slow subscribers
// are dropped+closed. Publish on a closed broadcaster is a silent no-op.
func (b *Broadcaster) Publish(ev Sequenced) {
	ev.SetSeq(b.seq.Add(1))
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subs == nil {
		return
	}
	b.history = append(b.history, ev)
	if len(b.history) > b.historySize {
		b.history = b.history[len(b.history)-b.historySize:]
	}
	for s := range b.subs {
		if s.closed {
			continue
		}
		select {
		case s.ch <- ev:
		default:
			close(s.done)
			close(s.ch)
			s.closed = true
			delete(b.subs, s)
		}
	}
}

// Unsubscribe deregisters the subscriber and closes its channel.
// The argument must be the channel returned from Subscribe.
// Idempotent: unsubscribing twice is a no-op.
func (b *Broadcaster) Unsubscribe(ch <-chan Sequenced) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs {
		if s.ch == ch {
			if !s.closed {
				close(s.done)
				close(s.ch)
				s.closed = true
			}
			delete(b.subs, s)
			return
		}
	}
}

// Close closes all subscriber channels and prevents new subscribers from
// receiving events. Publish after Close is a silent no-op.
func (b *Broadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs {
		if !s.closed {
			close(s.done)
			close(s.ch)
			s.closed = true
		}
	}
	b.subs = nil
}

// SubscriberCount returns the live subscriber count.
func (b *Broadcaster) SubscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}

// LastSeq returns the latest event seq.
func (b *Broadcaster) LastSeq() int64 { return b.seq.Load() }
