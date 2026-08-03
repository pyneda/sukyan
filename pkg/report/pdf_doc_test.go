package report

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func fixedTime() time.Time { return time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC) }

func testDoc(t *testing.T) *pdfDoc {
	t.Helper()
	d, err := newPDFDoc(ReportOptions{Title: "Test Report", GeneratedAt: fixedTime()})
	require.NoError(t, err)
	return d
}

func TestPDFDocProducesValidMultiPageDocument(t *testing.T) {
	d := testDoc(t)
	d.startBody()

	for i := 0; i < 40; i++ {
		d.h2(fmt.Sprintf("Finding %d", i), "High", 0)
		d.body("Some description text that fills a little space on the page so the document grows.")
	}

	require.NoError(t, d.finish())

	var buf bytes.Buffer
	require.NoError(t, d.pdf.Output(&buf))
	require.Greater(t, d.pdf.PageCount(), 1)
	require.True(t, bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")))
}

func TestPDFDocKeepTogetherBreaksRatherThanOrphaning(t *testing.T) {
	d := testDoc(t)
	d.startBody()

	d.pdf.SetY(d.bottom() - 5)
	before := d.pdf.PageNo()
	d.keepTogether(40, func() { d.body("block") })

	require.Equal(t, before+1, d.pdf.PageNo(), "must break rather than orphan the block")
	require.NoError(t, d.finish())
}

func TestPDFDocKeepTogetherStaysWhenBlockFits(t *testing.T) {
	d := testDoc(t)
	d.startBody()

	before := d.pdf.PageNo()
	d.keepTogether(10, func() { d.body("block") })

	require.Equal(t, before, d.pdf.PageNo())
	require.NoError(t, d.finish())
}

func TestPDFDocFinishRestoresPageCursor(t *testing.T) {
	d := testDoc(t)
	d.startBody()
	d.body("one")
	d.pdf.AddPage()
	d.body("two")

	d.pdf.SetPage(1) // what a table of contents backfill leaves behind
	require.NoError(t, d.finish())

	var buf bytes.Buffer
	require.NoError(t, d.pdf.Output(&buf))
	require.Equal(t, 2, d.pdf.PageCount(), "finish must restore the cursor or Output truncates")
}

func TestPDFDocEvidenceBlockRendersAndAnnotates(t *testing.T) {
	d := testDoc(t)
	d.startBody()

	ev := sanitizeEvidence(bytes.Repeat([]byte("A"), 5000), 1000)
	d.evidenceBlock("Response", ev)

	require.NoError(t, d.finish())
	require.NoError(t, d.pdf.Error())
}

func TestPDFDocEvidenceBlockSkipsEmpty(t *testing.T) {
	d := testDoc(t)
	d.startBody()

	before := d.pdf.GetY()
	d.evidenceBlock("Request", sanitizeEvidence(nil, 0))

	require.Equal(t, before, d.pdf.GetY(), "an empty block must not consume vertical space")
	require.NoError(t, d.finish())
}

func TestPDFDocSeverityChipRendersEverySeverity(t *testing.T) {
	d := testDoc(t)
	d.startBody()

	for _, sev := range severityOrder {
		d.severityChip(sev)
		d.pdf.Ln(8)
	}

	require.NoError(t, d.finish())
	require.NoError(t, d.pdf.Error())
}

func TestPDFDocTruncateToWidthShortensLongText(t *testing.T) {
	d := testDoc(t)
	d.startBody()
	d.pdf.SetFont(fontSans, "", sizeCaption)

	long := "a very long finding title that will certainly not fit inside a narrow running header column"
	got := d.truncateToWidth(long, 40)

	require.Less(t, len(got), len(long))
	require.LessOrEqual(t, d.pdf.GetStringWidth(got), 40.0)
	require.NoError(t, d.finish())
}
