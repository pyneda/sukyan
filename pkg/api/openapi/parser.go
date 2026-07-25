package openapi

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
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

// resolveServerURL turns a possibly-relative OpenAPI server URL (e.g. "/api/v3",
// common in Petstore specs) into an absolute one by resolving it against the
// definition's source URL. Without this, a relative server survives as the base URL
// and requests build to "/api/v3/pet/..." with no scheme or host, so the endpoint is
// unscannable. Absolute server URLs are returned unchanged.
func resolveServerURL(serverURL, sourceURL string) string {
	ref, err := url.Parse(serverURL)
	if err != nil {
		return serverURL
	}
	if ref.IsAbs() {
		return serverURL
	}
	base, err := url.Parse(sourceURL)
	if err != nil || !base.IsAbs() {
		return serverURL
	}
	return base.ResolveReference(ref).String()
}

// effectiveObjectProperties resolves an object-like schema into its combined property
// set and required list, following composition: allOf merges every subschema, while
// oneOf/anyOf contribute the first object-like variant (deterministically, by spec
// order). It returns isObject=true when the schema should be treated as a structured
// body, so composed schemas no longer collapse into a single opaque "body" param with
// a null value. A plain scalar/array body returns isObject=false so the caller keeps
// its single-parameter fallback.
func effectiveObjectProperties(schema *openapi3.Schema, depth int) (props openapi3.Schemas, required []string, isObject bool) {
	if schema == nil || depth > maxSchemaDepth {
		return nil, nil, false
	}

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
		subProps, subRequired, ok := effectiveObjectProperties(sub.Value, depth+1)
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
				subProps, subRequired, ok := effectiveObjectProperties(sub.Value, depth+1)
				if ok {
					for name, ref := range subProps {
						props[name] = ref
					}
					addRequired(subRequired)
					isObject = true
					break
				}
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
	if definition.Type != db.APIDefinitionTypeOpenAPI {
		return nil, fmt.Errorf("expected OpenAPI definition, got %s", definition.Type)
	}

	if len(definition.RawDefinition) == 0 {
		return nil, fmt.Errorf("empty raw definition")
	}

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	doc, err := loader.LoadFromData(definition.RawDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}

	baseURL := definition.BaseURL
	if baseURL == "" && len(doc.Servers) > 0 {
		baseURL = resolveServerURL(doc.Servers[0].URL, definition.SourceURL)
	}

	var operations []core.Operation

	if doc.Paths == nil {
		return operations, nil
	}

	for path, pathItem := range doc.Paths.Map() {
		for method, op := range pathItem.Operations() {
			operation := p.parseOperation(definition.ID, baseURL, path, method, op, doc)
			operations = append(operations, operation)
		}
	}

	log.Debug().
		Int("operations", len(operations)).
		Str("base_url", baseURL).
		Msg("Parsed OpenAPI definition")

	return operations, nil
}

func (p *Parser) parseOperation(definitionID uuid.UUID, baseURL, path, method string, op *openapi3.Operation, doc *openapi3.T) core.Operation {
	operation := core.Operation{
		ID:           uuid.New(),
		DefinitionID: definitionID,
		APIType:      core.APITypeOpenAPI,
		Name:         op.OperationID,
		Method:       method,
		Path:         path,
		BaseURL:      baseURL,
		OperationID:  op.OperationID,
		Summary:      op.Summary,
		Description:  op.Description,
		Deprecated:   op.Deprecated,
		Tags:         op.Tags,
		OpenAPI: &core.OpenAPIMetadata{
			Servers: p.extractServerURLs(doc.Servers),
		},
	}

	if doc.OpenAPI != "" {
		operation.OpenAPI.Version = doc.OpenAPI
	}

	for _, paramRef := range op.Parameters {
		if paramRef.Value == nil {
			continue
		}
		param := p.parseParameter(paramRef.Value)
		operation.Parameters = append(operation.Parameters, param)
	}

	if op.RequestBody != nil && op.RequestBody.Value != nil {
		bodyParams := p.parseRequestBody(op.RequestBody.Value)
		operation.Parameters = append(operation.Parameters, bodyParams...)

		operation.OpenAPI.RequestBody = &core.RequestBodyInfo{
			Required:    op.RequestBody.Value.Required,
			Description: op.RequestBody.Value.Description,
		}

		operation.OpenAPI.RequestBody.ContentType = selectBodyContentType(op.RequestBody.Value.Content)
	}

	operation.Security = p.parseSecurityRequirements(op, doc)
	operation.ContentTypes = p.parseContentTypes(op)

	return operation
}

func (p *Parser) parseParameter(param *openapi3.Parameter) core.Parameter {
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
		p.extractSchemaInfo(param.Schema.Value, &coreParam)
	}

	return coreParam
}

func (p *Parser) parseRequestBody(body *openapi3.RequestBody) []core.Parameter {
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
	props, required, isObject := effectiveObjectProperties(schema, 0)

	if isObject {
		for propName, propRef := range props {
			if propRef.Value == nil {
				continue
			}

			param := core.Parameter{
				Name:        propName,
				Location:    core.ParameterLocationBody,
				Required:    p.isPropertyRequired(propName, required),
				ContentType: contentType,
			}

			p.extractSchemaInfo(propRef.Value, &param)
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
	p.extractSchemaInfo(schema, &param)
	params = append(params, param)

	return params
}

const maxSchemaDepth = 10

func (p *Parser) extractSchemaInfoWithDepth(schema *openapi3.Schema, param *core.Parameter, visited map[string]bool, depth int) {
	if depth > maxSchemaDepth {
		return
	}

	if schema.Type != nil && len(schema.Type.Slice()) > 0 {
		param.DataType = p.mapDataType(schema.Type.Slice()[0])
	}

	param.Constraints.Format = schema.Format

	if schema.Min != nil {
		param.Constraints.Minimum = schema.Min
	}
	if schema.Max != nil {
		param.Constraints.Maximum = schema.Max
	}
	param.Constraints.ExclusiveMin = schema.ExclusiveMin
	param.Constraints.ExclusiveMax = schema.ExclusiveMax

	if schema.MinLength != 0 {
		minLen := int(schema.MinLength)
		param.Constraints.MinLength = &minLen
	}
	if schema.MaxLength != nil {
		maxLen := int(*schema.MaxLength)
		param.Constraints.MaxLength = &maxLen
	}

	param.Constraints.Pattern = schema.Pattern

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

	param.DefaultValue = schema.Default
	param.ExampleValue = schema.Example
	param.Nullable = schema.Nullable

	if schema.Items != nil && schema.Items.Value != nil {
		nestedParam := core.Parameter{Name: "items"}
		p.extractSchemaInfoWithDepth(schema.Items.Value, &nestedParam, visited, depth+1)
		param.NestedParams = append(param.NestedParams, nestedParam)
	}

	for propName, propRef := range schema.Properties {
		if propRef.Value == nil {
			continue
		}

		schemaRef := ""
		if propRef.Ref != "" {
			schemaRef = propRef.Ref
		}
		if schemaRef != "" && visited[schemaRef] {
			continue
		}
		if schemaRef != "" {
			visited[schemaRef] = true
		}

		nestedParam := core.Parameter{
			Name:     propName,
			Required: p.isPropertyRequired(propName, schema.Required),
		}
		p.extractSchemaInfoWithDepth(propRef.Value, &nestedParam, visited, depth+1)
		param.NestedParams = append(param.NestedParams, nestedParam)
	}
}

func (p *Parser) extractSchemaInfo(schema *openapi3.Schema, param *core.Parameter) {
	p.extractSchemaInfoWithDepth(schema, param, make(map[string]bool), 0)
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

func (p *Parser) extractServerURLs(servers openapi3.Servers) []string {
	var urls []string
	for _, s := range servers {
		urls = append(urls, s.URL)
	}
	return urls
}

func (p *Parser) parseSecurityRequirements(op *openapi3.Operation, doc *openapi3.T) []core.SecurityRequirement {
	var reqs []core.SecurityRequirement

	securityReqs := doc.Security
	if op.Security != nil {
		securityReqs = *op.Security
	}

	for _, req := range securityReqs {
		for schemeName, scopes := range req {
			secReq := core.SecurityRequirement{
				Name:   schemeName,
				Scopes: scopes,
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

	for _, responseRef := range op.Responses.Map() {
		if responseRef.Value == nil {
			continue
		}
		for contentType := range responseRef.Value.Content {
			ct.Response = append(ct.Response, contentType)
		}
	}

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
