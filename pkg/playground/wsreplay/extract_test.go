package wsreplay

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtraction_FullText(t *testing.T) {
	ext := Extraction{Name: "v", Method: ExtractMethodFull}
	got, ok := ext.Apply(Frame{Opcode: 1, Content: `{"a":1}`})
	require.True(t, ok)
	require.Equal(t, `{"a":1}`, got)
}

func TestExtraction_FullBinaryBase64Encoded(t *testing.T) {
	// Binary frames must be base64-encoded so the variable stays stringy and
	// safe to splice into a later text frame via ${name}.
	ext := Extraction{Name: "v", Method: ExtractMethodFull}
	raw := "\x00\x01\xff\xfe"
	got, ok := ext.Apply(Frame{Opcode: 2, Content: raw})
	require.True(t, ok)
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte(raw)), got)
}

func TestExtraction_JSONPath_StringValue(t *testing.T) {
	ext := Extraction{Name: "token", Method: ExtractMethodJSONPath, Group: "$.token"}
	got, ok := ext.Apply(Frame{Opcode: 1, Content: `{"ok":true,"token":"abc.def"}`})
	require.True(t, ok)
	require.Equal(t, "abc.def", got)
}

func TestExtraction_JSONPath_FailsOnBinaryFrame(t *testing.T) {
	// JSON-path against a binary frame is almost certainly a config mistake;
	// fail explicitly so the run aborts instead of capturing "".
	ext := Extraction{Name: "v", Method: ExtractMethodJSONPath, Group: "$.x"}
	_, ok := ext.Apply(Frame{Opcode: 2, Content: "binary garbage"})
	require.False(t, ok)
}

func TestExtraction_JSONPath_ReturnsFalseOnInvalidJSON(t *testing.T) {
	ext := Extraction{Name: "v", Method: ExtractMethodJSONPath, Group: "$.x"}
	_, ok := ext.Apply(Frame{Opcode: 1, Content: "not json"})
	require.False(t, ok)
}

func TestExtraction_JSONPath_ReturnsFalseOnMissingPath(t *testing.T) {
	ext := Extraction{Name: "v", Method: ExtractMethodJSONPath, Group: "$.absent"}
	_, ok := ext.Apply(Frame{Opcode: 1, Content: `{"present":1}`})
	require.False(t, ok)
}

func TestExtraction_RegexGroup_DefaultsToGroup1(t *testing.T) {
	ext := Extraction{Name: "v", Method: ExtractMethodRegexGroup, Pattern: `token=([A-Za-z0-9]+)`}
	got, ok := ext.Apply(Frame{Opcode: 1, Content: "session_token=ABC123;path=/"})
	require.True(t, ok)
	require.Equal(t, "ABC123", got)
}

func TestExtraction_RegexGroup_HonorsExplicitGroupIndex(t *testing.T) {
	ext := Extraction{
		Name:    "v",
		Method:  ExtractMethodRegexGroup,
		Pattern: `(user=)([^&]+)`,
		Group:   "2",
	}
	got, ok := ext.Apply(Frame{Opcode: 1, Content: "user=admin&x=1"})
	require.True(t, ok)
	require.Equal(t, "admin", got)
}

func TestExtraction_RegexGroup_FailsOnInvalidPattern(t *testing.T) {
	ext := Extraction{Name: "v", Method: ExtractMethodRegexGroup, Pattern: "(unclosed"}
	_, ok := ext.Apply(Frame{Opcode: 1, Content: "anything"})
	require.False(t, ok)
}

func TestExtraction_RegexGroup_FailsWhenGroupIndexOutOfRange(t *testing.T) {
	ext := Extraction{Name: "v", Method: ExtractMethodRegexGroup, Pattern: `(a)`, Group: "5"}
	_, ok := ext.Apply(Frame{Opcode: 1, Content: "aa"})
	require.False(t, ok)
}

func TestExtraction_UnknownMethodFails(t *testing.T) {
	ext := Extraction{Name: "v", Method: "header"} // header isn't valid for ws frames
	_, ok := ext.Apply(Frame{Opcode: 1, Content: "x"})
	require.False(t, ok)
}
