package report

import (
	"fmt"
	"html/template"
	"math"
	"strings"
)

const (
	donutSize   = 220.0
	donutStroke = 34.0
)

type donutSegment struct {
	count int
	token string
}

// severityDonut renders the severity split as inline SVG. Strokes reference CSS
// custom properties so the chart follows the theme and prints correctly.
func severityDonut(s Summary) template.HTML {
	segments := []donutSegment{
		{s.CriticalCount, "--severity-critical"},
		{s.HighCount, "--severity-high"},
		{s.MediumCount, "--severity-medium"},
		{s.LowCount, "--severity-low"},
		{s.InfoCount, "--severity-info"},
	}

	total, nonZero := 0, 0
	for _, seg := range segments {
		total += seg.count
		if seg.count > 0 {
			nonZero++
		}
	}

	center := donutSize / 2
	radius := center - donutStroke/2

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="donut" viewBox="0 0 %g %g" role="img" aria-label="Issues by severity">`, donutSize, donutSize)
	b.WriteString(`<title>Issues by severity</title>`)

	ring := func(token string) {
		fmt.Fprintf(&b, `<circle cx="%g" cy="%g" r="%g" fill="none" stroke="var(%s)" stroke-width="%g"/>`,
			center, center, radius, token, donutStroke)
	}

	if total == 0 {
		ring("--muted")
		b.WriteString(`</svg>`)
		return template.HTML(b.String())
	}

	// A single full-circle arc has coincident endpoints and renders as nothing.
	if nonZero == 1 {
		for _, seg := range segments {
			if seg.count > 0 {
				ring(seg.token)
			}
		}
		b.WriteString(`</svg>`)
		return template.HTML(b.String())
	}

	angle := -math.Pi / 2
	for _, seg := range segments {
		if seg.count == 0 {
			continue
		}

		sweep := 2 * math.Pi * float64(seg.count) / float64(total)
		end := angle + sweep

		largeArc := 0
		if sweep > math.Pi {
			largeArc = 1
		}

		fmt.Fprintf(&b,
			`<path d="M %.3f %.3f A %g %g 0 %d 1 %.3f %.3f" fill="none" stroke="var(%s)" stroke-width="%g" stroke-linecap="butt"/>`,
			center+radius*math.Cos(angle), center+radius*math.Sin(angle),
			radius, radius, largeArc,
			center+radius*math.Cos(end), center+radius*math.Sin(end),
			seg.token, donutStroke)

		angle = end
	}

	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// TopTypeBar is one row of the top issue types chart.
type TopTypeBar struct {
	Title   string
	Count   int
	Percent float64
}

func topTypeBars(s Summary) []TopTypeBar {
	if len(s.TopVulnTypes) == 0 {
		return nil
	}

	max := 0
	for _, t := range s.TopVulnTypes {
		if t.Count > max {
			max = t.Count
		}
	}

	bars := make([]TopTypeBar, 0, len(s.TopVulnTypes))
	for _, t := range s.TopVulnTypes {
		percent := 0.0
		if max > 0 {
			percent = float64(t.Count) / float64(max) * 100
		}
		bars = append(bars, TopTypeBar{Title: t.Title, Count: t.Count, Percent: percent})
	}
	return bars
}
