package active

import (
	"testing"
	"time"
)

// The audit used to spend its whole budget on the first source, so postMessage -
// scheduled last - never ran. Every planned source must get a slice.
func TestDOMXSSSourceBudgetLeavesTimeForEverySource(t *testing.T) {
	const total = 120 * time.Second
	sources := 13

	remaining := total
	for i := 0; i < sources; i++ {
		share := domXSSSourceBudget(remaining, sources-i)
		if share <= 0 {
			t.Fatalf("source %d of %d got no budget (remaining %s)", i+1, sources, remaining)
		}
		remaining -= share
	}
}

func TestDOMXSSSourceBudgetRollsUnusedTimeForward(t *testing.T) {
	// Three sources left, but the earlier ones returned early so the pool is
	// still nearly full: the next source should get a third of what is left,
	// which is more than an equal split of the original budget would have given.
	share := domXSSSourceBudget(90*time.Second, 3)

	if share != 30*time.Second {
		t.Errorf("got %s, want 30s", share)
	}
}

func TestDOMXSSSourceBudgetHandlesExhaustedPool(t *testing.T) {
	if got := domXSSSourceBudget(0, 5); got != 0 {
		t.Errorf("exhausted pool gave %s, want 0", got)
	}
	if got := domXSSSourceBudget(-time.Second, 5); got != 0 {
		t.Errorf("negative pool gave %s, want 0", got)
	}
}

func TestDOMXSSSourceBudgetGivesLastSourceTheRemainder(t *testing.T) {
	if got := domXSSSourceBudget(17*time.Second, 1); got != 17*time.Second {
		t.Errorf("got %s, want the full 17s remainder", got)
	}
	if got := domXSSSourceBudget(17*time.Second, 0); got != 17*time.Second {
		t.Errorf("got %s, want the full 17s remainder", got)
	}
}

// postMessage is the source the old ordering starved; it must be in the plan
// when enabled and absent when the operator turns it off.
func TestPlanDOMXSSSourcesIncludesPostMessage(t *testing.T) {
	plan := planDOMXSSSources(true, true)

	var found bool
	for _, entry := range plan {
		if entry.Source.Name == "postMessage" {
			found = true
			if entry.Kind != domXSSSourceKindPostMessage {
				t.Errorf("postMessage planned as kind %v, want postMessage kind", entry.Kind)
			}
		}
	}
	if !found {
		t.Error("postMessage missing from the plan")
	}
}

func TestPlanDOMXSSSourcesOmitsDisabledSources(t *testing.T) {
	plan := planDOMXSSSources(false, false)

	for _, entry := range plan {
		if entry.Kind == domXSSSourceKindPostMessage {
			t.Error("postMessage planned even though it is disabled")
		}
		if entry.Kind == domXSSSourceKindStorage {
			t.Error("storage planned even though it is disabled")
		}
	}
	if len(plan) == 0 {
		t.Error("plan is empty with storage and postMessage disabled, want the URL and document/window sources")
	}
}

func TestPlanDOMXSSSourcesPlansEachSourceOnce(t *testing.T) {
	plan := planDOMXSSSources(true, true)

	seen := map[string]int{}
	for _, entry := range plan {
		seen[entry.Source.Name]++
	}
	for name, count := range seen {
		if count != 1 {
			t.Errorf("source %s planned %d times, want 1", name, count)
		}
	}
}
