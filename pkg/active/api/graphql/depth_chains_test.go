package graphql

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	pkgGraphql "github.com/pyneda/sukyan/pkg/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// denseSchemaJSON builds an introspection document whose object types all
// reference one another, plus a root type exposed as a field. Real APIs are
// cross-referenced like this, and enumerating every simple path through such a
// graph is factorial.
func denseSchemaJSON(n int) []byte {
	var types []map[string]any
	var queryFields []map[string]any

	for i := 0; i < n; i++ {
		var fields []map[string]any
		for j := 0; j < n; j++ {
			if i != j {
				fields = append(fields, map[string]any{
					"name": fmt.Sprintf("to%d", j),
					"type": map[string]any{"kind": "OBJECT", "name": fmt.Sprintf("T%d", j)},
					"args": []any{},
				})
			}
		}
		fields = append(fields, map[string]any{
			"name": "id",
			"type": map[string]any{"kind": "SCALAR", "name": "ID"},
			"args": []any{},
		})

		types = append(types, map[string]any{"kind": "OBJECT", "name": fmt.Sprintf("T%d", i), "fields": fields})
		queryFields = append(queryFields, map[string]any{
			"name": fmt.Sprintf("get%d", i),
			"type": map[string]any{"kind": "OBJECT", "name": fmt.Sprintf("T%d", i)},
			"args": []any{},
		})
	}

	queryFields = append(queryFields, map[string]any{
		"name": "viewer",
		"type": map[string]any{"kind": "OBJECT", "name": "Query"},
		"args": []any{},
	})
	types = append(types, map[string]any{"kind": "OBJECT", "name": "Query", "fields": queryFields})

	body, _ := json.Marshal(map[string]any{"data": map[string]any{"__schema": map[string]any{
		"queryType": map[string]string{"name": "Query"},
		"types":     types,
	}}})
	return body
}

// Chain discovery has to terminate on a schema shaped like a real API's. Only the
// first few chains are ever turned into requests, so exploring the rest is work
// that buys nothing and, unbounded, never finishes.
func TestFindDeepTypeChainsTerminatesOnDenseSchemas(t *testing.T) {
	for _, n := range []int{8, 12, 20} {
		t.Run(fmt.Sprintf("types=%d", n), func(t *testing.T) {
			schema, err := pkgGraphql.NewParser().ParseFromJSON(denseSchemaJSON(n))
			require.NoError(t, err)

			done := make(chan []typeChain, 1)
			go func() { done <- findDeepTypeChains(schema) }()

			select {
			case chains := <-done:
				assert.LessOrEqual(t, len(chains), maxTypeChains)
				require.NotEmpty(t, chains)

				// A cyclic chain nests to any depth the audit asks for, so losing
				// them to the bound would weaken the test it produces.
				assert.True(t, chains[0].cyclic, "the best chain should still be a cyclic one")
				for _, chain := range chains {
					assert.LessOrEqual(t, len(chain.steps), maxTypeChainDepth)
				}
			case <-time.After(20 * time.Second):
				t.Fatal("findDeepTypeChains did not terminate")
			}
		})
	}
}

func TestSchemaAwareDepthTestCasesOnDenseSchema(t *testing.T) {
	schema, err := pkgGraphql.NewParser().ParseFromJSON(denseSchemaJSON(12))
	require.NoError(t, err)

	cases := getSchemaAwareDepthTestCases(schema)
	require.NotEmpty(t, cases)

	for _, tc := range cases {
		assert.NotEmpty(t, tc.query)
		assert.Equal(t, strings.Count(tc.query, "{"), strings.Count(tc.query, "}"),
			"unbalanced braces in %s: %s", tc.name, tc.query)
	}
}

// Interfaces and unions are composite types the audit can now walk into. A union
// declares no fields of its own, so anything selecting from one must fall back to
// __typename rather than emitting an empty selection set.
func TestDepthAuditHandlesAbstractTypes(t *testing.T) {
	body := []byte(`{"data":{"__schema":{
	  "queryType":{"name":"Query"},
	  "types":[
	    {"kind":"INTERFACE","name":"Node","fields":[
	      {"name":"id","type":{"kind":"NON_NULL","ofType":{"kind":"SCALAR","name":"ID"}},"args":[]},
	      {"name":"owner","type":{"kind":"OBJECT","name":"User"},"args":[]}],
	     "possibleTypes":[{"kind":"OBJECT","name":"User"}]},
	    {"kind":"UNION","name":"Any","possibleTypes":[{"kind":"OBJECT","name":"User"}]},
	    {"kind":"OBJECT","name":"User","fields":[
	      {"name":"id","type":{"kind":"SCALAR","name":"ID"},"args":[]},
	      {"name":"node","type":{"kind":"INTERFACE","name":"Node"},"args":[]},
	      {"name":"any","type":{"kind":"UNION","name":"Any"},"args":[]}]},
	    {"kind":"OBJECT","name":"Query","fields":[
	      {"name":"me","type":{"kind":"OBJECT","name":"User"},"args":[]}]}
	  ]}}}`)

	schema, err := pkgGraphql.NewParser().ParseFromJSON(body)
	require.NoError(t, err)

	assert.Equal(t, "__typename", findScalarField(schema, "Any"),
		"a union has no fields of its own to select")
	assert.Equal(t, "id", findScalarField(schema, "Node"),
		"an interface's own fields are selectable")

	for _, tc := range getSchemaAwareDepthTestCases(schema) {
		assert.Equal(t, strings.Count(tc.query, "{"), strings.Count(tc.query, "}"),
			"unbalanced braces in %s: %s", tc.name, tc.query)
	}
}
