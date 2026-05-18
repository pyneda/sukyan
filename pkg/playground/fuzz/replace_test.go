package fuzz

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReplacePayloadsBasic(t *testing.T) {
	raw := "GET /search?q=ORIG HTTP/1.1\r\nHost: example.com\r\n\r\n"
	positions := []FuzzerPosition{
		{Start: 14, End: 18, OriginalValue: "ORIG"},
	}
	got := ReplacePayloads(raw, positions, []string{"PAYLOAD"})
	require.Equal(t, "GET /search?q=PAYLOAD HTTP/1.1\r\nHost: example.com\r\n\r\n", got)
}

func TestReplacePayloadsRoundTripIdentity(t *testing.T) {
	raw := "GET /a HTTP/1.1\r\n\r\n"
	positions := []FuzzerPosition{{Start: 5, End: 6, OriginalValue: "a"}}
	got := ReplacePayloads(raw, positions, []string{"a"})
	require.Equal(t, raw, got, "replacing with original value must round-trip")
}

func TestReplacePayloadsHandlesUnsortedInput(t *testing.T) {
	// Same fuzz, two positions, passed in reverse order
	raw := "GET /a HTTP/1.1\r\nHost: b\r\n\r\n"
	sorted := []FuzzerPosition{
		{Start: 5, End: 6, OriginalValue: "a"},
		{Start: 23, End: 24, OriginalValue: "b"},
	}
	unsorted := []FuzzerPosition{sorted[1], sorted[0]}

	got1 := ReplacePayloads(raw, sorted, []string{"X", "Y"})
	got2 := ReplacePayloads(raw, unsorted, []string{"Y", "X"}) // payloads match unsorted order
	require.Equal(t, got1, got2, "result must not depend on caller's position order")
}

func TestReplacePayloadsAdjustsOffsetForLongerPayloads(t *testing.T) {
	raw := "GET /a/b HTTP/1.1\r\n\r\n"
	positions := []FuzzerPosition{
		{Start: 5, End: 6, OriginalValue: "a"},
		{Start: 7, End: 8, OriginalValue: "b"},
	}
	got := ReplacePayloads(raw, positions, []string{"FIRST", "SECOND"})
	require.Equal(t, "GET /FIRST/SECOND HTTP/1.1\r\n\r\n", got)
}

func TestReplacePayloadsAdjustsOffsetForShorterPayloads(t *testing.T) {
	raw := "GET /HELLO/WORLD HTTP/1.1\r\n\r\n"
	positions := []FuzzerPosition{
		{Start: 5, End: 10, OriginalValue: "HELLO"},
		{Start: 11, End: 16, OriginalValue: "WORLD"},
	}
	got := ReplacePayloads(raw, positions, []string{"x", "y"})
	require.Equal(t, "GET /x/y HTTP/1.1\r\n\r\n", got)
}

func TestReplacePayloadsHandlesEmptyPositions(t *testing.T) {
	raw := "anything"
	got := ReplacePayloads(raw, []FuzzerPosition{}, []string{})
	require.Equal(t, raw, got)
}

func TestReplacePayloadsMismatchedLengthsReturnsUnchanged(t *testing.T) {
	raw := "GET / HTTP/1.1\r\n\r\n"
	positions := []FuzzerPosition{{Start: 4, End: 5, OriginalValue: "/"}}
	got := ReplacePayloads(raw, positions, []string{"a", "b"}) // 2 payloads, 1 position
	require.Equal(t, raw, got, "mismatched lengths must not corrupt input")
}

func TestReplacePayloadsHandlesOutOfRangeGracefully(t *testing.T) {
	raw := "short"
	positions := []FuzzerPosition{
		{Start: 0, End: 100, OriginalValue: "out-of-range"},
	}
	require.NotPanics(t, func() {
		ReplacePayloads(raw, positions, []string{"x"})
	})
}

func TestReplacePayloadsEdgeByte0(t *testing.T) {
	raw := "abcdef"
	positions := []FuzzerPosition{{Start: 0, End: 1, OriginalValue: "a"}}
	got := ReplacePayloads(raw, positions, []string{"X"})
	require.Equal(t, "Xbcdef", got)
}

func TestReplacePayloadsEdgeLastByte(t *testing.T) {
	raw := "abcdef"
	positions := []FuzzerPosition{{Start: 5, End: 6, OriginalValue: "f"}}
	got := ReplacePayloads(raw, positions, []string{"Z"})
	require.Equal(t, "abcdeZ", got)
}

func TestReplacePayloadsFullCover(t *testing.T) {
	raw := "abcdef"
	positions := []FuzzerPosition{{Start: 0, End: 6, OriginalValue: "abcdef"}}
	got := ReplacePayloads(raw, positions, []string{"WHOLE"})
	require.Equal(t, "WHOLE", got)
}
