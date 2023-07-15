package lib

import (
	"reflect"
	"sort"
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

func TestGetUniqueItems(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		want  []string
	}{
		{
			name:  "Strings with duplicates",
			items: []string{"apple", "banana", "apple", "orange", "banana"},
			want:  []string{"apple", "banana", "orange"},
		},
		{
			name:  "Strings without duplicates",
			items: []string{"apple", "banana", "cherry"},
			want:  []string{"apple", "banana", "cherry"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetUniqueItems(tt.items)
			// Since map iteration is random, we need to sort the slices before comparing.
			sort.Strings(got)
			sort.Strings(tt.want)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetUniqueItems() = %v, want %v", got, tt.want)
			}
		})
	}
}
