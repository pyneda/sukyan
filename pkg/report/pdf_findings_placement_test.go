package report

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Details is recorded per instance (the payload used, timings, revalidation
// attempts), so it must not be hoisted onto the shared finding narrative.
func TestFindingNarrativeOmitsPerInstanceDetails(t *testing.T) {
	issues := []*ReportIssue{
		{Code: "cmd", Title: "OS Command Injection", Severity: "High", Confidence: 90,
			URL: "https://a.test/1", Description: "Generic description.",
			Remediation: "Generic remediation.", Details: "INSTANCE ONE DETAIL"},
		{Code: "cmd", Title: "OS Command Injection", Severity: "High", Confidence: 80,
			URL: "https://a.test/2", Description: "Generic description.",
			Remediation: "Generic remediation.", Details: "INSTANCE TWO DETAIL"},
	}
	f := buildPDFFindings(groupIssuesByType(issues), 0)[0]

	d, err := newPDFDoc(ReportOptions{Title: "T", GeneratedAt: fixedTime()})
	require.NoError(t, err)
	d.startBody()

	before := d.pdf.GetY()
	d.findingNarrative(f)
	narrativeHeight := d.pdf.GetY() - before

	d2, err := newPDFDoc(ReportOptions{Title: "T", GeneratedAt: fixedTime()})
	require.NoError(t, err)
	d2.startBody()
	beforeNoDetails := d2.pdf.GetY()
	noDetails := f
	noDetails.Group = &GroupedIssues{
		Code: "cmd", Title: "OS Command Injection", Severity: "High",
		Description: "Generic description.", Remediation: "Generic remediation.",
		Issues: []*ReportIssue{{Code: "cmd", Description: "Generic description."}},
	}
	d2.findingNarrative(noDetails)

	require.InDelta(t, d2.pdf.GetY()-beforeNoDetails, narrativeHeight, 0.01,
		"finding narrative must not grow with per-instance Details")
}

func TestInstanceRendersItsOwnDetails(t *testing.T) {
	withDetails := &ReportIssue{Code: "cmd", Title: "OS Command Injection", Severity: "High",
		URL: "https://a.test/1", Confidence: 90, Details: "The payload was inserted in the host variable."}
	without := &ReportIssue{Code: "cmd", Title: "OS Command Injection", Severity: "High",
		URL: "https://a.test/1", Confidence: 90}

	measure := func(issue *ReportIssue) float64 {
		d, err := newPDFDoc(ReportOptions{Title: "T", GeneratedAt: fixedTime()})
		require.NoError(t, err)
		d.startBody()
		before := d.pdf.GetY()
		d.renderInstance(issue, 1, defaultMaxEvidenceBytes)
		return d.pdf.GetY() - before
	}

	require.Greater(t, measure(withDetails), measure(without),
		"an instance carrying Details must render more than one without")
}
