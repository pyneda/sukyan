package scan

import (
	"fmt"
	"testing"
)

func TestIsHigherOrEqual(t *testing.T) {
	testCases := []struct {
		a        ScanMode
		b        ScanMode
		expected bool
	}{
		{ScanModeFast, ScanModeFast, true},
		{ScanModeFast, ScanModeSmart, false},
		{ScanModeFast, ScanModeFuzz, false},
		{ScanModeSmart, ScanModeFast, true},
		{ScanModeSmart, ScanModeSmart, true},
		{ScanModeSmart, ScanModeFuzz, false},
		{ScanModeFuzz, ScanModeFast, true},
		{ScanModeFuzz, ScanModeSmart, true},
		{ScanModeFuzz, ScanModeFuzz, true},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			actual := tc.a.IsHigherOrEqual(tc.b)
			if actual != tc.expected {
				t.Errorf("Test failed for a=%s, b=%s. Expected %v but got %v", tc.a, tc.b, tc.expected, actual)
			}
		})
	}
}
