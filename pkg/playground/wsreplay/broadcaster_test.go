package wsreplay

import (
	"sync"
	"testing"
	"time"
)

func TestBroadcasterPublishToSubscribers(t *testing.T) {
	b := NewBroadcaster(64)
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
	b := NewBroadcaster(64)
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
	b := NewBroadcaster(2)
	ch, _ := b.Subscribe(0)
	for i := 0; i < 100; i++ {
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
}
