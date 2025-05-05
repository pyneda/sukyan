package generation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCheckIfResultDurationIsHigher(t *testing.T) {
	tests := []struct {
		sleep          string
		resultDuration time.Duration
		expected       bool
	}{
		{"1000", time.Duration(1500) * time.Millisecond, true},
		{"1000", time.Duration(900) * time.Millisecond, false},
		{"1", time.Duration(2) * time.Second, true},
		{"1", time.Duration(500) * time.Millisecond, false},
		{"invalid", time.Duration(500) * time.Millisecond, false}, // Testing invalid sleep string
	}

	for _, test := range tests {
		t.Run("Testing sleep: "+test.sleep, func(t *testing.T) {
			method := &TimeBasedDetectionMethod{Sleep: test.sleep}
			result := method.CheckIfResultDurationIsHigher(test.resultDuration)
			if result != test.expected {
				t.Errorf("For sleep %s and result duration %s, expected %v but got %v",
					test.sleep, test.resultDuration, test.expected, result)
			}
		})
	}
}

func TestTimeBasedDetectionMethod_ParseSleepDuration_DirectCases(t *testing.T) {
	method := &TimeBasedDetectionMethod{}

	// Test standard duration formats
	assert.Equal(t, 5*time.Second, method.ParseSleepDuration("5s"))
	assert.Equal(t, 100*time.Millisecond, method.ParseSleepDuration("100ms"))
	assert.Equal(t, 2*time.Minute, method.ParseSleepDuration("2m"))
	assert.Equal(t, 2*time.Hour, method.ParseSleepDuration("2h"))

	// Test integer conversion logic
	assert.Equal(t, 5*time.Second, method.ParseSleepDuration("5"))
	assert.Equal(t, 999*time.Second, method.ParseSleepDuration("999"))
	assert.Equal(t, 1000*time.Millisecond, method.ParseSleepDuration("1000"))
	assert.Equal(t, 5000*time.Millisecond, method.ParseSleepDuration("5000"))

	// Test error handling (should return 0)
	assert.Equal(t, time.Duration(0), method.ParseSleepDuration("invalid"))
	assert.Equal(t, time.Duration(0), method.ParseSleepDuration(""))
}
