package stream

import (
	"sync"
	"testing"
	"time"
)

// testEvent is a minimal Sequenced implementation for broadcaster tests.
type testEvent struct {
	Type string
	Seq  int64
}

func (e *testEvent) GetSeq() int64    { return e.Seq }
func (e *testEvent) SetSeq(s int64)   { e.Seq = s }

func TestBroadcasterPublishToSubscribers(t *testing.T) {
	b := NewBroadcaster(64, 1000)
	ch1, _ := b.Subscribe(0)
	ch2, _ := b.Subscribe(0)
	b.Publish(&testEvent{Type: "x"})
	for i, ch := range []<-chan Sequenced{ch1, ch2} {
		select {
		case ev := <-ch:
			te, ok := ev.(*testEvent)
			if !ok || te.Type != "x" {
				t.Fatalf("subscriber %d got wrong event: %#v", i, ev)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d did not receive", i)
		}
	}
}

func TestBroadcasterReplaysSinceCursor(t *testing.T) {
	b := NewBroadcaster(64, 1000)
	b.Publish(&testEvent{Type: "a"})
	b.Publish(&testEvent{Type: "b"})
	b.Publish(&testEvent{Type: "c"})
	ch, _ := b.Subscribe(2) // since=2 → expect c (seq=3)
	select {
	case ev := <-ch:
		te, ok := ev.(*testEvent)
		if !ok || te.Type != "c" {
			t.Fatalf("expected replay of c, got %#v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive replay")
	}
}

func TestBroadcasterDropsSlowSubscriber(t *testing.T) {
	b := NewBroadcaster(2, 1000)
	ch, _ := b.Subscribe(0)
	for i := 0; i < 10; i++ {
		b.Publish(&testEvent{Type: "x"})
	}
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
	if got := b.SubscriberCount(); got != 0 {
		t.Errorf("expected 0 subscribers after drop, got %d", got)
	}
}

func TestBroadcasterReplaysHistoryLargerThanBuffer(t *testing.T) {
	b := NewBroadcaster(4, 1000)
	for i := 0; i < 50; i++ {
		b.Publish(&testEvent{Type: "x"})
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
	b.Publish(&testEvent{Type: "x"})
	<-ch
	b.Close()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel closed after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("channel did not close in time")
	}
	b.Publish(&testEvent{Type: "y"})
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
	b.Unsubscribe(ch)
}

func TestBroadcasterConcurrentPublishSubscribe(t *testing.T) {
	b := NewBroadcaster(256, 1000)
	var wg sync.WaitGroup
	for p := 0; p < 4; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				b.Publish(&testEvent{Type: "x"})
			}
		}()
	}
	for s := 0; s < 4; s++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, _ := b.Subscribe(0)
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

func TestBroadcasterSetsSeqOnPublish(t *testing.T) {
	b := NewBroadcaster(8, 100)
	ev := &testEvent{Type: "x"}
	b.Publish(ev)
	if ev.Seq != 1 {
		t.Fatalf("expected seq=1 after publish, got %d", ev.Seq)
	}
	ev2 := &testEvent{Type: "y"}
	b.Publish(ev2)
	if ev2.Seq != 2 {
		t.Fatalf("expected seq=2 after publish, got %d", ev2.Seq)
	}
}
