package wsreplay

import (
	"sync"
	"testing"
	"time"
)

func TestBroadcasterPublishToSubscribers(t *testing.T) {
	b := NewBroadcaster(64, 1000)
	ch1, _ := b.Subscribe(0)
	ch2, _ := b.Subscribe(0)
	b.Publish(Event{Type: "x"})
	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case ev := <-ch:
			if ev.Type != "x" {
				t.Fatalf("subscriber %d got wrong type", i)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d did not receive", i)
		}
	}
}

func TestBroadcasterReplaysSinceCursor(t *testing.T) {
	b := NewBroadcaster(64, 1000)
	b.Publish(Event{Type: "a"})
	b.Publish(Event{Type: "b"})
	b.Publish(Event{Type: "c"})
	ch, _ := b.Subscribe(2) // since=2 → expect c (seq=3)
	select {
	case ev := <-ch:
		if ev.Type != "c" {
			t.Fatalf("expected replay of c, got %s", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive replay")
	}
}

func TestBroadcasterDropsSlowSubscriber(t *testing.T) {
	b := NewBroadcaster(2, 1000)
	ch, _ := b.Subscribe(0)
	for i := 0; i < 10; i++ {
		b.Publish(Event{Type: "x"})
	}
	// Subscriber should be closed by now since it never read fast enough.
	// Drain whatever it got, expect channel to close.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		timeout := time.After(2 * time.Second)
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					return
				}
			case <-timeout:
				t.Error("subscriber channel was not closed in time")
				return
			}
		}
	}()
	wg.Wait()
	// And subscriber count is 0 since slow sub was dropped.
	if got := b.SubscriberCount(); got != 0 {
		t.Errorf("expected 0 subscribers after drop, got %d", got)
	}
}

func TestBroadcasterReplaysHistoryLargerThanBuffer(t *testing.T) {
	// The original deadlock case: history exceeds bufferPerSub.
	// With the async replay, this should not deadlock.
	b := NewBroadcaster(4, 1000)
	for i := 0; i < 50; i++ {
		b.Publish(Event{Type: "x"})
	}
	ch, _ := b.Subscribe(0)
	received := 0
	deadline := time.After(2 * time.Second)
	for received < 50 {
		select {
		case _, ok := <-ch:
			if !ok {
				t.Fatalf("channel closed after %d/50 events", received)
			}
			received++
		case <-deadline:
			t.Fatalf("timeout, only got %d/50 events", received)
		}
	}
}

func TestBroadcasterClose(t *testing.T) {
	b := NewBroadcaster(64, 1000)
	ch, _ := b.Subscribe(0)
	b.Publish(Event{Type: "x"})
	// Drain the published event.
	<-ch
	b.Close()
	// After Close, channel should be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel closed after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("channel did not close in time")
	}
	// Publish after Close is a no-op.
	b.Publish(Event{Type: "y"})
	if got := b.LastSeq(); got != 2 {
		t.Fatalf("seq still increments on no-op publish: %d", got)
	}
}

func TestBroadcasterUnsubscribe(t *testing.T) {
	b := NewBroadcaster(64, 1000)
	ch, _ := b.Subscribe(0)
	if b.SubscriberCount() != 1 {
		t.Fatalf("expected 1 sub, got %d", b.SubscriberCount())
	}
	b.Unsubscribe(ch)
	if b.SubscriberCount() != 0 {
		t.Fatalf("expected 0 subs after unsubscribe, got %d", b.SubscriberCount())
	}
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel closed after Unsubscribe")
		}
	case <-time.After(time.Second):
		t.Fatal("channel did not close in time")
	}
	// Idempotent — second call is a no-op.
	b.Unsubscribe(ch)
}

func TestBroadcasterConcurrentPublishSubscribe(t *testing.T) {
	// Smoke test for race-clean operation under concurrent load.
	b := NewBroadcaster(256, 1000)
	var wg sync.WaitGroup
	// 4 publishers
	for p := 0; p < 4; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				b.Publish(Event{Type: "x"})
			}
		}()
	}
	// 4 subscribers, drain whatever they get
	for s := 0; s < 4; s++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, _ := b.Subscribe(0)
			// Drain for a fixed window.
			timeout := time.After(500 * time.Millisecond)
			for {
				select {
				case <-ch:
				case <-timeout:
					return
				}
			}
		}()
	}
	wg.Wait()
}
