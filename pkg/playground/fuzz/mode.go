package fuzz

import (
	"context"
	"fmt"
)

// Assignment is one per-request payload distribution. Element i is the
// payload to inject into positions[i] for that request. The first element
// after Assignment is the index in the strategy's iteration sequence; the
// engine uses this both for FuzzResult.Index and for per-position baseline
// lookup in Single mode.
type Assignment struct {
	Payloads []string
	Index    int
	// PositionIndex is meaningful for Single mode (which position is being
	// fuzzed in this assignment); -1 for the other modes where every position
	// gets a fresh payload per request.
	PositionIndex int
}

// ModeStrategy yields per-request payload assignments for a given mode. Each
// strategy is independent of the engine's transport / persistence layer;
// adding new modes (Sample, Race) is a matter of adding a new implementation.
type ModeStrategy interface {
	// Mode returns the FuzzMode this strategy implements.
	Mode() FuzzMode

	// RequestCount computes the total request count for the given resolved
	// payloads. Returns an error if payloads are inconsistent with the mode's
	// requirements.
	RequestCount(positions []FuzzerPosition, payloads ResolvedPayloads) (int, error)

	// Iterate yields each assignment in order until exhausted or ctx is
	// cancelled. The channel is closed when iteration completes (naturally or
	// via cancellation). The caller MUST drain the channel after cancelling
	// to allow the producer goroutine to exit cleanly.
	Iterate(ctx context.Context, positions []FuzzerPosition, payloads ResolvedPayloads) <-chan Assignment
}

// StrategyFor returns the strategy implementation for the given mode, or an
// error if mode is unknown.
func StrategyFor(mode FuzzMode) (ModeStrategy, error) {
	switch mode {
	case ModeSingle:
		return singleStrategy{}, nil
	case ModeAll:
		return allStrategy{}, nil
	case ModePaired:
		return pairedStrategy{}, nil
	case ModeCombinations:
		return combinationsStrategy{}, nil
	}
	return nil, fmt.Errorf("unknown mode %q", mode)
}
