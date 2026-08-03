package report

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSeverityRank(t *testing.T) {
	assert.Greater(t, severityRank("Critical"), severityRank("High"))
	assert.Greater(t, severityRank("High"), severityRank("Medium"))
	assert.Greater(t, severityRank("Medium"), severityRank("Low"))
	assert.Greater(t, severityRank("Low"), severityRank("Info"))
	assert.Greater(t, severityRank("Info"), severityRank("Unknown"))
}

func TestSeverityRankUnrecognised(t *testing.T) {
	assert.Equal(t, 0, severityRank("Moderate"))
	assert.Equal(t, 0, severityRank(""))
}
