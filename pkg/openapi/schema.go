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

func (w *schemaWalk) Spend() bool {
	if w.budget <= 0 {
		return false
	}
	w.budget--
	return true
}

// enter marks every schema a view was composed from, reporting false when one of them
// is already being walked. Guarding the wrapper alone is not enough: a recursive model
// reached through anyOf or allOf is a different node from the model itself, so the
// cycle would only be caught one level deeper each time.
func (w *schemaWalk) enter(sources []*openapi3.Schema) bool {
	for _, source := range sources {
		if w.onPath[source] {
			return false
		}
	}
	for _, source := range sources {
		w.onPath[source] = true
	}
	return true
}

func (w *schemaWalk) leave(sources []*openapi3.Schema) {
	for _, source := range sources {
		delete(w.onPath, source)
	}
}

// schemaToMap flattens a schema into the simplified representation the value
// strategies consume. Cycles are broken by tracking the schemas on the current
// path, so a self-referential type yields a finite map instead of crashing.
func schemaToMap(ref *openapi3.SchemaRef) map[string]interface{} {
	return newSchemaWalk().mapSchema(ref, 0)
}

// schemaToMapBelow flattens a schema with the model containing it already on the cycle
// path. A body root walks its properties one at a time, and giving each a clean path
// would unroll a recursive model one level deeper there than under any other property.
func schemaToMapBelow(ref *openapi3.SchemaRef, roots []*openapi3.Schema) map[string]interface{} {
	walk := newSchemaWalk()
	walk.enter(roots)
	return walk.mapSchema(ref, 0)
}

func (w *schemaWalk) mapSchema(ref *openapi3.SchemaRef, depth int) map[string]interface{} {
	if ref == nil || ref.Value == nil || depth > maxSchemaDepth || !w.Spend() {
		return nil
	}

	// Composition is resolved before anything is read off the schema, so a property
	// spelled through allOf or anyOf describes the same value here as it does at a body
	// root. Reading Type and Properties directly is what made the two disagree.
	view := ResolveSchema(ref.Value, w)

	result := make(map[string]interface{})
	for _, source := range view.Sources {
		for key, value := range schemaKeywords(source) {
			result[key] = value
		}
	}
	if view.Type != "" {
		result["type"] = view.Type
	}
	if view.Nullable {
		result["nullable"] = true
	}
	if len(view.Required) > 0 {
		result["required"] = view.Required
	}

	if !w.enter(view.Sources) {
		return result
	}
	defer w.leave(view.Sources)

	if len(view.Properties) > 0 {
		mapped := make(map[string]interface{}, len(view.Properties))
		for _, name := range sortedSchemaNames(view.Properties) {
			if propMap := w.mapSchema(view.Properties[name], depth+1); propMap != nil {
				mapped[name] = propMap
			}
		}
		if len(mapped) > 0 {
			result["properties"] = mapped
		}
	}

	if view.Items != nil {
		if items := w.mapSchema(view.Items, depth+1); items != nil {
			result["items"] = items
			if _, ok := result["type"]; !ok {
				result["type"] = "array"
			}
		}
	}

	return result
}

// schemaKeywords returns the validation keywords a schema declares directly. They are
// applied per contributing schema rather than once for the wrapper, so a branch keeps
// its own format and enum while the keywords JSON Schema allows beside anyOf still
// constrain whatever the branch resolved to.
func schemaKeywords(schema *openapi3.Schema) map[string]interface{} {
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
