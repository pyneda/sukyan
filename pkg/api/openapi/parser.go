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

func (b *schemaBudget) Spend() bool {
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

// resolveBody returns the effective view of a request body schema. Composition is
// resolved by pkg/openapi so that a body root, a nested property and an array item all
// describe the same value — the divergence between a body-root resolver and a nested
// walk that understood no composition is what made the same schema yield different
// requests depending on its depth.
func resolveBody(schema *openapi3.Schema, document *documentBudget) pkgopenapi.SchemaView {
	return pkgopenapi.ResolveSchema(schema, newSchemaBudget(document))
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
		Responses: buildResponses(op),
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

		// The body's own schema, alongside the parameters extracted from it: the
		// parameter list flattens a body to leaves, which loses the nesting and the
		// composition a reader needs to see to write one by hand.
		if media := op.RequestBody.Value.Content[operation.OpenAPI.RequestBody.ContentType]; media != nil && media.Schema != nil {
			operation.OpenAPI.RequestBody.Schema = media.Schema.Value
		}
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
	return resolveBody(mediaType.Schema.Value, budget).IsObject()
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
	view := resolveBody(schema, budget)

	// The body root's own identity has to be on the cycle path before its properties
	// are walked, exactly as a nested property's reference is. Without it a recursive
	// model unrolls one level deeper at a body root than the same model nested under a
	// property, so what a schema describes would still depend on where it sits.
	rootRefs := append([]string{mediaType.Schema.Ref}, view.Refs...)

	if view.IsObject() {
		// Sorted: ranging the property map would reorder the parameters, and therefore
		// the generated body, between runs on an identical spec.
		names := make([]string, 0, len(view.Properties))
		for propName := range view.Properties {
			names = append(names, propName)
		}
		sort.Strings(names)

		for _, propName := range names {
			propRef := view.Properties[propName]
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
				Required:    p.isPropertyRequired(propName, view.Required),
				ContentType: contentType,
			}

			p.extractSchemaInfoBelow(propRef.Value, &param, budget, rootRefs, []string{propRef.Ref})
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
	p.extractSchemaInfoBelow(schema, &param, budget, rootRefs)
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
	if depth > maxSchemaDepth {
		return
	}

	// Composition is resolved before anything is read off the schema. An optional field
	// in OpenAPI 3.1 is anyOf:[{type:X},{type:null}] and an annotated reference is
	// allOf:[{$ref:X}] — in both the wrapper declares no type and no properties, so a
	// walk that reads schema.Type and schema.Properties directly leaves the parameter
	// untyped and the request carries null where the endpoint wants a value.
	//
	// The view also spends the node budget, so an exhausted budget yields an empty view
	// and the walk stops here.
	view := pkgopenapi.ResolveSchema(schema, budget)

	// Least specific first: a branch keeps its own format and enum, while the keywords
	// JSON Schema allows beside anyOf (enum, pattern, maxLength) constrain whatever the
	// branch resolved to and therefore come last.
	for _, source := range view.Sources {
		applyConstraints(source, param)
	}

	if view.Type != "" {
		param.DataType = p.mapDataType(view.Type)
	}
	if view.Nullable {
		param.Nullable = true
	}

	// The composition may have resolved through a model already being expanded further
	// up. What it describes is still known — a cut that left the type blank serialised
	// the whole field as JSON null — but descending into it again re-expands the cycle
	// at every level and drains the document budget every later operation needs.
	if !markRefs(onPath, view.Refs) {
		return
	}
	defer unmarkRefs(onPath, view.Refs)

	// The item schema is guarded like a property: an array of a self-referential type
	// (children: [Node]) is as much a cycle as a direct reference, and without the
	// mark it re-expands at every level until the depth limit stops it.
	if view.Items != nil && view.Items.Value != nil && !onPath[view.Items.Ref] {
		if view.Items.Ref != "" {
			onPath[view.Items.Ref] = true
		}
		nestedParam := core.Parameter{Name: "items"}
		p.extractSchemaInfoWithDepth(view.Items.Value, &nestedParam, onPath, depth+1, budget)
		param.NestedParams = append(param.NestedParams, nestedParam)
		if view.Items.Ref != "" {
			delete(onPath, view.Items.Ref)
		}
	}

	// Sorted: ranging the property map would reorder NestedParams between runs on an
	// identical spec.
	propNames := make([]string, 0, len(view.Properties))
	for propName := range view.Properties {
		propNames = append(propNames, propName)
	}
	sort.Strings(propNames)

	for _, propName := range propNames {
		propRef := view.Properties[propName]
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
			Required: p.isPropertyRequired(propName, view.Required),
		}
		p.extractSchemaInfoWithDepth(propRef.Value, &nestedParam, onPath, depth+1, budget)
		param.NestedParams = append(param.NestedParams, nestedParam)

		if schemaRef != "" {
			delete(onPath, schemaRef)
		}
	}
}

// markRefs claims every reference a composition passed through, reporting false when
// one of them is already being expanded.
func markRefs(onPath map[string]bool, refs []string) bool {
	for _, ref := range refs {
		if ref != "" && onPath[ref] {
			return false
		}
	}
	for _, ref := range refs {
		if ref != "" {
			onPath[ref] = true
		}
	}
	return true
}

func unmarkRefs(onPath map[string]bool, refs []string) {
	for _, ref := range refs {
		delete(onPath, ref)
	}
}

// resolveExclusiveBound collapses the two encodings of an exclusive bound into a
// single value: 3.0 spells it `minimum` plus a boolean `exclusiveMinimum`, while
// 3.1 carries the bound in `exclusiveMinimum` itself and omits `minimum`.
func resolveExclusiveBound(bound *float64, exclusive openapi3.ExclusiveBound) (*float64, bool) {
	if exclusive.Value != nil {
		return exclusive.Value, true
	}
	return bound, exclusive.IsTrue()
}

// applyConstraints copies the validation keywords one schema declares onto the
// parameter, leaving what it does not declare untouched so later, more specific
// sources layer over earlier ones instead of clobbering them.
func applyConstraints(schema *openapi3.Schema, param *core.Parameter) {
	if schema.Format != "" {
		param.Constraints.Format = schema.Format
	}

	minBound, minExclusive := resolveExclusiveBound(schema.Min, schema.ExclusiveMin)
	if minBound != nil {
		param.Constraints.Minimum = minBound
	}
	if minExclusive {
		param.Constraints.ExclusiveMin = true
	}

	maxBound, maxExclusive := resolveExclusiveBound(schema.Max, schema.ExclusiveMax)
	if maxBound != nil {
		param.Constraints.Maximum = maxBound
	}
	if maxExclusive {
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
}

func (p *Parser) extractSchemaInfo(schema *openapi3.Schema, param *core.Parameter, document *documentBudget) {
	p.extractSchemaInfoBelow(schema, param, document)
}

// extractSchemaInfoBelow walks a schema with the given references already claimed, so
// a model does not re-expand itself. The nested walk claims a property's reference
// before descending into it; a body root has to claim the same references by hand,
// otherwise a recursive model unrolls one level deeper there than anywhere else.
func (p *Parser) extractSchemaInfoBelow(schema *openapi3.Schema, param *core.Parameter, document *documentBudget, claimed ...[]string) {
	onPath := make(map[string]bool)
	for _, refs := range claimed {
		markRefs(onPath, refs)
	}
	p.extractSchemaInfoWithDepth(schema, param, onPath, 0, newSchemaBudget(document))
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

	constraints.Minimum, constraints.ExclusiveMin = resolveExclusiveBound(schema.Min, schema.ExclusiveMin)
	constraints.Maximum, constraints.ExclusiveMax = resolveExclusiveBound(schema.Max, schema.ExclusiveMax)

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

// buildResponses flattens an operation's declared responses into the shape the
// API surface serves. Status codes are sorted so a response list does not
// reorder between two reads of the same specification.
func buildResponses(op *openapi3.Operation) []core.ResponseInfo {
	if op == nil || op.Responses == nil {
		return nil
	}
	responses := op.Responses.Map()
	codes := make([]string, 0, len(responses))
	for code := range responses {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	out := make([]core.ResponseInfo, 0, len(codes))
	for _, code := range codes {
		ref := responses[code]
		if ref == nil || ref.Value == nil {
			continue
		}
		info := core.ResponseInfo{StatusCode: code}
		if ref.Value.Description != nil {
			info.Description = *ref.Value.Description
		}
		if contentType := selectResponseContentType(ref.Value.Content); contentType != "" {
			info.ContentType = contentType
			if media := ref.Value.Content[contentType]; media != nil && media.Schema != nil {
				info.Schema = media.Schema.Value
			}
		}
		out = append(out, info)
	}
	return out
}

// selectResponseContentType prefers JSON, then whatever sorts first, so the choice
// does not depend on Go's map iteration order.
func selectResponseContentType(content openapi3.Content) string {
	if len(content) == 0 {
		return ""
	}
	types := make([]string, 0, len(content))
	for contentType := range content {
		types = append(types, contentType)
	}
	sort.Strings(types)
	for _, contentType := range types {
		if strings.Contains(contentType, "json") {
			return contentType
		}
	}
	return types[0]
}
