package payloads

import (
	"testing"

	"github.com/pyneda/sukyan/pkg/web"
)

// primaryFamily mirrors what the ordering keys on: a payload's first target sink
// is the execution context it was written for.
func primaryFamily(p DOMXSSPayload) web.DOMXSSSinkType {
	if len(p.TargetSinks) == 0 {
		return web.SinkTypeHTMLExecution
	}
	return p.TargetSinks[0]
}

// A source that only gets a handful of payloads before its budget runs out must
// still cover every execution context, otherwise an eval-only sink is invisible
// unless the scanner happens to reach payload 14.
func TestOrderDOMXSSPayloadsBySinkFamilyCoversEveryFamilyEarly(t *testing.T) {
	ordered := OrderDOMXSSPayloadsBySinkFamily(GetDOMXSSPayloads())

	families := map[web.DOMXSSSinkType]bool{}
	for _, p := range ordered[:3] {
		families[primaryFamily(p)] = true
	}

	if len(families) != 3 {
		var got []string
		for _, p := range ordered[:3] {
			got = append(got, primaryFamily(p).String()+": "+p.Description)
		}
		t.Errorf("first 3 payloads cover %d sink families, want 3\n%v", len(families), got)
	}
}

func TestOrderDOMXSSPayloadsBySinkFamilyKeepsEveryPayload(t *testing.T) {
	original := GetDOMXSSPayloads()
	ordered := OrderDOMXSSPayloadsBySinkFamily(original)

	if len(ordered) != len(original) {
		t.Fatalf("got %d payloads, want %d", len(ordered), len(original))
	}

	seen := make(map[string]int, len(ordered))
	for _, p := range ordered {
		seen[p.Description]++
	}
	for _, p := range original {
		if seen[p.Description] != 1 {
			t.Errorf("payload %q appears %d times after ordering, want 1", p.Description, seen[p.Description])
		}
	}
}

// Round-robin means no family may take two consecutive slots while another
// family still has payloads waiting.
func TestOrderDOMXSSPayloadsBySinkFamilyRotatesWhileFamiliesRemain(t *testing.T) {
	ordered := OrderDOMXSSPayloadsBySinkFamily(GetDOMXSSPayloads())

	remaining := map[web.DOMXSSSinkType]int{}
	for _, p := range ordered {
		remaining[primaryFamily(p)]++
	}

	for i := 1; i < len(ordered); i++ {
		prev := primaryFamily(ordered[i-1])
		remaining[prev]--
		if primaryFamily(ordered[i]) != prev {
			continue
		}
		for family, left := range remaining {
			if family != prev && left > 0 {
				t.Fatalf("position %d repeats family %s while %s still has %d payloads waiting",
					i, prev, family, left)
			}
		}
	}
}
