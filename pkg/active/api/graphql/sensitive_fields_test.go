package graphql

import (
	"strings"
	"testing"

	pkgGraphql "github.com/pyneda/sukyan/pkg/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func userSchemaJSON(userFields string) []byte {
	return []byte(`{"data":{"__schema":{
	  "queryType":{"name":"Query"},
	  "types":[
	    {"kind":"OBJECT","name":"Query","fields":[
	      {"name":"users","args":[],"type":{"kind":"LIST","ofType":{"kind":"OBJECT","name":"User"}}},
	      {"name":"viewer","args":[],"type":{"kind":"OBJECT","name":"User"}}]},
	    {"kind":"OBJECT","name":"User","fields":[` + userFields + `]}
	  ]}}}`)
}

func scalarField(name, kind string) string {
	return `{"name":"` + name + `","args":[],"type":{"kind":"SCALAR","name":"` + kind + `"}}`
}

func matchPaths(matches []sensitiveMatch) []string {
	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		paths = append(paths, match.path())
	}
	return paths
}

// The fields that matter live on object types, not at the query root. The audit
// used to ask whether passwordHash was a root field, which no realistic schema
// answers yes to.
func TestSensitiveFieldReportedWithOwningType(t *testing.T) {
	schema := parseTestSchema(t, userSchemaJSON(
		scalarField("id", "ID")+","+scalarField("passwordHash", "String")))

	matches := findSensitiveSchemaFields(schema)
	require.Len(t, matches, 1)
	assert.Equal(t, "User.passwordHash", matches[0].path())
	assert.Equal(t, "critical", matches[0].probe.severity)

	reported := reportableMatches(matches)
	require.Len(t, reported, 1)

	details := buildSensitiveFieldsDetails(reported)
	assert.Contains(t, details, "User.passwordHash")
	assert.Contains(t, details, "critical")
}

// Broadening a name match into a substring search is how a detector starts
// reporting a hashing algorithm as a credential.
func TestSensitiveFieldNameMatchIsExact(t *testing.T) {
	cases := []struct {
		field string
		want  bool
	}{
		{"passwordHash", true},
		{"password_hash", true},
		{"PasswordHash", true},
		{"passwordHashAlgorithm", false},
		{"passwordHashRounds", false},
		{"hasPassword", false},
		{"passwordUpdatedAt", false},
		{"apiKey", true},
		{"apiKeyPrefix", false},
		{"apiKeyLastFour", false},
		{"secret", true},
		{"secretsManagerArn", false},
		{"twoFactorSecret", true},
		{"two_factor_secret", true},
		{"twoFactorSecretSetAt", false},
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			schema := parseTestSchema(t, userSchemaJSON(scalarField(tc.field, "String")))
			matches := findSensitiveSchemaFields(schema)

			if tc.want {
				require.Len(t, matches, 1, "expected %s to match, got %v", tc.field, matchPaths(matches))
				assert.Equal(t, "User."+tc.field, matches[0].path())
				return
			}
			assert.Empty(t, matches, "%s must not match: %v", tc.field, matchPaths(matches))
		})
	}
}

func TestSchemaWithNoSensitiveNamesYieldsNoFinding(t *testing.T) {
	schema := parseTestSchema(t, userSchemaJSON(
		scalarField("id", "ID")+","+
			scalarField("displayName", "String")+","+
			scalarField("avatarUrl", "String")+","+
			scalarField("createdAt", "String")))

	matches := findSensitiveSchemaFields(schema)
	assert.Empty(t, matches, "unexpected matches: %v", matchPaths(matches))
	assert.Empty(t, reportableMatches(matches))
}

// Leading underscores are the whole signal in names like __debug, so folding
// them away would merge distinct entries in the table.
func TestNormalizeFieldName(t *testing.T) {
	cases := map[string]string{
		"passwordHash":  "passwordhash",
		"password_hash": "passwordhash",
		"PASSWORD_HASH": "passwordhash",
		"__debug":       "__debug",
		"_private":      "_private",
		"__internal":    "__internal",
		"debug":         "debug",
		"isAdmin":       "isadmin",
		"is_admin":      "isadmin",
	}

	for input, want := range cases {
		assert.Equal(t, want, normalizeFieldName(input), "normalizeFieldName(%q)", input)
	}
}

// A login payload declares accessToken and every user model declares role. Those
// names are only a finding when a value comes back, so an unconfirmed match must
// stay out of the report.
func TestGenericNamesNeedAConfirmedValue(t *testing.T) {
	probes := sensitiveFieldIndex()

	for _, name := range []string{"token", "accessToken", "refreshToken", "role", "isAdmin", "env", "settings"} {
		assert.True(t, probes[normalizeFieldName(name)].requiresValue, "%s should need a confirmed value", name)
	}
	for _, name := range []string{"passwordHash", "apiKey", "privateKey", "twoFactorSecret", "ssn", "creditCard"} {
		assert.False(t, probes[normalizeFieldName(name)].requiresValue, "%s should report on schema presence", name)
	}

	unconfirmed := []sensitiveMatch{{
		ownerType: "AuthPayload",
		field:     pkgGraphql.Field{Name: "refreshToken"},
		probe:     probes[normalizeFieldName("refreshToken")],
	}}
	assert.Empty(t, reportableMatches(unconfirmed), "an unconfirmed generic name must not be reported")

	unconfirmed[0].confirmed = true
	assert.Len(t, reportableMatches(unconfirmed), 1, "a retrieved value is evidence")
}

// role and settings are in most schemas and their values are rarely secret, so a
// set containing nothing worse must not raise an issue of its own.
func TestLowSeverityMatchesAloneRaiseNoIssue(t *testing.T) {
	probes := sensitiveFieldIndex()

	lowOnly := []sensitiveMatch{
		{ownerType: "User", field: pkgGraphql.Field{Name: "role"}, probe: probes["role"], confirmed: true},
		{ownerType: "Workspace", field: pkgGraphql.Field{Name: "settings"}, probe: probes["settings"], confirmed: true},
	}
	assert.Empty(t, reportableMatches(lowOnly))

	withCritical := append(lowOnly, sensitiveMatch{
		ownerType: "User",
		field:     pkgGraphql.Field{Name: "passwordHash"},
		probe:     probes["passwordhash"],
	})
	assert.Len(t, reportableMatches(withCritical), 3, "low-severity names stay as context once something worse is found")
}

// Reaching a field means finding a root query that returns its type. A
// collection accessor answers the same way for everyone; a singular field like
// viewer resolves against a session the audit does not have.
func TestConfirmationPrefersListRootOverSessionField(t *testing.T) {
	schema := parseTestSchema(t, userSchemaJSON(
		scalarField("passwordHash", "String")+","+scalarField("apiKey", "String")))

	matches := findSensitiveSchemaFields(schema)
	require.Len(t, matches, 2)

	fields := []pkgGraphql.Field{matches[0].field, matches[1].field}
	query, ok := buildConfirmationQuery(schema, "User", false, fields)
	require.True(t, ok)

	document := decodeProbeQuery(t, query)
	assert.Contains(t, document, "users{")
	assert.NotContains(t, document, "viewer")
	assert.Contains(t, document, "apiKey")
	assert.Contains(t, document, "passwordHash")
}

// A root query that needs an argument is still usable; one that cannot be
// reached at all leaves its fields unconfirmed rather than unreported.
func TestConfirmationRendersRequiredRootArguments(t *testing.T) {
	schema := parseTestSchema(t, []byte(`{"data":{"__schema":{
	  "queryType":{"name":"Query"},
	  "types":[
	    {"kind":"OBJECT","name":"Query","fields":[
	      {"name":"webhooks","args":[
	         {"name":"workspaceId","type":{"kind":"NON_NULL","ofType":{"kind":"SCALAR","name":"ID"}}}],
	       "type":{"kind":"LIST","ofType":{"kind":"OBJECT","name":"Webhook"}}}]},
	    {"kind":"OBJECT","name":"Webhook","fields":[
	      {"name":"secret","args":[],"type":{"kind":"SCALAR","name":"String"}}]}
	  ]}}}`))

	matches := findSensitiveSchemaFields(schema)
	require.Len(t, matches, 1)

	query, ok := buildConfirmationQuery(schema, "Webhook", false, []pkgGraphql.Field{matches[0].field})
	require.True(t, ok)
	assert.Equal(t, `{webhooks(workspaceId: "1"){secret}}`, decodeProbeQuery(t, query))
}

// A sensitive field only a mutation can reach must not be triggered to find out
// what it does.
func TestMutationFieldsAreNeverProbed(t *testing.T) {
	schema := parseTestSchema(t, []byte(`{"data":{"__schema":{
	  "queryType":{"name":"Query"},
	  "mutationType":{"name":"Mutation"},
	  "types":[
	    {"kind":"OBJECT","name":"Query","fields":[
	      {"name":"health","args":[],"type":{"kind":"SCALAR","name":"String"}}]},
	    {"kind":"OBJECT","name":"Mutation","fields":[
	      {"name":"deleteUser","args":[],"type":{"kind":"OBJECT","name":"AuthPayload"}}]},
	    {"kind":"OBJECT","name":"AuthPayload","fields":[
	      {"name":"accessToken","args":[],"type":{"kind":"SCALAR","name":"String"}}]}
	  ]}}}`))

	matches := findSensitiveSchemaFields(schema)
	require.NotEmpty(t, matches)

	for _, match := range matches {
		_, ok := buildConfirmationQuery(schema, match.ownerType, match.ownerType == queryRootTypeName(schema), []pkgGraphql.Field{match.field})
		assert.False(t, ok, "%s must not be reachable by a probe", match.path())
	}

	// Both matches are generic names that need a value, and no safe probe can
	// produce one, so nothing is reported.
	assert.Empty(t, reportableMatches(matches))
}

// A match on the root query type is selected at the document root rather than
// underneath a parent field.
func TestQueryRootMatchesAreProbedAtDocumentRoot(t *testing.T) {
	schema := parseTestSchema(t, []byte(`{"data":{"__schema":{
	  "queryType":{"name":"Query"},
	  "types":[
	    {"kind":"OBJECT","name":"Query","fields":[
	      {"name":"allUsers","args":[],"type":{"kind":"LIST","ofType":{"kind":"OBJECT","name":"User"}}}]},
	    {"kind":"OBJECT","name":"User","fields":[
	      {"name":"id","args":[],"type":{"kind":"SCALAR","name":"ID"}}]}
	  ]}}}`))

	assert.Equal(t, "Query", queryRootTypeName(schema))

	matches := findSensitiveSchemaFields(schema)
	require.Len(t, matches, 1)
	assert.Equal(t, "Query.allUsers", matches[0].path())

	query, ok := buildConfirmationQuery(schema, "Query", true, []pkgGraphql.Field{matches[0].field})
	require.True(t, ok)

	document := decodeProbeQuery(t, query)
	assert.True(t, strings.HasPrefix(document, "{allUsers"), "expected a root selection, got %s", document)
	assert.Contains(t, document, "id", "a composite field needs a selection set")
}

func TestCollectNonNullFields(t *testing.T) {
	data := map[string]any{
		"users": []any{
			map[string]any{"passwordHash": "abc", "twoFactorSecret": nil},
			map[string]any{"passwordHash": "def", "twoFactorSecret": "seed"},
		},
		"viewer": nil,
	}

	found := collectNonNullFields(data)
	assert.True(t, found["users"])
	assert.True(t, found["passwordHash"])
	assert.True(t, found["twoFactorSecret"], "a value on any element counts")
	assert.False(t, found["viewer"])
	assert.False(t, found["missing"])
}

// The response is written by the target, so the walk over it has to be bounded.
func TestCollectNonNullFieldsIsDepthBounded(t *testing.T) {
	deep := map[string]any{"leaf": "value"}
	for i := 0; i < maxResponseWalkDepth*4; i++ {
		deep = map[string]any{"nested": deep}
	}

	done := make(chan map[string]bool, 1)
	go func() { done <- collectNonNullFields(deep) }()

	found := <-done
	assert.True(t, found["nested"])
	assert.False(t, found["leaf"], "the walk must stop before the bottom of an adversarial response")
}

// Without a schema the audit can only guess, so it guesses only at names that
// are plausible root operations instead of firing the whole table.
func TestRootNameProbesAreRestrictedToPlausibleRoots(t *testing.T) {
	var rootNames []string
	for _, probe := range getSensitiveFieldProbes() {
		if probe.rootName {
			rootNames = append(rootNames, probe.field)
		}
	}

	assert.Less(t, len(rootNames), len(getSensitiveFieldProbes())/2,
		"the degraded path should probe far fewer names than the full table")
	assert.Contains(t, rootNames, "allUsers")
	assert.Contains(t, rootNames, "adminPanel")
	assert.NotContains(t, rootNames, "passwordHash", "credential names live on object types")
	assert.NotContains(t, rootNames, "apiKey", "credential names live on object types")
	assert.NotContains(t, rootNames, "deleteUser", "the audit does not execute mutations")
}
