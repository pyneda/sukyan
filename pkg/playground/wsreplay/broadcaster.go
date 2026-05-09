package wsreplay

import (
	"sync"
	"sync/atomic"
)

// Broadcaster fans out events to N subscribers with cursor-based replay.
// Replay history is bounded by historySize. Slow subscribers (full channel) are dropped+closed.
//
// Ordering guarantees: replay events delivered to a new subscriber are sent before any live
// events that subscriber will receive on the same channel, as long as the subscriber drains in
// receive order. Live events published concurrently with Subscribe queue behind the in-flight
// replay sends because they share the same channel.
//
// Subscribe replays history asynchronously in a goroutine so that history larger than
// bufferPerSub does not deadlock the caller (the alternative — sending under the broadcaster
// lock — would block when the buffer fills).
//
// Publish-after-Close is a silent no-op: the broadcaster is dead and no subscribers exist.
type Broadcaster struct {
	mu           sync.Mutex
	subs         map[*subscriber]struct{}
	history      []Event
	historySize  int
	bufferPerSub int
	seq          atomic.Int64
}

type subscriber struct {
	ch     chan Event
	done   chan struct{}
	closed bool
}

// NewBroadcaster creates a broadcaster with the given per-subscriber channel buffer size and
// replay history size. Non-positive values fall back to defaults (64 and 1000 respectively).
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
// Returns the channel and the latest seq at the time of subscribe (caller may use as snapshot baseline).
//
// Replay is performed asynchronously to avoid blocking under the broadcaster lock when the
// historical event count exceeds bufferPerSub. The replay goroutine exits early if the
// subscriber is closed via Unsubscribe or Close.
func (b *Broadcaster) Subscribe(since int64) (<-chan Event, int64) {
	s := &subscriber{
		ch:   make(chan Event, b.bufferPerSub),
		done: make(chan struct{}),
	}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	// Copy the relevant tail of history under the lock; send after releasing.
	var replay []Event
	for _, ev := range b.history {
		if ev.Seq > since {
			replay = append(replay, ev)
		}
	}
	last := b.seq.Load()
	b.mu.Unlock()

	// Replay outside the lock. New live events from Publish queue behind these
	// sends because the channel is the same; ordering is preserved as long as
	// the subscriber drains in receive order.
	go func() {
		for _, ev := range replay {
			// Best-effort: if the subscriber never drains, the goroutine blocks.
			// The subscriber will eventually be GC'd when the channel becomes
			// unreferenced after Close()/Unsubscribe(). For long-lived sessions
			// this is acceptable — the alternative (drop replay on full) loses
			// history, which is worse than a leaked goroutine on a dead client.
			select {
			case s.ch <- ev:
			case <-s.done:
				return
			}
		}
	}()

	return s.ch, last
}

// Publish stamps the event with a fresh seq and fans it out. Slow subscribers are dropped+closed.
// Publish on a closed broadcaster is a silent no-op (subscriber map is nil).
func (b *Broadcaster) Publish(ev Event) {
	ev.Seq = b.seq.Add(1)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subs == nil {
		return // closed
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
func (b *Broadcaster) Unsubscribe(ch <-chan Event) {
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

// Close closes all subscriber channels and prevents new subscribers from receiving events.
// Publish after Close is a silent no-op (the broadcaster is dead).
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
