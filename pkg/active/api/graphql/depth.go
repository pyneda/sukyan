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
}

type typeChain struct {
	rootField string
	steps     []chainStep
	cyclic    bool
}

type chainStep struct {
	fieldName string
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

	baseURL := a.Definition.BaseURL
	if baseURL == "" {
		baseURL = a.Definition.SourceURL
	}

	auditLog.Info().Str("url", baseURL).Msg("Starting GraphQL depth limit audit")

	client := a.Options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	var testCases []depthTestCase

	if len(a.Definition.RawDefinition) > 0 {
		schema, err := pkgGraphql.NewParser().ParseFromJSON(a.Definition.RawDefinition)
		if err == nil && schema != nil {
			testCases = getSchemaAwareDepthTestCases(schema)
			auditLog.Debug().Int("test_cases", len(testCases)).Msg("Generated schema-aware depth test cases")
		}
	}

	if len(testCases) == 0 {
		testCases = getGenericDepthTestCases()
	}

	var results []depthTestResult
	for _, tc := range testCases {
		if a.Options.Ctx != nil {
			select {
			case <-a.Options.Ctx.Done():
				return
			default:
			}
		}
		result := a.executeDepthTest(baseURL, client, tc)
		results = append(results, result)
	}

	if a.Options.ScanMode.String() == "fuzz" {
		for _, tc := range getCircularFragmentTestCases() {
			if a.Options.Ctx != nil {
				select {
				case <-a.Options.Ctx.Done():
					return
				default:
				}
			}
			result := a.executeDepthTest(baseURL, client, tc)
			results = append(results, result)
		}
	}

	var passed []depthTestResult
	for _, r := range results {
		if r.passed {
			passed = append(passed, r)
		}
	}

	if len(passed) == 0 {
		auditLog.Info().Msg("No depth limit bypass detected")
		return
	}

	maxDepth := 0
	for _, r := range passed {
		if r.depth > maxDepth {
			maxDepth = r.depth
		}
	}

	const minReportableDepth = 8
	if maxDepth > 0 && maxDepth < minReportableDepth {
		auditLog.Info().Int("max_depth", maxDepth).
			Msg("Depth tests passed but within acceptable range, not reporting")
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
	return result
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

func getSchemaAwareDepthTestCases(schema *pkgGraphql.GraphQLSchema) []depthTestCase {
	var testCases []depthTestCase

	testCases = append(testCases, getGenericDepthTestCases()...)

	chains := findDeepTypeChains(schema)
	depths := []int{7, 10, 13}

	limit := min(3, len(chains))

	for _, chain := range chains[:limit] {
		for _, depth := range depths {
			query := buildDeepQueryFromChain(schema, chain, depth)
			if query == "" {
				continue
			}
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

func findDeepTypeChains(schema *pkgGraphql.GraphQLSchema) []typeChain {
	var chains []typeChain

	for _, query := range schema.Queries {
		returnTypeName := getBaseTypeName(query.ReturnType)
		if returnTypeName == "" {
			continue
		}

		visited := make(map[string]bool)
		var steps []chainStep
		found := findChainDFS(schema, returnTypeName, visited, steps, &chains, query.Name)

		if !found {
			if len(steps) > 0 {
				chains = append(chains, typeChain{
					rootField: query.Name,
					steps:     steps,
					cyclic:    false,
				})
			}
		}
	}

	sort.Slice(chains, func(i, j int) bool {
		if chains[i].cyclic != chains[j].cyclic {
			return chains[i].cyclic
		}
		return len(chains[i].steps) > len(chains[j].steps)
	})

	return chains
}

func findChainDFS(schema *pkgGraphql.GraphQLSchema, typeName string, visited map[string]bool, steps []chainStep, chains *[]typeChain, rootField string) bool {
	if visited[typeName] {
		*chains = append(*chains, typeChain{
			rootField: rootField,
			steps:     append([]chainStep(nil), steps...),
			cyclic:    true,
		})
		return true
	}

	typeDef, ok := schema.Types[typeName]
	if !ok {
		return false
	}

	visited[typeName] = true
	defer func() { visited[typeName] = false }()

	found := false
	for _, field := range typeDef.Fields {
		fieldTypeName := getBaseTypeName(field.Type)
		if fieldTypeName == "" {
			continue
		}
		if _, isObject := schema.Types[fieldTypeName]; !isObject {
			continue
		}

		newSteps := append(steps, chainStep{fieldName: field.Name, typeName: fieldTypeName})
		if findChainDFS(schema, fieldTypeName, visited, newSteps, chains, rootField) {
			found = true
		}
	}
	return found
}

func buildDeepQueryFromChain(schema *pkgGraphql.GraphQLSchema, chain typeChain, targetDepth int) string {
	if len(chain.steps) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`{"query":"{%s`, chain.rootField))
	currentDepth := 1

	if chain.cyclic {
		for currentDepth < targetDepth {
			idx := (currentDepth - 1) % len(chain.steps)
			step := chain.steps[idx]
			sb.WriteString(fmt.Sprintf("{%s", step.fieldName))
			currentDepth++
		}

		lastTypeName := chain.steps[(currentDepth-2)%len(chain.steps)].typeName
		scalarField := findScalarField(schema, lastTypeName)
		sb.WriteString(fmt.Sprintf("{%s}", scalarField))

		for i := 0; i < currentDepth-1; i++ {
			sb.WriteString("}")
		}
	} else {
		for i, step := range chain.steps {
			if i >= targetDepth-1 {
				break
			}
			sb.WriteString(fmt.Sprintf("{%s", step.fieldName))
			currentDepth++
		}

		lastTypeName := chain.steps[min(len(chain.steps)-1, targetDepth-2)].typeName
		scalarField := findScalarField(schema, lastTypeName)
		sb.WriteString(fmt.Sprintf("{%s}", scalarField))

		for i := 0; i < currentDepth-1; i++ {
			sb.WriteString("}")
		}
	}

	sb.WriteString(`}"}`)
	return sb.String()
}

func findScalarField(schema *pkgGraphql.GraphQLSchema, typeName string) string {
	typeDef, ok := schema.Types[typeName]
	if !ok {
		return "id"
	}

	preferredNames := []string{"id", "name", "title", "email", "slug"}
	for _, pref := range preferredNames {
		for _, field := range typeDef.Fields {
			if strings.EqualFold(field.Name, pref) && isScalarType(schema, field.Type) {
				return field.Name
			}
		}
	}

	for _, field := range typeDef.Fields {
		if isScalarType(schema, field.Type) {
			return field.Name
		}
	}

	return "__typename"
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
