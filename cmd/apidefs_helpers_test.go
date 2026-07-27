package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLooksLikeIntrospectionResult(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"wrapped in data", `{"data":{"__schema":{"types":[]}}}`, true},
		{"bare schema", `{"__schema":{"types":[]}}`, true},
		{"graphiql html page", `<!DOCTYPE html><html><body><div id="graphiql"></div></body></html>`, false},
		{"graphql error response", `{"errors":[{"message":"introspection is disabled"}]}`, false},
		{"openapi document", `{"openapi":"3.1.0","paths":{}}`, false},
		{"empty", ``, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, looksLikeIntrospectionResult([]byte(tt.content)))
		})
	}
}

// The overwhelmingly common deployment serves an IDE on GET and only answers the
// schema to a POSTed introspection query, so fetching the endpoint URL is not
// enough to build a definition.
func TestResolveGraphQLDefinitionContent_IntrospectsWhenGetReturnsUI(t *testing.T) {
	const schema = `{"data":{"__schema":{"queryType":{"name":"Query"},"types":[]}}}`

	var postedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!DOCTYPE html><html><div id="graphiql"></div></html>`))
			return
		}
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		postedBody = string(buf)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(schema))
	}))
	defer server.Close()

	uiPage := []byte(`<!DOCTYPE html><html><div id="graphiql"></div></html>`)
	got, err := resolveGraphQLDefinitionContent(uiPage, db.APIDefinitionTypeGraphQL, server.URL+"/graphql")
	require.NoError(t, err)
	assert.JSONEq(t, schema, string(got))
	assert.Contains(t, postedBody, "IntrospectionQuery")
}

func TestResolveGraphQLDefinitionContent_KeepsExistingSchema(t *testing.T) {
	const schema = `{"data":{"__schema":{"types":[]}}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be made when the content already holds a schema")
	}))
	defer server.Close()

	got, err := resolveGraphQLDefinitionContent([]byte(schema), db.APIDefinitionTypeGraphQL, server.URL+"/graphql")
	require.NoError(t, err)
	assert.JSONEq(t, schema, string(got))
}

func TestResolveGraphQLDefinitionContent_LeavesNonGraphQLUntouched(t *testing.T) {
	const spec = `{"openapi":"3.1.0","paths":{}}`

	got, err := resolveGraphQLDefinitionContent([]byte(spec), db.APIDefinitionTypeOpenAPI, "http://example.com/openapi.json")
	require.NoError(t, err)
	assert.Equal(t, spec, string(got))
}

// A disabled-introspection endpoint must fail with a message that names the
// cause rather than a JSON parse error about an HTML page.
func TestResolveGraphQLDefinitionContent_ReportsDisabledIntrospection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":[{"message":"GraphQL introspection is not allowed"}]}`))
	}))
	defer server.Close()

	_, err := resolveGraphQLDefinitionContent([]byte(`<html></html>`), db.APIDefinitionTypeGraphQL, server.URL+"/graphql")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "introspection")
}
