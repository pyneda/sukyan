package openapi

import (
	"sort"

	"github.com/getkin/kin-openapi/openapi3"
)

// maxCompositionDepth bounds how far a chain of allOf/oneOf/anyOf wrappers is
// followed. Composition nests independently of property nesting, so it needs a
// ceiling of its own on top of the node budget.
const maxCompositionDepth = 12

// NodeBudget bounds how many schema nodes a walk expands. Depth limits and cycle sets
// are both insufficient on their own: a schema referencing the previous level several
// times expands combinatorially with nothing ever being its own ancestor. Each parser
// owns its accounting — the scan-time one also bounds a whole document — so the
// resolver spends against whatever budget the caller already carries.
type NodeBudget interface {
	Spend() bool
}

type nodeBudget struct{ remaining int }

// NewNodeBudget returns a budget for one standalone resolution.
func NewNodeBudget() NodeBudget { return &nodeBudget{remaining: maxSchemaNodes} }

func (b *nodeBudget) Spend() bool {
	if b.remaining <= 0 {
		return false
	}
	b.remaining--
	return true
}

// SchemaView is what a schema actually describes once composition is applied: allOf
// merges every subschema, and oneOf/anyOf resolve to the branch carrying the most
// information. Consumers walk this instead of reading Type, Properties and Items off
// the schema directly, so a composed schema yields the same result wherever it
// appears — at a body root, under a property, or as an array item.
type SchemaView struct {
	// Sources are the schemas the view was composed from, least specific first. They
	// hold the validation keywords in precedence order — JSON Schema allows enum,
	// pattern and the rest beside anyOf, where they constrain whichever branch
	// applies, so the wrapper comes last and wins — and they are the nodes a
	// pointer-keyed cycle guard has to mark before walking any child.
	Sources []*openapi3.Schema
	// Refs are the reference strings the composition passed through. A caller that
	// guards recursion by reference string has to mark all of them: a recursive model
	// reached through a wrapper re-expands at every level otherwise.
	Refs []string

	Type       string
	Properties openapi3.Schemas
	Required   []string
	Items      *openapi3.SchemaRef
	Nullable   bool
}

// IsObject reports whether the schema describes named fields rather than a single
// opaque value, so a request body decomposes into parameters instead of collapsing
// into one payload.
func (v SchemaView) IsObject() bool { return v.Type == "object" }

// ResolveSchema returns the effective view of a schema, following composition.
func ResolveSchema(schema *openapi3.Schema, budget NodeBudget) SchemaView {
	resolver := &compositionResolver{budget: budget, onPath: make(map[*openapi3.Schema]bool)}
	return resolver.resolve(schema, 0)
}

type compositionResolver struct {
	budget NodeBudget
	onPath map[*openapi3.Schema]bool
}

func (r *compositionResolver) resolve(schema *openapi3.Schema, depth int) SchemaView {
	if schema == nil || depth > maxCompositionDepth || r.onPath[schema] || !r.budget.Spend() {
		return SchemaView{}
	}
	r.onPath[schema] = true
	defer delete(r.onPath, schema)

	view := SchemaView{
		Type:     SchemaType(schema),
		Items:    schema.Items,
		Nullable: schema.Nullable,
	}
	var required requiredNames

	if len(schema.Properties) > 0 {
		view.Properties = openapi3.Schemas{}
		for name, ref := range schema.Properties {
			view.Properties[name] = ref
		}
		required.add(schema.Required)
	}

	for _, sub := range schema.AllOf {
		view.absorb(r.branch(sub, depth), &required)
	}

	// The union is consulted only once allOf has been merged. Resolving it first
	// discards the allOf base of a base-plus-variant schema — exactly the fields such
	// specs mark required.
	if len(view.Properties) == 0 {
		branch, nullable := r.union(schema, depth)
		if branch != nil {
			view.absorb(*branch, &required)
		}
		view.Nullable = view.Nullable || nullable
	}

	// A schema carrying properties is an object even when it omits "type": object,
	// which is how several generators emit one. Leaving the type blank sends the whole
	// field as null and discards every property just resolved.
	if view.Type == "" && len(view.Properties) > 0 {
		view.Type = "object"
	}

	view.Required = required.sorted()
	view.Sources = append(view.Sources, schema)
	return view
}

// branch resolves one composition member, carrying its reference string back so a
// caller guarding recursion by reference can see what the wrapper resolved to.
func (r *compositionResolver) branch(ref *openapi3.SchemaRef, depth int) SchemaView {
	if ref == nil || ref.Value == nil {
		return SchemaView{}
	}
	view := r.resolve(ref.Value, depth+1)
	if ref.Ref != "" && len(view.Sources) > 0 {
		view.Refs = append([]string{ref.Ref}, view.Refs...)
	}
	return view
}

// union picks the branch of a oneOf/anyOf that describes the value and reports whether
// a null branch stood beside it. OpenAPI 3.1 spells every optional value this way —
// anyOf:[{type:X},{type:null}] — and the wrapper declares no type of its own, so a
// consumer reading schema.Type alone ends up with no type at all. A union with no null
// branch resolves the same way: one concrete member is a request the endpoint can
// accept, where an unresolved wrapper is not.
//
// A branch carrying properties wins over one that only declares a type. A bare
// {"type": "object"} standing first would otherwise send {} where the next branch held
// every field, and which branch is chosen must not depend on how deeply the schema is
// nested.
func (r *compositionResolver) union(schema *openapi3.Schema, depth int) (*SchemaView, bool) {
	groups := []openapi3.SchemaRefs{schema.AnyOf, schema.OneOf}

	nullable := false
	for _, group := range groups {
		for _, ref := range group {
			if ref != nil && ref.Value != nil && isNullSchema(ref.Value) {
				nullable = true
			}
		}
	}

	for _, group := range groups {
		var typed *SchemaView
		for _, ref := range group {
			if ref == nil || ref.Value == nil || isNullSchema(ref.Value) {
				continue
			}
			resolved := r.branch(ref, depth)
			if len(resolved.Properties) > 0 {
				return &resolved, nullable
			}
			if typed == nil && resolved.Type != "" {
				typed = &resolved
			}
		}
		if typed != nil {
			return typed, nullable
		}
	}
	return nil, nullable
}

// absorb folds a composition member into the view. Everything the view already knows
// from its own declarations wins, so a wrapper never loses its own type or items to a
// branch, and later members override earlier ones on a property name they share.
func (v *SchemaView) absorb(member SchemaView, required *requiredNames) {
	if len(member.Properties) > 0 {
		if v.Properties == nil {
			v.Properties = openapi3.Schemas{}
		}
		for name, ref := range member.Properties {
			v.Properties[name] = ref
		}
	}
	required.add(member.Required)

	if v.Type == "" {
		v.Type = member.Type
	}
	if v.Items == nil {
		v.Items = member.Items
	}
	v.Nullable = v.Nullable || member.Nullable
	v.Sources = append(v.Sources, member.Sources...)
	v.Refs = append(v.Refs, member.Refs...)
}

type requiredNames struct {
	seen  map[string]bool
	names []string
}

func (r *requiredNames) add(names []string) {
	for _, name := range names {
		if r.seen == nil {
			r.seen = make(map[string]bool)
		}
		if !r.seen[name] {
			r.seen[name] = true
			r.names = append(r.names, name)
		}
	}
}

func (r *requiredNames) sorted() []string {
	sort.Strings(r.names)
	return r.names
}
