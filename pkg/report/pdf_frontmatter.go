package report

import (
	"strconv"
	"strings"
)

const tocLinesPerPage = 44

type tocEntry struct {
	Title    string
	Severity string
	Count    int
	Page     int
	Link     int
}

func tocPagesNeeded(findings int) int {
	if findings <= 0 {
		return 1
	}
	return (findings + tocLinesPerPage - 1) / tocLinesPerPage
}

func (d *pdfDoc) coverPage(s Summary, scope string) {
	d.pdf.AddPage()

	d.pdf.SetY(70)
	d.pdf.SetFont(fontSans, "B", sizeCoverTitle)
	d.setInk(inkBody)
	d.pdf.MultiCell(contentWidth, 13, d.opts.Title, "", "L", false)

	d.pdf.Ln(4)
	d.setDraw(inkRule)
	d.pdf.SetLineWidth(0.4)
	y := d.pdf.GetY()
	d.pdf.Line(marginX, y, marginX+60, y)

	d.pdf.Ln(10)
	d.pdf.SetFont(fontSans, "", sizeBody)
	d.setInk(inkMuted)
	d.pdf.MultiCell(contentWidth, leadBody, scope, "", "L", false)

	d.pdf.Ln(2)
	d.pdf.MultiCell(contentWidth, leadBody, "Generated "+d.opts.GeneratedAt.Format("2 January 2006"), "", "L", false)

	d.pdf.SetY(-52)
	d.pdf.SetFont(fontSans, "B", sizeH2)
	d.setInk(inkBody)
	d.pdf.CellFormat(contentWidth, 8, strconv.Itoa(s.TotalIssues)+" findings", "", 1, "L", false, 0, "")

	d.pdf.SetFont(fontSans, "", sizeCaption)
	d.setInk(inkMuted)
	d.pdf.CellFormat(contentWidth, 5,
		strconv.Itoa(s.UniqueIssueTypes)+" issue types across "+strconv.Itoa(s.UniqueAffectedEndpoints)+" affected endpoints",
		"", 1, "L", false, 0, "")

	d.setInk(inkBody)
}

func (d *pdfDoc) executiveSummary(s Summary, scope string) {
	d.pdf.AddPage()
	d.h1("Executive summary")

	d.body("This report covers " + strings.TrimSuffix(scope, ".") + ". Sukyan performed automated " +
		"security testing against the in-scope hosts, recording every finding with the request and " +
		"response that produced it.")

	if s.TotalIssues == 0 {
		d.body("No issues were recorded. That is not the same as the scope being secure: it means " +
			"the checks that ran did not produce a finding. Coverage depends on what was reachable " +
			"during the scan.")
		return
	}

	d.body("Sukyan recorded " + strconv.Itoa(s.TotalIssues) + " findings of " +
		strconv.Itoa(s.UniqueIssueTypes) + " distinct types, affecting " +
		strconv.Itoa(s.UniqueAffectedEndpoints) + " endpoints. The split by severity is shown below. " +
		"Findings are grouped by type in the body of this report, with every affected location listed " +
		"beneath its finding.")

	d.pdf.Ln(2)
	d.label("Findings by severity")
	d.pdf.Ln(1)
	d.severityStackedBar(s)

	if actionable := s.CriticalCount + s.HighCount; actionable > 0 {
		d.body("Of these, " + strconv.Itoa(actionable) + " are rated Critical or High and warrant " +
			"attention before the others. They are presented first.")
	}

	if len(s.TopVulnTypes) > 0 {
		d.pdf.Ln(2)
		d.label("Most frequent issue types")
		d.pdf.Ln(1)
		d.topTypesChart(s)
	}
}

func (d *pdfDoc) severityDefinitions() {
	d.pdf.AddPage()
	d.h1("How to read severity")

	d.body("Severity describes the risk a finding presents in the context of this assessment. It " +
		"reflects both how exploitable the weakness is and what an attacker gains by exploiting it.")
	d.pdf.Ln(2)

	for _, sev := range severityOrder {
		d.keepTogether(20, func() {
			d.severityChip(sev)
			d.pdf.Ln(7)
			d.pdf.SetFont(fontSans, "", sizeBody)
			d.setInk(inkBody)
			d.pdf.MultiCell(contentWidth, leadBody, severityDescriptions[sev], "", "L", false)
			d.pdf.Ln(3)
		})
	}

	d.pdf.Ln(1)
	d.muted("Confidence is reported separately on each instance. It describes how certain sukyan is " +
		"that the finding is real, and is independent of severity.")
}

// reserveTOC adds blank pages for the contents, returning their numbers. The
// count is derived from the finding count before layout, so page numbers
// recorded while rendering the body stay valid when the pages are filled in.
func (d *pdfDoc) reserveTOC(findings int) []int {
	pages := make([]int, 0, tocPagesNeeded(findings))
	for i := 0; i < tocPagesNeeded(findings); i++ {
		d.pdf.AddPage()
		pages = append(pages, d.pdf.PageNo())
	}
	return pages
}

func (d *pdfDoc) writeTOC(pages []int, entries []tocEntry) {
	if len(pages) == 0 {
		return
	}

	cursor := 0
	for i, page := range pages {
		d.pdf.SetPage(page)
		d.pdf.SetY(marginTop)

		if i == 0 {
			d.h1("Contents")
		}

		d.pdf.SetFont(fontSans, "", sizeBody)
		for cursor < len(entries) && d.pdf.GetY() < d.bottom()-6 {
			d.tocRow(entries[cursor])
			cursor++
		}
	}
}

func (d *pdfDoc) tocRow(e tocEntry) {
	y := d.pdf.GetY()

	d.setFill(severityFill(e.Severity))
	d.pdf.Rect(marginX, y+1.4, 2.4, 2.4, "F")

	d.pdf.SetX(marginX + 5)
	d.setInk(inkBody)
	title := e.Title
	if e.Count > 1 {
		title += "  (" + strconv.Itoa(e.Count) + ")"
	}
	d.pdf.CellFormat(contentWidth-5-14, 5.6, d.truncateToWidth(title, contentWidth-5-16), "", 0, "L", false, 0, "")

	d.setInk(inkMuted)
	d.pdf.CellFormat(14, 5.6, strconv.Itoa(e.Page), "", 1, "R", false, 0, "")
	d.setInk(inkBody)

	if e.Link > 0 {
		d.pdf.Link(marginX, y, contentWidth, 5.6, e.Link)
	}
}
