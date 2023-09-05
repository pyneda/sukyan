package lib

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestParseHeadersStringToMap(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string][]string
	}{
		{
			name:  "Standard headers",
			input: "Authorization:Bearer XYZ,User-Agent:MyApp",
			expected: map[string][]string{
				"Authorization": {"Bearer XYZ"},
				"User-Agent":    {"MyApp"},
			},
		},
		{
			name:  "Multiple values for a single key",
			input: "Authorization:Bearer XYZ,User-Agent:MyApp,Authorization:AnotherToken",
			expected: map[string][]string{
				"Authorization": {"Bearer XYZ", "AnotherToken"},
				"User-Agent":    {"MyApp"},
			},
		},
		{
			name:  "Key with empty value",
			input: "KeyWithoutValue:",
			expected: map[string][]string{
				"KeyWithoutValue": {""},
			},
		},
		{
			name:     "Empty string",
			input:    "",
			expected: map[string][]string{},
		},
		{
			name:  "Leading and trailing spaces",
			input: " Authorization:Bearer XYZ , User-Agent:MyApp ",
			expected: map[string][]string{
				"Authorization": {"Bearer XYZ"},
				"User-Agent":    {"MyApp"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := ParseHeadersStringToMap(tt.input)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
