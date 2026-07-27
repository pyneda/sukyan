package graphql

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLiteral(t *testing.T) {
	tests := []struct {
		name    string
		literal string
		want    interface{}
	}{
		{"int", "10", int64(10)},
		{"negative int", "-42", int64(-42)},
		{"zero", "0", int64(0)},
		{"float", "1.5", 1.5},
		{"negative float", "-0.25", -0.25},
		{"exponent", "1e3", float64(1000)},
		{"negative exponent", "1.5e-3", 0.0015},
		{"true", "true", true},
		{"false", "false", false},
		{"null", "null", nil},
		{"string", `"hello"`, "hello"},
		{"empty string", `""`, ""},
		{"string that looks like a number", `"10"`, "10"},
		{"string with escapes", `"a\tb\nc\"d\\e"`, "a\tb\nc\"d\\e"},
		{"string with unicode escape", `"é"`, "é"},
		{"string with surrogate pair", `"😀"`, "😀"},
		{"enum", "ACTIVE", "ACTIVE"},
		{"enum that starts with underscore", "_INTERNAL", "_INTERNAL"},
		{"empty list", "[]", []interface{}{}},
		{"int list", "[1, 2, 3]", []interface{}{int64(1), int64(2), int64(3)}},
		{"list without spaces", "[1,2]", []interface{}{int64(1), int64(2)}},
		{"nested list", "[[1], [2]]", []interface{}{[]interface{}{int64(1)}, []interface{}{int64(2)}}},
		{"mixed list", `["a", 1, true, null]`, []interface{}{"a", int64(1), true, nil}},
		{"empty object", "{}", map[string]interface{}{}},
		{"object", `{name: "bob", age: 3}`, map[string]interface{}{"name": "bob", "age": int64(3)}},
		{"object without spaces", `{name:"bob"}`, map[string]interface{}{"name": "bob"}},
		{"nested object", `{a: {b: 1}}`, map[string]interface{}{"a": map[string]interface{}{"b": int64(1)}}},
		{"object holding a list", `{tags: ["x"]}`, map[string]interface{}{"tags": []interface{}{"x"}}},
		{"object with enum value", `{status: ACTIVE}`, map[string]interface{}{"status": "ACTIVE"}},
		{"leading whitespace", "  7 ", int64(7)},
		{"comma separated object fields", `{a: 1, b: 2}`, map[string]interface{}{"a": int64(1), "b": int64(2)}},
		{"block string", `"""hello"""`, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLiteral(tt.literal)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// A default has to survive the trip to the server, which is JSON, so the decoded
// value matters only insofar as it encodes to the right JSON.
func TestParseLiteralEncodesToExpectedJSON(t *testing.T) {
	tests := []struct {
		literal string
		want    string
	}{
		{"10", "10"},
		{"1.5", "1.5"},
		{`"10"`, `"10"`},
		{"true", "true"},
		{"null", "null"},
		{"ACTIVE", `"ACTIVE"`},
		{"[1, 2]", "[1,2]"},
		{`{a: 1}`, `{"a":1}`},
	}

	for _, tt := range tests {
		t.Run(tt.literal, func(t *testing.T) {
			value, err := ParseLiteral(tt.literal)
			require.NoError(t, err)

			encoded, err := json.Marshal(value)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(encoded))
		})
	}
}

func TestParseLiteralBlockStringSemantics(t *testing.T) {
	literal := "\"\"\"\n    line one\n      indented\n    line two\n    \"\"\""

	got, err := ParseLiteral(literal)
	require.NoError(t, err)
	assert.Equal(t, "line one\n  indented\nline two", got)
}

func TestParseLiteralRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name    string
		literal string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"unterminated string", `"abc`},
		{"unterminated list", "[1, 2"},
		{"unterminated object", "{a: 1"},
		{"newline inside string", "\"a\nb\""},
		{"object missing colon", `{a 1}`},
		{"object with non-name key", `{"a": 1}`},
		{"variable reference", "$id"},
		{"trailing junk", "1 2"},
		{"bare punctuation", "!"},
		{"lone minus", "-"},
		{"unknown escape", `"a\qb"`},
		{"truncated unicode escape", `"\u00"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseLiteral(tt.literal)
			assert.Error(t, err)
		})
	}
}

// A literal arrives from an untrusted endpoint, so nesting has to be refused
// rather than recursed into until the stack runs out.
func TestParseLiteralRejectsDeepNesting(t *testing.T) {
	depth := maxLiteralDepth + 10
	literal := strings.Repeat("[", depth) + "1" + strings.Repeat("]", depth)

	_, err := ParseLiteral(literal)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nested deeper")
}

func TestParseLiteralDoesNotPanicOnArbitraryInput(t *testing.T) {
	inputs := []string{
		"[", "]", "{", "}", `"`, `"""`, `\`, "\x00", "{a:", "[[[", `"\u`, `"\ud83d`,
		"1e", "1e+", "--1", "0x10", "{a:1,,}", "[,]", "#comment", "  #c\n1",
	}

	for _, input := range inputs {
		assert.NotPanics(t, func() { _, _ = ParseLiteral(input) }, "input %q", input)
	}
}

func TestIsValidName(t *testing.T) {
	valid := []string{"a", "A", "_", "_a", "user", "userName", "user_name", "a1", "__typename"}
	for _, name := range valid {
		assert.True(t, IsValidName(name), "%q should be a valid name", name)
	}

	// Anything rejected here would otherwise be written verbatim into a document
	// built from a schema the scanner does not control.
	invalid := []string{
		"", "1", "1a", "-a", "a-b", "a b", "a.b", "a{b", "a}b", "a(b", "a)b",
		"a\nb", "a$b", "user name", "...", "a\"b", "a#b", "é",
	}
	for _, name := range invalid {
		assert.False(t, IsValidName(name), "%q should be an invalid name", name)
	}
}
