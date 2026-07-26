package openapi

import (
	"sort"

	"github.com/getkin/kin-openapi/openapi3"
)

const (
	// maxSchemaDepth bounds how deep a schema is walked. Recursive schemas resolve
	// into cyclic pointers, and Go stack exhaustion is a fatal error that no recover
	// can catch, so every schema walk in this package must be bounded.
	maxSchemaDepth = 12
	// maxSchemaNodes bounds the total nodes a single walk expands. Depth alone is not
	// enough: a shallow DAG that references the same subschema several times per level
	// expands combinatorially, so a few kilobytes of spec can flatten into gigabytes.
	maxSchemaNodes = 2000
)

// schemaWalk carries the cycle and size limits across one traversal.
type schemaWalk struct {
	onPath map[*openapi3.Schema]bool
	budget int
}

func newSchemaWalk() *schemaWalk {
	return &schemaWalk{onPath: make(map[*openapi3.Schema]bool), budget: maxSchemaNodes}
}

// schemaToMap flattens a schema into the simplified representation the value
// strategies consume. Cycles are broken by tracking the schemas on the current
// path, so a self-referential type yields a finite map instead of crashing.
func schemaToMap(ref *openapi3.SchemaRef) map[string]interface{} {
	return newSchemaWalk().mapSchema(ref, 0)
}

func (w *schemaWalk) mapSchema(ref *openapi3.SchemaRef, depth int) map[string]interface{} {
	if ref == nil || ref.Value == nil || depth > maxSchemaDepth || w.budget <= 0 {
		return nil
	}
	w.budget--
	schema := ref.Value

	// A 3.1 optional is anyOf:[{type:X},{type:null}]. Resolving it here rather than
	// only in the scan-time parser keeps both consumers of this package agreeing on
	// the same document: without it the generator emits "test" for an integer field
	// and drops its format and enum, so the playground, CLI and report describe a
	// request the scanner would never send.
	if variant, nullable := TypedVariant(schema); variant != nil && !w.onPath[schema] {
		w.onPath[schema] = true
		resolved := w.mapSchema(variant, depth)
		delete(w.onPath, schema)
		if resolved != nil {
			if nullable {
				resolved["nullable"] = true
			}
			for key, value := range mapSchemaOwnKeywords(schema) {
				resolved[key] = value
			}
			return resolved
		}
	}

	result := make(map[string]interface{})
	if declared := SchemaType(schema); declared != "" {
		result["type"] = declared
	}
	if schema.Format != "" {
		result["format"] = schema.Format
	}
	if schema.Example != nil {
		result["example"] = schema.Example
	}
	if schema.Default != nil {
		result["default"] = schema.Default
	}
	if len(schema.Enum) > 0 {
		result["enum"] = schema.Enum
	}
	if schema.Nullable {
		result["nullable"] = true
	}
	if schema.Min != nil {
		result["minimum"] = *schema.Min
	}
	if schema.Max != nil {
		result["maximum"] = *schema.Max
	}
	if schema.MinLength > 0 {
		result["minLength"] = schema.MinLength
	}
	if schema.MaxLength != nil {
		result["maxLength"] = *schema.MaxLength
	}
	if schema.Pattern != "" {
		result["pattern"] = schema.Pattern
	}

	if w.onPath[schema] {
		return result
	}

	// objectSchema tracks this schema itself, so it has to run before the mark below
	// is set: marking first makes its own cycle guard reject the schema being mapped
	// and every object collapses to a property-less {}.
	properties, required, isObject := w.objectSchema(schema, depth)

	w.onPath[schema] = true
	defer delete(w.onPath, schema)

	if len(properties) > 0 {
		mapped := make(map[string]interface{}, len(properties))
		for _, name := range sortedSchemaNames(properties) {
			if propMap := w.mapSchema(properties[name], depth+1); propMap != nil {
				mapped[name] = propMap
			}
		}
		if len(mapped) > 0 {
			result["properties"] = mapped
			result["type"] = "object"
		}
	} else if isObject {
		result["type"] = "object"
	}
	if len(required) > 0 {
		result["required"] = required
	}

	if schema.Items != nil {
		if items := w.mapSchema(schema.Items, depth+1); items != nil {
			result["items"] = items
			if _, ok := result["type"]; !ok {
				result["type"] = "array"
			}
		}
	}

	return result
}

// mapSchemaOwnKeywords returns the validation keywords a schema declares directly.
// JSON Schema allows them beside anyOf/oneOf, where they constrain whichever branch
// applies, so they have to survive the resolution of that wrapper.
func mapSchemaOwnKeywords(schema *openapi3.Schema) map[string]interface{} {
	own := make(map[string]interface{})
	if schema.Format != "" {
		own["format"] = schema.Format
	}
	if schema.Example != nil {
		own["example"] = schema.Example
	}
	if schema.Default != nil {
		own["default"] = schema.Default
	}
	if len(schema.Enum) > 0 {
		own["enum"] = schema.Enum
	}
	if schema.Min != nil {
		own["minimum"] = *schema.Min
	}
	if schema.Max != nil {
		own["maximum"] = *schema.Max
	}
	if schema.MinLength > 0 {
		own["minLength"] = schema.MinLength
	}
	if schema.MaxLength != nil {
		own["maxLength"] = *schema.MaxLength
	}
	if schema.Pattern != "" {
		own["pattern"] = schema.Pattern
	}
	return own
}

func sortedSchemaNames(schemas openapi3.Schemas) []string {
	names := make([]string, 0, len(schemas))
	for name := range schemas {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SchemaType returns the schema's primary type. OpenAPI 3.1 allows a list of types
// such as ["string", "null"]; the meaningful one is the first non-null entry.
func SchemaType(schema *openapi3.Schema) string {
	if schema == nil || schema.Type == nil {
		return ""
	}
	for _, declared := range schema.Type.Slice() {
		if declared != "" && declared != "null" {
			return declared
		}
	}
	return ""
}

// TypedVariant resolves a wrapper that describes its value only through
// anyOf/oneOf to the first branch that carries one, reporting whether a "null"
// branch stood beside it. OpenAPI 3.1 spells every optional value this way —
// anyOf:[{type:X},{type:null}] — and the wrapper declares no type of its own, so
// a consumer that reads schema.Type alone ends up with no type at all. A union
// with no null branch resolves the same way: one concrete member is a request the
// endpoint can accept, where an unresolved wrapper is not.
//
// The whole SchemaRef is returned, not just its value: the branch is usually a
// $ref, and callers guard against recursive models by tracking reference strings.
// Handing back the value alone would strip the only thing that identifies a cycle.
//
// Returns nil when the schema declares a type or properties of its own, since
// then there is nothing to resolve.
func TypedVariant(schema *openapi3.Schema) (*openapi3.SchemaRef, bool) {
	if schema == nil || SchemaType(schema) != "" || len(schema.Properties) > 0 {
		return nil, false
	}

	nullable := false
	for _, group := range []openapi3.SchemaRefs{schema.AnyOf, schema.OneOf} {
		var typed *openapi3.SchemaRef
		for _, ref := range group {
			if ref == nil || ref.Value == nil {
				continue
			}
			if isNullSchema(ref.Value) {
				nullable = true
				continue
			}
			// A branch carrying properties describes an object even when it omits
			// "type": object, which is how several generators emit one. Rejecting it
			// would leave the field untyped here while the body-root resolver in
			// effectiveObjectSchema expands the very same schema.
			if typed == nil && (SchemaType(ref.Value) != "" || len(ref.Value.Properties) > 0) {
				typed = ref
			}
		}
		if typed != nil {
			return typed, nullable
		}
	}
	return nil, false
}

func isNullSchema(schema *openapi3.Schema) bool {
	if schema == nil || schema.Type == nil {
		return false
	}
	declared := schema.Type.Slice()
	if len(declared) == 0 {
		return false
	}
	for _, t := range declared {
		if t != "null" {
			return false
		}
	}
	return true
}

// effectiveObjectSchema resolves an object-like schema into its combined property
// set and required list, following composition: allOf merges every subschema while
// oneOf/anyOf contribute the first object-like variant. Without this, a composed
// request body has no direct properties and serialises to an empty object.
func effectiveObjectSchema(schema *openapi3.Schema, depth int) (openapi3.Schemas, []string, bool) {
	return newSchemaWalk().objectSchema(schema, depth)
}

// objectSchema shares the walk's node budget. A cycle set alone does not bound
// composition: a schema whose allOf references the previous level several times
// expands combinatorially even though nothing is its own ancestor.
func (w *schemaWalk) objectSchema(schema *openapi3.Schema, depth int) (openapi3.Schemas, []string, bool) {
	if schema == nil || depth > maxSchemaDepth || w.budget <= 0 || w.onPath[schema] {
		return nil, nil, false
	}
	w.budget--
	w.onPath[schema] = true
	defer delete(w.onPath, schema)

	properties := openapi3.Schemas{}
	var required []string
	seenRequired := make(map[string]bool)
	addRequired := func(names []string) {
		for _, name := range names {
			if !seenRequired[name] {
				seenRequired[name] = true
				required = append(required, name)
			}
		}
	}

	isObject := SchemaType(schema) == "object"

	if len(schema.Properties) > 0 {
		for name, ref := range schema.Properties {
			properties[name] = ref
		}
		addRequired(schema.Required)
		isObject = true
	}

	for _, sub := range schema.AllOf {
		if sub == nil || sub.Value == nil {
			continue
		}
		subProperties, subRequired, ok := w.objectSchema(sub.Value, depth+1)
		if !ok {
			continue
		}
		for name, ref := range subProperties {
			properties[name] = ref
		}
		addRequired(subRequired)
		isObject = true
	}

	if len(properties) == 0 {
		for _, group := range []openapi3.SchemaRefs{schema.OneOf, schema.AnyOf} {
			for _, sub := range group {
				if sub == nil || sub.Value == nil {
					continue
				}
				subProperties, subRequired, ok := w.objectSchema(sub.Value, depth+1)
				if !ok || len(subProperties) == 0 {
					continue
				}
				for name, ref := range subProperties {
					properties[name] = ref
				}
				addRequired(subRequired)
				isObject = true
				break
			}
			if len(properties) > 0 {
				break
			}
		}
	}

	sort.Strings(required)
	return properties, required, isObject
}

// bodySchema returns the schema describing a request body's chosen media type.
func bodySchema(body *openapi3.RequestBody, contentType string) *openapi3.SchemaRef {
	if body == nil {
		return nil
	}
	mediaType, ok := body.Content[contentType]
	if !ok || mediaType == nil {
		return nil
	}
	return mediaType.Schema
}
