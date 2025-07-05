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

func TestSanitizeUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Valid UTF-8 string",
			input:    "Hello, 世界",
			expected: "Hello, 世界",
		},
		{
			name:     "ASCII only",
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Invalid UTF-8 with 0x80 byte",
			input:    "Hello\x80World",
			expected: "Hello\uFFFDWorld",
		},
		{
			name:     "Invalid UTF-8 with incomplete sequence",
			input:    "Hello\xC0\x80World",
			expected: "Hello\uFFFD\uFFFDWorld",
		},
		{
			name:     "Multiple invalid sequences",
			input:    "Test\x80\xFF\xFEString",
			expected: "Test\uFFFD\uFFFD\uFFFDString",
		},
		{
			name:     "Mixed valid and invalid UTF-8",
			input:    "Hello\x80世界\xFFTest",
			expected: "Hello\uFFFD世界\uFFFDTest",
		},
		{
			name:     "String with null bytes",
			input:    "Hello\x00World\x00Test",
			expected: "HelloWorldTest",
		},
		{
			name:     "String with only null bytes",
			input:    "\x00\x00\x00",
			expected: "",
		},
		{
			name:     "Mixed null bytes and invalid UTF-8",
			input:    "Test\x00\x80\x00String",
			expected: "Test\uFFFDString",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := SanitizeUTF8(test.input)
			if result != test.expected {
				t.Errorf("Expected %q, got %q", test.expected, result)
			}
		})
	}
}
