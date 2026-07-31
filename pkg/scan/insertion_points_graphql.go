package scan

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
)

var graphQLOperationKeywords = []string{"query", "mutation", "subscription"}

// isGraphQLBody discriminates a GraphQL envelope from any other JSON body carrying a
// "query" field, which search APIs commonly do. An operation keyword only counts when
// it ends at a GraphQL token boundary — "queryable products" is a search term, not an
// operation — and must be followed by a selection set, which every valid operation has
// and prose such as "query results for shoes" does not.
func isGraphQLBody(jsonData map[string]any) bool {
	queryVal, ok := jsonData["query"]
	if !ok {
		return false
	}
	queryStr, ok := queryVal.(string)
	if !ok {
		return false
	}

	trimmed := strings.TrimSpace(queryStr)
	if strings.HasPrefix(trimmed, "{") {
		return true
	}

	for _, keyword := range graphQLOperationKeywords {
		if !strings.HasPrefix(trimmed, keyword) {
			continue
		}
		rest := trimmed[len(keyword):]
		if rest == "" || !isGraphQLKeywordBoundary(rest[0]) {
			continue
		}
		return strings.Contains(rest, "{")
	}

	return false
}

func isGraphQLKeywordBoundary(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '(', '{', '@':
		return true
	}
	return false
}

func extractGraphQLVariablePoints(path string, variables map[string]any, originalBody string) []InsertionPoint {
	var points []InsertionPoint

	for key, value := range variables {
		currentPath := key
		if path != "" {
			currentPath = path + "." + key
		}

		switch v := value.(type) {
		case map[string]any:
			points = append(points, extractGraphQLVariablePoints(currentPath, v, originalBody)...)

		case []any:
			points = append(points, extractGraphQLArrayPoints(currentPath, v, originalBody)...)

		default:
			valueStr := fmt.Sprintf("%v", v)
			points = append(points, InsertionPoint{
				Type:         InsertionPointTypeGraphQLVariable,
				Name:         currentPath,
				Value:        valueStr,
				ValueType:    lib.GuessDataType(valueStr),
				OriginalData: originalBody,
			})
		}
	}

	return points
}

func extractGraphQLArrayPoints(path string, array []any, originalBody string) []InsertionPoint {
	var points []InsertionPoint

	for i, item := range array {
		currentPath := fmt.Sprintf("%s[%d]", path, i)

		switch v := item.(type) {
		case map[string]any:
			points = append(points, extractGraphQLVariablePoints(currentPath, v, originalBody)...)

		case []any:
			points = append(points, extractGraphQLArrayPoints(currentPath, v, originalBody)...)

		default:
			valueStr := fmt.Sprintf("%v", v)
			points = append(points, InsertionPoint{
				Type:         InsertionPointTypeGraphQLVariable,
				Name:         currentPath,
				Value:        valueStr,
				ValueType:    lib.GuessDataType(valueStr),
				OriginalData: originalBody,
			})
		}
	}

	return points
}

func modifyGraphQLVariables(body []byte, builders []InsertionPointBuilder) ([]byte, error) {
	var fullBody map[string]any
	if err := json.Unmarshal(body, &fullBody); err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL body: %w", err)
	}

	if _, hasQuery := fullBody["query"]; !hasQuery {
		return nil, fmt.Errorf("GraphQL body missing required 'query' field")
	}

	variables, ok := fullBody["variables"].(map[string]any)
	if !ok {
		variables = make(map[string]any)
	}

	for _, builder := range builders {
		setNestedValue(variables, builder.Point.Name, builder.Payload)
	}

	fullBody["variables"] = variables
	return json.Marshal(fullBody)
}

func setNestedValue(obj map[string]any, path string, payload string) {
	parts := strings.SplitN(path, ".", 2)
	key := parts[0]

	if idx, name, isArray := parseGraphQLArrayAccess(key); isArray {
		arr, ok := obj[name].([]any)
		if !ok || idx >= len(arr) {
			log.Warn().Str("path", path).Int("index", idx).Msg("GraphQL variable array index out of bounds during injection")
			return
		}

		if len(parts) == 1 {
			arr[idx] = coercePayloadType(arr[idx], payload)
			obj[name] = arr
			return
		}

		if nested, ok := arr[idx].(map[string]any); ok {
			setNestedValue(nested, parts[1], payload)
			arr[idx] = nested
			obj[name] = arr
		}
		return
	}

	if len(parts) == 1 {
		obj[key] = coercePayloadType(obj[key], payload)
		return
	}

	nested, ok := obj[key].(map[string]any)
	if !ok {
		log.Warn().Str("path", path).Str("key", key).Msg("GraphQL variable path not found during injection")
		return
	}
	setNestedValue(nested, parts[1], payload)
}

func coercePayloadType(original any, payload string) any {
	if original == nil {
		return payload
	}
	switch original.(type) {
	case float64:
		if v, err := strconv.ParseFloat(payload, 64); err == nil {
			return v
		}
	case bool:
		if payload == "true" {
			return true
		}
		if payload == "false" {
			return false
		}
	}
	return payload
}

func parseGraphQLArrayAccess(s string) (int, string, bool) {
	bracketIdx := strings.Index(s, "[")
	if bracketIdx == -1 || !strings.HasSuffix(s, "]") {
		return 0, "", false
	}

	name := s[:bracketIdx]
	idxStr := s[bracketIdx+1 : len(s)-1]
	idx, err := strconv.Atoi(idxStr)
	if err != nil {
		return 0, "", false
	}

	return idx, name, true
}

// Inline arguments are the only injectable surface a crawled GraphQL request exposes
// when the operation embeds its values in the document instead of passing variables.
// Every scalar leaf gets its own point, including leaves nested inside list and
// input-object literals, addressed by the byte span of its literal inside the decoded
// query string - not inside OriginalData, which is the JSON envelope the query was
// extracted from and never the string the rewriter edits.
//
// Addressing by span rather than by argument name is what keeps repeated arguments
// (`{ a(id: 1) b(id: 2) }`) independently targetable, and what keeps a payload out of
// the operation's variable definition list when an argument shares a variable's name.
const (
	// maxGraphQLInlineArgPoints bounds a single operation's contribution. Every point
	// costs a full payload sweep, so a wide input object would otherwise multiply that
	// request's traffic without bound.
	maxGraphQLInlineArgPoints = 25
	maxGraphQLValueDepth      = 10
)

var (
	graphQLNumberLiteralPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)
	graphQLNamePattern          = regexp.MustCompile(`^[_A-Za-z][_0-9A-Za-z]*$`)
)

type graphQLLiteralKind int

const (
	graphQLLiteralString graphQLLiteralKind = iota
	graphQLLiteralNumber
	graphQLLiteralBoolean
	graphQLLiteralNull
	graphQLLiteralEnum
)

// dataType reports the type implied by the literal's syntactic form rather than by
// its text: `id: "10"` is a string argument even though its content parses as an
// integer, and only the form tells a launch condition which payloads can reach the
// resolver behind it.
func (k graphQLLiteralKind) dataType(value string) lib.DataType {
	switch k {
	case graphQLLiteralNumber:
		return lib.GuessDataType(value)
	case graphQLLiteralBoolean:
		return lib.TypeBoolean
	case graphQLLiteralNull:
		return lib.TypeNull
	default:
		return lib.TypeString
	}
}

type graphQLArgCandidate struct {
	field string
	path  string
	value string
	kind  graphQLLiteralKind
	span  InsertionPointSpan
}

func (c graphQLArgCandidate) qualifiedName() string {
	if c.field == "" {
		return c.path
	}
	return c.field + "." + c.path
}

func extractGraphQLInlineArgPoints(query string, originalBody string) []InsertionPoint {
	return nameGraphQLArgPoints(collectGraphQLArgCandidates(query), originalBody)
}

func collectGraphQLArgCandidates(query string) []graphQLArgCandidate {
	var candidates []graphQLArgCandidate
	i, n := 0, len(query)

	for i < n {
		switch query[i] {
		case '(':
			field := graphQLNameBefore(query, i)
			i++
			collectGraphQLArgListCandidates(query, &i, n, field, &candidates)
		case '"':
			skipGraphQLString(query, &i, n)
		default:
			i++
		}
	}

	return candidates
}

// graphQLNameBefore returns the name an argument list is attached to, which is what
// disambiguates arguments repeated across sibling fields. An alias wins over the field
// it renames: valid GraphQL requires one whenever sibling fields differ in their
// arguments, so it is what identifies the call a finding belongs to.
func graphQLNameBefore(s string, parenIndex int) string {
	end := parenIndex
	for end > 0 && isGraphQLSpace(s[end-1]) {
		end--
	}
	start := end
	for start > 0 && isNameChar(s[start-1]) {
		start--
	}
	if start == end {
		return ""
	}

	i := start
	for i > 0 && isGraphQLSpace(s[i-1]) {
		i--
	}
	if i == 0 || s[i-1] != ':' {
		return s[start:end]
	}

	i--
	for i > 0 && isGraphQLSpace(s[i-1]) {
		i--
	}
	aliasEnd := i
	for i > 0 && isNameChar(s[i-1]) {
		i--
	}
	if i == aliasEnd {
		return s[start:end]
	}
	return s[i:aliasEnd]
}

// isVariableDefinitionList distinguishes an operation's variable definition list -
// `query GetUsers($role: String)` - from a field's argument list. The two are
// syntactically identical up to the opening parenthesis, and rewriting inside the
// former makes the whole document unparseable while leaving the real sink untested.
func isVariableDefinitionList(query string, pos int, n int) bool {
	for pos < n && isGraphQLSpace(query[pos]) {
		pos++
	}
	return pos < n && query[pos] == '$'
}

func skipParenthesized(query string, pos *int, n int) {
	depth := 1
	for *pos < n && depth > 0 {
		switch query[*pos] {
		case '(':
			depth++
		case ')':
			depth--
		case '"':
			skipGraphQLString(query, pos, n)
			continue
		}
		*pos++
	}
}

func collectGraphQLArgListCandidates(query string, pos *int, n int, field string, out *[]graphQLArgCandidate) {
	if isVariableDefinitionList(query, *pos, n) {
		skipParenthesized(query, pos, n)
		return
	}

	for *pos < n {
		skipWhitespace(query, pos, n)
		if *pos >= n {
			return
		}
		if query[*pos] == ')' {
			*pos++
			return
		}

		name := readArgName(query, pos, n)
		if name == "" {
			*pos++
			continue
		}

		skipWhitespace(query, pos, n)
		if *pos >= n || query[*pos] != ':' {
			continue
		}
		*pos++
		skipWhitespace(query, pos, n)

		collectGraphQLValueCandidates(query, pos, n, field, name, 0, out)

		skipWhitespace(query, pos, n)
		if *pos < n && query[*pos] == ',' {
			*pos++
		}
	}
}

// collectGraphQLValueCandidates walks a single argument value, descending into list
// and input-object literals so their scalar leaves stay individually addressable, and
// names each leaf with the same path convention extractGraphQLVariablePoints uses.
func collectGraphQLValueCandidates(query string, pos *int, n int, field string, path string, depth int, out *[]graphQLArgCandidate) {
	if *pos >= n {
		return
	}
	if depth > maxGraphQLValueDepth {
		skipGraphQLValue(query, pos, n)
		return
	}

	switch query[*pos] {
	case '{':
		*pos++
		for *pos < n {
			skipWhitespace(query, pos, n)
			if *pos >= n || query[*pos] == '}' {
				break
			}

			name := readArgName(query, pos, n)
			if name == "" {
				*pos++
				continue
			}

			skipWhitespace(query, pos, n)
			if *pos >= n || query[*pos] != ':' {
				continue
			}
			*pos++
			skipWhitespace(query, pos, n)

			collectGraphQLValueCandidates(query, pos, n, field, path+"."+name, depth+1, out)

			skipWhitespace(query, pos, n)
			if *pos < n && query[*pos] == ',' {
				*pos++
			}
		}
		if *pos < n {
			*pos++
		}

	case '[':
		*pos++
		for index := 0; *pos < n; index++ {
			skipWhitespace(query, pos, n)
			if *pos >= n || query[*pos] == ']' {
				break
			}

			collectGraphQLValueCandidates(query, pos, n, field, fmt.Sprintf("%s[%d]", path, index), depth+1, out)

			skipWhitespace(query, pos, n)
			if *pos < n && query[*pos] == ',' {
				*pos++
			}
		}
		if *pos < n {
			*pos++
		}

	case '$':
		skipUntilArgBoundary(query, pos, n)

	default:
		start := *pos
		value, ok := readGraphQLScalar(query, pos, n)
		if !ok {
			return
		}
		*out = append(*out, graphQLArgCandidate{
			field: field,
			path:  path,
			value: value,
			kind:  graphQLLiteralKindAt(query, start),
			span:  InsertionPointSpan{Start: start, End: *pos, Valid: true},
		})
	}
}

// nameGraphQLArgPoints keeps the bare argument path while it is unambiguous, so
// templates gating on insertion_point_name still match, and qualifies it with the
// enclosing field only when repeated siblings would otherwise be indistinguishable.
func nameGraphQLArgPoints(candidates []graphQLArgCandidate, originalBody string) []InsertionPoint {
	pathCount := make(map[string]int, len(candidates))
	qualifiedCount := make(map[string]int, len(candidates))
	for _, c := range candidates {
		pathCount[c.path]++
		qualifiedCount[c.qualifiedName()]++
	}

	occurrence := make(map[string]int, len(candidates))
	points := make([]InsertionPoint, 0, len(candidates))
	for _, c := range candidates {
		name := c.path
		if pathCount[c.path] > 1 {
			name = c.qualifiedName()
			if qualifiedCount[name] > 1 {
				occurrence[name]++
				name = fmt.Sprintf("%s[%d]", name, occurrence[name])
			}
		}

		points = append(points, InsertionPoint{
			Type:         InsertionPointTypeGraphQLInlineArg,
			Name:         name,
			Value:        c.value,
			ValueType:    c.kind.dataType(c.value),
			OriginalData: originalBody,
			Span:         c.span,
		})

		if len(points) >= maxGraphQLInlineArgPoints {
			break
		}
	}

	return points
}

func skipWhitespace(s string, pos *int, n int) {
	for *pos < n && isGraphQLSpace(s[*pos]) {
		*pos++
	}
}

func isGraphQLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func readArgName(s string, pos *int, n int) string {
	start := *pos
	for *pos < n && isNameChar(s[*pos]) {
		*pos++
	}
	if *pos == start {
		return ""
	}
	return s[start:*pos]
}

func isNameChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

func isGraphQLValueTerminator(c byte) bool {
	return isGraphQLSpace(c) || c == ')' || c == ',' || c == '}' || c == ']'
}

func skipUntilArgBoundary(s string, pos *int, n int) {
	for *pos < n && !isGraphQLValueTerminator(s[*pos]) {
		*pos++
	}
}

// readGraphQLScalar consumes one scalar literal and reports whether it produced a
// rewritable value. Block strings are consumed but rejected: they have no single
// quoted span a payload can replace without corrupting the document.
func readGraphQLScalar(s string, pos *int, n int) (string, bool) {
	if strings.HasPrefix(s[*pos:], `"""`) {
		skipGraphQLString(s, pos, n)
		return "", false
	}
	if s[*pos] == '"' {
		return readQuotedString(s, pos, n), true
	}

	start := *pos
	for *pos < n && !isGraphQLValueTerminator(s[*pos]) {
		*pos++
	}
	if *pos == start {
		*pos++
		return "", false
	}
	return s[start:*pos], true
}

func readQuotedString(s string, pos *int, n int) string {
	*pos++
	var buf strings.Builder
	for *pos < n && s[*pos] != '"' {
		if s[*pos] == '\\' && *pos+1 < n {
			*pos++
			buf.WriteByte(s[*pos])
		} else {
			buf.WriteByte(s[*pos])
		}
		*pos++
	}
	if *pos < n {
		*pos++
	}
	return buf.String()
}

func skipGraphQLString(s string, pos *int, n int) {
	if strings.HasPrefix(s[*pos:], `"""`) {
		*pos += 3
		for *pos < n && !strings.HasPrefix(s[*pos:], `"""`) {
			*pos++
		}
		if *pos < n {
			*pos += 3
		}
		return
	}

	*pos++
	for *pos < n && s[*pos] != '"' {
		if s[*pos] == '\\' {
			*pos++
		}
		*pos++
	}
	if *pos < n {
		*pos++
	}
}

func skipGraphQLValue(s string, pos *int, n int) {
	if *pos >= n {
		return
	}
	switch s[*pos] {
	case '"':
		skipGraphQLString(s, pos, n)
	case '[', '{':
		skipBracketedValue(s, pos, n)
	case '$':
		skipUntilArgBoundary(s, pos, n)
	default:
		skipUntilArgBoundary(s, pos, n)
	}
}

func skipBracketedValue(s string, pos *int, n int) {
	open := s[*pos]
	closing := byte('}')
	if open == '[' {
		closing = ']'
	}

	depth := 1
	*pos++
	for *pos < n && depth > 0 {
		switch s[*pos] {
		case open:
			depth++
		case closing:
			depth--
		case '"':
			skipGraphQLString(s, pos, n)
			continue
		}
		*pos++
	}
}

func graphQLLiteralKindAt(s string, pos int) graphQLLiteralKind {
	if pos >= len(s) {
		return graphQLLiteralEnum
	}

	c := s[pos]
	if c == '"' {
		return graphQLLiteralString
	}
	if c == '-' || (c >= '0' && c <= '9') {
		return graphQLLiteralNumber
	}

	rest := s[pos:]
	switch {
	case hasGraphQLKeyword(rest, "true"), hasGraphQLKeyword(rest, "false"):
		return graphQLLiteralBoolean
	case hasGraphQLKeyword(rest, "null"):
		return graphQLLiteralNull
	}
	return graphQLLiteralEnum
}

func hasGraphQLKeyword(s string, keyword string) bool {
	if !strings.HasPrefix(s, keyword) {
		return false
	}
	return len(s) == len(keyword) || !isNameChar(s[len(keyword)])
}

func modifyGraphQLInlineArg(body []byte, builders []InsertionPointBuilder) ([]byte, error) {
	var fullBody map[string]any
	if err := json.Unmarshal(body, &fullBody); err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL body: %w", err)
	}

	queryStr, ok := fullBody["query"].(string)
	if !ok {
		return nil, fmt.Errorf("GraphQL body missing required 'query' field")
	}

	fullBody["query"] = applyGraphQLInlineArgPayloads(queryStr, builders)
	return json.Marshal(fullBody)
}

// applyGraphQLInlineArgPayloads splices each payload into its own literal's byte span,
// last span first so a completed write never shifts the offsets of the pending ones.
func applyGraphQLInlineArgPayloads(query string, builders []InsertionPointBuilder) string {
	ordered := make([]InsertionPointBuilder, len(builders))
	copy(ordered, builders)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Point.Span.Start > ordered[j].Point.Span.Start
	})

	applied := make([]InsertionPointSpan, 0, len(ordered))
	for _, builder := range ordered {
		span := builder.Point.Span
		if !span.Valid || span.Start < 0 || span.End < span.Start || span.End > len(query) {
			log.Warn().Str("point", builder.Point.Name).Msg("Skipping GraphQL inline argument without a usable span")
			continue
		}
		if overlapsAppliedSpan(applied, span) {
			log.Debug().Str("point", builder.Point.Name).Msg("Skipping GraphQL inline argument overlapping an earlier one")
			continue
		}

		literal := renderGraphQLLiteral(query[span.Start:span.End], builder.Payload)
		query = query[:span.Start] + literal + query[span.End:]
		applied = append(applied, span)
	}

	return query
}

// renderGraphQLLiteral emits the payload in the syntactic form the original literal
// used. A bare payload where a string was, or a quoted payload where a bare
// number/enum/boolean was, yields a document the server rejects before any resolver
// runs, so the probe would test nothing. Where the payload cannot take the original
// form the fallback is a quoted string, which stays parseable and is what ID-typed
// arguments - numeric-looking but string-accepting - need.
func renderGraphQLLiteral(original string, payload string) string {
	switch graphQLLiteralKindAt(original, 0) {
	case graphQLLiteralNumber:
		if graphQLNumberLiteralPattern.MatchString(payload) {
			return payload
		}
	case graphQLLiteralBoolean, graphQLLiteralNull:
		if payload == "true" || payload == "false" || payload == "null" {
			return payload
		}
	case graphQLLiteralEnum:
		if isGraphQLEnumValue(payload) {
			return payload
		}
	}
	return quoteGraphQLString(payload)
}

func isGraphQLEnumValue(s string) bool {
	if s == "true" || s == "false" || s == "null" {
		return false
	}
	return graphQLNamePattern.MatchString(s)
}

func quoteGraphQLString(payload string) string {
	var buf strings.Builder
	buf.Grow(len(payload) + 2)
	buf.WriteByte('"')
	for _, r := range payload {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&buf, `\u%04X`, r)
				continue
			}
			buf.WriteRune(r)
		}
	}
	buf.WriteByte('"')
	return buf.String()
}
