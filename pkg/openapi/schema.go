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

// TypedVariant resolves the OpenAPI 3.1 spelling of an optional value —
// anyOf/oneOf carrying a "null" branch beside a typed one — to its typed branch,
// reporting whether a null branch was present. Generators emit every optional
// field this way, and the wrapper declares no type of its own, so a consumer that
// only reads schema.Type ends up with no type at all. Returns nil when the schema
// already declares a type or has nothing to resolve.
func TypedVariant(schema *openapi3.Schema) (*openapi3.Schema, bool) {
	if schema == nil || SchemaType(schema) != "" || len(schema.Properties) > 0 {
		return nil, false
	}

	nullable := false
	for _, group := range []openapi3.SchemaRefs{schema.AnyOf, schema.OneOf} {
		var typed *openapi3.Schema
		for _, ref := range group {
			if ref == nil || ref.Value == nil {
				continue
			}
			if isNullSchema(ref.Value) {
				nullable = true
				continue
			}
			if typed == nil && SchemaType(ref.Value) != "" {
				typed = ref.Value
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
