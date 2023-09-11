package lib

import (
	"testing"
)

func TestGuessDataType(t *testing.T) {
	tests := []struct {
		input    string
		expected DataType
	}{
		{"42", TypeInt},
		{"0", TypeInt},
		{"-1", TypeInt},
		{"3.14", TypeFloat},
		{"0.0", TypeFloat},
		{"-0.99", TypeFloat},
		{`{"key": "value"}`, TypeJSON},
		{`{"arr": [1, 2, 3]}`, TypeJSON},
		{`{"bool": true}`, TypeJSON},
		{`<xml></xml>`, TypeXML},
		{`<note><to>Tove</to></note>`, TypeXML},
		{`<a><b></b></a>`, TypeHTML},
		{"<svg></svg>", TypeSVG},
		{"2023-09-11", TypeDate1},
		{"1999-12-31", TypeDate1},
		{"2000-01-01", TypeDate1},
		{"09/11/2023", TypeDate2},
		{"12/31/1999", TypeDate2},
		{"01/01/2000", TypeDate2},
		{"apple,banana,cherry", TypeArray},
		{"1,2,3", TypeArray},
		{"true,false,true", TypeArray},
		{"true", TypeBoolean},
		{"false", TypeBoolean},
		{"True", TypeBoolean}, // Case insensitive
		{"john.doe@example.com", TypeEmail},
		{"jane.doe@example.co.uk", TypeEmail},
		{"email@example.com", TypeEmail},
		{"https://www.example.com", TypeURL},
		{"http://www.example.com", TypeURL},
		{"ftp://files.example.com", TypeURL},
		{"SGVsbG8gd29ybGQ=", TypeBase64},
		{"aGVsbG8=", TypeBase64},
		{"U28gbG9uZyBhbmQgdGhhbmtzIGZvciBhbGwgdGhlIGZpc2gu", TypeBase64},
		{"550e8400-e29b-41d4-a716-446655440000", TypeUUID},
		{"123e4567-e89b-12d3-a456-426614174000", TypeUUID},
		{"6ba7b810-9dad-11d1-80b4-00c04fd430c8", TypeUUID},
		{"1f", TypeHex},
		{"a1", TypeHex},
		{"beef", TypeHex},
		{"<html></html>", TypeHTML},
		{"<body></body>", TypeHTML},
		{"<p>Hello, world!</p>", TypeHTML},
		{"function test() { return 42; }", TypeJSCode},
		{"var x = 10;", TypeJSCode},
		{"if (true) { console.log('true'); }", TypeJSCode},
	}

	for _, test := range tests {
		got := GuessDataType(test.input)
		if got != test.expected {
			t.Errorf("For value: %q, expected: %q, but got: %q", test.input, test.expected, got)
		}
	}
}
