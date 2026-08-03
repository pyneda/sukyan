package report

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func aggIssue(code, url string, conf int) *ReportIssue {
	return &ReportIssue{Code: code, Title: code, Severity: "Low", URL: url, Confidence: conf}
}

func TestBuildPDFFindingsRollsUpHostsAndURLs(t *testing.T) {
	groups := []*GroupedIssues{{Code: "c", Title: "C", Severity: "Low", Issues: []*ReportIssue{
		aggIssue("c", "https://a.test/1", 50),
		aggIssue("c", "https://a.test/1", 50),
		aggIssue("c", "https://a.test/2", 50),
		aggIssue("c", "https://b.test/1", 50),
	}}}

	f := buildPDFFindings(groups, 0)

	require.Len(t, f, 1)
	require.Equal(t, 3, f[0].UniqueURLs)
	require.Equal(t, []HostRollup{{Host: "a.test", URLCount: 2}, {Host: "b.test", URLCount: 1}}, f[0].Hosts)
	require.Equal(t, 4, f[0].TotalInstances)
	require.False(t, f[0].Capped())
}

func TestBuildPDFFindingsCapsInstancesButNotRollups(t *testing.T) {
	var issues []*ReportIssue
	for i := 0; i < 100; i++ {
		issues = append(issues, aggIssue("c", fmt.Sprintf("https://a.test/%d", i), 50))
	}
	groups := []*GroupedIssues{{Code: "c", Title: "C", Severity: "Low", Issues: issues}}

	f := buildPDFFindings(groups, 25)

	require.Len(t, f[0].Instances, 25)
	require.Equal(t, 100, f[0].TotalInstances)
	require.Equal(t, 75, f[0].OmittedInstances())
	require.Equal(t, 100, f[0].UniqueURLs, "rollups must count every instance, not just rendered ones")
	require.True(t, f[0].Capped())
}

func TestBuildPDFFindingsZeroMeansUnlimited(t *testing.T) {
	var issues []*ReportIssue
	for i := 0; i < 300; i++ {
		issues = append(issues, aggIssue("c", fmt.Sprintf("https://a.test/%d", i), 50))
	}
	groups := []*GroupedIssues{{Code: "c", Title: "C", Severity: "Low", Issues: issues}}

	f := buildPDFFindings(groups, 0)

	require.Len(t, f[0].Instances, 300)
	require.False(t, f[0].Capped())
	require.Equal(t, 0, f[0].OmittedInstances())
}

func TestBuildPDFFindingsHostOrderIsTotal(t *testing.T) {
	groups := []*GroupedIssues{{Code: "c", Title: "C", Severity: "Low", Issues: []*ReportIssue{
		aggIssue("c", "https://b.test/1", 50),
		aggIssue("c", "https://a.test/1", 50),
		aggIssue("c", "https://c.test/1", 50),
	}}}

	want := buildPDFFindings(groups, 0)[0].Hosts
	for i := 0; i < 200; i++ {
		require.Equal(t, want, buildPDFFindings(groups, 0)[0].Hosts)
	}
}

func TestBuildPDFFindingsHandlesUnparseableURL(t *testing.T) {
	groups := []*GroupedIssues{{Code: "c", Title: "C", Severity: "Low", Issues: []*ReportIssue{
		aggIssue("c", "::not a url::", 50),
	}}}

	f := buildPDFFindings(groups, 0)

	require.Len(t, f[0].Hosts, 1)
	require.Equal(t, 1, f[0].UniqueURLs)
}

func TestBuildPDFFindingsHandlesEmptyGroups(t *testing.T) {
	require.Empty(t, buildPDFFindings(nil, 50))
}
