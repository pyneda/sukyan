package graphql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/pyneda/sukyan/db"
	pkgGraphql "github.com/pyneda/sukyan/pkg/graphql"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

type SensitiveFieldsAudit struct {
	Options     *GraphQLAuditOptions
	Definition  *db.APIDefinition
	BaseHistory *db.History
}

type sensitiveFieldProbe struct {
	field       string
	category    string
	description string
	severity    string
	// requiresValue marks names that legitimately appear in ordinary schemas. An
	// authentication payload declares accessToken and most user models declare
	// role, so the declaration is not the finding: only a value handed back to an
	// unauthenticated caller is. Names left false are ones an API should not
	// expose at all, where the declaration is the finding.
	requiresValue bool
	// rootName marks names plausible as root operations. It is only consulted
	// when the endpoint exposes no schema to walk, which is the only situation
	// left where the audit has to guess at field names.
	rootName bool
}

func getSensitiveFieldProbes() []sensitiveFieldProbe {
	return []sensitiveFieldProbe{
		{field: "password", category: "auth", description: "Password field", severity: "critical"},
		{field: "passwordHash", category: "auth", description: "Password hash field", severity: "critical"},
		{field: "hashedPassword", category: "auth", description: "Hashed password field", severity: "critical"},
		{field: "secret", category: "auth", description: "Secret field", severity: "critical"},
		{field: "secretKey", category: "auth", description: "Secret key field", severity: "critical"},
		{field: "clientSecret", category: "auth", description: "OAuth client secret field", severity: "critical"},
		{field: "apiKey", category: "auth", description: "API key field", severity: "critical"},
		{field: "apiSecret", category: "auth", description: "API secret field", severity: "critical"},
		{field: "privateKey", category: "auth", description: "Private key field", severity: "critical"},
		{field: "encryptionKey", category: "auth", description: "Encryption key field", severity: "critical"},
		{field: "twoFactorSecret", category: "auth", description: "Two-factor seed field", severity: "critical"},
		{field: "oauthToken", category: "auth", description: "Stored OAuth token field", severity: "critical"},
		{field: "oauthRefreshToken", category: "auth", description: "Stored OAuth refresh token field", severity: "critical"},

		{field: "token", category: "auth", description: "Token field", severity: "high", requiresValue: true},
		{field: "accessToken", category: "auth", description: "Access token field", severity: "critical", requiresValue: true},
		{field: "refreshToken", category: "auth", description: "Refresh token field", severity: "critical", requiresValue: true},
		{field: "sessionToken", category: "auth", description: "Session token field", severity: "critical", requiresValue: true},

		{field: "ssn", category: "pii", description: "Social Security Number", severity: "critical"},
		{field: "socialSecurityNumber", category: "pii", description: "Social Security Number", severity: "critical"},
		{field: "creditCard", category: "pii", description: "Credit card field", severity: "critical"},
		{field: "creditCardNumber", category: "pii", description: "Credit card number", severity: "critical"},
		{field: "cvv", category: "pii", description: "CVV field", severity: "critical"},
		{field: "cardNumber", category: "pii", description: "Card number field", severity: "critical"},
		{field: "bankAccount", category: "pii", description: "Bank account field", severity: "high"},
		{field: "taxId", category: "pii", description: "Tax ID field", severity: "high"},

		{field: "__debug", category: "internal", description: "Debug field", severity: "medium", requiresValue: true, rootName: true},
		{field: "__internal", category: "internal", description: "Internal field", severity: "medium", requiresValue: true, rootName: true},
		{field: "_private", category: "internal", description: "Private field", severity: "medium", requiresValue: true, rootName: true},
		{field: "debug", category: "internal", description: "Debug field", severity: "low", requiresValue: true, rootName: true},
		{field: "internal", category: "internal", description: "Internal field", severity: "low", requiresValue: true, rootName: true},
		{field: "config", category: "internal", description: "Config field", severity: "medium", requiresValue: true, rootName: true},
		{field: "configuration", category: "internal", description: "Configuration field", severity: "medium", requiresValue: true, rootName: true},
		{field: "settings", category: "internal", description: "Settings field", severity: "low", requiresValue: true, rootName: true},
		{field: "env", category: "internal", description: "Environment field", severity: "medium", requiresValue: true, rootName: true},
		{field: "environment", category: "internal", description: "Environment field", severity: "medium", requiresValue: true, rootName: true},

		{field: "admin", category: "admin", description: "Admin field", severity: "medium", requiresValue: true, rootName: true},
		{field: "adminPanel", category: "admin", description: "Admin panel field", severity: "medium", requiresValue: true, rootName: true},
		{field: "isAdmin", category: "admin", description: "Admin flag field", severity: "medium", requiresValue: true},
		{field: "isSuperuser", category: "admin", description: "Superuser flag field", severity: "medium", requiresValue: true},
		{field: "role", category: "admin", description: "Role field", severity: "low", requiresValue: true},
		{field: "permissions", category: "admin", description: "Permissions field", severity: "medium", requiresValue: true},
		{field: "allUsers", category: "admin", description: "All users query", severity: "medium", requiresValue: true, rootName: true},
		// deleteUser is deliberately not a rootName: it is only ever a mutation,
		// and this audit does not execute mutations to find out what they do.
		{field: "deleteUser", category: "admin", description: "Delete user mutation", severity: "high", requiresValue: true},
	}
}

// maxConfirmationProbes bounds the requests spent proving that a matched field
// really answers with data. One probe covers every match on a type, so the cap
// is on types rather than fields.
const maxConfirmationProbes = 12

type sensitiveMatch struct {
	ownerType string
	field     pkgGraphql.Field
	probe     sensitiveFieldProbe
	// confirmed records that a probe retrieved a non-null value for the field,
	// which is the difference between a name in a schema and data leaving the
	// server.
	confirmed bool
	evidence  string
	history   *db.History
}

func (m sensitiveMatch) path() string {
	return m.ownerType + "." + m.field.Name
}

func (a *SensitiveFieldsAudit) Run() {
	auditLog := log.With().
		Str("audit", "graphql-sensitive-fields").
		Uint("workspace", a.Options.WorkspaceID).
		Logger()

	if a.cancelled() {
		auditLog.Debug().Msg("Context cancelled, skipping sensitive fields audit")
		return
	}

	if a.Definition == nil {
		return
	}

	baseURL := a.Definition.RequestURL()

	auditLog.Info().Str("url", baseURL).Msg("Starting GraphQL sensitive fields audit")

	client := a.Options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	var matches []sensitiveMatch
	if schema := a.parseSchema(); schema != nil {
		matches = findSensitiveSchemaFields(schema)
		auditLog.Debug().Int("matches", len(matches)).Msg("Matched sensitive field names in schema")
		a.confirmMatches(baseURL, client, schema, matches)
	} else {
		auditLog.Debug().Msg("No parseable schema, probing plausible root field names only")
		matches = a.probeRootNames(baseURL, client)
	}

	reported := reportableMatches(matches)
	if len(reported) == 0 {
		auditLog.Info().Msg("No sensitive fields discovered")
		return
	}

	confirmed := 0
	for _, m := range reported {
		if m.confirmed {
			confirmed++
		}
	}

	// A retrieved value is proof the data leaves the server; a name in the schema
	// is only proof the field is declared, which a runtime guard may still deny.
	confidence := 60
	if confirmed > 0 {
		confidence = 90
	}

	issue, err := db.CreateIssueFromHistoryAndTemplate(
		firstHistory(reported, a.BaseHistory),
		db.GraphqlSensitiveFieldsExposedCode,
		buildSensitiveFieldsDetails(reported),
		confidence,
		"",
		&a.Options.WorkspaceID,
		&a.Options.TaskID,
		&a.Options.TaskJobID,
		&a.Options.ScanID,
		&a.Options.ScanJobID,
	)
	if err != nil {
		auditLog.Error().Err(err).Msg("Failed to create sensitive fields issue")
		return
	}

	if additional := additionalHistories(reported); len(additional) > 0 {
		if err := issue.AppendHistories(additional); err != nil {
			auditLog.Warn().Err(err).Uint("issue_id", issue.ID).Msg("Failed to link additional histories to issue")
		}
	}

	auditLog.Info().
		Uint("issue_id", issue.ID).
		Int("fields_reported", len(reported)).
		Int("fields_confirmed", confirmed).
		Msg("Created sensitive fields issue")
}

func (a *SensitiveFieldsAudit) cancelled() bool {
	if a.Options.Ctx == nil {
		return false
	}
	select {
	case <-a.Options.Ctx.Done():
		return true
	default:
		return false
	}
}

func (a *SensitiveFieldsAudit) parseSchema() *pkgGraphql.GraphQLSchema {
	if len(a.Definition.RawDefinition) == 0 {
		return nil
	}
	schema, err := pkgGraphql.NewParser().ParseSchema(a.Definition.RawDefinition)
	if err != nil {
		return nil
	}
	return schema
}

// normalizeFieldName folds the spellings the same field carries across
// languages, so password_hash and passwordHash compare equal. Leading
// underscores survive because they are the whole signal in names like __debug.
// Nothing else is folded: matching stays an exact lookup, because a field named
// passwordHashAlgorithm holding "bcrypt" is not a credential leak.
func normalizeFieldName(name string) string {
	lower := strings.ToLower(name)

	prefix := 0
	for prefix < len(lower) && lower[prefix] == '_' {
		prefix++
	}

	return lower[:prefix] + strings.ReplaceAll(lower[prefix:], "_", "")
}

func sensitiveFieldIndex() map[string]sensitiveFieldProbe {
	index := make(map[string]sensitiveFieldProbe)
	for _, probe := range getSensitiveFieldProbes() {
		index[normalizeFieldName(probe.field)] = probe
	}
	return index
}

func findSensitiveSchemaFields(schema *pkgGraphql.GraphQLSchema) []sensitiveMatch {
	index := sensitiveFieldIndex()

	typeNames := make([]string, 0, len(schema.Types))
	for name := range schema.Types {
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)

	var matches []sensitiveMatch
	for _, typeName := range typeNames {
		for _, field := range schema.Types[typeName].Fields {
			probe, ok := index[normalizeFieldName(field.Name)]
			if !ok {
				continue
			}
			matches = append(matches, sensitiveMatch{ownerType: typeName, field: field, probe: probe})
		}
	}

	return matches
}

// confirmMatches spends one request per owning type trying to retrieve the
// matched fields. Reaching a type means finding a root query that returns it;
// mutations are never executed, so a sensitive field only a mutation can reach
// stays unconfirmed rather than being triggered to find out.
func (a *SensitiveFieldsAudit) confirmMatches(baseURL string, client *http.Client, schema *pkgGraphql.GraphQLSchema, matches []sensitiveMatch) {
	byType := make(map[string][]int)
	var order []string
	for i, match := range matches {
		if _, seen := byType[match.ownerType]; !seen {
			order = append(order, match.ownerType)
		}
		byType[match.ownerType] = append(byType[match.ownerType], i)
	}

	queryRoot := queryRootTypeName(schema)
	spent := 0

	for _, typeName := range order {
		if spent >= maxConfirmationProbes || a.cancelled() {
			return
		}

		indexes := byType[typeName]
		fields := make([]pkgGraphql.Field, 0, len(indexes))
		for _, i := range indexes {
			fields = append(fields, matches[i].field)
		}

		query, ok := buildConfirmationQuery(schema, typeName, typeName == queryRoot, fields)
		if !ok {
			continue
		}

		spent++
		history, values := a.executeConfirmation(baseURL, client, query)
		if history == nil {
			continue
		}

		for _, i := range indexes {
			matches[i].history = history
			matches[i].evidence = query
			matches[i].confirmed = values[matches[i].field.Name]
		}
	}
}

// queryRootTypeName recovers which type declares the root query fields. The
// parsed schema keeps root operations and named types side by side without
// linking them, and the link is what decides whether a match is selected at the
// document root or underneath a root field.
func queryRootTypeName(schema *pkgGraphql.GraphQLSchema) string {
	if len(schema.Queries) == 0 {
		return ""
	}

	for name, def := range schema.Types {
		if len(def.Fields) != len(schema.Queries) {
			continue
		}
		same := true
		for i, field := range def.Fields {
			if field.Name != schema.Queries[i].Name {
				same = false
				break
			}
		}
		if same {
			return name
		}
	}

	return ""
}

func buildConfirmationQuery(schema *pkgGraphql.GraphQLSchema, ownerType string, isQueryRoot bool, fields []pkgGraphql.Field) (string, bool) {
	var selections []string
	for _, field := range fields {
		if selection, ok := confirmationSelection(schema, field); ok {
			selections = append(selections, selection)
		}
	}
	if len(selections) == 0 {
		return "", false
	}

	inner := strings.Join(selections, " ")
	if isQueryRoot {
		return encodeGraphQLBody("{" + inner + "}"), true
	}

	root, rootArgs, ok := rootQueryReturning(schema, ownerType)
	if !ok {
		return "", false
	}

	return encodeGraphQLBody("{" + root + rootArgs + "{" + inner + "}}"), true
}

func confirmationSelection(schema *pkgGraphql.GraphQLSchema, field pkgGraphql.Field) (string, bool) {
	args, ok := schema.RenderRequiredArguments(field.Arguments)
	if !ok {
		return "", false
	}

	selectionSet := schema.BuildSelectionSet(field.Type, pkgGraphql.SelectionOptions{
		MaxDepth:          1,
		MaxFieldsPerLevel: 5,
		MaxTotalFields:    20,
	})

	return field.Name + args + selectionSet, true
}

// rootQueryReturning picks the root query most likely to actually yield the
// type. Fields needing no arguments come first, because an argument the audit
// had to invent usually addresses a record that does not exist. Among those a
// list wins: a collection accessor answers the same way for everyone, while a
// singular field such as viewer resolves against the caller's session and is
// null without one.
func rootQueryReturning(schema *pkgGraphql.GraphQLSchema, typeName string) (string, string, bool) {
	best := -1
	var name, args string

	for _, query := range schema.Queries {
		if getBaseTypeName(query.ReturnType) != typeName {
			continue
		}
		rendered, ok := schema.RenderRequiredArguments(query.Arguments)
		if !ok {
			continue
		}

		score := 0
		if rendered != "" {
			score += 2
		}
		if !query.ReturnType.IsList {
			score++
		}

		if best == -1 || score < best {
			best, name, args = score, query.Name, rendered
		}
	}

	return name, args, best != -1
}

func (a *SensitiveFieldsAudit) executeConfirmation(baseURL string, client *http.Client, query string) (*db.History, map[string]bool) {
	req, err := http.NewRequestWithContext(a.Options.Ctx, "POST", baseURL, bytes.NewBufferString(query))
	if err != nil {
		return nil, nil
	}
	req.Header.Set("Content-Type", "application/json")

	result := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		Client:        client,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: a.Options.WorkspaceID,
			ScanID:      a.Options.ScanID,
			ScanJobID:   a.Options.ScanJobID,
		},
	})

	if result.Err != nil || result.History == nil {
		return nil, nil
	}

	body, _ := result.History.ResponseBody()

	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		return result.History, nil
	}

	return result.History, collectNonNullFields(response["data"])
}

// maxResponseWalkDepth bounds the search through a response the target
// controls, which is otherwise free to nest as deeply as it likes.
const maxResponseWalkDepth = 24

func collectNonNullFields(node any) map[string]bool {
	found := make(map[string]bool)
	walkNonNullFields(node, found, 0)
	return found
}

func walkNonNullFields(node any, found map[string]bool, depth int) {
	if depth > maxResponseWalkDepth {
		return
	}

	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			if child != nil {
				found[key] = true
			}
			walkNonNullFields(child, found, depth+1)
		}
	case []any:
		for _, item := range value {
			walkNonNullFields(item, found, depth+1)
		}
	}
}

// probeRootNames is the reduced behaviour for an endpoint that exposes no schema
// to walk. Only names plausible as root operations are tried, and only as
// queries: asking a server to run an unknown mutation is not a safe way to learn
// whether it exists.
func (a *SensitiveFieldsAudit) probeRootNames(baseURL string, client *http.Client) []sensitiveMatch {
	var matches []sensitiveMatch

	for _, probe := range getSensitiveFieldProbes() {
		if !probe.rootName {
			continue
		}
		if a.cancelled() {
			return matches
		}

		query := encodeGraphQLBody("query{" + probe.field + "}")
		history, values := a.executeConfirmation(baseURL, client, query)
		if history == nil || !values[probe.field] {
			continue
		}

		matches = append(matches, sensitiveMatch{
			ownerType: "Query",
			field:     pkgGraphql.Field{Name: probe.field},
			probe:     probe,
			confirmed: true,
			evidence:  query,
			history:   history,
		})
	}

	return matches
}

func severityRank(severity string) int {
	switch severity {
	case "critical":
		return 3
	case "high":
		return 2
	case "medium":
		return 1
	}
	return 0
}

func reportableMatches(matches []sensitiveMatch) []sensitiveMatch {
	var reported []sensitiveMatch
	highest := 0

	for _, match := range matches {
		if match.probe.requiresValue && !match.confirmed {
			continue
		}
		if rank := severityRank(match.probe.severity); rank > highest {
			highest = rank
		}
		reported = append(reported, match)
	}

	// Low-severity names on their own are context rather than a finding: role and
	// settings are in most schemas and their values are rarely secret. They stay
	// in the details of an issue raised by something worse, but never raise one.
	if highest == 0 {
		return nil
	}

	return reported
}

func firstHistory(matches []sensitiveMatch, fallback *db.History) *db.History {
	for _, match := range matches {
		if match.confirmed && match.history != nil {
			return match.history
		}
	}
	for _, match := range matches {
		if match.history != nil {
			return match.history
		}
	}
	return fallback
}

func additionalHistories(matches []sensitiveMatch) []*db.History {
	first := firstHistory(matches, nil)
	seen := make(map[uint]bool)
	var histories []*db.History

	for _, match := range matches {
		if match.history == nil || match.history == first || seen[match.history.ID] {
			continue
		}
		seen[match.history.ID] = true
		histories = append(histories, match.history)
	}

	return histories
}

func buildSensitiveFieldsDetails(matches []sensitiveMatch) string {
	var confirmed, declared []sensitiveMatch
	for _, match := range matches {
		if match.confirmed {
			confirmed = append(confirmed, match)
		} else {
			declared = append(declared, match)
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Matched %d sensitive field names against the endpoint schema.\n", len(matches))

	if len(confirmed) > 0 {
		sb.WriteString("\nRetrieved a value (field returned data to an unauthenticated query):\n")
		for _, match := range confirmed {
			fmt.Fprintf(&sb, "  - %s [%s] %s\n", match.path(), match.probe.severity, match.probe.description)
			fmt.Fprintf(&sb, "    query: %s\n", match.evidence)
		}
	}

	if len(declared) > 0 {
		sb.WriteString("\nDeclared in the schema (no value retrieved):\n")
		for _, match := range declared {
			fmt.Fprintf(&sb, "  - %s [%s] %s\n", match.path(), match.probe.severity, match.probe.description)
		}
	}

	return sb.String()
}
