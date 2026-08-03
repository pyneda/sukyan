package report

import (
	"strconv"
	"strings"

	"codeberg.org/go-pdf/fpdf"
)

// orphanGuard is the space a heading needs below it before it may sit on a page.
const orphanGuard = 25.0

// maxEvidenceLines bounds an evidence block regardless of its byte budget, so a
// long single-token body cannot turn a page into a wall of characters.
const maxEvidenceLines = 24

type pdfDoc struct {
	pdf            *fpdf.Fpdf
	opts           ReportOptions
	currentFinding string
	showChrome     bool
	pageHeight     float64
	// noHeaderPage carries the title in the body already, so repeating it above
	// the heading would be redundant.
	noHeaderPage int
}

func newPDFDoc(opts ReportOptions) (*pdfDoc, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetCatalogSort(true)
	pdf.SetCreationDate(opts.GeneratedAt)
	pdf.SetModificationDate(opts.GeneratedAt)

	if err := registerFonts(pdf); err != nil {
		return nil, err
	}

	pdf.SetTitle(opts.Title, true)
	pdf.SetAuthor("sukyan", true)
	pdf.SetSubject("Security assessment report", true)
	pdf.SetCreator("sukyan", true)

	pdf.SetMargins(marginX, marginTop, marginX)
	pdf.SetAutoPageBreak(true, marginBottom)

	_, pageHeight := pdf.GetPageSize()
	d := &pdfDoc{pdf: pdf, opts: opts, pageHeight: pageHeight}

	pdf.SetHeaderFunc(d.header)
	pdf.SetFooterFunc(d.footer)
	pdf.AliasNbPages("{nb}")

	return d, nil
}

func (d *pdfDoc) suppressHeaderOn(page int) { d.noHeaderPage = page }

func (d *pdfDoc) header() {
	if !d.showChrome || d.currentFinding == "" || d.pdf.PageNo() == d.noHeaderPage {
		return
	}

	d.pdf.SetY(10)
	d.setInk(inkMuted)
	d.pdf.SetFont(fontSans, "", sizeCaption)
	d.pdf.CellFormat(contentWidth, 4, d.truncateToWidth(d.currentFinding, contentWidth), "", 0, "L", false, 0, "")

	d.setDraw(inkRule)
	d.pdf.SetLineWidth(0.2)
	d.pdf.Line(marginX, 15, marginX+contentWidth, 15)

	d.pdf.SetY(marginTop)
	d.setInk(inkBody)
}

func (d *pdfDoc) footer() {
	if !d.showChrome {
		return
	}

	d.pdf.SetY(-13)
	d.pdf.SetFont(fontSans, "", sizeCaption)
	d.setInk(inkMuted)

	half := contentWidth / 2
	d.pdf.CellFormat(half, 5, d.truncateToWidth(d.opts.Title, half-4), "", 0, "L", false, 0, "")
	d.pdf.CellFormat(half, 5, "Page "+strconv.Itoa(d.pdf.PageNo())+" of {nb}", "", 0, "R", false, 0, "")

	d.setInk(inkBody)
}

func (d *pdfDoc) setInk(c rgb)    { d.pdf.SetTextColor(c.R, c.G, c.B) }
func (d *pdfDoc) setFill(c rgb)   { d.pdf.SetFillColor(c.R, c.G, c.B) }
func (d *pdfDoc) setDraw(c rgb)   { d.pdf.SetDrawColor(c.R, c.G, c.B) }
func (d *pdfDoc) bottom() float64 { return d.pageHeight - marginBottom }

// startBody turns on the running header and footer and opens the first body page.
func (d *pdfDoc) startBody() {
	d.showChrome = true
	d.pdf.AddPage()
}

// keepTogether breaks to a new page when the block would not fit whole, so
// headings are not orphaned and short evidence blocks are not split.
func (d *pdfDoc) keepTogether(height float64, render func()) {
	if d.pdf.GetY()+height > d.bottom() {
		d.pdf.AddPage()
	}
	render()
}

func (d *pdfDoc) h1(title string) {
	d.pdf.SetFont(fontSans, "B", sizeH1)
	d.setInk(inkBody)
	d.pdf.MultiCell(contentWidth, leadH1, title, "", "L", false)
	d.pdf.Ln(3)
}

func (d *pdfDoc) h2(title, severity string, link int) {
	d.keepTogether(leadH2+orphanGuard, func() {
		if link > 0 {
			d.pdf.SetLink(link, -1, -1)
		}
		d.pdf.Bookmark(title, 1, -1)

		if severity != "" {
			d.severityChip(severity)
			d.pdf.Ln(6)
		}

		d.pdf.SetFont(fontSans, "B", sizeH2)
		d.setInk(inkBody)
		d.pdf.MultiCell(contentWidth, leadH2, title, "", "L", false)
		d.pdf.Ln(1.5)
	})
	d.currentFinding = title
}

func (d *pdfDoc) h3(title string) {
	d.keepTogether(leadH3+orphanGuard, func() {
		d.pdf.Bookmark(title, 2, -1)
		d.pdf.SetFont(fontSans, "B", sizeH3)
		d.setInk(inkBody)
		d.pdf.MultiCell(contentWidth, leadH3, title, "", "L", false)
		d.pdf.Ln(0.5)
	})
}

func (d *pdfDoc) label(text string) {
	d.pdf.SetFont(fontSans, "B", sizeCaption)
	d.setInk(inkMuted)
	d.pdf.MultiCell(contentWidth, 4, strings.ToUpper(text), "", "L", false)
	d.setInk(inkBody)
}

func (d *pdfDoc) body(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	d.pdf.SetFont(fontSans, "", sizeBody)
	d.setInk(inkBody)
	d.pdf.MultiCell(contentWidth, leadBody, text, "", "L", false)
	d.pdf.Ln(1.5)
}

func (d *pdfDoc) muted(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	d.pdf.SetFont(fontSans, "I", sizeCaption)
	d.setInk(inkMuted)
	d.pdf.MultiCell(contentWidth, 4.2, text, "", "L", false)
	d.setInk(inkBody)
	d.pdf.Ln(1)
}

func (d *pdfDoc) rule() {
	d.setDraw(inkRule)
	d.pdf.SetLineWidth(0.2)
	y := d.pdf.GetY()
	d.pdf.Line(marginX, y, marginX+contentWidth, y)
	d.pdf.Ln(2.5)
}

func (d *pdfDoc) severityChip(severity string) {
	label := strings.ToUpper(severity)
	d.pdf.SetFont(fontSans, "B", sizeCaption)

	w := d.pdf.GetStringWidth(label) + 5
	x, y := d.pdf.GetX(), d.pdf.GetY()

	d.setFill(severityFill(severity))
	d.pdf.Rect(x, y, w, 4.6, "F")

	d.setInk(severityChipInk(severity))
	d.pdf.SetXY(x, y)
	d.pdf.CellFormat(w, 4.6, label, "", 0, "C", false, 0, "")

	d.setInk(inkBody)
	d.pdf.SetXY(marginX, y)
}

// wrapMono wraps one logical line to the evidence column. SplitLines returns no
// parts for an empty string, which would drop the blank line separating HTTP
// headers from the body, so an empty line maps to a single blank row.
func (d *pdfDoc) wrapMono(line string) []string {
	if line == "" {
		return []string{""}
	}

	parts := d.pdf.SplitLines([]byte(line), evidenceWidth-2)
	if len(parts) == 0 {
		return []string{""}
	}

	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, string(p))
	}
	return out
}

func (d *pdfDoc) monoRow(text string) {
	d.keepTogether(leadMono, func() {
		d.setFill(inkEvidenceBG)
		d.pdf.Rect(marginX+evidenceInset, d.pdf.GetY(), evidenceWidth, leadMono, "F")
		d.pdf.SetX(marginX + evidenceInset + 1)
		d.setInk(inkBody)
		d.pdf.CellFormat(evidenceWidth-2, leadMono, text, "", 1, "L", false, 0, "")
	})
}

// evidenceBlock renders monospace captured bytes on a tinted panel, wrapping
// long lines at the column rather than overflowing the frame.
func (d *pdfDoc) evidenceBlock(title string, ev evidenceText) {
	if ev.Empty() {
		return
	}

	d.label(title)

	d.pdf.SetFont(fontMono, "", sizeMono)

	rendered, omittedLines := 0, 0
	for _, line := range ev.Lines {
		for _, wrapped := range d.wrapMono(line) {
			if rendered >= maxEvidenceLines {
				omittedLines++
				continue
			}
			rendered++
			d.monoRow(wrapped)
		}
	}

	notes := make([]string, 0, 3)
	if ev.NotUTF8 {
		notes = append(notes, "Contains bytes that are not valid UTF-8; shown as ·.")
	}
	if omittedLines > 0 {
		notes = append(notes, "Shown to "+strconv.Itoa(maxEvidenceLines)+" lines; "+
			strconv.Itoa(omittedLines)+" further "+plural(omittedLines, "line", "lines")+" omitted.")
	}
	if ev.Truncated {
		notes = append(notes, "Truncated: "+strconv.Itoa(ev.OmittedBytes)+" further bytes omitted.")
	}
	if len(notes) > 0 {
		d.pdf.Ln(0.5)
		d.muted(strings.Join(notes, " "))
	}

	d.pdf.Ln(2)
}

func (d *pdfDoc) truncateToWidth(s string, width float64) string {
	if d.pdf.GetStringWidth(s) <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 1 {
		runes = runes[:len(runes)-1]
		if d.pdf.GetStringWidth(string(runes)+"…") <= width {
			return string(runes) + "…"
		}
	}
	return s
}

// finish restores the page cursor. fpdf's Output emits pages only up to the
// cursor, so returning from a backfilled TOC page would truncate the document.
func (d *pdfDoc) finish() error {
	d.pdf.SetPage(d.pdf.PageCount())
	return d.pdf.Error()
}
