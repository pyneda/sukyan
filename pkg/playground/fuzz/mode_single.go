package fuzz

import (
	"context"
	"fmt"
)

// singleStrategy: one shared payload list, injected into one position at a
// time. The other positions keep their original values.
//
// Iteration order: for each payload p, cycle through positions 0..N-1.
// (Same as ffuf's --mode single-ish semantics; matches Burp Sniper exactly.)
type singleStrategy struct{}

func (singleStrategy) Mode() FuzzMode { return ModeSingle }

func (singleStrategy) RequestCount(positions []FuzzerPosition, payloads ResolvedPayloads) (int, error) {
	if payloads.Shared == nil {
		return 0, fmt.Errorf("single: shared payloads required")
	}
	return len(payloads.Shared) * len(positions), nil
}

func (singleStrategy) Iterate(ctx context.Context, positions []FuzzerPosition, payloads ResolvedPayloads) <-chan Assignment {
	out := make(chan Assignment)
	go func() {
		defer close(out)
		if payloads.Shared == nil || len(positions) == 0 {
			return
		}
		idx := 0
		for _, p := range payloads.Shared {
			for posIdx := range positions {
				vals := make([]string, len(positions))
				for j, pos := range positions {
					vals[j] = pos.OriginalValue
				}
				vals[posIdx] = p
				select {
				case out <- Assignment{Payloads: vals, Index: idx, PositionIndex: posIdx}:
					idx++
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}
