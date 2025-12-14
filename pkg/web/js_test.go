package web

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscapeJSString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple string",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "single quotes",
			input:    "it's a test",
			expected: `it\'s a test`,
		},
		{
			name:     "double quotes",
			input:    `say "hello"`,
			expected: `say \"hello\"`,
		},
		{
			name:     "backslash",
			input:    `path\to\file`,
			expected: `path\\to\\file`,
		},
		{
			name:     "newline",
			input:    "line1\nline2",
			expected: `line1\nline2`,
		},
		{
			name:     "carriage return",
			input:    "line1\rline2",
			expected: `line1\rline2`,
		},
		{
			name:     "tab",
			input:    "col1\tcol2",
			expected: `col1\tcol2`,
		},
		{
			name:     "null byte",
			input:    "before\x00after",
			expected: `before\x00after`,
		},
		{
			name:     "backtick",
			input:    "`template`",
			expected: "\\`template\\`",
		},
		{
			name:     "template interpolation",
			input:    "${variable}",
			expected: `\${variable}`,
		},
		{
			name:     "line separator U+2028",
			input:    "before\u2028after",
			expected: `before\u2028after`,
		},
		{
			name:     "paragraph separator U+2029",
			input:    "before\u2029after",
			expected: `before\u2029after`,
		},
		{
			name:     "XSS payload",
			input:    `<img src=x onerror=alert('xss')>`,
			expected: `<img src=x onerror=alert(\'xss\')>`,
		},
		{
			name:     "complex payload",
			input:    "');alert('xss');//",
			expected: `\');alert(\'xss\');//`,
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EscapeJSString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
