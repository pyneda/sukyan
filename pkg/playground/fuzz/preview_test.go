package fuzz

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreviewSingleCount(t *testing.T) {
	res, err := Preview(ModeSingle,
		[]FuzzerPosition{mkPos(0, 1, "a"), mkPos(2, 3, "b")},
		&FuzzerPayloadsGroup{Payloads: []string{"x", "y", "z"}},
	)
	require.NoError(t, err)
	require.Equal(t, 6, res.RequestCount)
	require.Empty(t, res.Warnings)
}

func TestPreviewAllCount(t *testing.T) {
	res, err := Preview(ModeAll,
		[]FuzzerPosition{mkPos(0, 1, "a"), mkPos(2, 3, "b")},
		&FuzzerPayloadsGroup{Payloads: []string{"x", "y", "z"}},
	)
	require.NoError(t, err)
	require.Equal(t, 3, res.RequestCount)
}

func TestPreviewPairedTruncates(t *testing.T) {
	res, err := Preview(ModePaired,
		[]FuzzerPosition{
			mkPosWithGroup(0, 1, "a", "p1", "p2", "p3"),
			mkPosWithGroup(2, 3, "b", "q1"),
		},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, 1, res.RequestCount, "min(3,1)")
}

func TestPreviewCombinationsMultiplies(t *testing.T) {
	res, err := Preview(ModeCombinations,
		[]FuzzerPosition{
			mkPosWithGroup(0, 1, "a", "p1", "p2"),
			mkPosWithGroup(2, 3, "b", "q1", "q2", "q3"),
		},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, 6, res.RequestCount)
}

func TestPreviewWarningThresholds(t *testing.T) {
	// 200,001 to clear the 100k threshold but stay under 1M.
	bigPayloads := make([]string, 200001)
	for i := range bigPayloads {
		bigPayloads[i] = "x"
	}
	res, err := Preview(ModeAll,
		[]FuzzerPosition{mkPos(0, 1, "a")},
		&FuzzerPayloadsGroup{Payloads: bigPayloads},
	)
	require.NoError(t, err)
	require.NotEmpty(t, res.Warnings)
	require.True(t, strings.Contains(res.Warnings[0], "large fuzz"), "expected large-fuzz warning, got %q", res.Warnings[0])
}

func TestPreviewValidationErrors(t *testing.T) {
	_, err := Preview(ModeSingle, []FuzzerPosition{mkPos(0, 1, "a")}, nil)
	require.Error(t, err)
}
