package passive

import (
	"strings"

	"github.com/BishopFox/jsluice"
)

// OriginValidation classifies how a message handler checks event.origin.
type OriginValidation int

const (
	// OriginValidationUnknown means a check exists but could not be resolved,
	// typically because it is a call into a helper. Never reported: proving it
	// wrong would need whole-program analysis.
	OriginValidationUnknown OriginValidation = iota
	// OriginValidationNone means the handler never reads origin at all.
	OriginValidationNone
	// OriginValidationStrong means an exact comparison or allowlist membership.
	OriginValidationStrong
	// OriginValidationSubstring matches the needle anywhere in the origin.
	OriginValidationSubstring
	// OriginValidationPrefix admits https://trusted.com.evil.com.
	OriginValidationPrefix
	// OriginValidationSuffix admits https://eviltrusted.com.
	OriginValidationSuffix
	// OriginValidationUnanchoredRegex is a substring match in disguise.
	OriginValidationUnanchoredRegex
	// OriginValidationNullAccepted trusts the literal "null" origin, which any
	// sandboxed iframe or data: URI can produce.
	OriginValidationNullAccepted
)

func (v OriginValidation) String() string {
	switch v {
	case OriginValidationNone:
		return "none"
	case OriginValidationStrong:
		return "strong"
	case OriginValidationSubstring:
		return "substring"
	case OriginValidationPrefix:
		return "prefix"
	case OriginValidationSuffix:
		return "suffix"
	case OriginValidationUnanchoredRegex:
		return "unanchored-regex"
	case OriginValidationNullAccepted:
		return "null-origin-accepted"
	default:
		return "unknown"
	}
}

// Weak reports whether this classification is worth an issue on its own.
func (v OriginValidation) Weak() bool {
	switch v {
	case OriginValidationSubstring, OriginValidationPrefix, OriginValidationSuffix,
		OriginValidationUnanchoredRegex, OriginValidationNullAccepted:
		return true
	}
	return false
}

// weakness orders classifications so the loosest check in a handler wins: an
// attacker only needs one permissive path into the allow branch.
func (v OriginValidation) weakness() int {
	switch v {
	case OriginValidationSubstring, OriginValidationUnanchoredRegex:
		return 1
	case OriginValidationNullAccepted:
		return 2
	case OriginValidationPrefix, OriginValidationSuffix:
		return 3
	}
	return 100
}

// MessageHandlerFinding is one registered message handler and how it validates.
type MessageHandlerFinding struct {
	Validation OriginValidation
	Evidence   string
	EventParam string
	Sinks      []string
}

const messageListenerQuery = `[
	(call_expression
		function: (member_expression property: (property_identifier) @method)
		arguments: (arguments) @args)
	(call_expression
		function: (identifier) @method
		arguments: (arguments) @args)
]`

const messagePropertyQuery = `[
	(assignment_expression
		left: (member_expression property: (property_identifier) @prop)
		right: [(function_expression) (arrow_function)] @handler)
	(assignment_expression
		left: (identifier) @prop
		right: [(function_expression) (arrow_function)] @handler)
]`

// registrationMethods are the calls that can attach a message listener.
var registrationMethods = map[string]bool{
	"addEventListener": true,
	"attachEvent":      true,
	"on":               true, // jQuery and friends
}

func analyzeMessageHandlers(analyzer *jsluice.Analyzer) []MessageHandlerFinding {
	var findings []MessageHandlerFinding

	analyzer.QueryMulti(messageListenerQuery, func(res jsluice.QueryResult) {
		method, args := res["method"], res["args"]
		if method == nil || args == nil || !registrationMethods[method.Content()] {
			return
		}

		params := args.NamedChildren()
		if len(params) < 2 || params[0].Type() != "string" {
			return
		}
		if !isMessageEventName(params[0].RawString()) {
			return
		}

		if handler := asFunction(params[1]); handler != nil {
			findings = append(findings, analyzeHandler(handler, analyzer.RootNode()))
		}
	})

	analyzer.QueryMulti(messagePropertyQuery, func(res jsluice.QueryResult) {
		prop, handler := res["prop"], res["handler"]
		if prop == nil || handler == nil || !isMessageEventName(prop.Content()) {
			return
		}
		findings = append(findings, analyzeHandler(handler, analyzer.RootNode()))
	})

	return findings
}

func isMessageEventName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "message", "onmessage":
		return true
	}
	return false
}

func asFunction(n *jsluice.Node) *jsluice.Node {
	if n == nil {
		return nil
	}
	switch n.Type() {
	case "function_expression", "arrow_function", "function_declaration", "function":
		return n
	}
	return nil
}

// analyzeHandler binds the event parameter and classifies every origin read.
func analyzeHandler(handler *jsluice.Node, root *jsluice.Node) MessageHandlerFinding {
	finding := MessageHandlerFinding{Validation: OriginValidationNone}

	eventParam, destructured := handlerEventBindings(handler)
	finding.EventParam = eventParam

	body := handler.ChildByFieldName("body")
	if body == nil || !body.IsValid() {
		return finding
	}

	// Names inside the body that hold the origin: either destructured in the
	// parameter list, or destructured from the event object in the body.
	originAliases := map[string]bool{}
	for _, name := range destructured {
		originAliases[name] = true
	}
	collectBodyDestructuring(body, eventParam, originAliases)

	// Only reads that actually gate the handler count. Logging or reporting the
	// origin is common and must not be mistaken for validating it, otherwise a
	// handler with no check at all reads as "checked, but unresolvable".
	var guards []*jsluice.Node
	for _, node := range collectOriginReads(body, eventParam, originAliases) {
		if inGuardPosition(node) {
			guards = append(guards, node)
		}
	}

	if len(guards) == 0 {
		finding.Sinks = collectSinks(body)
		return finding
	}

	best := OriginValidationUnknown
	bestSet := false
	for _, node := range guards {
		got, evidence := classifyOriginGuard(node, root)
		if got == OriginValidationUnknown {
			continue
		}
		if !bestSet || got.weakness() < best.weakness() {
			best, bestSet = got, true
			finding.Evidence = evidence
		}
	}

	if !bestSet {
		// origin is read but every guard was unresolvable
		finding.Validation = OriginValidationUnknown
	} else {
		finding.Validation = best
	}
	finding.Sinks = collectSinks(body)
	return finding
}

// handlerEventBindings returns the event parameter name, or the names bound by
// destructuring it directly in the parameter list.
func handlerEventBindings(handler *jsluice.Node) (string, []string) {
	var first *jsluice.Node

	if params := handler.ChildByFieldName("parameters"); params != nil && params.IsValid() {
		if kids := params.NamedChildren(); len(kids) > 0 {
			first = kids[0]
		}
	} else if p := handler.ChildByFieldName("parameter"); p != nil && p.IsValid() {
		first = p
	}

	if first == nil {
		return "", nil
	}

	switch first.Type() {
	case "identifier":
		return first.Content(), nil
	case "object_pattern":
		return "", objectPatternNames(first)
	}
	return "", nil
}

// objectPatternNames returns the local names a destructuring pattern binds for
// the origin property, honouring renames such as {origin: o}.
func objectPatternNames(pattern *jsluice.Node) []string {
	var names []string
	for _, child := range pattern.NamedChildren() {
		switch child.Type() {
		case "shorthand_property_identifier_pattern", "shorthand_property_identifier":
			if child.Content() == "origin" {
				names = append(names, "origin")
			}
		case "pair_pattern", "pair":
			key := child.ChildByFieldName("key")
			value := child.ChildByFieldName("value")
			if key != nil && value != nil && key.Content() == "origin" {
				names = append(names, value.Content())
			}
		}
	}
	return names
}

// collectBodyDestructuring finds `const {origin} = event` inside the body.
func collectBodyDestructuring(body *jsluice.Node, eventParam string, aliases map[string]bool) {
	if eventParam == "" {
		return
	}
	body.Query(`(variable_declarator name: (object_pattern) value: (identifier)) @decl`, func(decl *jsluice.Node) {
		value := decl.ChildByFieldName("value")
		name := decl.ChildByFieldName("name")
		if value == nil || name == nil || value.Content() != eventParam {
			return
		}
		for _, bound := range objectPatternNames(name) {
			aliases[bound] = true
		}
	})
}

// collectOriginReads returns every node in the body that evaluates to the
// message origin: `event.origin` member reads and destructured aliases.
func collectOriginReads(body *jsluice.Node, eventParam string, aliases map[string]bool) []*jsluice.Node {
	var nodes []*jsluice.Node

	if eventParam != "" {
		body.Query(`(member_expression) @m`, func(m *jsluice.Node) {
			object := m.ChildByFieldName("object")
			property := m.ChildByFieldName("property")
			if object == nil || property == nil {
				return
			}
			if object.Content() == eventParam && property.Content() == "origin" {
				nodes = append(nodes, m)
			}
		})
	}

	if len(aliases) > 0 {
		body.Query(`(identifier) @id`, func(id *jsluice.Node) {
			if !aliases[id.Content()] {
				return
			}
			// skip the binding site itself
			if parent := id.Parent(); parent != nil {
				switch parent.Type() {
				case "object_pattern", "pair_pattern", "formal_parameters":
					return
				}
			}
			nodes = append(nodes, id)
		})
	}

	return nodes
}

// inGuardPosition reports whether an origin read actually controls whether the
// handler proceeds, as opposed to being logged or reported. The walk stops at
// the enclosing function so a read never inherits an outer scope's condition.
func inGuardPosition(origin *jsluice.Node) bool {
	for cur := origin.Parent(); cur != nil && cur.IsValid(); cur = cur.Parent() {
		switch cur.Type() {
		case "function_expression", "arrow_function", "function_declaration", "program":
			return false
		case "if_statement", "ternary_expression", "while_statement", "do_statement",
			"switch_statement", "return_statement":
			return true
		case "variable_declarator", "assignment_expression":
			// captured into a name that may gate later; treat as a guard so an
			// unresolved check is never mistaken for an absent one
			return true
		case "unary_expression":
			if op := cur.ChildByFieldName("operator"); op != nil && op.Content() == "!" {
				return true
			}
		case "binary_expression":
			if op := cur.ChildByFieldName("operator"); op != nil && isGuardOperator(op.Content()) {
				return true
			}
		}
	}
	return false
}

func isGuardOperator(op string) bool {
	switch op {
	case "===", "!==", "==", "!=", "&&", "||", "<", ">", "<=", ">=":
		return true
	}
	return false
}

// resolveBinding finds the value assigned to a top-level name. Real code hoists
// origin allowlists and patterns into constants rather than inlining them.
func resolveBinding(root *jsluice.Node, name string) *jsluice.Node {
	if root == nil || name == "" {
		return nil
	}
	var found *jsluice.Node
	root.Query(`(variable_declarator) @decl`, func(decl *jsluice.Node) {
		if found != nil {
			return
		}
		n := decl.ChildByFieldName("name")
		v := decl.ChildByFieldName("value")
		if n != nil && v != nil && n.Type() == "identifier" && n.Content() == name {
			found = v
		}
	})
	return found
}

// classifyOriginGuard walks up from an origin read to the comparison or call
// that guards on it.
func classifyOriginGuard(origin *jsluice.Node, root *jsluice.Node) (OriginValidation, string) {
	parent := origin.Parent()
	if parent == nil || !parent.IsValid() {
		return OriginValidationUnknown, ""
	}

	switch parent.Type() {
	case "member_expression":
		// origin.<method>(...) - the receiver is the origin
		if object := parent.ChildByFieldName("object"); object == nil || object.Content() != origin.Content() {
			return OriginValidationUnknown, ""
		}
		property := parent.ChildByFieldName("property")
		call := parent.Parent()
		if property == nil || call == nil || call.Type() != "call_expression" {
			return OriginValidationUnknown, ""
		}
		return classifyOriginMethod(property.Content(), call, root), strings.TrimSpace(call.Content())

	case "arguments":
		// something(origin) - either allowlist membership or an opaque helper
		call := parent.Parent()
		if call == nil || call.Type() != "call_expression" {
			return OriginValidationUnknown, ""
		}
		return classifyOriginAsArgument(call, root), strings.TrimSpace(call.Content())

	case "binary_expression":
		return classifyOriginComparison(parent, origin), strings.TrimSpace(parent.Content())
	}

	return OriginValidationUnknown, ""
}

// classifyOriginMethod handles origin.<method>(...) calls.
func classifyOriginMethod(method string, call *jsluice.Node, root *jsluice.Node) OriginValidation {
	switch method {
	case "indexOf", "includes", "search", "lastIndexOf", "contains":
		return OriginValidationSubstring
	case "startsWith":
		return OriginValidationPrefix
	case "endsWith":
		return OriginValidationSuffix
	case "match", "matches":
		if args := call.ChildByFieldName("arguments"); args != nil {
			if kids := args.NamedChildren(); len(kids) > 0 {
				if v := classifyRegexNode(kids[0], root); v != OriginValidationUnknown {
					return v
				}
			}
		}
		return OriginValidationSubstring
	}
	return OriginValidationUnknown
}

// classifyOriginAsArgument handles calls that take the origin as an argument.
func classifyOriginAsArgument(call *jsluice.Node, root *jsluice.Node) OriginValidation {
	fn := call.ChildByFieldName("function")
	if fn == nil || fn.Type() != "member_expression" {
		// bare helper(origin): unresolvable, never claim there is no check
		return OriginValidationUnknown
	}

	property := fn.ChildByFieldName("property")
	object := fn.ChildByFieldName("object")
	if property == nil {
		return OriginValidationUnknown
	}

	switch property.Content() {
	case "test", "exec":
		return classifyRegexNode(object, root)
	case "includes", "indexOf", "contains", "has":
		// allowlist.includes(origin): membership in a set, the safe direction -
		// unless the set itself admits the null origin.
		if allowlistAdmitsNullOrigin(object, root) {
			return OriginValidationNullAccepted
		}
		return OriginValidationStrong
	}
	return OriginValidationUnknown
}

// classifyRegexNode grades a regex used against the origin, resolving it through
// a variable binding first: real code hoists patterns into a constant.
func classifyRegexNode(node *jsluice.Node, root *jsluice.Node) OriginValidation {
	if node == nil || !node.IsValid() {
		return OriginValidationUnknown
	}
	if node.Type() == "identifier" {
		node = resolveBinding(root, node.Content())
		if node == nil || !node.IsValid() {
			return OriginValidationUnknown
		}
	}
	if node.Type() != "regex" {
		return OriginValidationUnknown
	}
	if regexAnchored(node.Content()) {
		return OriginValidationStrong
	}
	return OriginValidationUnanchoredRegex
}

// allowlistAdmitsNullOrigin reports whether an origin allowlist contains the
// literal "null", which any sandboxed iframe or data: URI can present.
func allowlistAdmitsNullOrigin(node *jsluice.Node, root *jsluice.Node) bool {
	if node == nil || !node.IsValid() {
		return false
	}
	if node.Type() == "identifier" {
		node = resolveBinding(root, node.Content())
		if node == nil || !node.IsValid() {
			return false
		}
	}
	if node.Type() != "array" {
		return false
	}
	for _, element := range node.NamedChildren() {
		if element.Type() == "string" && element.RawString() == "null" {
			return true
		}
	}
	return false
}

// classifyOriginComparison handles origin === "https://trusted.example".
func classifyOriginComparison(expr *jsluice.Node, origin *jsluice.Node) OriginValidation {
	operator := expr.ChildByFieldName("operator")
	if operator == nil {
		return OriginValidationUnknown
	}
	op := operator.Content()
	if op != "===" && op != "==" && op != "!==" && op != "!=" {
		return OriginValidationUnknown
	}

	left := expr.ChildByFieldName("left")
	right := expr.ChildByFieldName("right")
	other := right
	if right != nil && right.Content() == origin.Content() {
		other = left
	}
	if other == nil {
		return OriginValidationUnknown
	}

	if other.Type() == "string" && other.RawString() == "null" {
		if op == "===" || op == "==" {
			return OriginValidationNullAccepted
		}
		// rejecting "null" is not by itself an origin allowlist
		return OriginValidationUnknown
	}

	return OriginValidationStrong
}

// regexAnchored reports whether a JavaScript regex literal is pinned at both
// ends. Anything else matches a substring of the origin.
func regexAnchored(literal string) bool {
	pattern := strings.TrimPrefix(literal, "/")
	if i := strings.LastIndex(pattern, "/"); i >= 0 {
		pattern = pattern[:i]
	}
	return strings.HasPrefix(pattern, "^") && strings.HasSuffix(pattern, "$")
}

// collectSinks reports which dangerous sinks appear inside the handler body.
func collectSinks(body *jsluice.Node) []string {
	found := FindDOMXSSSinks(body.Content())
	if len(found) == 0 {
		return nil
	}
	return found
}
