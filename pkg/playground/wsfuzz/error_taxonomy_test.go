package wsfuzz

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStatus_CountsTowardErrorRate pins the error-rate watchdog taxonomy.
// Adding a new status without an entry here will fail the test, forcing the
// author to make an explicit decision about whether it counts.
func TestStatus_CountsTowardErrorRate(t *testing.T) {
	cases := []struct {
		s        IterationStatus
		expected bool
	}{
		{StatusCompleted, false},
		{StatusCheckFailed, false},
		{StatusStepFailedTimeout, true},
		{StatusStepFailedNoMatch, false},
		{StatusStepFailedExtraction, false},
		{StatusPeerClosed, false},
		{StatusConnectionError, true},
		{StatusIterationTimeout, true},
	}
	for _, c := range cases {
		t.Run(string(c.s), func(t *testing.T) {
			require.Equal(t, c.expected, c.s.CountsTowardErrorRate())
		})
	}
}
