package fuzz

import (
	"sort"
)

// ReplacePayloads injects payloads[i] into positions[i] of raw, returning the
// new string. Positions are sorted by Start internally so the caller may pass
// them in any order. Overlap is not validated here (Validate handles that at
// the API boundary); if overlapping positions slip through, the result is
// undefined but the function does not panic.
//
// Length contract: len(payloads) must equal len(positions). The caller is
// responsible for ensuring this; mode strategies that yield assignments
// satisfy it by construction.
func ReplacePayloads(raw string, positions []FuzzerPosition, payloads []string) string {
	if len(positions) == 0 || len(payloads) != len(positions) {
		return raw
	}
	// Sort an index permutation, not the slice itself, so payloads[i] still
	// refers to positions[i] from the caller's perspective.
	order := make([]int, len(positions))
	for i := range positions {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool {
		return positions[order[a]].Start < positions[order[b]].Start
	})

	// Walk the sorted positions, tracking byte offset adjustment as
	// replacements shift content downstream.
	out := raw
	offset := 0
	for _, idx := range order {
		p := positions[idx]
		payload := payloads[idx]
		start := p.Start + offset
		end := p.End + offset
		if start < 0 || end > len(out) || start > end {
			// Defensive: out-of-range range. Skip this position rather than panic.
			continue
		}
		out = out[:start] + payload + out[end:]
		offset += len(payload) - (p.End - p.Start)
	}
	return out
}
