package report

import (
	"strconv"
)

const (
	severityBarHeight = 7.0
	topTypeBarHeight  = 5.0
	topTypeBarWeight  = 3.0
	topTypeLabelWidth = 78.0
	topTypeCountWidth = 12.0
)

// severityStackedBar draws the severity split as one horizontal bar. A stacked
// bar rather than a donut: it reads more accurately in print at small sizes.
func (d *pdfDoc) severityStackedBar(s Summary) {
	x, y := float64(marginX), d.pdf.GetY()

	if s.TotalIssues == 0 {
		d.setFill(inkRule)
		d.pdf.Rect(x, y, contentWidth, severityBarHeight, "F")
		d.pdf.SetY(y + severityBarHeight + 3)
		d.muted("No issues were recorded for this scope.")
		return
	}

	for _, sev := range severityOrder {
		count := s.SeverityCounts[sev]
		if count == 0 {
			continue
		}

		w := contentWidth * float64(count) / float64(s.TotalIssues)
		d.setFill(severityFill(sev))
		d.pdf.Rect(x, y, w, severityBarHeight, "F")
		x += w
	}

	d.pdf.SetY(y + severityBarHeight + 3)
	d.severityLegend(s)
}

func (d *pdfDoc) severityLegend(s Summary) {
	d.pdf.SetFont(fontSans, "", sizeCaption)

	for _, sev := range severityOrder {
		count := s.SeverityCounts[sev]
		if count == 0 {
			continue
		}

		y := d.pdf.GetY()
		d.setFill(severityFill(sev))
		d.pdf.Rect(d.pdf.GetX(), y+0.9, 2.6, 2.6, "F")
		d.pdf.SetX(d.pdf.GetX() + 4)

		d.setInk(inkMuted)
		text := sev + " " + strconv.Itoa(count)
		d.pdf.CellFormat(d.pdf.GetStringWidth(text)+7, 4.4, text, "", 0, "L", false, 0, "")
	}

	d.setInk(inkBody)
	d.pdf.Ln(7)
}

func (d *pdfDoc) topTypesChart(s Summary) {
	bars := topTypeBars(s)
	if len(bars) == 0 {
		return
	}

	barArea := contentWidth - topTypeLabelWidth - topTypeCountWidth

	for _, bar := range bars {
		d.keepTogether(topTypeBarHeight+2.2, func() {
			y := d.pdf.GetY()

			d.pdf.SetFont(fontSans, "", sizeCaption)
			d.setInk(inkBody)
			d.pdf.CellFormat(topTypeLabelWidth, topTypeBarHeight+1,
				d.truncateToWidth(bar.Title, topTypeLabelWidth-3), "", 0, "L", false, 0, "")

			w := max(barArea*bar.Percent/100, 0.8)
			d.setFill(inkMuted)
			d.pdf.Rect(marginX+topTypeLabelWidth, y+(topTypeBarHeight-topTypeBarWeight)/2, w, topTypeBarWeight, "F")

			d.pdf.SetX(marginX + topTypeLabelWidth + barArea)
			d.setInk(inkMuted)
			d.pdf.CellFormat(topTypeCountWidth, topTypeBarHeight+1, strconv.Itoa(bar.Count), "", 1, "R", false, 0, "")

			d.setInk(inkBody)
		})
	}

	d.pdf.Ln(2)
}
