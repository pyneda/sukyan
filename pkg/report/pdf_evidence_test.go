package report

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestSanitizeEvidenceKeepsPlainText(t *testing.T) {
	got := sanitizeEvidence([]byte("GET / HTTP/1.1\nHost: a.test\n"), 0)

	require.Equal(t, []string{"GET / HTTP/1.1", "Host: a.test"}, got.Lines)
	require.False(t, got.Truncated)
	require.False(t, got.NotUTF8)
}

func TestSanitizeEvidenceReplacesInvalidUTF8(t *testing.T) {
	got := sanitizeEvidence([]byte{0x47, 0x45, 0x54, 0xff, 0xfe, 0x0a}, 0)

	require.True(t, got.NotUTF8)
	joined := strings.Join(got.Lines, "\n")
	require.True(t, utf8.ValidString(joined), "output handed to fpdf must be valid UTF-8")
	require.NotContains(t, joined, "\xff")
}

func TestSanitizeEvidenceStripsControlBytes(t *testing.T) {
	got := sanitizeEvidence([]byte("a\x00b\x07c\n"), 0)

	require.Equal(t, []string{"a·b·c"}, got.Lines)
}

func TestSanitizeEvidenceNormalisesCRLF(t *testing.T) {
	got := sanitizeEvidence([]byte("a\r\nb\r\n"), 0)

	require.Equal(t, []string{"a", "b"}, got.Lines)
}

func TestSanitizeEvidencePreservesTabs(t *testing.T) {
	got := sanitizeEvidence([]byte("a\tb\n"), 0)

	require.Equal(t, []string{"a\tb"}, got.Lines)
}

func TestSanitizeEvidenceTruncatesToByteBudget(t *testing.T) {
	got := sanitizeEvidence(bytes.Repeat([]byte("x"), 5000), 1000)

	require.True(t, got.Truncated)
	require.Equal(t, 4000, got.OmittedBytes)
}

func TestSanitizeEvidenceUnlimitedWhenBudgetZero(t *testing.T) {
	got := sanitizeEvidence(bytes.Repeat([]byte("x"), 5000), 0)

	require.False(t, got.Truncated)
	require.Equal(t, 0, got.OmittedBytes)
}

func TestSanitizeEvidenceNeverSplitsAMultibyteRuneAtTheBudget(t *testing.T) {
	raw := []byte(strings.Repeat("é", 100)) // 200 bytes, 2 per rune
	got := sanitizeEvidence(raw, 51)        // lands mid-rune

	require.True(t, utf8.ValidString(strings.Join(got.Lines, "\n")))
	require.False(t, got.NotUTF8, "a clean cut at the budget is not corruption")
	require.True(t, got.Truncated)
}

func TestSanitizeEvidenceHandlesEmptyInput(t *testing.T) {
	got := sanitizeEvidence(nil, 0)

	require.Empty(t, got.Lines)
	require.False(t, got.Truncated)
}

func TestSanitizeEvidenceHandlesVeryLongSingleLine(t *testing.T) {
	got := sanitizeEvidence(bytes.Repeat([]byte("A"), 10000), 0)

	require.Len(t, got.Lines, 1)
	require.Len(t, got.Lines[0], 10000)
}

func TestHexDumpFormatsBinary(t *testing.T) {
	lines := hexDump([]byte{0x00, 0x01, 0x41, 0x42}, 0)

	require.Len(t, lines, 1)
	require.Contains(t, lines[0], "00000000")
	require.Contains(t, lines[0], "00 01 41 42")
	require.Contains(t, lines[0], "..AB")
}

func TestHexDumpSplitsSixteenBytesPerRow(t *testing.T) {
	lines := hexDump(bytes.Repeat([]byte{0x41}, 40), 0)

	require.Len(t, lines, 3)
}

func TestHexDumpRespectsBudget(t *testing.T) {
	lines := hexDump(bytes.Repeat([]byte{0x41}, 400), 32)

	require.Len(t, lines, 2)
}

func TestDecodeBase64FieldTolerantOfGarbage(t *testing.T) {
	require.Equal(t, []byte("hi"), decodeBase64Field("aGk="))
	require.Nil(t, decodeBase64Field("!!!not base64!!!"))
	require.Nil(t, decodeBase64Field(""))
}
