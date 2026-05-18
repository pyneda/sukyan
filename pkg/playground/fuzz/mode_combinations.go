package fuzz

import (
	"context"
	"fmt"
)

// combinationsStrategy: full cartesian product across per-position payload
// lists. Request count = product of list sizes. (Burp: Cluster Bomb.)
//
// Iteration uses an index-counter (treat the position list lengths as a
// mixed-radix number, increment to enumerate combinations). We never
// materialise the cartesian — a 4-position fuzz with 1000-element lists is
// 10^12 entries.
type combinationsStrategy struct{}

func (combinationsStrategy) Mode() FuzzMode { return ModeCombinations }

func (combinationsStrategy) RequestCount(positions []FuzzerPosition, payloads ResolvedPayloads) (int, error) {
	if len(payloads.PerPosition) != len(positions) {
		return 0, fmt.Errorf("combinations: payload list count (%d) does not match position count (%d)",
			len(payloads.PerPosition), len(positions))
	}
	if len(positions) == 0 {
		return 0, nil
	}
	total := 1
	for i, list := range payloads.PerPosition {
		if len(list) == 0 {
			return 0, fmt.Errorf("combinations: position %d has an empty payload list", i)
		}
		// Overflow guard: int math saturates rather than wrapping silently.
		// 10^12 fits in int64; we cap above ~9e18 (math.MaxInt64) to be safe.
		const maxBeforeOverflow = 1 << 62
		if total > maxBeforeOverflow/len(list) {
			return 0, fmt.Errorf("combinations: cartesian product overflows int")
		}
		total *= len(list)
	}
	return total, nil
}

func (combinationsStrategy) Iterate(ctx context.Context, positions []FuzzerPosition, payloads ResolvedPayloads) <-chan Assignment {
	out := make(chan Assignment)
	go func() {
		defer close(out)
		if len(payloads.PerPosition) != len(positions) || len(positions) == 0 {
			return
		}
		// Per-position cursors. Increment from the rightmost position; carry
		// to the left when it wraps. Standard mixed-radix enumeration.
		idx := make([]int, len(positions))
		assignmentIdx := 0
		for {
			vals := make([]string, len(positions))
			for j, c := range idx {
				if c >= len(payloads.PerPosition[j]) {
					// One of the lists is empty; nothing to yield.
					return
				}
				vals[j] = payloads.PerPosition[j][c]
			}
			select {
			case out <- Assignment{Payloads: vals, Index: assignmentIdx, PositionIndex: -1}:
				assignmentIdx++
			case <-ctx.Done():
				return
			}
			// Increment with carry, from the right.
			carry := 1
			for j := len(idx) - 1; j >= 0 && carry > 0; j-- {
				idx[j] += carry
				if idx[j] < len(payloads.PerPosition[j]) {
					carry = 0
				} else {
					idx[j] = 0
					carry = 1
				}
			}
			if carry > 0 {
				return // overflow off the left: we're done
			}
		}
	}()
	return out
}
