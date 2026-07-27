package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The endpoint being introspected is the target of a scan, so it is untrusted in
// every respect: it can stall, answer with megabytes, answer with the wrong
// status, or describe a schema that does not hold together. None of that may
// take the scanner down or produce a request built on nonsense.

func introspectionServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func TestParseFromURL(t *testing.T) {
	body := introspectionFromSDL(t, `type Query { hello: String }`)

	var gotMethod, gotContentType, gotAuth string
	var gotBody map[string]interface{}

	server := introspectionServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		_, _ = w.Write(body)
	})

	schema, err := NewParser().WithHeaders(map[string]string{"Authorization": "Bearer token"}).ParseFromURL(server.URL)
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "application/json", gotContentType)
	assert.Equal(t, "Bearer token", gotAuth, "configured headers must reach the target")
	assert.Contains(t, gotBody["query"], "__schema")
	assert.Len(t, schema.Queries, 1)
}

// A cancelled scan must stop the request, not leave it running to the client's
// own timeout while the job that owns it is already gone.
func TestParseFromURLHonoursContextCancellation(t *testing.T) {
	released := make(chan struct{})
	server := introspectionServer(t, func(w http.ResponseWriter, r *http.Request) {
		<-released
	})
	t.Cleanup(func() { close(released) })

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	done := make(chan error, 1)
	go func() {
		_, err := NewParser().ParseFromURLContext(ctx, server.URL)
		done <- err
	}()

	select {
	case err := <-done:
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("introspection ignored the cancelled context")
	}
}

// An unbounded read against a target that streams indefinitely exhausts the
// scanner's memory, so the response is capped and an oversized one is reported.
func TestParseFromURLBoundsResponseSize(t *testing.T) {
	server := introspectionServer(t, func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 1024; i++ {
			_, _ = w.Write([]byte(strings.Repeat("A", 1024)))
		}
	})

	_, err := NewParser().WithMaxResponseSize(4096).ParseFromURL(server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds 4096 bytes")
}

// Servers routinely answer introspection with a non-2xx status while still
// returning a usable schema, so the body decides and the status only shapes the
// error when the body has nothing in it.
func TestParseFromURLAcceptsSchemaOnNon200(t *testing.T) {
	body := introspectionFromSDL(t, `type Query { hello: String }`)

	server := introspectionServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(body)
	})

	schema, err := NewParser().ParseFromURL(server.URL)
	require.NoError(t, err)
	assert.Len(t, schema.Queries, 1)
}

func TestParseFromURLErrors(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantMessage string
	}{
		{
			name:        "introspection disabled",
			status:      http.StatusOK,
			body:        `{"errors":[{"message":"GraphQL introspection is not allowed"}]}`,
			wantMessage: "GraphQL introspection is not allowed",
		},
		{
			name:        "html error page",
			status:      http.StatusForbidden,
			body:        "<html><body>Forbidden</body></html>",
			wantMessage: "unexpected status 403",
		},
		{
			name:        "not json",
			status:      http.StatusOK,
			body:        "not json at all",
			wantMessage: "failed to parse introspection response",
		},
		{
			name:        "json without a schema",
			status:      http.StatusOK,
			body:        `{"data":{"something":1}}`,
			wantMessage: "schema is nil",
		},
		{
			name:        "empty body",
			status:      http.StatusOK,
			body:        "",
			wantMessage: "failed to parse introspection response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := introspectionServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})

			_, err := NewParser().ParseFromURL(server.URL)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMessage)
		})
	}
}

// The failing body is quoted into an error that gets logged and stored, so it
// cannot carry the target's whole response along with it.
func TestParseFromURLTruncatesErrorBody(t *testing.T) {
	server := introspectionServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(strings.Repeat("X", 10000)))
	})

	_, err := NewParser().ParseFromURL(server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "(truncated)")
	assert.Less(t, len(err.Error()), 1024)
}

func TestFetchIntrospectionRawReturnsBody(t *testing.T) {
	body := introspectionFromSDL(t, `type Query { hello: String }`)

	server := introspectionServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	})

	raw, err := NewParser().FetchIntrospectionRaw(server.URL)
	require.NoError(t, err)
	assert.JSONEq(t, string(body), string(raw))
}

func TestParseFromJSONAcceptsBareSchemaObject(t *testing.T) {
	full := introspectionFromSDL(t, `type Query { hello: String }`)

	var response IntrospectionResponse
	require.NoError(t, json.Unmarshal(full, &response))

	bare, err := json.Marshal(response.Data)
	require.NoError(t, err)

	schema, err := NewParser().ParseFromJSON(bare)
	require.NoError(t, err)
	assert.Len(t, schema.Queries, 1)
}

func TestParseFromJSONReportsGraphQLErrors(t *testing.T) {
	_, err := NewParser().ParseFromJSON([]byte(`{"data":null,"errors":[{"message":"introspection disabled"}]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "introspection disabled")
}

// A schema is a document the target controls. Every shape below is malformed in
// a way a real server would not produce, and none of them may panic.
func TestParseFromJSONSurvivesMalformedSchemas(t *testing.T) {
	tests := []struct {
		name   string
		schema string
		assert func(t *testing.T, schema *GraphQLSchema, err error)
	}{
		{
			name:   "no root types at all",
			schema: `{"queryType":null,"mutationType":null,"subscriptionType":null,"types":[]}`,
			assert: func(t *testing.T, schema *GraphQLSchema, err error) {
				require.NoError(t, err)
				assert.Empty(t, schema.Queries)
				assert.Empty(t, schema.Mutations)
			},
		},
		{
			name:   "query root names a type that is not defined",
			schema: `{"queryType":{"name":"Missing"},"types":[]}`,
			assert: func(t *testing.T, schema *GraphQLSchema, err error) {
				require.NoError(t, err)
				assert.Empty(t, schema.Queries)
			},
		},
		{
			name:   "mutation only, no query root",
			schema: `{"queryType":null,"mutationType":{"name":"Mutation"},"types":[{"kind":"OBJECT","name":"Mutation","fields":[{"name":"doIt","type":{"kind":"SCALAR","name":"String"},"args":[]}]}]}`,
			assert: func(t *testing.T, schema *GraphQLSchema, err error) {
				require.NoError(t, err)
				assert.Len(t, schema.Mutations, 1)
			},
		},
		{
			name:   "type with an empty name",
			schema: `{"queryType":{"name":"Query"},"types":[{"kind":"OBJECT","name":"","fields":[]},{"kind":"OBJECT","name":"Query","fields":[]}]}`,
			assert: func(t *testing.T, schema *GraphQLSchema, err error) {
				require.NoError(t, err)
				assert.NotContains(t, schema.Types, "")
			},
		},
		{
			name:   "unknown type kind",
			schema: `{"queryType":{"name":"Query"},"types":[{"kind":"SOMETHING_NEW","name":"Weird"},{"kind":"OBJECT","name":"Query","fields":[]}]}`,
			assert: func(t *testing.T, schema *GraphQLSchema, err error) {
				require.NoError(t, err)
				assert.NotContains(t, schema.Types, "Weird")
			},
		},
		{
			name:   "introspection meta types are excluded",
			schema: `{"queryType":{"name":"Query"},"types":[{"kind":"OBJECT","name":"__Type","fields":[]},{"kind":"OBJECT","name":"Query","fields":[]}]}`,
			assert: func(t *testing.T, schema *GraphQLSchema, err error) {
				require.NoError(t, err)
				assert.NotContains(t, schema.Types, "__Type")
			},
		},
		{
			name:   "field type is an empty object",
			schema: `{"queryType":{"name":"Query"},"types":[{"kind":"OBJECT","name":"Query","fields":[{"name":"broken","type":{},"args":[]}]}]}`,
			assert: func(t *testing.T, schema *GraphQLSchema, err error) {
				require.NoError(t, err)
				require.Len(t, schema.Queries, 1)
				assert.Empty(t, schema.Queries[0].ReturnType.Name)
			},
		},
		{
			name:   "null fields and args",
			schema: `{"queryType":{"name":"Query"},"types":[{"kind":"OBJECT","name":"Query","fields":null},{"kind":"INPUT_OBJECT","name":"In","inputFields":null},{"kind":"ENUM","name":"E","enumValues":null}]}`,
			assert: func(t *testing.T, schema *GraphQLSchema, err error) {
				require.NoError(t, err)
				assert.Empty(t, schema.Queries)
				assert.Contains(t, schema.InputTypes, "In")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"data":{"__schema":` + tt.schema + `}}`)

			var schema *GraphQLSchema
			var err error
			require.NotPanics(t, func() { schema, err = NewParser().ParseFromJSON(body) })
			tt.assert(t, schema, err)
		})
	}
}

// Nothing bounds how deeply a hostile document may nest ofType, and every
// routine that walks a TypeRef recurses, so the chain is cut before the stack is.
func TestParseFromJSONBoundsTypeRefNesting(t *testing.T) {
	depth := maxTypeRefDepth * 4

	var ref strings.Builder
	for i := 0; i < depth; i++ {
		ref.WriteString(`{"kind":"LIST","ofType":`)
	}
	ref.WriteString(`{"kind":"SCALAR","name":"String"}`)
	ref.WriteString(strings.Repeat("}", depth))

	body := []byte(fmt.Sprintf(
		`{"data":{"__schema":{"queryType":{"name":"Query"},"types":[{"kind":"OBJECT","name":"Query","fields":[{"name":"deep","type":{"kind":"SCALAR","name":"String"},"args":[{"name":"arg","type":%s}]}]}]}}}`,
		ref.String()))

	var schema *GraphQLSchema
	var err error
	require.NotPanics(t, func() { schema, err = NewParser().ParseFromJSON(body) })
	require.NoError(t, err)

	require.Len(t, schema.Queries, 1)
	require.Len(t, schema.Queries[0].Arguments, 1)

	// The truncated reference renders as nothing, which is how every caller
	// recognises an argument it must not write into a document.
	assert.Empty(t, schema.Queries[0].Arguments[0].Type.Signature())

	endpoints := NewGenerator(schema, DefaultGenerationConfig()).GenerateRequests()
	require.Len(t, endpoints, 1)
	assert.NotContains(t, endpoints[0].Requests[0].Query, "$arg")
	assert.NotContains(t, endpoints[0].Requests[0].Variables, "arg")
}

// Introspection reports defaults as GraphQL literals. Passed through as text
// they become the wrong type in the variables map and the server rejects them.
func TestParserDecodesDefaultValues(t *testing.T) {
	const sdl = `
	enum Status { ACTIVE ARCHIVED }
	input Filter { term: String = "all", limit: Int = 5 }
	type Query {
	  search(
	    limit: Int = 10
	    ratio: Float = 1.5
	    term: String = "hello"
	    enabled: Boolean = true
	    status: Status = ACTIVE
	    tags: [String!] = ["a", "b"]
	    filter: Filter = {term: "x", limit: 1}
	    missing: String
	  ): String
	}
	`

	schema, _ := parseSDL(t, sdl)
	require.Len(t, schema.Queries, 1)

	defaults := map[string]interface{}{}
	literals := map[string]string{}
	for _, arg := range schema.Queries[0].Arguments {
		defaults[arg.Name] = arg.DefaultValue
		literals[arg.Name] = arg.DefaultLiteral
	}

	assert.Equal(t, int64(10), defaults["limit"], "an Int default must not stay a string")
	assert.Equal(t, 1.5, defaults["ratio"])
	assert.Equal(t, "hello", defaults["term"], "a String default arrives quoted and must be unquoted")
	assert.Equal(t, true, defaults["enabled"])
	assert.Equal(t, "ACTIVE", defaults["status"], "an enum travels as its name")
	assert.Equal(t, []interface{}{"a", "b"}, defaults["tags"])
	assert.Equal(t, map[string]interface{}{"term": "x", "limit": int64(1)}, defaults["filter"])
	assert.Nil(t, defaults["missing"])

	assert.Equal(t, "10", literals["limit"], "the literal form is kept for inline use")
	assert.Equal(t, `"hello"`, literals["term"])
	assert.Empty(t, literals["missing"])

	require.Contains(t, schema.InputTypes, "Filter")
	for _, field := range schema.InputTypes["Filter"].Fields {
		if field.Name == "limit" {
			assert.Equal(t, int64(5), field.DefaultValue, "input field defaults are decoded too")
		}
	}
}

func TestParserPreservesDeprecation(t *testing.T) {
	const sdl = `
	enum Role { ADMIN, GUEST @deprecated(reason: "use ADMIN") }
	type User { id: ID!, old: String @deprecated(reason: "gone") }
	type Query { me: User, legacy: String @deprecated(reason: "removed") }
	`

	schema, _ := parseSDL(t, sdl)

	var legacy Operation
	for _, op := range schema.Queries {
		if op.Name == "legacy" {
			legacy = op
		}
	}
	assert.True(t, legacy.IsDeprecated)
	assert.Equal(t, "removed", legacy.Deprecation)

	user := schema.Types["User"]
	require.Len(t, user.Fields, 2)
	assert.True(t, user.Fields[1].IsDeprecated)

	role := schema.Enums["Role"]
	require.Len(t, role.Values, 2)
	assert.False(t, role.Values[0].IsDeprecated)
	assert.True(t, role.Values[1].IsDeprecated)
	assert.Equal(t, "use ADMIN", role.Values[1].Deprecation)
}

// A schema may expose a root type as an ordinary field. Dropping root types from
// the type map leaves that field looking like a scalar and breaks the document.
func TestParserKeepsRootTypesInTypeMap(t *testing.T) {
	const sdl = `
	type Query { viewer: Query, hello: String }
	`

	schema, oracle := parseSDL(t, sdl)

	require.Contains(t, schema.Types, "Query")

	endpoints := NewGenerator(schema, DefaultGenerationConfig()).GenerateRequests()
	for _, endpoint := range endpoints {
		for _, req := range endpoint.Requests {
			assertValidDocument(t, oracle, req.Query)
		}
	}
}
