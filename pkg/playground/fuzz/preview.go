package fuzz

import (
	"fmt"
)

// PreviewResult is what the request-count preview endpoint returns. Warnings
// are advisory and intended to surface in the UI (yellow at >100k, red at >1M).
type PreviewResult struct {
	RequestCount int      `json:"request_count"`
	Warnings     []string `json:"warnings,omitempty"`
}

// Preview computes the request count + warnings for the given mode/payloads
// without launching anything. Errors are validation failures; the caller
// should surface them as 400s.
func Preview(mode FuzzMode, positions []FuzzerPosition, shared *FuzzerPayloadsGroup) (PreviewResult, error) {
	if err := Validate(mode, positions, shared); err != nil {
		return PreviewResult{}, err
	}
	strategy, err := StrategyFor(mode)
	if err != nil {
		return PreviewResult{}, err
	}
	resolved := Resolve(mode, positions, shared)
	count, err := strategy.RequestCount(positions, resolved)
	if err != nil {
		return PreviewResult{}, err
	}

	res := PreviewResult{RequestCount: count}
	switch {
	case count > 1_000_000:
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s mode will send %d requests — this is a very large fuzz; consider narrowing payloads or switching to a smaller-cardinality mode.", mode, count))
	case count > 100_000:
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s mode will send %d requests — this is a large fuzz, expect a long run.", mode, count))
	case count > 10_000:
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s mode will send %d requests.", mode, count))
	}
	return res, nil
}
