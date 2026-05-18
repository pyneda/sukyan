package fuzz

import (
	"context"
	"fmt"
)

// allStrategy: one shared payload list, injected into every position
// simultaneously. (Burp: Battering Ram.) Useful when the same input needs to
// appear in multiple places — same auth token in N headers, same value in
// duplicated form fields, etc.
type allStrategy struct{}

func (allStrategy) Mode() FuzzMode { return ModeAll }

func (allStrategy) RequestCount(positions []FuzzerPosition, payloads ResolvedPayloads) (int, error) {
	if payloads.Shared == nil {
		return 0, fmt.Errorf("all: shared payloads required")
	}
	return len(payloads.Shared), nil
}

func (allStrategy) Iterate(ctx context.Context, positions []FuzzerPosition, payloads ResolvedPayloads) <-chan Assignment {
	out := make(chan Assignment)
	go func() {
		defer close(out)
		if payloads.Shared == nil || len(positions) == 0 {
			return
		}
		for i, p := range payloads.Shared {
			vals := make([]string, len(positions))
			for j := range positions {
				vals[j] = p
			}
			select {
			case out <- Assignment{Payloads: vals, Index: i, PositionIndex: -1}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
