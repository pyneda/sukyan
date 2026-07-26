package crawl

import (
	"sync"
	"testing"

	"github.com/pyneda/sukyan/db"
)

func historyWithBody(body string) *db.History {
	return &db.History{
		RawResponse: []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n" + body),
	}
}

func TestShouldExtractFirstSeenBody(t *testing.T) {
	c := &Crawler{}
	h := historyWithBody(`<a href="/a">a</a>`)

	if !c.shouldExtract(h) {
		t.Fatal("a first-seen response body should be extracted")
	}
}

func TestShouldExtractSkipsBodyAlreadyClaimedByListener(t *testing.T) {
	c := &Crawler{}
	h := historyWithBody(`<a href="/a">a</a>`)

	// The listener claims the hash at crawler.go:174 once it has scheduled the URLs.
	c.processedResponseHashes.Store(h.ResponseHash(), true)

	if c.shouldExtract(h) {
		t.Fatal("a body already claimed by the listener must not be extracted again")
	}
}

func TestShouldExtractDoesNotClaimTheHash(t *testing.T) {
	c := &Crawler{}
	h := historyWithBody(`<a href="/a">a</a>`)

	c.shouldExtract(h)

	if _, claimed := c.processedResponseHashes.Load(h.ResponseHash()); claimed {
		t.Fatal("shouldExtract must peek, never store: claiming here would suppress " +
			"later responses if extraction panics before the channel send")
	}
}

func TestShouldExtractDifferentBodiesAreIndependent(t *testing.T) {
	c := &Crawler{}
	first := historyWithBody(`<a href="/a">a</a>`)
	second := historyWithBody(`<a href="/b">b</a>`)

	c.processedResponseHashes.Store(first.ResponseHash(), true)

	if !c.shouldExtract(second) {
		t.Fatal("a different body must still be extracted")
	}
}

func TestShouldExtractNilHistory(t *testing.T) {
	c := &Crawler{}

	if c.shouldExtract(nil) {
		t.Fatal("a nil history must not be extracted")
	}
}

func TestShouldExtractConcurrent(t *testing.T) {
	c := &Crawler{}
	h := historyWithBody(`<a href="/a">a</a>`)

	const goroutines = 50
	results := make([]bool, goroutines)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = c.shouldExtract(h)
		}(i)
	}
	wg.Wait()

	// Peeking never claims, so every concurrent caller sees an unseen body.
	// If this ever returns false for some callers, shouldExtract has started
	// storing and the panic-suppression hazard is back.
	for i, extracted := range results {
		if !extracted {
			t.Fatalf("goroutine %d was told not to extract; shouldExtract must not claim the hash", i)
		}
	}
	if _, claimed := c.processedResponseHashes.Load(h.ResponseHash()); claimed {
		t.Fatal("shouldExtract stored the hash under concurrency")
	}
}
