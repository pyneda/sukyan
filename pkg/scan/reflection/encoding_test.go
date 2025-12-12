package reflection

import (
	"testing"
)

func TestComputeEfficiencyFlags(t *testing.T) {
	tests := []struct {
		name         string
		efficiencies []CharacterEfficiency
		expected     EfficiencyFlags
	}{
		{
			name: "all blocked",
			efficiencies: []CharacterEfficiency{
				{Char: "<", Efficiency: 0},
				{Char: ">", Efficiency: 0},
				{Char: `"`, Efficiency: 0},
				{Char: "'", Efficiency: 0},
				{Char: "`", Efficiency: 0},
				{Char: "(", Efficiency: 0},
				{Char: ")", Efficiency: 0},
				{Char: "/", Efficiency: 0},
				{Char: "=", Efficiency: 0},
				{Char: ";", Efficiency: 0},
				{Char: `\`, Efficiency: 0},
			},
			expected: EfficiencyFlags{
				CanInjectTags:       false,
				CanBreakDoubleQuote: false,
				CanBreakSingleQuote: false,
				CanUseBackticks:     false,
				CanCallFunctions:    false,
				CanUseSlash:         false,
				CanUseEquals:        false,
				CanUseSemicolon:     false,
				CanEscape:           false,
			},
		},
		{
			name: "all passed",
			efficiencies: []CharacterEfficiency{
				{Char: "<", Efficiency: 100},
				{Char: ">", Efficiency: 100},
				{Char: `"`, Efficiency: 100},
				{Char: "'", Efficiency: 100},
				{Char: "`", Efficiency: 100},
				{Char: "(", Efficiency: 100},
				{Char: ")", Efficiency: 100},
				{Char: "/", Efficiency: 100},
				{Char: "=", Efficiency: 100},
				{Char: ";", Efficiency: 100},
				{Char: `\`, Efficiency: 100},
			},
			expected: EfficiencyFlags{
				CanInjectTags:       true,
				CanBreakDoubleQuote: true,
				CanBreakSingleQuote: true,
				CanUseBackticks:     true,
				CanCallFunctions:    true,
				CanUseSlash:         true,
				CanUseEquals:        true,
				CanUseSemicolon:     true,
				CanEscape:           true,
			},
		},
		{
			name: "tags blocked quotes passed",
			efficiencies: []CharacterEfficiency{
				{Char: "<", Efficiency: 30}, // HTML encoded
				{Char: ">", Efficiency: 30}, // HTML encoded
				{Char: `"`, Efficiency: 100},
				{Char: "'", Efficiency: 100},
				{Char: "`", Efficiency: 100},
				{Char: "(", Efficiency: 100},
				{Char: ")", Efficiency: 100},
				{Char: "/", Efficiency: 100},
				{Char: "=", Efficiency: 100},
				{Char: ";", Efficiency: 100},
				{Char: `\`, Efficiency: 100},
			},
			expected: EfficiencyFlags{
				CanInjectTags:       false,
				CanBreakDoubleQuote: true,
				CanBreakSingleQuote: true,
				CanUseBackticks:     true,
				CanCallFunctions:    true,
				CanUseSlash:         true,
				CanUseEquals:        true,
				CanUseSemicolon:     true,
				CanEscape:           true,
			},
		},
		{
			name: "only one angle bracket passed",
			efficiencies: []CharacterEfficiency{
				{Char: "<", Efficiency: 100},
				{Char: ">", Efficiency: 0}, // blocked
				{Char: `"`, Efficiency: 100},
				{Char: "'", Efficiency: 100},
			},
			expected: EfficiencyFlags{
				CanInjectTags:       false, // Need both < and >
				CanBreakDoubleQuote: true,
				CanBreakSingleQuote: true,
			},
		},
		{
			name: "parentheses partially blocked",
			efficiencies: []CharacterEfficiency{
				{Char: "(", Efficiency: 100},
				{Char: ")", Efficiency: 0}, // blocked
			},
			expected: EfficiencyFlags{
				CanCallFunctions: false, // Need both ( and )
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputeEfficiencyFlags(tt.efficiencies)

			if result.CanInjectTags != tt.expected.CanInjectTags {
				t.Errorf("CanInjectTags = %v, want %v", result.CanInjectTags, tt.expected.CanInjectTags)
			}
			if result.CanBreakDoubleQuote != tt.expected.CanBreakDoubleQuote {
				t.Errorf("CanBreakDoubleQuote = %v, want %v", result.CanBreakDoubleQuote, tt.expected.CanBreakDoubleQuote)
			}
			if result.CanBreakSingleQuote != tt.expected.CanBreakSingleQuote {
				t.Errorf("CanBreakSingleQuote = %v, want %v", result.CanBreakSingleQuote, tt.expected.CanBreakSingleQuote)
			}
			if result.CanUseBackticks != tt.expected.CanUseBackticks {
				t.Errorf("CanUseBackticks = %v, want %v", result.CanUseBackticks, tt.expected.CanUseBackticks)
			}
			if result.CanCallFunctions != tt.expected.CanCallFunctions {
				t.Errorf("CanCallFunctions = %v, want %v", result.CanCallFunctions, tt.expected.CanCallFunctions)
			}
			if result.CanUseSlash != tt.expected.CanUseSlash {
				t.Errorf("CanUseSlash = %v, want %v", result.CanUseSlash, tt.expected.CanUseSlash)
			}
			if result.CanUseEquals != tt.expected.CanUseEquals {
				t.Errorf("CanUseEquals = %v, want %v", result.CanUseEquals, tt.expected.CanUseEquals)
			}
			if result.CanUseSemicolon != tt.expected.CanUseSemicolon {
				t.Errorf("CanUseSemicolon = %v, want %v", result.CanUseSemicolon, tt.expected.CanUseSemicolon)
			}
			if result.CanEscape != tt.expected.CanEscape {
				t.Errorf("CanEscape = %v, want %v", result.CanEscape, tt.expected.CanEscape)
			}
		})
	}
}

func TestGetEfficiencyForChar(t *testing.T) {
	efficiencies := []CharacterEfficiency{
		{Char: "<", Efficiency: 100},
		{Char: ">", Efficiency: 30},
		{Char: `"`, Efficiency: 0},
	}

	tests := []struct {
		char     string
		expected int
	}{
		{"<", 100},
		{">", 30},
		{`"`, 0},
		{"'", 0}, // Not in list
		{"x", 0}, // Not in list
	}

	for _, tt := range tests {
		t.Run(tt.char, func(t *testing.T) {
			result := GetEfficiencyForChar(efficiencies, tt.char)
			if result != tt.expected {
				t.Errorf("GetEfficiencyForChar(%q) = %d, want %d", tt.char, result, tt.expected)
			}
		})
	}
}

func TestGetHTMLEncodedEfficiency(t *testing.T) {
	tests := []struct {
		char     string
		expected int
	}{
		{"<", 30},
		{">", 30},
		{`"`, 40},
		{"'", 40},
		{"x", 20},
		{"&", 20},
	}

	for _, tt := range tests {
		t.Run(tt.char, func(t *testing.T) {
			result := getHTMLEncodedEfficiency(tt.char)
			if result != tt.expected {
				t.Errorf("getHTMLEncodedEfficiency(%q) = %d, want %d", tt.char, result, tt.expected)
			}
		})
	}
}

func TestGetNumericHTMLEntity(t *testing.T) {
	tests := []struct {
		char     string
		expected string
	}{
		{"<", "&#60;"},
		{">", "&#62;"},
		{`"`, "&#34;"},
		{"'", "&#39;"},
		{"", ""},
		{"ab", ""}, // Multi-char returns empty
	}

	for _, tt := range tests {
		t.Run(tt.char, func(t *testing.T) {
			result := getNumericHTMLEntity(tt.char)
			if result != tt.expected {
				t.Errorf("getNumericHTMLEntity(%q) = %q, want %q", tt.char, result, tt.expected)
			}
		})
	}
}

func TestTestCharacters(t *testing.T) {
	// Ensure all critical characters are in the test set
	required := []string{"<", ">", `"`, "'", "`", "(", ")", "/", "=", ";", `\`}

	for _, char := range required {
		found := false
		for _, testChar := range TestCharacters {
			if testChar == char {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Character %q not found in TestCharacters", char)
		}
	}
}
