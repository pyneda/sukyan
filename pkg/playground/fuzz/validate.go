package fuzz

import (
	"fmt"
	"sort"
)

// Validate checks that positions, payloads, and mode are mutually consistent.
// Returns the first error found, or nil. Designed to be called from the API
// layer before constructing a run.
//
// Rules enforced:
//   - Mode must be one of the known values.
//   - At least one position must be supplied.
//   - Position byte ranges must satisfy Start < End and be non-overlapping.
//   - Single / All require SharedPayloads with at least one resolved payload.
//   - Single / All must NOT have per-position PayloadGroups (caller bug
//     otherwise; signals confused intent).
//   - Paired / Combinations require SharedPayloads to be nil.
//   - Paired / Combinations require every position to carry at least one
//     PayloadGroup that resolves to a non-empty list.
func Validate(mode FuzzMode, positions []FuzzerPosition, shared *FuzzerPayloadsGroup) error {
	if !mode.IsValid() {
		return fmt.Errorf("invalid mode %q (want one of: single, all, paired, combinations)", mode)
	}
	if len(positions) == 0 {
		return fmt.Errorf("at least one insertion position is required")
	}
	for i, p := range positions {
		if p.End <= p.Start {
			return fmt.Errorf("position %d: end (%d) must be greater than start (%d)", i, p.End, p.Start)
		}
	}
	if err := checkNoOverlap(positions); err != nil {
		return err
	}

	switch mode {
	case ModeSingle, ModeAll:
		if shared == nil {
			return fmt.Errorf("mode %q requires shared_payloads", mode)
		}
		if len(ResolveGroup(*shared)) == 0 {
			return fmt.Errorf("mode %q: shared_payloads resolved to an empty list", mode)
		}
		for i, p := range positions {
			if len(p.PayloadGroups) > 0 {
				return fmt.Errorf("mode %q: position %d must not have payload_groups (payloads come from shared_payloads)", mode, i)
			}
		}
	case ModePaired, ModeCombinations:
		if shared != nil {
			return fmt.Errorf("mode %q: shared_payloads must be nil (payloads come from each position's payload_groups)", mode)
		}
		for i, p := range positions {
			if len(p.PayloadGroups) == 0 {
				return fmt.Errorf("mode %q: position %d requires payload_groups", mode, i)
			}
			if len(ResolvePositionPayloads(p)) == 0 {
				return fmt.Errorf("mode %q: position %d's payload_groups resolved to an empty list", mode, i)
			}
		}
	}
	return nil
}

// checkNoOverlap returns an error if any two positions occupy overlapping
// byte ranges. Sorts a copy to make the check O(n log n).
func checkNoOverlap(positions []FuzzerPosition) error {
	if len(positions) < 2 {
		return nil
	}
	indexed := make([]struct {
		idx int
		pos FuzzerPosition
	}, len(positions))
	for i, p := range positions {
		indexed[i] = struct {
			idx int
			pos FuzzerPosition
		}{i, p}
	}
	sort.Slice(indexed, func(i, j int) bool { return indexed[i].pos.Start < indexed[j].pos.Start })
	for i := 1; i < len(indexed); i++ {
		prev, cur := indexed[i-1], indexed[i]
		if prev.pos.End > cur.pos.Start {
			return fmt.Errorf("positions %d and %d overlap (positions %d-%d and %d-%d)",
				prev.idx, cur.idx, prev.pos.Start, prev.pos.End, cur.pos.Start, cur.pos.End)
		}
	}
	return nil
}
