package graphql

import "testing"

// TestIntrospectionSucceeded guards D6: the introspection oracle must key on the
// introspection result coming back under data (data.__schema / data.__type, or an
// aliased equivalent), not on the absence of the word "error". Schemas commonly
// contain error-named types or an `errors` field; the old "!contains(error)" oracle
// produced false negatives on exactly those (the cases marked "regression" below),
// while still needing to reject genuinely blocked introspection.
func TestIntrospectionSucceeded(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"basic schema", `{"data":{"__schema":{"types":[{"name":"Query"}]}}}`, true},
		{"full introspection", `{"data":{"__schema":{"queryType":{"name":"Query"},"types":[{"name":"Query"}],"directives":[]}}}`, true},
		{"regression: schema exposes UserError type", `{"data":{"__schema":{"types":[{"name":"UserError"},{"name":"Query"}]}}}`, true},
		{"regression: schema has errors field", `{"data":{"__schema":{"queryType":{"name":"Query"},"types":[{"name":"Mutation","fields":[{"name":"errors"}]}]}}}`, true},
		{"__type query", `{"data":{"__type":{"name":"Query","fields":[{"name":"id"}]}}}`, true},
		{"aliased introspection", `{"data":{"a":{"types":[{"name":"Query"}]},"b":{"name":"Query"}}}`, true},

		{"introspection disabled: data null + errors", `{"errors":[{"message":"GraphQL introspection is not allowed"}],"data":null}`, false},
		{"errors only, no data", `{"errors":[{"message":"Cannot query field __schema on type Query"}]}`, false},
		{"empty data object", `{"data":{}}`, false},
		{"data is a scalar string mentioning __schema types", `{"data":"__schema types name"}`, false},
		{"html error page", `<html><body>500 Internal Server Error</body></html>`, false},
		{"empty body", ``, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := introspectionSucceeded([]byte(tc.body)); got != tc.want {
				t.Errorf("introspectionSucceeded(%s) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}
