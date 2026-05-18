package fuzz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// drainAssignments collects all assignments yielded by a strategy until the
// channel closes. Returns them in yield order.
func drainAssignments(ch <-chan Assignment) []Assignment {
	out := []Assignment{}
	for a := range ch {
		out = append(out, a)
	}
	return out
}

func TestSingleStrategy(t *testing.T) {
	positions := []FuzzerPosition{
		{Start: 0, End: 1, OriginalValue: "a"},
		{Start: 2, End: 3, OriginalValue: "b"},
	}
	resolved := ResolvedPayloads{Shared: []string{"X", "Y", "Z"}}
	s := singleStrategy{}

	count, err := s.RequestCount(positions, resolved)
	require.NoError(t, err)
	require.Equal(t, 6, count, "single: payloads(3) × positions(2)")

	got := drainAssignments(s.Iterate(context.Background(), positions, resolved))
	require.Len(t, got, 6)

	// Expected order: for each payload, cycle through positions
	wantPayloads := [][]string{
		{"X", "b"}, {"a", "X"},
		{"Y", "b"}, {"a", "Y"},
		{"Z", "b"}, {"a", "Z"},
	}
	wantPosIdx := []int{0, 1, 0, 1, 0, 1}
	for i, a := range got {
		require.Equal(t, wantPayloads[i], a.Payloads, "assignment %d payloads", i)
		require.Equal(t, wantPosIdx[i], a.PositionIndex, "assignment %d positionIndex", i)
		require.Equal(t, i, a.Index, "assignment %d index", i)
	}
}

func TestSingleStrategyEmptyPayloadsReturnsNothing(t *testing.T) {
	positions := []FuzzerPosition{{Start: 0, End: 1, OriginalValue: "a"}}
	resolved := ResolvedPayloads{Shared: []string{}}
	got := drainAssignments(singleStrategy{}.Iterate(context.Background(), positions, resolved))
	require.Empty(t, got)
}

func TestAllStrategy(t *testing.T) {
	positions := []FuzzerPosition{
		{Start: 0, End: 1, OriginalValue: "a"},
		{Start: 2, End: 3, OriginalValue: "b"},
		{Start: 4, End: 5, OriginalValue: "c"},
	}
	resolved := ResolvedPayloads{Shared: []string{"X", "Y"}}
	s := allStrategy{}

	count, err := s.RequestCount(positions, resolved)
	require.NoError(t, err)
	require.Equal(t, 2, count, "all: just payloads count")

	got := drainAssignments(s.Iterate(context.Background(), positions, resolved))
	require.Len(t, got, 2)
	require.Equal(t, []string{"X", "X", "X"}, got[0].Payloads)
	require.Equal(t, []string{"Y", "Y", "Y"}, got[1].Payloads)
	require.Equal(t, -1, got[0].PositionIndex)
}

func TestPairedStrategyTruncatesToSmallest(t *testing.T) {
	positions := []FuzzerPosition{
		{Start: 0, End: 1, OriginalValue: "a"},
		{Start: 2, End: 3, OriginalValue: "b"},
	}
	resolved := ResolvedPayloads{
		PerPosition: [][]string{
			{"x1", "x2", "x3", "x4"},
			{"y1", "y2"}, // shorter — paired stops at len 2
		},
	}
	s := pairedStrategy{}

	count, err := s.RequestCount(positions, resolved)
	require.NoError(t, err)
	require.Equal(t, 2, count, "paired: min(4,2)=2")

	got := drainAssignments(s.Iterate(context.Background(), positions, resolved))
	require.Len(t, got, 2)
	require.Equal(t, []string{"x1", "y1"}, got[0].Payloads)
	require.Equal(t, []string{"x2", "y2"}, got[1].Payloads)
}

func TestPairedStrategyMismatchedListCountErrors(t *testing.T) {
	positions := []FuzzerPosition{
		{Start: 0, End: 1, OriginalValue: "a"},
		{Start: 2, End: 3, OriginalValue: "b"},
	}
	resolved := ResolvedPayloads{PerPosition: [][]string{{"x"}}} // only 1 list for 2 positions
	_, err := pairedStrategy{}.RequestCount(positions, resolved)
	require.Error(t, err)
}

func TestCombinationsStrategyEnumeratesCartesian(t *testing.T) {
	positions := []FuzzerPosition{
		{Start: 0, End: 1, OriginalValue: "a"},
		{Start: 2, End: 3, OriginalValue: "b"},
	}
	resolved := ResolvedPayloads{
		PerPosition: [][]string{
			{"x1", "x2"},
			{"y1", "y2", "y3"},
		},
	}
	s := combinationsStrategy{}

	count, err := s.RequestCount(positions, resolved)
	require.NoError(t, err)
	require.Equal(t, 6, count, "combinations: 2 × 3")

	got := drainAssignments(s.Iterate(context.Background(), positions, resolved))
	require.Len(t, got, 6)
	// Verify every combination appears exactly once
	seen := map[string]bool{}
	for _, a := range got {
		key := a.Payloads[0] + "|" + a.Payloads[1]
		require.False(t, seen[key], "duplicate combo %q", key)
		seen[key] = true
	}
	require.Len(t, seen, 6)
}

func TestCombinationsStrategyOverflowGuard(t *testing.T) {
	positions := []FuzzerPosition{{}, {}, {}}
	// 3 lists of huge size → product overflows
	huge := make([]string, 1<<30)
	resolved := ResolvedPayloads{PerPosition: [][]string{huge, huge, huge}}
	_, err := combinationsStrategy{}.RequestCount(positions, resolved)
	require.Error(t, err, "should reject overflow")
}

func TestStrategyForReturnsCorrectMode(t *testing.T) {
	cases := []FuzzMode{ModeSingle, ModeAll, ModePaired, ModeCombinations}
	for _, m := range cases {
		s, err := StrategyFor(m)
		require.NoError(t, err)
		require.Equal(t, m, s.Mode(), "mode %s", m)
	}
	_, err := StrategyFor(FuzzMode("bogus"))
	require.Error(t, err)
}

func TestIterateCancellation(t *testing.T) {
	positions := []FuzzerPosition{{}, {}}
	resolved := ResolvedPayloads{
		PerPosition: [][]string{
			make([]string, 1000),
			make([]string, 1000),
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch := combinationsStrategy{}.Iterate(ctx, positions, resolved)
	cancel()
	// Drain — channel must close.
	for range ch {
	}
}
