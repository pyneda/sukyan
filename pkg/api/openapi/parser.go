package openapi

import (
	"encoding/json"
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
	"github.com/rs/zerolog/log"
)

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
		baseURL = doc.Servers[0].URL
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

		for contentType := range op.RequestBody.Value.Content {
			operation.OpenAPI.RequestBody.ContentType = contentType
			break
		}
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

	for contentType, mediaType := range body.Content {
		if mediaType.Schema == nil || mediaType.Schema.Value == nil {
			continue
		}

		schema := mediaType.Schema.Value

		if schema.Type != nil && len(schema.Type.Slice()) > 0 && schema.Type.Slice()[0] == "object" {
			for propName, propRef := range schema.Properties {
				if propRef.Value == nil {
					continue
				}

				param := core.Parameter{
					Name:        propName,
					Location:    core.ParameterLocationBody,
					Required:    p.isPropertyRequired(propName, schema.Required),
					ContentType: contentType,
				}

				p.extractSchemaInfo(propRef.Value, &param)
				params = append(params, param)
			}
		} else {
			param := core.Parameter{
				Name:        "body",
				Location:    core.ParameterLocationBody,
				Required:    body.Required,
				ContentType: contentType,
			}
			p.extractSchemaInfo(schema, &param)
			params = append(params, param)
		}

		break
	}

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
