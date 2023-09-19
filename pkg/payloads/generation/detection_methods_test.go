package generation

import (
	"testing"
	"time"
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
