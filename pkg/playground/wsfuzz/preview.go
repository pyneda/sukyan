package wsfuzz

import (
	"github.com/pyneda/sukyan/pkg/playground/fuzz"
)

// Preview computes the planned iteration count, position count, and surfaces
// validator warnings/errors without launching a run.
func Preview(cfg WsFuzzerConfig) (iterations int, positions int, warnings []string, errors []string) {
	warnings, errors = Validate(cfg)
	refs := BuildPositionRefs(cfg.Script)
	positions = len(refs)
	if positions == 0 {
		return 0, 0, warnings, errors
	}
	flat := FlatPositions(refs)
	iterations = computePlannedCount(cfg.Mode, flat, cfg.SharedPayloads)
	return iterations, positions, warnings, errors
}

func computePlannedCount(mode fuzz.FuzzMode, positions []fuzz.FuzzerPosition, shared *fuzz.FuzzerPayloadsGroup) int {
	sharedN := 0
	if shared != nil {
		sharedN = len(shared.Payloads)
	}
	switch mode {
	case fuzz.ModeAll:
		return sharedN
	case fuzz.ModeSingle:
		return sharedN * len(positions)
	case fuzz.ModePaired:
		minN := -1
		for _, p := range positions {
			n := 0
			for _, g := range p.PayloadGroups {
				n += len(g.Payloads)
			}
			if minN < 0 || n < minN {
				minN = n
			}
		}
		if minN < 0 {
			return 0
		}
		return minN
	case fuzz.ModeCombinations:
		prod := 1
		for _, p := range positions {
			n := 0
			for _, g := range p.PayloadGroups {
				n += len(g.Payloads)
			}
			if n == 0 {
				return 0
			}
			prod *= n
		}
		return prod
	}
	return 0
}
