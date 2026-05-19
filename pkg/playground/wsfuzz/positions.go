package wsfuzz

import "github.com/pyneda/sukyan/pkg/playground/fuzz"

// BuildPositionRefs walks the script and returns a flat slice of position
// references in canonical order (step 0 positions, then step 1, ...). Only
// steps with Role=fuzz contribute positions.
func BuildPositionRefs(script []WsFuzzStep) []WsPositionRef {
	var refs []WsPositionRef
	for i, s := range script {
		if s.Role != RoleFuzz {
			continue
		}
		for _, p := range s.Positions {
			refs = append(refs, WsPositionRef{StepIndex: i, Position: p})
		}
	}
	return refs
}

// FlatPositions returns the parallel []FuzzerPosition slice from a
// []WsPositionRef, preserving order. This is what the mode strategies consume.
func FlatPositions(refs []WsPositionRef) []fuzz.FuzzerPosition {
	out := make([]fuzz.FuzzerPosition, len(refs))
	for i, r := range refs {
		out[i] = r.Position
	}
	return out
}

// PositionsAndPayloadsForStep returns the (positions, payloads) subset
// corresponding to a specific step index. `payloads` is the per-position
// payload slice produced by the mode strategy's Assignment, positionally
// aligned with `refs`. Both returned slices preserve relative order.
func PositionsAndPayloadsForStep(stepIdx int, refs []WsPositionRef, payloads []string) ([]fuzz.FuzzerPosition, []string) {
	var pos []fuzz.FuzzerPosition
	var pay []string
	for i, r := range refs {
		if r.StepIndex != stepIdx {
			continue
		}
		pos = append(pos, r.Position)
		if i < len(payloads) {
			pay = append(pay, payloads[i])
		}
	}
	return pos, pay
}
