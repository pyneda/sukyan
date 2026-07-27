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

// bodilessRedirect builds the response shape a plain `nginx return 301;` produces:
// headers only, no body, the target reachable solely through Location.
func bodilessRedirect(location string) *db.History {
	return &db.History{
		RawResponse: []byte("HTTP/1.1 307 Temporary Redirect\r\nLocation: " + location + "\r\n\r\n"),
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

// TestBodilessResponsesDeliberatelyShareOneDedupKey pins a known collision rather
// than a fix. Bodiless responses all hash to sha256(""), so the second one onwards is
// neither extracted nor scheduled even though its Location names a distinct target.
// The long note on shouldExtract records why that is tolerated and what would make it
// a real recall bug; read it before changing the key to make this test pass.
func TestBodilessResponsesDeliberatelyShareOneDedupKey(t *testing.T) {
	c := &Crawler{}
	first := bodilessRedirect("/one")
	second := bodilessRedirect("/two")

	if first.ResponseHash() != second.ResponseHash() {
		t.Fatal("bodiless responses no longer share a dedup key: the collision documented " +
			"on shouldExtract is gone, so revisit that note and the trailing-slash duplication " +
			"it was suppressing")
	}

	if !c.shouldExtract(first) {
		t.Fatal("the first bodiless response must still be extracted")
	}

	// The listener claims the hash once it has scheduled the first response's URLs.
	c.processedResponseHashes.Store(first.ResponseHash(), true)

	if c.shouldExtract(second) {
		t.Fatal("a second bodiless response is expected to be suppressed; if this now " +
			"extracts, the empty-body class was split and the note on shouldExtract is stale")
	}
}

// A bodiless response is only suppressed once something has claimed sha256(""); on a
// fresh crawler its Location is still reachable.
func TestBodilessRedirectExtractsWhenNothingHasClaimedTheEmptyHash(t *testing.T) {
	c := &Crawler{}

	if !c.shouldExtract(bodilessRedirect("/only")) {
		t.Fatal("an unclaimed bodiless redirect must be extracted so its Location is found")
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
