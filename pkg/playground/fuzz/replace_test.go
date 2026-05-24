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

// Regression for GAP2-01: positions arrive as UTF-16 code unit offsets from the
// browser textarea but the raw request can contain multi-byte UTF-8 characters
// (notably the § insertion marker itself). The substitution must operate on
// the logical character range, not naive byte slicing.
func TestReplacePayloadsHandlesMultiByteMarker(t *testing.T) {
	// `§` (U+00A7) is 1 UTF-16 unit and 2 UTF-8 bytes. `?id=§§` puts the two
	// markers at UTF-16 positions 4..6 (and bytes 4..8).
	raw := "GET /q?id=§§ HTTP/1.1\r\n\r\n"
	positions := []FuzzerPosition{{Start: 10, End: 12, OriginalValue: "§§"}}
	got := ReplacePayloads(raw, positions, []string{"abc"})
	require.Equal(t, "GET /q?id=abc HTTP/1.1\r\n\r\n", got)
}

// Regression for GAP2-01: multi-byte char before the insertion point shifts
// byte offsets but not UTF-16 offsets. Replacement must still target the
// correct logical range.
func TestReplacePayloadsHandlesMultiByteBeforeInsertion(t *testing.T) {
	// `café` precedes `INJ` (positions 5..8 in UTF-16 units; bytes 6..9).
	raw := "café INJ end"
	positions := []FuzzerPosition{{Start: 5, End: 8, OriginalValue: "INJ"}}
	got := ReplacePayloads(raw, positions, []string{"abc"})
	require.Equal(t, "café abc end", got)
}

// Regression for GAP4-07: WS-fuzz binary content with § markers must also be
// replaced cleanly.
func TestReplacePayloadsBinaryContentWithMarkers(t *testing.T) {
	raw := "HELLO§INJ§WORLD"
	// `INJ` (with surrounding §) spans UTF-16 positions 5..10 (the entire
	// `§INJ§` block) — but a typical user selects just `INJ` at 6..9.
	positions := []FuzzerPosition{{Start: 6, End: 9, OriginalValue: "INJ"}}
	got := ReplacePayloads(raw, positions, []string{"abc"})
	require.Equal(t, "HELLO§abc§WORLD", got)
}

// Regression: surrogate pair (emoji) counts as 2 UTF-16 units.
func TestReplacePayloadsHandlesSurrogatePair(t *testing.T) {
	// 😀 (U+1F600) = 2 UTF-16 units, 4 UTF-8 bytes. `XX` follows at UTF-16
	// positions 2..4.
	raw := "😀XX"
	positions := []FuzzerPosition{{Start: 2, End: 4, OriginalValue: "XX"}}
	got := ReplacePayloads(raw, positions, []string{"YY"})
	require.Equal(t, "😀YY", got)
}
