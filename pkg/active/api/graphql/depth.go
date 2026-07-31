package graphql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/pyneda/sukyan/db"
	pkgGraphql "github.com/pyneda/sukyan/pkg/graphql"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

type DepthLimitAudit struct {
	Options     *GraphQLAuditOptions
	Definition  *db.APIDefinition
	BaseHistory *db.History
}

type depthTestCase struct {
	name        string
	query       string
	depth       int
	description string
}

type depthTestResult struct {
	history     *db.History
	testName    string
	depth       int
	description string
	passed      bool
	// invalid marks a probe the server refused to execute because the document
	// was malformed for its schema. It is the audit's own defect rather than a
	// property of the target, and it is what makes the generic fallback matter.
	invalid bool
}

type typeChain struct {
	rootField string
	rootArgs  string
	steps     []chainStep
	cyclic    bool
	// cycleStart indexes the first step of the segment that returns to a type
	// already on the path. Only that segment may be replayed to nest further:
	// replaying from step zero selects fields the current type does not declare,
	// which the server rejects as a validation error.
	cycleStart int
}

type chainStep struct {
	fieldName string
	args      string
	typeName  string
}

func (a *DepthLimitAudit) Run() {
	auditLog := log.With().
		Str("audit", "graphql-depth-limit").
		Uint("workspace", a.Options.WorkspaceID).
		Logger()

	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			auditLog.Debug().Msg("Context cancelled, skipping depth limit audit")
			return
		default:
		}
	}

	if a.Definition == nil {
		return
	}

	baseURL := a.Definition.RequestURL()

	auditLog.Info().Str("url", baseURL).Msg("Starting GraphQL depth limit audit")

	client := a.Options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	var results []depthTestResult

	if schema := a.parseSchema(); schema != nil {
		schemaCases := getSchemaAwareDepthTestCases(schema)
		auditLog.Debug().Int("test_cases", len(schemaCases)).Msg("Generated schema-aware depth test cases")
		results = a.runTestCases(baseURL, client, schemaCases)
	}

	if a.cancelled() {
		return
	}

	// Schema-aware probes are the ones that reach real resolvers, but they depend
	// on the audit generating a document the target's schema accepts. Nested
	// introspection needs no schema at all, so it is the fallback whenever the
	// schema-derived probes proved nothing.
	if !reachedReportableDepth(results) {
		auditLog.Debug().
			Int("schema_probes", len(results)).
			Int("invalid_probes", countInvalid(results)).
			Msg("Schema-aware depth probes inconclusive, falling back to generic introspection")
		results = append(results, a.runTestCases(baseURL, client, getGenericDepthTestCases())...)
	}

	if a.cancelled() {
		return
	}

	if a.Options.ScanMode.String() == "fuzz" {
		results = append(results, a.runTestCases(baseURL, client, getCircularFragmentTestCases())...)
	}

	if a.cancelled() {
		return
	}

	passed := reportableFinding(results)
	if len(passed) == 0 {
		auditLog.Info().Int("probes", len(results)).Msg("No reportable depth limit bypass detected")
		return
	}

	confidence := calculateConfidence(passed)
	details := buildConsolidatedDetails(passed)

	issue, err := db.CreateIssueFromHistoryAndTemplate(
		passed[0].history,
		db.GraphqlDepthLimitMissingCode,
		details,
		confidence,
		"",
		&a.Options.WorkspaceID,
		&a.Options.TaskID,
		&a.Options.TaskJobID,
		&a.Options.ScanID,
		&a.Options.ScanJobID,
	)
	if err != nil {
		auditLog.Error().Err(err).Msg("Failed to create depth limit issue")
		return
	}

	if len(passed) > 1 {
		var additionalHistories []*db.History
		for _, r := range passed[1:] {
			additionalHistories = append(additionalHistories, r.history)
		}
		if err := issue.AppendHistories(additionalHistories); err != nil {
			auditLog.Warn().Err(err).Uint("issue_id", issue.ID).Int("history_count", len(additionalHistories)).
				Msg("Failed to link additional histories to issue")
		}
	}

	auditLog.Info().Uint("issue_id", issue.ID).Int("tests_passed", len(passed)).Msg("Created consolidated depth limit issue")
}

func (a *DepthLimitAudit) parseSchema() *pkgGraphql.GraphQLSchema {
	if len(a.Definition.RawDefinition) == 0 {
		return nil
	}
	schema, err := pkgGraphql.NewParser().ParseSchema(a.Definition.RawDefinition)
	if err != nil {
		return nil
	}
	return schema
}

func (a *DepthLimitAudit) cancelled() bool {
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

func (a *DepthLimitAudit) runTestCases(baseURL string, client *http.Client, cases []depthTestCase) []depthTestResult {
	var results []depthTestResult
	for _, tc := range cases {
		if a.cancelled() {
			return results
		}
		results = append(results, a.executeDepthTest(baseURL, client, tc))
	}
	return results
}

// reportableFinding returns the probes that together prove the target enforces
// no useful depth limit, or nil when the evidence does not clear
// minReportableDepth. A server that rejects deep queries, or that only ever
// answered shallow ones, must produce no finding.
func reportableFinding(results []depthTestResult) []depthTestResult {
	var passed []depthTestResult
	maxDepth := 0

	for _, r := range results {
		if !r.passed {
			continue
		}
		passed = append(passed, r)
		if r.depth > maxDepth {
			maxDepth = r.depth
		}
	}

	if maxDepth < minReportableDepth {
		return nil
	}

	return passed
}

func reachedReportableDepth(results []depthTestResult) bool {
	for _, r := range results {
		if r.passed && r.depth >= minReportableDepth {
			return true
		}
	}
	return false
}

func countInvalid(results []depthTestResult) int {
	count := 0
	for _, r := range results {
		if r.invalid {
			count++
		}
	}
	return count
}

func (a *DepthLimitAudit) executeDepthTest(baseURL string, client *http.Client, tc depthTestCase) depthTestResult {
	result := depthTestResult{
		testName:    tc.name,
		depth:       tc.depth,
		description: tc.description,
		passed:      false,
	}

	req, err := http.NewRequestWithContext(a.Options.Ctx, "POST", baseURL, bytes.NewBufferString(tc.query))
	if err != nil {
		return result
	}
	req.Header.Set("Content-Type", "application/json")

	execResult := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		Client:        client,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: a.Options.WorkspaceID,
			ScanID:      a.Options.ScanID,
			ScanJobID:   a.Options.ScanJobID,
		},
	})

	if execResult.Err != nil || execResult.History == nil {
		return result
	}

	result.history = execResult.History
	body, _ := execResult.History.ResponseBody()
	result.passed = analyzeResponse(body, execResult.History.StatusCode)
	result.invalid = isInvalidDocument(body)
	return result
}

func isInvalidDocument(body []byte) bool {
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		return false
	}
	return containsSyntaxOrValidationError(parseErrorsArray(response))
}

func analyzeResponse(body []byte, statusCode int) bool {
	if statusCode >= 400 {
		return false
	}

	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		return false
	}

	errors := parseErrorsArray(response)
	if len(errors) > 0 {
		if isDepthRejection(errors) {
			return false
		}
		if containsSyntaxOrValidationError(errors) {
			return false
		}
	}

	dataVal, hasData := response["data"]
	if !hasData {
		return false
	}

	if dataVal == nil {
		return false
	}

	dataMap, ok := dataVal.(map[string]any)
	if !ok {
		return false
	}

	for _, v := range dataMap {
		if v != nil {
			return true
		}
	}

	return false
}

func parseErrorsArray(response map[string]any) []map[string]any {
	errorsRaw, ok := response["errors"]
	if !ok {
		return nil
	}

	errorsSlice, ok := errorsRaw.([]any)
	if !ok {
		return nil
	}

	var result []map[string]any
	for _, e := range errorsSlice {
		if errMap, ok := e.(map[string]any); ok {
			result = append(result, errMap)
		}
	}
	return result
}

func isDepthRejection(errors []map[string]any) bool {
	depthPhrases := []string{
		"maximum query depth",
		"depth limit",
		"too deep",
		"query too complex",
		"exceeds maximum depth",
		"nesting too deep",
		"max depth",
		"query depth exceeded",
		"maximum depth",
		"depth exceeded",
	}

	for _, errMap := range errors {
		msg, _ := errMap["message"].(string)
		msgLower := strings.ToLower(msg)

		for _, phrase := range depthPhrases {
			if strings.Contains(msgLower, phrase) {
				return true
			}
		}

		extensions, _ := errMap["extensions"].(map[string]any)
		if extensions != nil {
			code, _ := extensions["code"].(string)
			codeLower := strings.ToLower(code)
			if strings.Contains(codeLower, "depth") || strings.Contains(codeLower, "complexity") {
				return true
			}
		}
	}
	return false
}

func containsSyntaxOrValidationError(errors []map[string]any) bool {
	syntaxPhrases := []string{
		"syntax error",
		"parse error",
		"unexpected",
		"cannot query",
		"unknown field",
		"validation error",
		"is not defined",
		"did you mean",
		"field does not exist",
		"unknown type",
	}

	for _, errMap := range errors {
		msg, _ := errMap["message"].(string)
		msgLower := strings.ToLower(msg)

		for _, phrase := range syntaxPhrases {
			if strings.Contains(msgLower, phrase) {
				return true
			}
		}
	}
	return false
}

func getGenericDepthTestCases() []depthTestCase {
	return []depthTestCase{
		{
			name:        "introspection_depth_8",
			query:       `{"query":"{__schema{types{fields{type{fields{type{fields{type{fields{type{name}}}}}}}}}}}"}`,
			depth:       8,
			description: "8-level nested introspection query",
		},
		{
			name:        "introspection_depth_12",
			query:       `{"query":"{__schema{types{fields{type{fields{type{fields{type{fields{type{fields{type{fields{type{name}}}}}}}}}}}}}}}"}`,
			depth:       12,
			description: "12-level nested introspection query",
		},
		{
			name:        "introspection_depth_20",
			query:       `{"query":"{__schema{types{fields{type{fields{type{fields{type{fields{type{fields{type{fields{type{fields{type{fields{type{fields{type{fields{type{name}}}}}}}}}}}}}}}}}}}}}}}"}`,
			depth:       20,
			description: "20-level nested introspection query",
		},
		{
			name:        "fragment_depth",
			query:       `{"query":"query{...A} fragment A on Query{__schema{...B}} fragment B on __Schema{types{...C}} fragment C on __Type{fields{...D}} fragment D on __Field{type{...E}} fragment E on __Type{fields{type{name}}}"}`,
			depth:       7,
			description: "Fragment-based depth (harder to detect)",
		},
		{
			name:        "inline_fragment_depth",
			query:       `{"query":"{__schema{... on __Schema{types{... on __Type{fields{... on __Field{type{... on __Type{name}}}}}}}}}"}`,
			depth:       6,
			description: "Inline fragment depth",
		},
	}
}

// getSchemaAwareDepthTestCases builds probes from the target's own schema. It
// returns only schema-derived cases; the generic introspection probes are the
// caller's fallback for when these prove nothing.
func getSchemaAwareDepthTestCases(schema *pkgGraphql.GraphQLSchema) []depthTestCase {
	var testCases []depthTestCase
	seen := make(map[string]bool)

	for _, chain := range selectProbeChains(findDeepTypeChains(schema)) {
		for _, target := range probeDepths {
			query, depth := buildDeepQueryFromChain(schema, chain, target)
			if query == "" || seen[query] {
				continue
			}
			seen[query] = true

			testCases = append(testCases, depthTestCase{
				name:        fmt.Sprintf("schema_%s_depth_%d", chain.rootField, depth),
				query:       query,
				depth:       depth,
				description: fmt.Sprintf("Schema-aware %d-level query via %s", depth, chain.rootField),
			})
		}
	}

	return testCases
}

// selectProbeChains keeps one chain per root field. Several chains through the
// same root field render to the same document at the shallower depths and share
// a resolver that may answer null for all of them, so spending the whole probe
// budget on one root field buys neither coverage nor evidence.
func selectProbeChains(chains []typeChain) []typeChain {
	var selected []typeChain
	used := make(map[string]bool)

	for _, chain := range chains {
		if len(selected) >= maxProbeChains {
			break
		}
		if used[chain.rootField] {
			continue
		}
		used[chain.rootField] = true
		selected = append(selected, chain)
	}

	return selected
}

// Enumerating every simple path through a type graph is factorial in the number
// of types, and a real API's schema is densely cross-referenced enough that it
// never finishes. Only the first few chains are ever turned into requests, and no
// test nests deeper than the largest entry in depths, so both are bounded here.
const (
	maxTypeChains      = 200
	maxTypeChainDepth  = 15
	maxProbeChains     = 3
	minReportableDepth = 8
)

var probeDepths = []int{7, 10, 13}

func findDeepTypeChains(schema *pkgGraphql.GraphQLSchema) []typeChain {
	var chains []typeChain

	for _, query := range schema.Queries {
		if len(chains) >= maxTypeChains {
			break
		}

		returnTypeName := getBaseTypeName(query.ReturnType)
		if returnTypeName == "" {
			continue
		}

		// A root field whose required arguments cannot be rendered can never be
		// selected, so every chain under it would be refused during validation.
		rootArgs, ok := schema.RenderRequiredArguments(query.Arguments)
		if !ok {
			continue
		}

		walker := &chainWalker{
			schema:   schema,
			root:     query.Name,
			rootArgs: rootArgs,
			rootType: returnTypeName,
			visited:  make(map[string]bool),
			chains:   &chains,
		}
		walker.walk(returnTypeName, nil)
	}

	// SliceStable keeps chain discovery order for equally ranked chains, so the
	// same schema always yields the same probes and a finding is reproducible.
	sort.SliceStable(chains, func(i, j int) bool {
		if chains[i].cyclic != chains[j].cyclic {
			return chains[i].cyclic
		}
		// A root field whose arguments had to be invented is likely to resolve to
		// null, which the oracle cannot distinguish from an enforced limit.
		if (chains[i].rootArgs == "") != (chains[j].rootArgs == "") {
			return chains[i].rootArgs == ""
		}
		return len(chains[i].steps) > len(chains[j].steps)
	})

	return chains
}

type chainWalker struct {
	schema   *pkgGraphql.GraphQLSchema
	root     string
	rootArgs string
	rootType string
	visited  map[string]bool
	chains   *[]typeChain
}

func (w *chainWalker) walk(typeName string, steps []chainStep) {
	if len(*w.chains) >= maxTypeChains {
		return
	}

	if w.visited[typeName] {
		w.record(steps, true)
		return
	}

	// A chain longer than the deepest test is already long enough to nest to any
	// depth the audit asks for, so it is recorded rather than extended.
	if len(steps) >= maxTypeChainDepth {
		w.record(steps, false)
		return
	}

	typeDef, ok := w.schema.LookupType(typeName)
	if !ok {
		return
	}

	w.visited[typeName] = true
	defer func() { w.visited[typeName] = false }()

	for _, field := range typeDef.Fields {
		if len(*w.chains) >= maxTypeChains {
			return
		}

		fieldTypeName := getBaseTypeName(field.Type)
		if fieldTypeName == "" {
			continue
		}
		if _, isComposite := w.schema.LookupType(fieldTypeName); !isComposite {
			continue
		}

		args, ok := w.schema.RenderRequiredArguments(field.Arguments)
		if !ok {
			continue
		}

		w.walk(fieldTypeName, append(steps, chainStep{fieldName: field.Name, args: args, typeName: fieldTypeName}))
	}
}

func (w *chainWalker) record(steps []chainStep, cyclic bool) {
	if len(steps) == 0 {
		return
	}

	chain := typeChain{
		rootField: w.root,
		rootArgs:  w.rootArgs,
		steps:     append([]chainStep(nil), steps...),
		cyclic:    cyclic,
	}
	if cyclic {
		chain.cycleStart = w.cycleStart(chain.steps)
	}

	*w.chains = append(*w.chains, chain)
}

// cycleStart finds where the walk last stood on the type it has just returned
// to. The steps from there to the end are what maps that type back to itself,
// and are the only segment that may be repeated.
func (w *chainWalker) cycleStart(steps []chainStep) int {
	revisited := steps[len(steps)-1].typeName
	if revisited == w.rootType {
		return 0
	}
	for i := 0; i < len(steps)-1; i++ {
		if steps[i].typeName == revisited {
			return i + 1
		}
	}
	return 0
}

// buildDeepQueryFromChain renders the chain as a request body nested to at most
// targetDepth levels, returning the body and the depth actually reached. A
// non-cyclic chain may be shorter than asked for, and reporting the depth
// requested rather than the depth sent would claim evidence that was never
// gathered.
func buildDeepQueryFromChain(schema *pkgGraphql.GraphQLSchema, chain typeChain, targetDepth int) (string, int) {
	selections := chainSelections(chain, targetDepth)
	if len(selections) == 0 {
		return "", 0
	}

	var sb strings.Builder
	sb.WriteString("{" + chain.rootField + chain.rootArgs)
	for _, step := range selections {
		sb.WriteString("{" + step.fieldName + step.args)
	}
	sb.WriteString("{" + findLeafSelection(schema, selections[len(selections)-1].typeName) + "}")
	sb.WriteString(strings.Repeat("}", len(selections)+1))

	return encodeGraphQLBody(sb.String()), len(selections) + 1
}

// chainSelections trims or extends a chain to the requested nesting. Only the
// cyclic segment may be replayed; a prefix of any chain is a valid path, but
// appending arbitrary steps is not.
func chainSelections(chain typeChain, targetDepth int) []chainStep {
	wanted := targetDepth - 1
	if wanted <= 0 || len(chain.steps) == 0 {
		return nil
	}

	if len(chain.steps) >= wanted {
		return chain.steps[:wanted]
	}
	if !chain.cyclic {
		return chain.steps
	}

	cycle := chain.steps[chain.cycleStart:]
	if len(cycle) == 0 {
		return chain.steps
	}

	extended := append([]chainStep(nil), chain.steps...)
	for len(extended) < wanted {
		extended = append(extended, cycle[(len(extended)-len(chain.steps))%len(cycle)])
	}
	return extended
}

func encodeGraphQLBody(query string) string {
	body, err := json.Marshal(struct {
		Query string `json:"query"`
	}{Query: query})
	if err != nil {
		return ""
	}
	return string(body)
}

// findLeafSelection picks a field that terminates the nesting. It carries the
// field's required arguments because a leaf missing them invalidates the whole
// document, and falls back to __typename, which every composite type answers.
func findLeafSelection(schema *pkgGraphql.GraphQLSchema, typeName string) string {
	typeDef, ok := schema.Types[typeName]
	if !ok {
		return "__typename"
	}

	preferredNames := []string{"id", "name", "title", "email", "slug"}
	for _, pref := range preferredNames {
		for _, field := range typeDef.Fields {
			if !strings.EqualFold(field.Name, pref) {
				continue
			}
			if selection, ok := leafSelection(schema, field); ok {
				return selection
			}
		}
	}

	for _, field := range typeDef.Fields {
		if selection, ok := leafSelection(schema, field); ok {
			return selection
		}
	}

	return "__typename"
}

func leafSelection(schema *pkgGraphql.GraphQLSchema, field pkgGraphql.Field) (string, bool) {
	if !isScalarType(schema, field.Type) {
		return "", false
	}
	args, ok := schema.RenderRequiredArguments(field.Arguments)
	if !ok {
		return "", false
	}
	return field.Name + args, true
}

func isScalarType(schema *pkgGraphql.GraphQLSchema, typeRef pkgGraphql.TypeRef) bool {
	baseName := getBaseTypeName(typeRef)
	builtinScalars := map[string]bool{
		"String": true, "Int": true, "Float": true, "Boolean": true, "ID": true,
	}
	if builtinScalars[baseName] {
		return true
	}
	if slices.Contains(schema.Scalars, baseName) {
		return true
	}
	if _, ok := schema.Enums[baseName]; ok {
		return true
	}
	return false
}

func getBaseTypeName(ref pkgGraphql.TypeRef) string {
	if ref.Name != "" {
		return ref.Name
	}
	if ref.OfType != nil {
		return getBaseTypeName(*ref.OfType)
	}
	return ""
}

func getCircularFragmentTestCases() []depthTestCase {
	return []depthTestCase{
		{
			name:        "self_reference",
			query:       `{"query":"query{...F} fragment F on Query{...F}"}`,
			depth:       999,
			description: "Direct self-referencing fragment",
		},
		{
			name:        "mutual_reference",
			query:       `{"query":"query{...A} fragment A on Query{...B} fragment B on Query{...A}"}`,
			depth:       999,
			description: "Mutually referencing fragments (A->B->A)",
		},
		{
			name:        "chain_reference",
			query:       `{"query":"query{...A} fragment A on Query{...B} fragment B on Query{...C} fragment C on Query{...A}"}`,
			depth:       999,
			description: "Chain of fragments forming a cycle (A->B->C->A)",
		},
	}
}

func buildConsolidatedDetails(passed []depthTestResult) string {
	var sb strings.Builder
	sb.WriteString("GraphQL query depth limit is not enforced or is too permissive.\n\n")

	maxDepth := 0
	for _, r := range passed {
		if r.depth > maxDepth {
			maxDepth = r.depth
		}
	}

	sb.WriteString(fmt.Sprintf("Maximum bypass depth observed: %d levels\n", maxDepth))
	sb.WriteString(fmt.Sprintf("Tests bypassed: %d\n\n", len(passed)))

	sb.WriteString("Successful test cases:\n")
	for _, r := range passed {
		depthStr := fmt.Sprintf("%d", r.depth)
		if r.depth == 999 {
			depthStr = "infinite (circular)"
		}
		sb.WriteString(fmt.Sprintf("- %s (depth: %s): %s\n", r.testName, depthStr, r.description))
	}

	return sb.String()
}

func calculateConfidence(passed []depthTestResult) int {
	confidence := 65

	maxDepth := 0
	hasSchemaAware := false
	hasCircular := false
	for _, r := range passed {
		if r.depth > maxDepth {
			maxDepth = r.depth
		}
		if strings.HasPrefix(r.testName, "schema_") {
			hasSchemaAware = true
		}
		if r.depth == 999 {
			hasCircular = true
		}
	}

	if maxDepth >= 20 {
		confidence += 20
	} else if maxDepth >= 12 {
		confidence += 15
	} else if maxDepth >= 8 {
		confidence += 10
	}

	if hasSchemaAware {
		confidence += 10
	}

	if hasCircular {
		confidence += 5
	}

	if len(passed) >= 3 {
		confidence += 5
	}

	if confidence > 95 {
		confidence = 95
	}
	return confidence
}
