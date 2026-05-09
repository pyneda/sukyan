package wsreplay

import (
	"sync"
	"sync/atomic"
)

// Broadcaster fans out events to N subscribers with cursor-based replay.
// Replay history is bounded by historySize. Slow subscribers (full channel) are dropped+closed.
type Broadcaster struct {
	mu          sync.Mutex
	subs        map[*subscriber]struct{}
	history     []Event
	historySize int
	seq         atomic.Int64
}

type subscriber struct {
	ch     chan Event
	closed bool
}

// NewBroadcaster creates a broadcaster. The bufferPerSub argument is currently informational —
// per-subscriber buffer size is fixed at 64; the parameter exists so future tuning is non-breaking.
func NewBroadcaster(bufferPerSub int) *Broadcaster {
	return &Broadcaster{
		subs:        make(map[*subscriber]struct{}),
		historySize: 1000,
	}
}

// Subscribe registers a receiver. Events with seq > since are replayed first.
// Returns the channel and the latest seq at the time of subscribe (caller may use as snapshot baseline).
func (b *Broadcaster) Subscribe(since int64) (<-chan Event, int64) {
	s := &subscriber{ch: make(chan Event, 64)}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[s] = struct{}{}
	for _, ev := range b.history {
		if ev.Seq > since {
			s.ch <- ev
		}
	}
	last := b.seq.Load()
	return s.ch, last
}

// Publish stamps the event with a fresh seq and fans it out. Slow subscribers are dropped+closed.
func (b *Broadcaster) Publish(ev Event) {
	ev.Seq = b.seq.Add(1)
	b.mu.Lock()
	defer b.mu.Unlock()
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
			close(s.ch)
			s.closed = true
			delete(b.subs, s)
		}
	}
}

// SubscriberCount returns the live subscriber count.
func (b *Broadcaster) SubscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}

// LastSeq returns the latest event seq.
func (b *Broadcaster) LastSeq() int64 { return b.seq.Load() }
