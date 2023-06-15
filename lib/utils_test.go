package lib

import (
	"net/url"
	"reflect"
	"testing"
)

func TestGenerateRandomString(t *testing.T) {
	r1 := GenerateRandomString(20)
	if len(r1) != 20 {
		t.Error()
	}
	r2 := GenerateRandomString(50)
	if len(r2) != 50 {
		t.Error()
	}
	r3 := GenerateRandomString(5000)
	if len(r3) != 5000 {
		t.Error()
	}
}


func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{
			name:     "Item is in the slice",
			slice:    []string{"apple", "banana", "cherry"},
			item:     "banana",
			expected: true,
		},
		{
			name:     "Item is not in the slice",
			slice:    []string{"apple", "banana", "cherry"},
			item:     "grape",
			expected: false,
		},
		{
			name:     "Slice is empty",
			slice:    []string{},
			item:     "apple",
			expected: false,
		},
		{
			name:     "Item is empty",
			slice:    []string{"apple", "banana", "cherry"},
			item:     "",
			expected: false,
		},
		{
			name:     "Item and slice are empty",
			slice:    []string{},
			item:     "",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := Contains(tc.slice, tc.item)
			if result != tc.expected {
				t.Errorf("expected %v, but got %v", tc.expected, result)
			}
		})
	}
}