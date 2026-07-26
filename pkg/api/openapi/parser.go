package openapi

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
	pkgopenapi "github.com/pyneda/sukyan/pkg/openapi"
	"github.com/rs/zerolog/log"
)

// selectBodyContentType deterministically picks one content type from an OpenAPI
// request body. Go map iteration order is randomized, so ranging over Content and
// taking the first entry made both the extracted body params and the Content-Type
// header vary from run to run (and risked disagreeing with each other). Preference:
// exact application/json, then any json-bearing type, then the lexicographically
// smallest, so a given spec always yields the same request.
func selectBodyContentType(content openapi3.Content) string {
	if len(content) == 0 {
		return ""
	}
	if _, ok := content["application/json"]; ok {
		return "application/json"
	}
	best := ""
	for ct := range content {
		if strings.Contains(ct, "json") && (best == "" || ct < best) {
			best = ct
		}
	}
	if best != "" {
		return best
	}
	for ct := range content {
		if best == "" || ct < best {
			best = ct
		}
	}
	return best
}

// schemaBudget bounds one traversal. Depth alone is not enough: a schema whose allOf
// or properties reference the previous level several times expands combinatorially
// even though nothing is its own ancestor, so a few kilobytes of spec can otherwise
// consume gigabytes.
type schemaBudget struct {
	remaining int
	document  *documentBudget
	onPath    map[*openapi3.Schema]bool
}

func newSchemaBudget(document *documentBudget) *schemaBudget {
	return &schemaBudget{remaining: maxSchemaNodes, document: document, onPath: make(map[*openapi3.Schema]bool)}
}

func (b *schemaBudget) spend() bool {
	if b.remaining <= 0 {
		return false
	}
	if !b.document.spend() {
		return false
	}
	b.remaining--
	return true
}

// documentBudget bounds every traversal made while parsing one document. Bounding
// each traversal on its own does not bound the document: hundreds of parameters
// pointing at the same wide $ref graph each get a full traversal, so the per-walk
// limits multiply and tens of kilobytes of spec cost gigabytes. Real specs stay far
// below the ceiling — the largest in this repo expands to ~500 nodes in total.
type documentBudget struct {
	remaining int
}

func newDocumentBudget() *documentBudget {
	return &documentBudget{remaining: maxDocumentSchemaNodes}
}

func (b *documentBudget) spend() bool {
	if b == nil {
		return true
	}
	if b.remaining <= 0 {
		return false
	}
	b.remaining--
	return true
}

// effectiveObjectProperties resolves an object-like schema into its combined property
// set and required list, following composition: allOf merges every subschema, while
// oneOf/anyOf contribute the first object-like variant (deterministically, by spec
// order). It returns isObject=true when the schema should be treated as a structured
// body, so composed schemas no longer collapse into a single opaque "body" param with
// a null value. A plain scalar/array body returns isObject=false so the caller keeps
// its single-parameter fallback.
func effectiveObjectProperties(schema *openapi3.Schema, depth int, document *documentBudget) (openapi3.Schemas, []string, bool) {
	return effectiveObjectPropertiesBounded(schema, depth, newSchemaBudget(document))
}

func effectiveObjectPropertiesBounded(schema *openapi3.Schema, depth int, budget *schemaBudget) (props openapi3.Schemas, required []string, isObject bool) {
	if schema == nil || depth > maxSchemaDepth || !budget.spend() || budget.onPath[schema] {
		return nil, nil, false
	}
	budget.onPath[schema] = true
	defer delete(budget.onPath, schema)

	props = openapi3.Schemas{}
	seenRequired := map[string]bool{}
	addRequired := func(names []string) {
		for _, n := range names {
			if !seenRequired[n] {
				seenRequired[n] = true
				required = append(required, n)
			}
		}
	}

	if schema.Type != nil {
		for _, t := range schema.Type.Slice() {
			if t == "object" {
				isObject = true
			}
		}
	}

	if len(schema.Properties) > 0 {
		for name, ref := range schema.Properties {
			props[name] = ref
		}
		addRequired(schema.Required)
		isObject = true
	}

	for _, sub := range schema.AllOf {
		if sub == nil || sub.Value == nil {
			continue
		}
		subProps, subRequired, ok := effectiveObjectPropertiesBounded(sub.Value, depth+1, budget)
		if ok {
			for name, ref := range subProps {
				props[name] = ref
			}
			addRequired(subRequired)
			isObject = true
		}
	}

	if len(props) == 0 {
		for _, group := range []openapi3.SchemaRefs{schema.OneOf, schema.AnyOf} {
			for _, sub := range group {
				if sub == nil || sub.Value == nil {
					continue
				}
				// A variant that contributes nothing is skipped rather than accepted:
				// taking it marks the body structured with no fields, so the request
				// goes out empty while the next variant held every property.
				subProps, subRequired, ok := effectiveObjectPropertiesBounded(sub.Value, depth+1, budget)
				if !ok || len(subProps) == 0 {
					continue
				}
				for name, ref := range subProps {
					props[name] = ref
				}
				addRequired(subRequired)
				isObject = true
				break
			}
			if len(props) > 0 {
				break
			}
		}
	}

	return props, required, isObject
}

type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(definition *db.APIDefinition) ([]core.Operation, error) {
	if definition == nil {
		return nil, fmt.Errorf("nil API definition")
	}
	if definition.Type != db.APIDefinitionTypeOpenAPI {
		return nil, fmt.Errorf("expected OpenAPI definition, got %s", definition.Type)
	}

	if len(definition.RawDefinition) == 0 {
		return nil, fmt.Errorf("empty raw definition")
	}

	// Loading goes through pkg/openapi so that this parser and the discovery
	// persistence that created the definition see exactly the same document:
	// Swagger 2.0 conversion, OpenAPI 3.1 normalisation, size limits and the
	// no-external-references policy all live there.
	document, err := pkgopenapi.ParseWithOptions(definition.RawDefinition, pkgopenapi.ParseOptions{
		SourceURL: definition.SourceURL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}
	doc := document.Spec()

	baseURL := definition.BaseURL
	if baseURL == "" {
		baseURL = document.BaseURL()
	}

	// Servers come from the document so that variables are substituted and relative
	// URLs resolved, rather than reporting a template nothing can request.
	servers := document.Servers()

	entries := document.Operations()
	operations := make([]core.Operation, 0, len(entries))
	budget := newDocumentBudget()
	for _, entry := range entries {
		operations = append(operations, p.parseOperation(definition.ID, baseURL, entry, servers, doc, budget))
	}

	log.Debug().
		Int("operations", len(operations)).
		Str("base_url", baseURL).
		Msg("Parsed OpenAPI definition")

	return operations, nil
}

func (p *Parser) parseOperation(definitionID uuid.UUID, baseURL string, entry pkgopenapi.OperationEntry, servers []string, doc *openapi3.T, budget *documentBudget) core.Operation {
	op := entry.Operation
	operation := core.Operation{
		ID:           uuid.New(),
		DefinitionID: definitionID,
		APIType:      core.APITypeOpenAPI,
		Name:         op.OperationID,
		Method:       entry.Method,
		Path:         entry.Path,
		BaseURL:      baseURL,
		OperationID:  op.OperationID,
		Summary:      op.Summary,
		Description:  op.Description,
		Deprecated:   op.Deprecated,
		Tags:         op.Tags,
		OpenAPI: &core.OpenAPIMetadata{
			Servers: servers,
		},
	}

	if doc.OpenAPI != "" {
		operation.OpenAPI.Version = doc.OpenAPI
	}

	// entry.Parameters merges the path item's shared parameters with the operation's
	// own; without them a path template is never substituted.
	for _, param := range entry.Parameters {
		operation.Parameters = append(operation.Parameters, p.parseParameter(param, budget))
	}

	if op.RequestBody != nil && op.RequestBody.Value != nil {
		bodyParams := p.parseRequestBody(op.RequestBody.Value, budget)
		operation.Parameters = append(operation.Parameters, bodyParams...)

		operation.OpenAPI.RequestBody = &core.RequestBodyInfo{
			Required:    op.RequestBody.Value.Required,
			Description: op.RequestBody.Value.Description,
		}

		operation.OpenAPI.RequestBody.ContentType = selectBodyContentType(op.RequestBody.Value.Content)
		operation.OpenAPI.RequestBody.Structured = isStructuredBody(op.RequestBody.Value, budget)
	}

	operation.Security = p.parseSecurityRequirements(op, doc)
	operation.ContentTypes = p.parseContentTypes(op)

	return operation
}

func (p *Parser) parseParameter(param *openapi3.Parameter, budget *documentBudget) core.Parameter {
	coreParam := core.Parameter{
		Name:        param.Name,
		Location:    p.mapLocation(param.In),
		Required:    param.Required,
		Description: param.Description,
		Deprecated:  param.Deprecated,
		AllowEmpty:  param.AllowEmptyValue,
		Style:       param.Style,
	}

	if param.Explode != nil {
		coreParam.Explode = param.Explode
	}

	if param.Schema != nil && param.Schema.Value != nil {
		p.extractSchemaInfo(param.Schema.Value, &coreParam, budget)
	}

	return coreParam
}

// isStructuredBody reports whether the body decomposes into named properties. A
// scalar or array body yields a single "body" parameter that holds the whole payload.
func isStructuredBody(body *openapi3.RequestBody, budget *documentBudget) bool {
	contentType := selectBodyContentType(body.Content)
	if contentType == "" {
		return false
	}
	mediaType := body.Content[contentType]
	if mediaType == nil || mediaType.Schema == nil || mediaType.Schema.Value == nil {
		return false
	}
	_, _, isObject := effectiveObjectProperties(mediaType.Schema.Value, 0, budget)
	return isObject
}

func (p *Parser) parseRequestBody(body *openapi3.RequestBody, budget *documentBudget) []core.Parameter {
	var params []core.Parameter

	contentType := selectBodyContentType(body.Content)
	if contentType == "" {
		return params
	}
	mediaType := body.Content[contentType]
	if mediaType == nil || mediaType.Schema == nil || mediaType.Schema.Value == nil {
		return params
	}

	schema := mediaType.Schema.Value
	props, required, isObject := effectiveObjectProperties(schema, 0, budget)

	if isObject {
		// Sorted: ranging the property map would reorder the parameters, and therefore
		// the generated body, between runs on an identical spec.
		names := make([]string, 0, len(props))
		for propName := range props {
			names = append(names, propName)
		}
		sort.Strings(names)

		for _, propName := range names {
			propRef := props[propName]
			if propRef == nil || propRef.Value == nil {
				continue
			}
			// A readOnly property is response-only; sending it makes many APIs reject
			// the whole request, so the endpoint would never be exercised.
			if propRef.Value.ReadOnly {
				continue
			}

			param := core.Parameter{
				Name:        propName,
				Location:    core.ParameterLocationBody,
				Required:    p.isPropertyRequired(propName, required),
				ContentType: contentType,
			}

			p.extractSchemaInfo(propRef.Value, &param, budget)
			params = append(params, param)
		}
		return params
	}

	param := core.Parameter{
		Name:        "body",
		Location:    core.ParameterLocationBody,
		Required:    body.Required,
		ContentType: contentType,
	}
	p.extractSchemaInfo(schema, &param, budget)
	params = append(params, param)

	return params
}

const (
	maxSchemaDepth = 10
	// maxSchemaNodes bounds how many schema nodes one traversal expands.
	maxSchemaNodes = 2000
	// maxDocumentSchemaNodes bounds how many schema nodes one document expands in
	// total, across every parameter and body of every operation.
	maxDocumentSchemaNodes = 50000
)

func (p *Parser) extractSchemaInfoWithDepth(schema *openapi3.Schema, param *core.Parameter, onPath map[string]bool, depth int, budget *schemaBudget) {
	if depth > maxSchemaDepth || !budget.spend() {
		return
	}

	// An optional field in OpenAPI 3.1 is anyOf:[{type:X},{type:null}], and the
	// wrapper declares no type. Walking it as-is leaves the parameter untyped, so the
	// request carries null where the endpoint wants a value and the type-driven
	// payload sets never reach the field.
	//
	// The branch is walked under the same onPath guard as a property $ref: a
	// recursive model reached through such a wrapper (parent: Optional["Node"]) is a
	// cycle like any other, and expanding it once per depth level costs the shared
	// document budget every later operation still needs. Walking the branch first and
	// falling through — rather than returning — lets the keywords JSON Schema allows
	// beside anyOf (enum, pattern, maxLength) override what the branch declared.
	if variant, nullable := pkgopenapi.TypedVariant(schema); variant != nil {
		if variant.Ref == "" || !onPath[variant.Ref] {
			if variant.Ref != "" {
				onPath[variant.Ref] = true
				defer delete(onPath, variant.Ref)
			}
			p.extractSchemaInfoWithDepth(variant.Value, param, onPath, depth, budget)
		}
		param.Nullable = param.Nullable || nullable
	}

	if declared := pkgopenapi.SchemaType(schema); declared != "" {
		param.DataType = p.mapDataType(declared)
	}

	if schema.Format != "" {
		param.Constraints.Format = schema.Format
	}

	if schema.Min != nil {
		param.Constraints.Minimum = schema.Min
	}
	if schema.Max != nil {
		param.Constraints.Maximum = schema.Max
	}
	if schema.ExclusiveMin {
		param.Constraints.ExclusiveMin = true
	}
	if schema.ExclusiveMax {
		param.Constraints.ExclusiveMax = true
	}

	if schema.MinLength != 0 {
		minLen := int(schema.MinLength)
		param.Constraints.MinLength = &minLen
	}
	if schema.MaxLength != nil {
		maxLen := int(*schema.MaxLength)
		param.Constraints.MaxLength = &maxLen
	}

	if schema.Pattern != "" {
		param.Constraints.Pattern = schema.Pattern
	}

	if len(schema.Enum) > 0 {
		param.Constraints.Enum = schema.Enum
	}

	if schema.MinItems != 0 {
		minItems := int(schema.MinItems)
		param.Constraints.MinItems = &minItems
	}
	if schema.MaxItems != nil {
		maxItems := int(*schema.MaxItems)
		param.Constraints.MaxItems = &maxItems
	}

	if schema.Default != nil {
		param.DefaultValue = schema.Default
	}
	if schema.Example != nil {
		param.ExampleValue = schema.Example
	}
	if schema.Nullable {
		param.Nullable = true
	}

	// The item schema is guarded like a property: an array of a self-referential type
	// (children: [Node]) is as much a cycle as a direct reference, and without the
	// mark it re-expands at every level until the depth limit stops it.
	if schema.Items != nil && schema.Items.Value != nil && !onPath[schema.Items.Ref] {
		if schema.Items.Ref != "" {
			onPath[schema.Items.Ref] = true
		}
		nestedParam := core.Parameter{Name: "items"}
		p.extractSchemaInfoWithDepth(schema.Items.Value, &nestedParam, onPath, depth+1, budget)
		param.NestedParams = append(param.NestedParams, nestedParam)
		if schema.Items.Ref != "" {
			delete(onPath, schema.Items.Ref)
		}
	}

	// Sorted: ranging the property map would reorder NestedParams between runs on an
	// identical spec.
	propNames := make([]string, 0, len(schema.Properties))
	for propName := range schema.Properties {
		propNames = append(propNames, propName)
	}
	sort.Strings(propNames)

	for _, propName := range propNames {
		propRef := schema.Properties[propName]
		if propRef == nil || propRef.Value == nil {
			continue
		}

		// onPath tracks the ancestors of the current node, not every schema ever seen,
		// so a cycle still terminates while a type legitimately used twice in sibling
		// branches keeps its nested parameters in both.
		schemaRef := propRef.Ref
		if schemaRef != "" {
			if onPath[schemaRef] {
				continue
			}
			onPath[schemaRef] = true
		}

		nestedParam := core.Parameter{
			Name:     propName,
			Required: p.isPropertyRequired(propName, schema.Required),
		}
		p.extractSchemaInfoWithDepth(propRef.Value, &nestedParam, onPath, depth+1, budget)
		param.NestedParams = append(param.NestedParams, nestedParam)

		if schemaRef != "" {
			delete(onPath, schemaRef)
		}
	}

	// A schema that carries properties but omits "type": object is still an object;
	// leaving the type blank sends the whole field as null and discards every nested
	// parameter just collected.
	if param.DataType == "" && len(schema.Properties) > 0 {
		param.DataType = core.DataTypeObject
	}
}

func (p *Parser) extractSchemaInfo(schema *openapi3.Schema, param *core.Parameter, document *documentBudget) {
	p.extractSchemaInfoWithDepth(schema, param, make(map[string]bool), 0, newSchemaBudget(document))
}

func (p *Parser) mapLocation(in string) core.ParameterLocation {
	switch in {
	case "path":
		return core.ParameterLocationPath
	case "query":
		return core.ParameterLocationQuery
	case "header":
		return core.ParameterLocationHeader
	case "cookie":
		return core.ParameterLocationCookie
	case "body":
		return core.ParameterLocationBody
	default:
		return core.ParameterLocationQuery
	}
}

func (p *Parser) mapDataType(schemaType string) core.DataType {
	switch schemaType {
	case "string":
		return core.DataTypeString
	case "integer":
		return core.DataTypeInteger
	case "number":
		return core.DataTypeNumber
	case "boolean":
		return core.DataTypeBoolean
	case "array":
		return core.DataTypeArray
	case "object":
		return core.DataTypeObject
	case "file":
		return core.DataTypeFile
	default:
		return core.DataTypeString
	}
}

func (p *Parser) isPropertyRequired(propName string, required []string) bool {
	for _, r := range required {
		if r == propName {
			return true
		}
	}
	return false
}


func (p *Parser) parseSecurityRequirements(op *openapi3.Operation, doc *openapi3.T) []core.SecurityRequirement {
	var reqs []core.SecurityRequirement

	securityReqs := doc.Security
	if op.Security != nil {
		securityReqs = *op.Security
	}

	for _, req := range securityReqs {
		// Sorted: ranging the requirement map would reorder the schemes of an AND
		// requirement between parses of an identical document.
		schemeNames := make([]string, 0, len(req))
		for schemeName := range req {
			schemeNames = append(schemeNames, schemeName)
		}
		sort.Strings(schemeNames)

		for _, schemeName := range schemeNames {
			secReq := core.SecurityRequirement{
				Name:   schemeName,
				Scopes: req[schemeName],
			}

			if doc.Components != nil && doc.Components.SecuritySchemes != nil {
				if schemeRef, ok := doc.Components.SecuritySchemes[schemeName]; ok && schemeRef.Value != nil {
					secReq.Type = schemeRef.Value.Type
				}
			}

			reqs = append(reqs, secReq)
		}
	}

	return reqs
}

func (p *Parser) parseContentTypes(op *openapi3.Operation) core.RequestContentTypes {
	var ct core.RequestContentTypes

	if op.RequestBody != nil && op.RequestBody.Value != nil {
		for contentType := range op.RequestBody.Value.Content {
			ct.Request = append(ct.Request, contentType)
		}
	}

	seenResponse := make(map[string]bool)
	for _, responseRef := range op.Responses.Map() {
		if responseRef == nil || responseRef.Value == nil {
			continue
		}
		for contentType := range responseRef.Value.Content {
			if seenResponse[contentType] {
				continue
			}
			seenResponse[contentType] = true
			ct.Response = append(ct.Response, contentType)
		}
	}

	// Sorted: both lists are built by ranging maps, which reorders them on every parse
	// of an identical document.
	sort.Strings(ct.Request)
	sort.Strings(ct.Response)

	return ct
}

func ParseFromRawDefinition(rawDefinition []byte) ([]core.Operation, error) {
	parser := NewParser()

	tempDef := &db.APIDefinition{
		Type:          db.APIDefinitionTypeOpenAPI,
		RawDefinition: rawDefinition,
	}

	return parser.Parse(tempDef)
}

func ExtractConstraintsFromSchema(schemaJSON []byte) (core.Constraints, error) {
	var schema openapi3.Schema
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return core.Constraints{}, err
	}

	var constraints core.Constraints

	constraints.Format = schema.Format

	if schema.Min != nil {
		constraints.Minimum = schema.Min
	}
	if schema.Max != nil {
		constraints.Maximum = schema.Max
	}
	constraints.ExclusiveMin = schema.ExclusiveMin
	constraints.ExclusiveMax = schema.ExclusiveMax

	if schema.MinLength != 0 {
		minLen := int(schema.MinLength)
		constraints.MinLength = &minLen
	}
	if schema.MaxLength != nil {
		maxLen := int(*schema.MaxLength)
		constraints.MaxLength = &maxLen
	}

	constraints.Pattern = schema.Pattern

	if len(schema.Enum) > 0 {
		constraints.Enum = schema.Enum
	}

	if schema.MinItems != 0 {
		minItems := int(schema.MinItems)
		constraints.MinItems = &minItems
	}
	if schema.MaxItems != nil {
		maxItems := int(*schema.MaxItems)
		constraints.MaxItems = &maxItems
	}

	return constraints, nil
}
