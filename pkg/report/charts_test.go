package report

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeverityDonutEmpty(t *testing.T) {
	svg := string(severityDonut(Summary{}))

	assert.Contains(t, svg, `stroke="var(--muted)"`, "empty report draws a placeholder ring")
	assert.NotContains(t, svg, "<path", "no arcs when there is nothing to chart")
}

func TestSeverityDonutSingleSeverity(t *testing.T) {
	svg := string(severityDonut(Summary{HighCount: 7}))

	assert.NotContains(t, svg, "<path", "a single severity draws a full ring, not an arc")
	assert.Contains(t, svg, `stroke="var(--severity-high)"`)
}

func TestSeverityDonutAllSeverities(t *testing.T) {
	svg := string(severityDonut(Summary{
		CriticalCount: 1, HighCount: 2, MediumCount: 3, LowCount: 4, InfoCount: 5,
	}))

	assert.Equal(t, 5, strings.Count(svg, "<path"))
	for _, token := range []string{"critical", "high", "medium", "low", "info"} {
		assert.Contains(t, svg, `stroke="var(--severity-`+token+`)"`)
	}
}

func TestSeverityDonutOmitsZeroSeverities(t *testing.T) {
	svg := string(severityDonut(Summary{CriticalCount: 2, InfoCount: 3}))

	assert.Equal(t, 2, strings.Count(svg, "<path"))
	assert.NotContains(t, svg, "--severity-low")
	assert.NotContains(t, svg, "--severity-medium")
}

func TestSeverityDonutIsAccessible(t *testing.T) {
	svg := string(severityDonut(Summary{HighCount: 1, LowCount: 1}))

	assert.Contains(t, svg, `role="img"`)
	assert.Contains(t, svg, "<title>")
}

func TestTopTypeBarsScaleToLargest(t *testing.T) {
	bars := topTypeBars(Summary{TopVulnTypes: []TopVulnType{
		{Code: "a", Title: "Alpha", Count: 10},
		{Code: "b", Title: "Beta", Count: 5},
	}})

	require.Len(t, bars, 2)
	assert.Equal(t, 100.0, bars[0].Percent, "the largest bar fills the track")
	assert.Equal(t, 50.0, bars[1].Percent)
}

func TestTopTypeBarsEmpty(t *testing.T) {
	assert.Empty(t, topTypeBars(Summary{}))
}
