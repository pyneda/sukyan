package graphql

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pkgGraphql "github.com/pyneda/sukyan/pkg/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// argumentSchemaJSON mirrors the shape that broke the audit in the field: every
// route into the type graph goes through a root field with a required argument,
// and one of the fields along the chain requires one too.
func argumentSchemaJSON() []byte {
	return []byte(`{"data":{"__schema":{
	  "queryType":{"name":"Query"},
	  "types":[
	    {"kind":"OBJECT","name":"Query","fields":[
	      {"name":"comments","args":[
	         {"name":"issueId","type":{"kind":"NON_NULL","ofType":{"kind":"SCALAR","name":"ID"}}},
	         {"name":"first","type":{"kind":"SCALAR","name":"Int"}}],
	       "type":{"kind":"NON_NULL","ofType":{"kind":"OBJECT","name":"CommentConnection"}}}]},
	    {"kind":"OBJECT","name":"CommentConnection","fields":[
	      {"name":"edges","args":[],"type":{"kind":"OBJECT","name":"CommentEdge"}},
	      {"name":"totalCount","args":[],"type":{"kind":"SCALAR","name":"Int"}}]},
	    {"kind":"OBJECT","name":"CommentEdge","fields":[
	      {"name":"node","args":[],"type":{"kind":"OBJECT","name":"Comment"}}]},
	    {"kind":"OBJECT","name":"Comment","fields":[
	      {"name":"id","args":[],"type":{"kind":"SCALAR","name":"ID"}},
	      {"name":"replies","args":[
	         {"name":"depth","type":{"kind":"NON_NULL","ofType":{"kind":"SCALAR","name":"Int"}}}],
	       "type":{"kind":"OBJECT","name":"CommentConnection"}}]}
	  ]}}}`)
}

func parseTestSchema(t *testing.T, raw []byte) *pkgGraphql.GraphQLSchema {
	t.Helper()
	schema, err := pkgGraphql.NewParser().ParseFromJSON(raw)
	require.NoError(t, err)
	return schema
}

func decodeProbeQuery(t *testing.T, body string) string {
	t.Helper()
	var decoded struct {
		Query string `json:"query"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &decoded))
	return decoded.Query
}

// A field that declares a required argument cannot be selected without one. The
// audit used to omit them, so every schema-aware probe was refused during
// validation and the endpoint looked like it enforced a depth limit.
func TestSchemaAwareProbesCarryRequiredArguments(t *testing.T) {
	schema := parseTestSchema(t, argumentSchemaJSON())

	cases := getSchemaAwareDepthTestCases(schema)
	require.NotEmpty(t, cases)

	for _, tc := range cases {
		query := decodeProbeQuery(t, tc.query)

		assert.Contains(t, query, `comments(issueId: "1")`,
			"root field must carry its required argument in %s: %s", tc.name, query)
		assert.NotContains(t, query, "comments{",
			"root field selected without its required argument in %s: %s", tc.name, query)

		if strings.Contains(query, "replies") {
			assert.Contains(t, query, "replies(depth: 1)",
				"chained field must carry its required argument in %s: %s", tc.name, query)
		}

		assert.Equal(t, strings.Count(query, "{"), strings.Count(query, "}"),
			"unbalanced braces in %s: %s", tc.name, query)
	}
}

// The probe body is JSON. Argument literals contain quotes, so the document has
// to be encoded rather than pasted into a hand-built string.
func TestSchemaAwareProbeBodiesAreValidJSON(t *testing.T) {
	schema := parseTestSchema(t, argumentSchemaJSON())

	for _, tc := range getSchemaAwareDepthTestCases(schema) {
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(tc.query), &body), "probe %s is not valid JSON: %s", tc.name, tc.query)
		assert.NotEmpty(t, body["query"])
	}
}

// Repeating a cyclic chain from its first step selects fields the current type
// does not declare. Only the segment that returns to the revisited type may be
// replayed.
func TestCyclicChainRepeatsOnlyTheCycle(t *testing.T) {
	body := []byte(`{"data":{"__schema":{
	  "queryType":{"name":"Query"},
	  "types":[
	    {"kind":"OBJECT","name":"Query","fields":[
	      {"name":"feed","args":[],"type":{"kind":"OBJECT","name":"Connection"}}]},
	    {"kind":"OBJECT","name":"Connection","fields":[
	      {"name":"edges","args":[],"type":{"kind":"OBJECT","name":"Edge"}}]},
	    {"kind":"OBJECT","name":"Edge","fields":[
	      {"name":"node","args":[],"type":{"kind":"OBJECT","name":"Post"}}]},
	    {"kind":"OBJECT","name":"Post","fields":[
	      {"name":"id","args":[],"type":{"kind":"SCALAR","name":"ID"}},
	      {"name":"author","args":[],"type":{"kind":"OBJECT","name":"User"}}]},
	    {"kind":"OBJECT","name":"User","fields":[
	      {"name":"id","args":[],"type":{"kind":"SCALAR","name":"ID"}},
	      {"name":"posts","args":[],"type":{"kind":"OBJECT","name":"Post"}}]}
	  ]}}}`)

	schema := parseTestSchema(t, body)

	chains := findDeepTypeChains(schema)
	require.NotEmpty(t, chains)

	query, depth := buildDeepQueryFromChain(schema, chains[0], 10)
	require.NotEmpty(t, query)
	assert.Equal(t, 10, depth)

	document := decodeProbeQuery(t, query)

	// The cycle is Post -> author -> User -> posts -> Post. Replaying from step
	// zero would put `edges` on a User, which is what the server used to reject.
	assert.NotContains(t, document, "author{edges", "cycle replayed from the wrong step: %s", document)
	assert.NotContains(t, document, "posts{edges", "cycle replayed from the wrong step: %s", document)
	assert.Contains(t, document, "author{posts", "expected the cycle segment to repeat: %s", document)
}

// A chain that cannot be nested as far as asked must report the depth it
// actually reached. Claiming the requested depth would put evidence in the issue
// that was never gathered, and could push a shallow probe past the reporting
// guard.
func TestNonCyclicChainReportsTheDepthItReached(t *testing.T) {
	schema := parseTestSchema(t, []byte(`{"data":{"__schema":{
	  "queryType":{"name":"Query"},
	  "types":[
	    {"kind":"OBJECT","name":"Query","fields":[
	      {"name":"company","args":[],"type":{"kind":"OBJECT","name":"Company"}}]},
	    {"kind":"OBJECT","name":"Company","fields":[
	      {"name":"team","args":[],"type":{"kind":"OBJECT","name":"Team"}}]},
	    {"kind":"OBJECT","name":"Team","fields":[
	      {"name":"name","args":[],"type":{"kind":"SCALAR","name":"String"}}]}
	  ]}}}`))

	chain := typeChain{
		rootField: "company",
		steps:     []chainStep{{fieldName: "team", typeName: "Team"}},
	}

	query, depth := buildDeepQueryFromChain(schema, chain, 13)
	require.NotEmpty(t, query)
	assert.Equal(t, 2, depth, "a two-level chain cannot prove a thirteen-level query")
}

// Several chains through one root field render to the same document at the
// shallower depths, and share a resolver that may answer null for all of them.
func TestProbeChainsAreDiverseAndDeduplicated(t *testing.T) {
	schema := parseTestSchema(t, denseSchemaJSON(10))

	chains := selectProbeChains(findDeepTypeChains(schema))
	require.NotEmpty(t, chains)
	assert.LessOrEqual(t, len(chains), maxProbeChains)

	roots := make(map[string]bool)
	for _, chain := range chains {
		assert.False(t, roots[chain.rootField], "root field %s probed twice", chain.rootField)
		roots[chain.rootField] = true
	}

	queries := make(map[string]bool)
	for _, tc := range getSchemaAwareDepthTestCases(schema) {
		assert.False(t, queries[tc.query], "duplicate probe body: %s", tc.query)
		queries[tc.query] = true
	}
}

// A root field whose required arguments cannot be rendered can never be
// selected, so no chain under it is worth building.
func TestRootFieldWithUnrenderableArgumentIsSkipped(t *testing.T) {
	schema := parseTestSchema(t, []byte(`{"data":{"__schema":{
	  "queryType":{"name":"Query"},
	  "types":[
	    {"kind":"OBJECT","name":"Query","fields":[
	      {"name":"search","args":[
	         {"name":"filter","type":{"kind":"NON_NULL","ofType":{"kind":"INPUT_OBJECT","name":"Filter"}}}],
	       "type":{"kind":"OBJECT","name":"Node"}}]},
	    {"kind":"INPUT_OBJECT","name":"Filter","inputFields":[
	      {"name":"bad name","type":{"kind":"NON_NULL","ofType":{"kind":"SCALAR","name":"String"}}}]},
	    {"kind":"OBJECT","name":"Node","fields":[
	      {"name":"id","args":[],"type":{"kind":"SCALAR","name":"ID"}},
	      {"name":"child","args":[],"type":{"kind":"OBJECT","name":"Node"}}]}
	  ]}}}`))

	assert.Empty(t, findDeepTypeChains(schema))
	assert.Empty(t, getSchemaAwareDepthTestCases(schema))
}

// probeTarget runs generated probes against a real server and classifies the
// answers exactly as the audit does, without needing the history store.
func probeTarget(t *testing.T, url string, cases []depthTestCase) []depthTestResult {
	t.Helper()

	var results []depthTestResult
	for _, tc := range cases {
		response, err := http.Post(url, "application/json", strings.NewReader(tc.query))
		require.NoError(t, err)

		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())

		results = append(results, depthTestResult{
			testName: tc.name,
			depth:    tc.depth,
			passed:   analyzeResponse(body, response.StatusCode),
			invalid:  isInvalidDocument(body),
		})
	}
	return results
}

func graphqlServer(t *testing.T, handler func(query string) (int, string)) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		status, response := handler(request.Query)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(server.Close)

	return server
}

// When every schema-aware probe comes back as a validation error the audit has
// proved nothing about the target, and the generic introspection probes - which
// need no schema at all - have to run.
func TestGenericFallbackWhenSchemaAwareProbesAreInvalid(t *testing.T) {
	server := graphqlServer(t, func(query string) (int, string) {
		if strings.Contains(query, "__schema") {
			return http.StatusOK, `{"data":{"__schema":{"types":[{"fields":[{"type":{"name":"ID"}}]}]}}}`
		}
		return http.StatusBadRequest, `{"errors":[{"message":"Cannot query field \"edges\" on type \"User\"."}]}`
	})

	schema := parseTestSchema(t, argumentSchemaJSON())

	schemaResults := probeTarget(t, server.URL, getSchemaAwareDepthTestCases(schema))
	require.NotEmpty(t, schemaResults)
	assert.Equal(t, len(schemaResults), countInvalid(schemaResults), "expected every schema-aware probe to be refused")
	require.False(t, reachedReportableDepth(schemaResults), "invalid probes must not count as evidence")

	all := append(schemaResults, probeTarget(t, server.URL, getGenericDepthTestCases())...)
	assert.NotEmpty(t, reportableFinding(all), "generic introspection should have carried the finding")
}

// The generic probes are skipped once a schema-aware probe has already reached a
// reportable depth; there is nothing left for them to prove.
func TestGenericFallbackSkippedWhenSchemaAwareProbesSucceed(t *testing.T) {
	results := []depthTestResult{
		{testName: "schema_feed_depth_13", depth: 13, passed: true},
	}
	assert.True(t, reachedReportableDepth(results))
}

// A shallow schema-aware pass proves nothing on its own, so the generic probes
// must still run rather than being suppressed by a depth-4 success.
func TestGenericFallbackRunsAfterShallowSchemaAwarePass(t *testing.T) {
	results := []depthTestResult{
		{testName: "schema_feed_depth_4", depth: 4, passed: true},
	}
	assert.False(t, reachedReportableDepth(results))
	assert.Empty(t, reportableFinding(results))
}

// A server that really enforces a depth limit must produce no finding, whether
// it rejects with an error message, an extensions code, or a plain 400.
func TestEnforcedDepthLimitProducesNoFinding(t *testing.T) {
	const limit = 5

	server := graphqlServer(t, func(query string) (int, string) {
		// Every spec-compliant server refuses a fragment that spreads itself,
		// before any depth rule gets a say.
		if strings.Contains(query, "fragment") && strings.Count(query, "...") > 1 {
			return http.StatusOK, `{"errors":[{"message":"Cannot spread fragment \"A\" within itself."}]}`
		}
		if strings.Count(query, "{") > limit {
			return http.StatusOK, `{"errors":[{"message":"Query exceeds maximum depth of 5","extensions":{"code":"DEPTH_LIMIT_EXCEEDED"}}],"data":null}`
		}
		return http.StatusOK, `{"data":{"comments":{"totalCount":0}}}`
	})

	schema := parseTestSchema(t, argumentSchemaJSON())

	results := probeTarget(t, server.URL, getSchemaAwareDepthTestCases(schema))
	require.NotEmpty(t, results)
	require.False(t, reachedReportableDepth(results))

	results = append(results, probeTarget(t, server.URL, getGenericDepthTestCases())...)
	results = append(results, probeTarget(t, server.URL, getCircularFragmentTestCases())...)

	assert.Empty(t, reportableFinding(results), "a server enforcing a depth limit must not be reported")
}

// A target with no depth limit answers the generated probes with data, and the
// deepest of them clears the reporting guard.
func TestUnlimitedDepthProducesFinding(t *testing.T) {
	server := graphqlServer(t, func(query string) (int, string) {
		return http.StatusOK, `{"data":{"comments":{"totalCount":3}}}`
	})

	schema := parseTestSchema(t, argumentSchemaJSON())

	results := probeTarget(t, server.URL, getSchemaAwareDepthTestCases(schema))
	require.NotEmpty(t, results)
	assert.Zero(t, countInvalid(results))

	passed := reportableFinding(results)
	require.NotEmpty(t, passed)

	maxDepth := 0
	for _, r := range passed {
		if r.depth > maxDepth {
			maxDepth = r.depth
		}
	}
	assert.GreaterOrEqual(t, maxDepth, minReportableDepth)
}
