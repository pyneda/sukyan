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

func TestFilterOutString(t *testing.T) {
	testCases := []struct {
		name   string
		slice  []string
		target string
		want   []string
	}{
		{
			name:   "Target present in slice",
			slice:  []string{"apple", "banana", "apple", "orange", "apple"},
			target: "apple",
			want:   []string{"banana", "orange"},
		},
		{
			name:   "Target not present in slice",
			slice:  []string{"apple", "banana", "orange"},
			target: "pear",
			want:   []string{"apple", "banana", "orange"},
		},
		{
			name:   "Empty slice",
			slice:  []string{},
			target: "apple",
			want:   []string{},
		},
		{
			name:   "All elements match target",
			slice:  []string{"apple", "apple", "apple"},
			target: "apple",
			want:   []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := FilterOutString(tc.slice, tc.target)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FilterOutString(%v, %q) = %v; want %v", tc.slice, tc.target, got, tc.want)
			}
		})
	}
}

func TestBytesCountToHumanReadable(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{1, "1 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1025, "1.0 KB"},
		{1_048_576, "1.0 MB"},
		{2_621_440, "2.5 MB"},
		{1_073_741_824, "1.0 GB"},
		{1_099_511_627_776, "1.0 TB"},
		{1_125_899_906_842_624, "1.0 PB"},
		{1_152_921_504_606_846_976, "1.0 EB"},
	}

	for _, test := range tests {
		output := BytesCountToHumanReadable(test.input)
		if output != test.expected {
			t.Errorf("For input %d expected %s but got %s", test.input, test.expected, output)
		}
	}
}

func TestSlicesIntersect(t *testing.T) {
	tests := []struct {
		slice1   []string
		slice2   []string
		expected bool
	}{
		{[]string{"a", "b", "c"}, []string{"d", "e", "f"}, false},
		{[]string{"a", "b", "c"}, []string{"b", "e", "f"}, true},
		{[]string{"a", "b", "c"}, []string{"a", "b", "c"}, true},
		{[]string{"a", "b", "c"}, []string{}, false},
		{[]string{}, []string{"a", "b", "c"}, false},
		{[]string{}, []string{}, false},
	}

	for _, test := range tests {
		result := SlicesIntersect(test.slice1, test.slice2)
		if result != test.expected {
			t.Errorf("SlicesIntersect(%v, %v) = %v; want %v", test.slice1, test.slice2, result, test.expected)
		}
	}
}
