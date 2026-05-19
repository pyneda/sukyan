package wsfuzz

import (
	"testing"

	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/stretchr/testify/require"
)

func TestBuildPositionRefs(t *testing.T) {
	script := []WsFuzzStep{
		{ID: "s0", Role: RoleSetup},
		{ID: "s1", Role: RoleFuzz, Positions: []fuzz.FuzzerPosition{
			{Start: 5, End: 10},
			{Start: 15, End: 20},
		}},
		{ID: "s2", Role: RoleSetup},
		{ID: "s3", Role: RoleFuzz, Positions: []fuzz.FuzzerPosition{
			{Start: 8, End: 12},
		}},
		{ID: "s4", Role: RoleCheck},
	}
	refs := BuildPositionRefs(script)
	require.Len(t, refs, 3)
	require.Equal(t, 1, refs[0].StepIndex)
	require.Equal(t, 1, refs[1].StepIndex)
	require.Equal(t, 3, refs[2].StepIndex)
	require.Equal(t, 5, refs[0].Position.Start)
	require.Equal(t, 8, refs[2].Position.Start)
}

func TestFlatPositions(t *testing.T) {
	refs := []WsPositionRef{
		{StepIndex: 1, Position: fuzz.FuzzerPosition{Start: 1}},
		{StepIndex: 3, Position: fuzz.FuzzerPosition{Start: 2}},
	}
	out := FlatPositions(refs)
	require.Len(t, out, 2)
	require.Equal(t, 1, out[0].Start)
	require.Equal(t, 2, out[1].Start)
}

func TestPositionsAndPayloadsForStep(t *testing.T) {
	refs := []WsPositionRef{
		{StepIndex: 1, Position: fuzz.FuzzerPosition{Start: 5, End: 10}},
		{StepIndex: 3, Position: fuzz.FuzzerPosition{Start: 8, End: 12}},
		{StepIndex: 1, Position: fuzz.FuzzerPosition{Start: 15, End: 20}},
	}
	payloads := []string{"A", "B", "C"}

	step1Pos, step1Pay := PositionsAndPayloadsForStep(1, refs, payloads)
	require.Len(t, step1Pos, 2)
	require.Equal(t, 5, step1Pos[0].Start)
	require.Equal(t, 15, step1Pos[1].Start)
	require.Equal(t, []string{"A", "C"}, step1Pay)

	step3Pos, step3Pay := PositionsAndPayloadsForStep(3, refs, payloads)
	require.Len(t, step3Pos, 1)
	require.Equal(t, 8, step3Pos[0].Start)
	require.Equal(t, []string{"B"}, step3Pay)

	emptyPos, emptyPay := PositionsAndPayloadsForStep(0, refs, payloads)
	require.Empty(t, emptyPos)
	require.Empty(t, emptyPay)
}
