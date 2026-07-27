package graphql

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

// Introspection documents reach this package from endpoints under test and from
// definitions stored earlier, neither of which is trustworthy. Whatever comes
// in, parsing must not panic, and any document built from what did parse must
// still be a syntactically valid GraphQL document -- otherwise a schema can
// reshape the requests the scanner sends.

func FuzzParseFromJSON(f *testing.F) {
	f.Add(`{"data":{"__schema":{"queryType":{"name":"Query"},"types":[{"kind":"OBJECT","name":"Query","fields":[{"name":"hello","type":{"kind":"SCALAR","name":"String"},"args":[]}]}]}}}`)
	f.Add(`{"__schema":{"queryType":{"name":"Query"},"types":[]}}`)
	f.Add(`{"data":{"__schema":{"queryType":null,"types":null}}}`)
	f.Add(`{"errors":[{"message":"nope"}]}`)
	f.Add(`{"data":{"__schema":{"queryType":{"name":"Q"},"types":[{"kind":"UNION","name":"U","possibleTypes":[{"kind":"OBJECT","name":"A"}]}]}}}`)
	f.Add(`{"data":{"__schema":{"queryType":{"name":"Q"},"types":[{"kind":"OBJECT","name":"Q","fields":[{"name":"a b","type":{"kind":"OBJECT","name":"Q"},"args":[{"name":"x","type":{"kind":"NON_NULL","ofType":{"kind":"SCALAR","name":"String"}},"defaultValue":"{{"}]}]}]}}}`)
	f.Add(`{"data":{"__schema":{"queryType":{"name":"Q"},"types":[{"kind":"INPUT_OBJECT","name":"I","inputFields":[{"name":"self","type":{"kind":"NON_NULL","ofType":{"kind":"INPUT_OBJECT","name":"I"}}}]}]}}}`)
	f.Add(`{}`)
	f.Add(``)

	f.Fuzz(func(t *testing.T, body string) {
		schema, err := NewParser().ParseFromJSON([]byte(body))
		if err != nil {
			return
		}
		if schema == nil {
			t.Fatal("ParseFromJSON returned no schema and no error")
		}

		config := DefaultGenerationConfig()
		config.FuzzingEnabled = true

		for _, endpoint := range NewGenerator(schema, config).GenerateRequests() {
			for _, req := range endpoint.Requests {
				if _, err := parser.ParseQuery(&ast.Source{Input: req.Query}); err != nil {
					t.Fatalf("generated an unparseable document: %v\n--- query ---\n%s", err, req.Query)
				}
				if _, err := req.ToHTTPRequest("http://example.com/graphql"); err != nil {
					t.Fatalf("generated a request that will not marshal: %v", err)
				}
			}
		}
	})
}

func FuzzParseLiteral(f *testing.F) {
	for _, seed := range []string{
		"10", "-1.5e3", `"text"`, `"""block"""`, "true", "null", "ENUM_VALUE",
		"[1,2,3]", "{a: 1, b: [2]}", `{a: {b: {c: 1}}}`, `"é"`, `"😀"`,
		"[", "{", `"`, `"\`, "1e", "#comment", "[[[[[[[[[[1]]]]]]]]]]",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, literal string) {
		value, err := ParseLiteral(literal)
		if err != nil {
			return
		}

		// Whatever came back has to survive the trip to the server, which is JSON.
		if _, err := jsonRoundTrip(value); err != nil {
			t.Fatalf("ParseLiteral(%q) produced a value that will not encode: %v", literal, err)
		}
	})
}

// A name written into a document must never be able to close the selection set
// it sits in and open something else.
func FuzzIsValidName(f *testing.F) {
	for _, seed := range []string{"ok", "", "a b", "a}b", "a{b", "évil", "__typename", "1a", "a(b)"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, name string) {
		if !IsValidName(name) {
			return
		}

		if strings.ContainsAny(name, " \t\n\r{}()[]:,\"'#$@!.|=") {
			t.Fatalf("IsValidName accepted %q, which can break out of a document", name)
		}

		query := "query " + name + " { " + name + " }"
		if _, err := parser.ParseQuery(&ast.Source{Input: query}); err != nil {
			t.Fatalf("IsValidName accepted %q but it does not parse as a name: %v", name, err)
		}
	})
}

func jsonRoundTrip(value interface{}) (interface{}, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	var decoded interface{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}
