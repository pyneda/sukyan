package lib

import (
	"testing"
)

func TestBase64Encode(t *testing.T) {
	if Base64Encode("test") != "dGVzdA==" {
		t.Error()
	}
}

func TestDecodeBase36(t *testing.T) {
	tests := []struct {
		input  string
		output int64
		err    bool
	}{
		{"0", 0, false},
		{"a", 10, false},
		{"z", 35, false},
		{"10", 36, false},
		{"zz", 1295, false},
		{"hello", 29234652, false},
		{"!", 0, true},
		{"", 0, false},
		{"zzzzzzzzzzzzzz", 0, true}, // Very long valid string, but causes overflow
		{"zzzzzzzzzzzzz!", 0, true}, // Very long invalid string
		{"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", 0, true}, // Extremely long valid string, but causes overflow
		{"1a2b3c4d", 100271551261, false},
		{"zzzzz1", 2176782301, false},
		{"zzzzz!", 0, true}}

	for _, test := range tests {
		result, err := DecodeBase36(test.input)
		if err != nil && !test.err {
			t.Errorf("Expected no error for input %s, got %s", test.input, err)
		} else if err == nil && test.err {
			t.Errorf("Expected an error for input %s, got none", test.input)
		} else if result != test.output {
			t.Errorf("For input %s, expected %d but got %d", test.input, test.output, result)
		}
	}
}
