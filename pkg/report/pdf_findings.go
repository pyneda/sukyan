package report

import (
	"strconv"
	"strings"
)

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (d *pdfDoc) renderFindings(findings []pdfFinding, maxEvidenceBytes int) []tocEntry {
	entries := make([]tocEntry, 0, len(findings))

	for i, f := range findings {
		// Set before the break: the header for a new page is drawn as the page is
		// added, so assigning after would stamp the previous finding's title on it.
		d.currentFinding = f.Group.Title
		if i > 0 {
			d.pdf.AddPage()
		}
		d.suppressHeaderOn(d.pdf.PageNo())

		link := d.pdf.AddLink()
		d.h2(f.Group.Title, f.Group.Severity, link)

		entries = append(entries, tocEntry{
			Title:    f.Group.Title,
			Severity: f.Group.Severity,
			Count:    f.TotalInstances,
			Page:     d.pdf.PageNo(),
			Link:     link,
		})

		d.findingOverview(f)
		d.findingNarrative(f)
		d.findingLocations(f)

		if f.Capped() {
			d.pdf.Ln(1)
			d.muted("Showing " + strconv.Itoa(len(f.Instances)) + " of " +
				strconv.Itoa(f.TotalInstances) + " instances. The remaining " +
				strconv.Itoa(f.OmittedInstances()) + " affect " + strconv.Itoa(f.OmittedURLs()) +
				" further " + plural(f.OmittedURLs(), "URL", "URLs") +
				"; generate the JSON report for the complete set.")
		}

		for n, issue := range f.Instances {
			d.renderInstance(issue, n+1, maxEvidenceBytes)
		}
	}

	return entries
}

func (d *pdfDoc) findingOverview(f pdfFinding) {
	parts := []string{
		strconv.Itoa(f.TotalInstances) + " " + plural(f.TotalInstances, "occurrence", "occurrences"),
		strconv.Itoa(f.UniqueURLs) + " " + plural(f.UniqueURLs, "URL", "URLs"),
		strconv.Itoa(len(f.Hosts)) + " " + plural(len(f.Hosts), "host", "hosts"),
	}

	d.pdf.SetFont(fontSans, "", sizeCaption)
	d.setInk(inkMuted)
	line := strings.Join(parts, " · ")
	if f.Group.CWE > 0 {
		line += " · CWE-" + strconv.Itoa(f.Group.CWE)
	}
	d.pdf.MultiCell(contentWidth, 4.4, line, "", "L", false)
	d.setInk(inkBody)

	d.pdf.Ln(1.5)
	d.rule()
}

// findingNarrative carries only what is generic to the issue type. Per-instance
// prose lives on the instance; see renderInstance.
func (d *pdfDoc) findingNarrative(f pdfFinding) {
	first := f.Group.Issues[0]

	d.body(f.Group.Description)

	if strings.TrimSpace(f.Group.Remediation) != "" {
		d.pdf.Ln(1)
		d.label("Remediation")
		d.body(f.Group.Remediation)
	}

	if len(first.References) > 0 {
		d.pdf.Ln(0.5)
		d.label("References")
		d.pdf.SetFont(fontSans, "", sizeCaption)
		d.setInk(inkMuted)
		for _, ref := range first.References {
			d.pdf.MultiCell(contentWidth, 4.2, ref, "", "L", false)
		}
		d.setInk(inkBody)
		d.pdf.Ln(1.5)
	}
}

func (d *pdfDoc) findingLocations(f pdfFinding) {
	if len(f.Hosts) <= 1 {
		return
	}

	d.pdf.Ln(0.5)
	d.label("Affected hosts")
	d.pdf.SetFont(fontSans, "", sizeCaption)

	for _, h := range f.Hosts {
		d.keepTogether(4.6, func() {
			d.setInk(inkBody)
			d.pdf.CellFormat(contentWidth-24, 4.4, d.truncateToWidth(h.Host, contentWidth-26), "", 0, "L", false, 0, "")
			d.setInk(inkMuted)
			d.pdf.CellFormat(24, 4.4, strconv.Itoa(h.URLCount)+" "+plural(h.URLCount, "URL", "URLs"), "", 1, "R", false, 0, "")
		})
	}

	d.setInk(inkBody)
	d.pdf.Ln(2)
}

func (d *pdfDoc) renderInstance(issue *ReportIssue, n int, maxEvidenceBytes int) {
	method := issue.HTTPMethod
	if method == "" {
		method = "GET"
	}

	d.pdf.Ln(1)
	d.h3("Instance " + strconv.Itoa(n) + " — " + method + " " + issue.URL)

	meta := []string{}
	if issue.StatusCode > 0 {
		meta = append(meta, "Status "+strconv.Itoa(issue.StatusCode))
	}
	if issue.Confidence > 0 {
		meta = append(meta, "Confidence "+strconv.Itoa(issue.Confidence))
	}
	if issue.CreatedAt != "" {
		meta = append(meta, issue.CreatedAt)
	}
	if issue.FalsePositive {
		meta = append(meta, "Marked false positive")
	}
	if len(meta) > 0 {
		d.pdf.SetFont(fontSans, "", sizeCaption)
		d.setInk(inkMuted)
		d.pdf.MultiCell(contentWidth, 4.2, strings.Join(meta, " · "), "", "L", false)
		d.setInk(inkBody)
	}

	if strings.TrimSpace(issue.Details) != "" {
		d.pdf.Ln(1)
		d.body(issue.Details)
	}

	if strings.TrimSpace(issue.Payload) != "" {
		d.pdf.Ln(0.5)
		d.evidenceBlock("Payload", sanitizeEvidence([]byte(issue.Payload), maxEvidenceBytes))
	}

	if strings.TrimSpace(issue.Note) != "" {
		d.pdf.Ln(0.5)
		d.body(issue.Note)
	}

	d.pdf.Ln(1)
	d.evidenceBlock("Request", sanitizeEvidence(decodeBase64Field(issue.Request), maxEvidenceBytes))
	d.evidenceBlock("Response", sanitizeEvidence(decodeBase64Field(issue.Response), maxEvidenceBytes))

	if strings.TrimSpace(issue.CURLCommand) != "" {
		d.evidenceBlock("Reproduce", sanitizeEvidence([]byte(issue.CURLCommand), maxEvidenceBytes))
	}

	d.renderInteractions(issue.Interactions, maxEvidenceBytes)
	d.renderWebSocket(issue.WebSocketConnection, maxEvidenceBytes)
}

// renderInteractions presents out-of-band callbacks, which are the strongest
// proof sukyan produces and must survive onto paper.
func (d *pdfDoc) renderInteractions(interactions []*ReportInteraction, maxEvidenceBytes int) {
	if len(interactions) == 0 {
		return
	}

	d.pdf.Ln(0.5)
	d.label("Out-of-band callbacks")
	d.body("The scanner sent a payload referencing a unique domain it controls, and observed the " +
		"target contact that domain. The callback below is that observation.")

	for _, in := range interactions {
		d.keepTogether(16, func() {
			d.pdf.SetFont(fontSans, "B", sizeCaption)
			d.setInk(inkBody)
			d.pdf.MultiCell(contentWidth, 4.4, strings.ToUpper(in.Protocol)+" callback from "+in.RemoteAddress, "", "L", false)

			d.pdf.SetFont(fontSans, "", sizeCaption)
			d.setInk(inkMuted)
			detail := []string{in.Timestamp}
			if in.QType != "" {
				detail = append(detail, "query type "+in.QType)
			}
			if in.FullID != "" {
				detail = append(detail, in.FullID)
			}
			d.pdf.MultiCell(contentWidth, 4.2, strings.Join(detail, " · "), "", "L", false)
			d.setInk(inkBody)
		})

		if in.Cause != nil {
			d.pdf.SetFont(fontSans, "", sizeCaption)
			d.setInk(inkMuted)
			d.pdf.MultiCell(contentWidth, 4.2,
				"Triggered by "+in.Cause.TestName+" at "+in.Cause.InsertionPoint, "", "L", false)
			d.setInk(inkBody)
			d.evidenceBlock("Payload sent", sanitizeEvidence([]byte(in.Cause.Payload), maxEvidenceBytes))
		}

		d.evidenceBlock("Callback received", sanitizeEvidence(decodeBase64Field(in.RawRequest), maxEvidenceBytes))
	}
}

func (d *pdfDoc) renderWebSocket(conn *ReportWebSocketConnection, maxEvidenceBytes int) {
	if conn == nil {
		return
	}

	d.pdf.Ln(0.5)
	d.label("WebSocket connection")

	d.pdf.SetFont(fontSans, "", sizeCaption)
	d.setInk(inkMuted)
	summary := conn.URL
	if conn.StatusCode > 0 {
		summary += " · " + strconv.Itoa(conn.StatusCode) + " " + conn.StatusText
	}
	summary += " · " + strconv.Itoa(len(conn.Messages)) + " " + plural(len(conn.Messages), "message", "messages")
	d.pdf.MultiCell(contentWidth, 4.2, summary, "", "L", false)
	d.setInk(inkBody)
	d.pdf.Ln(1)

	for _, msg := range conn.Messages {
		arrow := "->"
		if strings.EqualFold(msg.Direction, "receive") || strings.EqualFold(msg.Direction, "received") {
			arrow = "<-"
		}

		var lines []string
		if msg.IsBinary {
			lines = hexDump([]byte(msg.PayloadData), maxEvidenceBytes)
		} else {
			lines = sanitizeEvidence([]byte(msg.PayloadData), maxEvidenceBytes).Lines
		}
		if len(lines) == 0 {
			lines = []string{"(empty frame)"}
		}

		d.keepTogether(leadMono*2, func() {
			d.pdf.SetFont(fontMono, "B", sizeMono)
			d.setInk(inkMuted)
			d.pdf.CellFormat(contentWidth, leadMono+1, arrow+"  "+msg.Timestamp, "", 1, "L", false, 0, "")
			d.setInk(inkBody)
		})

		d.pdf.SetFont(fontMono, "", sizeMono)
		for _, line := range lines {
			for _, wrapped := range d.wrapMono(line) {
				d.monoRow(wrapped)
			}
		}
		d.pdf.Ln(1.5)
	}
}
