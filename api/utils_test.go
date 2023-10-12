package api

import (
	"reflect"
	"testing"
)

func TestStringToUintSlice(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		acceptedValues []uint
		silentFail     bool
		expectedOutput []uint
		expectError    bool
	}{
		{"EmptyInput", "", nil, false, []uint{}, false},
		{"BasicConversion", "1,2,3", nil, false, []uint{1, 2, 3}, false},
		{"InvalidValue", "1,2,3", []uint{1, 2}, false, nil, true},
		{"InvalidValueSilentFail", "1,2,3", []uint{1, 2}, true, []uint{1, 2}, false},
		{"BadConversion", "1,2,a", nil, false, nil, true},
		{"BadConversionSilentFail", "1,2,a", nil, true, []uint{1, 2}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := stringToUintSlice(tt.input, tt.acceptedValues, tt.silentFail)
			if (err != nil) != tt.expectError {
				t.Errorf("expected error, got %v", err)
			}
			if !reflect.DeepEqual(output, tt.expectedOutput) {
				t.Errorf("expected %v, got %v", tt.expectedOutput, output)
			}
		})
	}
}

func TestStringToIntSlice(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		acceptedValues []int
		silentFail     bool
		expectedOutput []int
		expectError    bool
	}{
		{"EmptyInput", "", nil, false, []int{}, false},
		{"BasicConversion", "1,2,-3", nil, false, []int{1, 2, -3}, false},
		{"InvalidValue", "1,2,3", []int{1, 2}, false, nil, true},
		{"InvalidValueSilentFail", "1,2,3", []int{1, 2}, true, []int{1, 2}, false},
		{"BadConversion", "1,2,a", nil, false, nil, true},
		{"BadConversionSilentFail", "1,2,a", nil, true, []int{1, 2}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := stringToIntSlice(tt.input, tt.acceptedValues, tt.silentFail)
			if (err != nil) != tt.expectError {
				t.Errorf("expected error, got %v", err)
			}
			if !reflect.DeepEqual(output, tt.expectedOutput) {
				t.Errorf("expected %v, got %v", tt.expectedOutput, output)
			}
		})
	}
}

func TestStringToSlice(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		acceptedValues []string
		silentFail     bool
		expectedOutput []string
		expectError    bool
	}{
		{"EmptyInput", "", nil, false, []string{}, false},
		{"BasicConversion", "a,b,c", nil, false, []string{"a", "b", "c"}, false},
		{"InvalidValue", "a,b,c", []string{"a", "b"}, false, nil, true},
		{"InvalidValueSilentFail", "a,b,c", []string{"a", "b"}, true, []string{"a", "b"}, false},
		{"BadConversion", "a,b,", nil, false, nil, true},
		{"BadConversionSilentFail", "a,b,", nil, true, []string{"a", "b"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := stringToSlice(tt.input, tt.acceptedValues, tt.silentFail)
			if (err != nil) != tt.expectError {
				t.Errorf("expected error, got %v", err)
			}
			if !reflect.DeepEqual(output, tt.expectedOutput) {
				t.Errorf("expected %v, got %v", tt.expectedOutput, output)
			}
		})
	}
}
