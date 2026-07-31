package discovery

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dedupSchemaSDL = `
type Query { user(id: ID!): User search(term: String): [User!] }
type Mutation { createUser(input: UserInput!): User }
input UserInput { name: String! email: String }
type User { id: ID! name: String! email: String }
`

// The same schema with every declaration reordered. A server is free to render
// its schema this way on one request and the other way on the next, so an alias
// serving it must still resolve to the definition already stored.
const dedupSchemaSDLReordered = `
type User { email: String name: String! id: ID! }
input UserInput { email: String name: String! }
type Mutation { createUser(input: UserInput!): User }
type Query { search(term: String): [User!] user(id: ID!): User }
`

const dedupOtherSchemaSDL = `
type Query { order(id: ID!): Order }
type Order { id: ID! total: Float! }
`

func graphQLTestOrigin() string {
	return "http://gql-" + uuid.NewString() + ".test"
}

func persistGraphQLFrom(t *testing.T, workspaceID uint, sourceURL, schema string) *db.APIDefinition {
	t.Helper()

	definition, err := PersistGraphQLDefinition(
		historyForSpec(t, workspaceID, sourceURL, schema),
		APIPersistenceOptions{WorkspaceID: workspaceID},
	)
	require.NoError(t, err)
	require.NotNil(t, definition)
	return definition
}

func graphQLDefinitionsForOrigin(t *testing.T, workspaceID uint, origin string) []*db.APIDefinition {
	t.Helper()

	definitions, err := db.ListGraphQLAPIDefinitionsByBaseURL(db.Connection().DB(), workspaceID, origin)
	require.NoError(t, err)
	return definitions
}

func parseTestSchema(t *testing.T, sdl string) *graphql.GraphQLSchema {
	t.Helper()

	schema, err := graphql.NewParser().ParseSchema([]byte(sdl))
	require.NoError(t, err)
	return schema
}

// A prefix-mounted GraphQL server answers the same schema on every path below its
// mount point, and discovery probes about forty candidate paths. Keyed on the
// source URL alone this stored one definition per alias: a measured Apollo scan
// produced eight identical definitions of 89 operations, 712 api_scan jobs where
// 89 would do, and eight copies of every finding.
func TestGraphQLAliasesOnOneOriginShareOneDefinition(t *testing.T) {
	workspace := setupTestWorkspace(t)
	origin := graphQLTestOrigin()

	canonical := persistGraphQLFrom(t, workspace.ID, origin+"/graphql", dedupSchemaSDL)

	for _, alias := range []string{"/graphql/playground", "/graphql/v2", "/api/graphql", "/graphql/console"} {
		reused := persistGraphQLFrom(t, workspace.ID, origin+alias, dedupSchemaSDL)
		assert.Equal(t, canonical.ID, reused.ID, "%s serves the schema already stored for this origin", alias)
	}

	assert.Len(t, graphQLDefinitionsForOrigin(t, workspace.ID, origin), 1)
	assertCountMatchesEndpoints(t, canonical.ID, 3)
	assert.Equal(t, origin+"/graphql", storedDefinition(t, canonical.ID).SourceURL)
}

// The aliases are probed concurrently, so the first one to reach the database is
// not the one the scan should send its requests to.
func TestGraphQLCanonicalSourceURLPrefersTheShortestAlias(t *testing.T) {
	workspace := setupTestWorkspace(t)
	origin := graphQLTestOrigin()

	first := persistGraphQLFrom(t, workspace.ID, origin+"/graphql/playground", dedupSchemaSDL)
	assert.Equal(t, origin+"/graphql/playground", storedDefinition(t, first.ID).SourceURL)

	second := persistGraphQLFrom(t, workspace.ID, origin+"/graphql", dedupSchemaSDL)
	require.Equal(t, first.ID, second.ID)
	assert.Equal(t, origin+"/graphql", storedDefinition(t, first.ID).SourceURL)

	third := persistGraphQLFrom(t, workspace.ID, origin+"/api/v1/graphql", dedupSchemaSDL)
	require.Equal(t, first.ID, third.ID)
	assert.Equal(t, origin+"/graphql", storedDefinition(t, first.ID).SourceURL,
		"a longer alias arriving later must not take the canonical URL back")
}

func TestGraphQLAliasServingAReorderedSchemaStillDeduplicates(t *testing.T) {
	workspace := setupTestWorkspace(t)
	origin := graphQLTestOrigin()

	canonical := persistGraphQLFrom(t, workspace.ID, origin+"/graphql", dedupSchemaSDL)
	reused := persistGraphQLFrom(t, workspace.ID, origin+"/graphql/api", dedupSchemaSDLReordered)

	assert.Equal(t, canonical.ID, reused.ID)
	assert.Len(t, graphQLDefinitionsForOrigin(t, workspace.ID, origin), 1)
}

// Two hosts serving one schema are two targets: they answer differently under
// test and their findings belong apart.
func TestGraphQLSameSchemaOnTwoOriginsStaysTwoDefinitions(t *testing.T) {
	workspace := setupTestWorkspace(t)
	firstOrigin := graphQLTestOrigin()
	secondOrigin := graphQLTestOrigin()

	first := persistGraphQLFrom(t, workspace.ID, firstOrigin+"/graphql", dedupSchemaSDL)
	second := persistGraphQLFrom(t, workspace.ID, secondOrigin+"/graphql", dedupSchemaSDL)

	assert.NotEqual(t, first.ID, second.ID)
	assert.Len(t, graphQLDefinitionsForOrigin(t, workspace.ID, firstOrigin), 1)
	assert.Len(t, graphQLDefinitionsForOrigin(t, workspace.ID, secondOrigin), 1)
	assertCountMatchesEndpoints(t, first.ID, 3)
	assertCountMatchesEndpoints(t, second.ID, 3)
}

func TestGraphQLDifferentSchemasOnOneOriginStayTwoDefinitions(t *testing.T) {
	workspace := setupTestWorkspace(t)
	origin := graphQLTestOrigin()

	public := persistGraphQLFrom(t, workspace.ID, origin+"/graphql", dedupSchemaSDL)
	internal := persistGraphQLFrom(t, workspace.ID, origin+"/api/graphql", dedupOtherSchemaSDL)

	require.NotEqual(t, public.ID, internal.ID)
	assert.Len(t, graphQLDefinitionsForOrigin(t, workspace.ID, origin), 2)
	assertCountMatchesEndpoints(t, public.ID, 3)
	assertCountMatchesEndpoints(t, internal.ID, 1)
	assert.Equal(t, origin+"/graphql", storedDefinition(t, public.ID).SourceURL)
	assert.Equal(t, origin+"/api/graphql", storedDefinition(t, internal.ID).SourceURL)
}

// Re-encountering a source URL still resolves without parsing anything back out
// of the database.
func TestGraphQLPersistReusesTheDefinitionForAKnownSourceURL(t *testing.T) {
	workspace := setupTestWorkspace(t)
	sourceURL := graphQLTestOrigin() + "/graphql"

	first := persistGraphQLFrom(t, workspace.ID, sourceURL, dedupSchemaSDL)
	second := persistGraphQLFrom(t, workspace.ID, sourceURL, dedupSchemaSDL)

	assert.Equal(t, first.ID, second.ID)
	assertCountMatchesEndpoints(t, first.ID, 3)
}

// Explicit imports stay additive. Schema dedup answers discovery re-encountering
// one endpoint, not a user importing a document on purpose.
func TestGraphQLImportsAreNotDeduplicatedBySchema(t *testing.T) {
	workspace := setupTestWorkspace(t)
	sourceURL := graphQLTestOrigin() + "/graphql"

	first, err := PersistAPIDefinitionFromContent([]byte(dedupSchemaSDL), db.APIDefinitionTypeGraphQL, APIPersistenceFromContentOptions{
		WorkspaceID: workspace.ID,
		SourceURL:   sourceURL,
	})
	require.NoError(t, err)

	second, err := PersistAPIDefinitionFromContent([]byte(dedupSchemaSDL), db.APIDefinitionTypeGraphQL, APIPersistenceFromContentOptions{
		WorkspaceID: workspace.ID,
		SourceURL:   sourceURL,
	})
	require.NoError(t, err)

	assert.NotEqual(t, first.ID, second.ID)
}

// Discovery probes the candidate paths concurrently, and workers on separate
// machines probe the same target at the same time. A check-then-act dedup lets
// every one of them read "not stored yet" and insert.
func TestGraphQLConcurrentAliasesStoreOneDefinition(t *testing.T) {
	workspace := setupTestWorkspace(t)
	origin := graphQLTestOrigin()

	aliases := []string{
		"/graphql", "/graphql/api", "/graphql/v1", "/graphql/v2",
		"/graphql/console", "/graphql/explorer", "/graphql/playground", "/graphql/schema",
	}

	histories := make([]*db.History, 0, len(aliases))
	for _, alias := range aliases {
		histories = append(histories, historyForSpec(t, workspace.ID, origin+alias, dedupSchemaSDL))
	}

	var wg sync.WaitGroup
	results := make([]*db.APIDefinition, len(histories))
	errs := make([]error, len(histories))
	start := make(chan struct{})

	for i, history := range histories {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = PersistGraphQLDefinition(history, APIPersistenceOptions{WorkspaceID: workspace.ID})
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, aliases[i])
	}

	definitions := graphQLDefinitionsForOrigin(t, workspace.ID, origin)
	require.Len(t, definitions, 1, "concurrent aliases of one endpoint must store one definition")

	for i, result := range results {
		assert.Equal(t, definitions[0].ID, result.ID, aliases[i])
	}
	assertCountMatchesEndpoints(t, definitions[0].ID, 3)
	assert.Equal(t, origin+"/graphql", storedDefinition(t, definitions[0].ID).SourceURL)
}

func TestGraphQLSchemaFingerprintIgnoresDeclarationOrder(t *testing.T) {
	fingerprint := graphQLSchemaFingerprint(parseTestSchema(t, dedupSchemaSDL))

	assert.NotEmpty(t, fingerprint)
	assert.Equal(t, fingerprint, graphQLSchemaFingerprint(parseTestSchema(t, dedupSchemaSDLReordered)))
	assert.NotEqual(t, fingerprint, graphQLSchemaFingerprint(parseTestSchema(t, dedupOtherSchemaSDL)))
}

// The surface the scanner injects into is what the fingerprint has to track: an
// extra operation, an extra argument or a changed type is another API.
func TestGraphQLSchemaFingerprintTracksTheExposedSurface(t *testing.T) {
	base := graphQLSchemaFingerprint(parseTestSchema(t, dedupSchemaSDL))

	variants := map[string]string{
		"added operation": `
			type Query { user(id: ID!): User search(term: String): [User!] admin: User }
			type Mutation { createUser(input: UserInput!): User }
			input UserInput { name: String! email: String }
			type User { id: ID! name: String! email: String }
		`,
		"added argument": `
			type Query { user(id: ID!, trace: String): User search(term: String): [User!] }
			type Mutation { createUser(input: UserInput!): User }
			input UserInput { name: String! email: String }
			type User { id: ID! name: String! email: String }
		`,
		"changed argument type": `
			type Query { user(id: String!): User search(term: String): [User!] }
			type Mutation { createUser(input: UserInput!): User }
			input UserInput { name: String! email: String }
			type User { id: ID! name: String! email: String }
		`,
		"added input field": `
			type Query { user(id: ID!): User search(term: String): [User!] }
			type Mutation { createUser(input: UserInput!): User }
			input UserInput { name: String! email: String role: String }
			type User { id: ID! name: String! email: String }
		`,
	}

	for name, sdl := range variants {
		t.Run(name, func(t *testing.T) {
			assert.NotEqual(t, base, graphQLSchemaFingerprint(parseTestSchema(t, sdl)))
		})
	}
}

func TestIsMoreCanonicalGraphQLURL(t *testing.T) {
	tests := []struct {
		candidate string
		current   string
		expected  bool
	}{
		{"http://a.test/graphql", "http://a.test/graphql/playground", true},
		{"http://a.test/graphql/playground", "http://a.test/graphql", false},
		{"http://a.test/gql", "http://a.test/graphql", true},
		{"http://a.test/graphql", "http://a.test/gql", false},
		{"http://a.test/graphql", "http://a.test/graphql", false},
		{"http://a.test/graphql", "", true},
		{"", "http://a.test/graphql", false},
		// Equal depth and equal length settle lexicographically so concurrent
		// workers converge on the same alias whatever order they arrive in.
		{"http://a.test/aaaaaaa", "http://a.test/bbbbbbb", true},
		{"http://a.test/bbbbbbb", "http://a.test/aaaaaaa", false},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, isMoreCanonicalGraphQLURL(test.candidate, test.current),
			"candidate %q current %q", test.candidate, test.current)
	}
}
