package report

import (
	"io"
	"testing"

	"codeberg.org/go-pdf/fpdf"
	"github.com/stretchr/testify/require"
)

func TestSeverityPaletteCoversAllSeverities(t *testing.T) {
	for _, sev := range severityOrder {
		_, ok := severityFills[sev]
		require.True(t, ok, "missing fill for %s", sev)
		require.NotEmpty(t, severityDescriptions[sev], "missing description for %s", sev)
	}
}

func TestSeverityFillFallsBackForUnknownSeverity(t *testing.T) {
	require.Equal(t, severityFills["Info"], severityFill("Nonsense"))
}

func TestSeverityPaletteHasNoPurple(t *testing.T) {
	for name, c := range severityFills {
		purple := c.B > c.G+30 && c.R > c.G+30
		require.False(t, purple, "%s is purple: %+v", name, c)
	}
}

func TestSeverityPaletteLightnessDescends(t *testing.T) {
	luma := func(c rgb) float64 {
		return 0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)
	}
	for i := 1; i < len(severityOrder); i++ {
		prev, cur := severityFills[severityOrder[i-1]], severityFills[severityOrder[i]]
		require.Less(t, luma(prev), luma(cur),
			"%s must be darker than %s so greyscale stays readable", severityOrder[i-1], severityOrder[i])
	}
}

func TestFontsEmbedAndRegister(t *testing.T) {
	pdf := fpdf.New("P", "mm", "A4", "")
	require.NoError(t, registerFonts(pdf))

	pdf.AddPage()
	for _, style := range []string{"", "B", "I"} {
		pdf.SetFont(fontSans, style, sizeBody)
		pdf.Cell(0, 5, "sukyan — ünïcödé — 你好")
		pdf.Ln(5)
	}
	for _, style := range []string{"", "B"} {
		pdf.SetFont(fontMono, style, sizeMono)
		pdf.Cell(0, 5, "GET /?q=<svg/onload=alert(1)> HTTP/1.1")
		pdf.Ln(5)
	}

	require.NoError(t, pdf.Output(io.Discard))
	require.NoError(t, pdf.Error())
}
