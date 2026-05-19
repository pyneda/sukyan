package wsfuzz

import (
	"testing"

	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/stretchr/testify/require"
)

func TestPreview_SingleMode(t *testing.T) {
	cfg := WsFuzzerConfig{
		TargetURL: "ws://x/ws",
		Mode:      fuzz.ModeSingle,
		Script: []WsFuzzStep{
			{Role: RoleFuzz, Content: "x", Positions: []fuzz.FuzzerPosition{
				{Start: 0, End: 1}, {Start: 2, End: 3},
			}},
		},
		SharedPayloads: &fuzz.FuzzerPayloadsGroup{Payloads: []string{"a", "b", "c"}},
	}
	iters, pos, _, errs := Preview(cfg)
	require.Empty(t, errs)
	require.Equal(t, 2, pos)
	require.Equal(t, 2*3, iters)
}

func TestPreview_CombinationsModeAcrossSteps(t *testing.T) {
	cfg := WsFuzzerConfig{
		TargetURL: "ws://x/ws",
		Mode:      fuzz.ModeCombinations,
		Script: []WsFuzzStep{
			{Role: RoleFuzz, Content: "x", Positions: []fuzz.FuzzerPosition{
				{Start: 0, End: 1, PayloadGroups: []fuzz.FuzzerPayloadsGroup{{Payloads: []string{"a", "b"}}}},
			}},
			{Role: RoleSetup, Content: "y"},
			{Role: RoleFuzz, Content: "z", Positions: []fuzz.FuzzerPosition{
				{Start: 0, End: 1, PayloadGroups: []fuzz.FuzzerPayloadsGroup{{Payloads: []string{"1", "2", "3"}}}},
			}},
		},
	}
	iters, pos, _, errs := Preview(cfg)
	require.Empty(t, errs)
	require.Equal(t, 2, pos)
	require.Equal(t, 6, iters)
}
