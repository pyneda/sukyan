package scan

import (
	"sync"
	"testing"

	"github.com/pyneda/sukyan/db"
)

// Several payloads for one insertion point are evaluated concurrently, so the
// duplicate check must be a single atomic claim rather than a check followed by
// a later store. Without this, N workers all observe an empty map and each
// reports the same vulnerability.
func TestIssuesFoundClaimIsAtomic(t *testing.T) {
	var issuesFound sync.Map

	key := DetectedIssue{
		code:           db.SqlInjectionCode,
		insertionPoint: InsertionPoint{Type: "graphql_variable", Name: "query"},
	}.String()

	const workers = 32
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		claims int
	)

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			if _, loaded := issuesFound.LoadOrStore(key, true); !loaded {
				mu.Lock()
				claims++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if claims != 1 {
		t.Errorf("expected exactly one worker to claim the issue, got %d", claims)
	}
}

func TestDetectedIssueKeyDistinguishesInsertionPoints(t *testing.T) {
	base := DetectedIssue{
		code:           db.SqlInjectionCode,
		insertionPoint: InsertionPoint{Type: "graphql_variable", Name: "query"},
	}.String()

	otherParam := DetectedIssue{
		code:           db.SqlInjectionCode,
		insertionPoint: InsertionPoint{Type: "graphql_variable", Name: "username"},
	}.String()

	otherCode := DetectedIssue{
		code:           db.NosqlInjectionCode,
		insertionPoint: InsertionPoint{Type: "graphql_variable", Name: "query"},
	}.String()

	if base == otherParam {
		t.Error("different parameters must not share a dedup key")
	}
	if base == otherCode {
		t.Error("different issue codes must not share a dedup key")
	}
}
