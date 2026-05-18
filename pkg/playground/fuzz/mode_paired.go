package fuzz

import (
	"context"
	"fmt"
)

// pairedStrategy: per-position payload lists walked in lockstep by index.
// Request count = min(list sizes). (Burp: Pitchfork.) Useful for correlated
// inputs — username[i] paired with password[i], or a token with a matching id.
type pairedStrategy struct{}

func (pairedStrategy) Mode() FuzzMode { return ModePaired }

func (pairedStrategy) RequestCount(positions []FuzzerPosition, payloads ResolvedPayloads) (int, error) {
	if len(payloads.PerPosition) != len(positions) {
		return 0, fmt.Errorf("paired: payload list count (%d) does not match position count (%d)",
			len(payloads.PerPosition), len(positions))
	}
	if len(positions) == 0 {
		return 0, nil
	}
	n := len(payloads.PerPosition[0])
	for _, list := range payloads.PerPosition[1:] {
		if len(list) < n {
			n = len(list)
		}
	}
	return n, nil
}

func (pairedStrategy) Iterate(ctx context.Context, positions []FuzzerPosition, payloads ResolvedPayloads) <-chan Assignment {
	out := make(chan Assignment)
	go func() {
		defer close(out)
		if len(payloads.PerPosition) != len(positions) || len(positions) == 0 {
			return
		}
		n := len(payloads.PerPosition[0])
		for _, list := range payloads.PerPosition[1:] {
			if len(list) < n {
				n = len(list)
			}
		}
		for i := 0; i < n; i++ {
			vals := make([]string, len(positions))
			for j := range positions {
				vals[j] = payloads.PerPosition[j][i]
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
