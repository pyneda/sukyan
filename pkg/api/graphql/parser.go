package graphql

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
	pkgGraphql "github.com/pyneda/sukyan/pkg/graphql"
	"github.com/rs/zerolog/log"
)

type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(definition *db.APIDefinition) ([]core.Operation, error) {
	if definition.Type != db.APIDefinitionTypeGraphQL {
		return nil, fmt.Errorf("expected GraphQL definition, got %s", definition.Type)
	}

	if len(definition.RawDefinition) == 0 {
		return nil, fmt.Errorf("empty raw definition")
	}

	parser := pkgGraphql.NewParser()
	schema, err := parser.ParseFromJSON(definition.RawDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL schema: %w", err)
	}

	baseURL := definition.BaseURL
	if baseURL == "" {
		baseURL = definition.SourceURL
	}

	var operations []core.Operation

	for _, query := range schema.Queries {
		op := p.convertOperation(definition.ID, baseURL, "query", query, schema)
		operations = append(operations, op)
	}

	for _, mutation := range schema.Mutations {
		op := p.convertOperation(definition.ID, baseURL, "mutation", mutation, schema)
		operations = append(operations, op)
	}

	for _, subscription := range schema.Subscriptions {
		op := p.convertOperation(definition.ID, baseURL, "subscription", subscription, schema)
		operations = append(operations, op)
	}

	log.Debug().
		Int("operations", len(operations)).
		Int("queries", len(schema.Queries)).
		Int("mutations", len(schema.Mutations)).
		Int("subscriptions", len(schema.Subscriptions)).
		Msg("Parsed GraphQL definition")

	return operations, nil
}

func (p *Parser) convertOperation(definitionID uuid.UUID, baseURL, operationType string, op pkgGraphql.Operation, schema *pkgGraphql.GraphQLSchema) core.Operation {
	operation := core.Operation{
		ID:           uuid.New(),
		DefinitionID: definitionID,
		APIType:      core.APITypeGraphQL,
		Name:         op.Name,
		Method:       "POST",
		Path:         "",
		BaseURL:      baseURL,
		Summary:      op.Description,
		Description:  op.Description,
		Deprecated:   op.IsDeprecated,
		GraphQL: &core.GraphQLMetadata{
			OperationType: operationType,
			ReturnType:    p.formatTypeRef(op.ReturnType),
			IsDeprecated:  op.IsDeprecated,
		},
	}

	for _, arg := range op.Arguments {
		param := p.convertArgument(arg, schema)
		operation.Parameters = append(operation.Parameters, param)
	}

	return operation
}

func (p *Parser) convertArgument(arg pkgGraphql.Argument, schema *pkgGraphql.GraphQLSchema) core.Parameter {
	param := core.Parameter{
		Name:        arg.Name,
		Location:    core.ParameterLocationArgument,
		Required:    arg.Type.Required,
		Description: arg.Description,
	}

	param.DataType = p.mapGraphQLType(arg.Type, schema)
	p.extractGraphQLConstraints(arg.Type, schema, &param)

	if arg.DefaultValue != nil {
		param.DefaultValue = arg.DefaultValue
	}

	if arg.Type.Kind == pkgGraphql.TypeKindInputObject {
		param.NestedParams = p.extractInputObjectFields(arg.Type.Name, schema)
	}

	return param
}

func (p *Parser) mapGraphQLType(typeRef pkgGraphql.TypeRef, schema *pkgGraphql.GraphQLSchema) core.DataType {
	baseName := p.getBaseTypeName(typeRef)

	switch baseName {
	case "String", "ID":
		return core.DataTypeString
	case "Int":
		return core.DataTypeInteger
	case "Float":
		return core.DataTypeNumber
	case "Boolean":
		return core.DataTypeBoolean
	}

	if typeRef.IsList {
		return core.DataTypeArray
	}

	if _, ok := schema.InputTypes[baseName]; ok {
		return core.DataTypeObject
	}

	if _, ok := schema.Enums[baseName]; ok {
		return core.DataTypeString
	}

	return core.DataTypeString
}

func (p *Parser) extractGraphQLConstraints(typeRef pkgGraphql.TypeRef, schema *pkgGraphql.GraphQLSchema, param *core.Parameter) {
	baseName := p.getBaseTypeName(typeRef)

	if enumDef, ok := schema.Enums[baseName]; ok {
		for _, ev := range enumDef.Values {
			param.Constraints.Enum = append(param.Constraints.Enum, ev.Name)
		}
	}

	switch baseName {
	case "Int":
		param.Constraints.Format = "int32"
	case "Float":
		param.Constraints.Format = "double"
	case "ID":
		param.Constraints.Format = "id"
	case "String":
		param.Constraints.Format = "string"
	}
}

const maxGraphQLDepth = 10

func (p *Parser) extractInputObjectFields(typeName string, schema *pkgGraphql.GraphQLSchema) []core.Parameter {
	return p.extractInputObjectFieldsWithVisited(typeName, schema, make(map[string]bool), 0)
}

func (p *Parser) extractInputObjectFieldsWithVisited(typeName string, schema *pkgGraphql.GraphQLSchema, visited map[string]bool, depth int) []core.Parameter {
	var params []core.Parameter

	if depth > maxGraphQLDepth {
		return params
	}

	if visited[typeName] {
		return params
	}
	visited[typeName] = true

	inputType, ok := schema.InputTypes[typeName]
	if !ok {
		return params
	}

	for _, field := range inputType.Fields {
		param := core.Parameter{
			Name:        field.Name,
			Location:    core.ParameterLocationBody,
			Required:    field.Type.Required,
			Description: field.Description,
			DataType:    p.mapGraphQLTypeRef(field.Type, schema),
		}

		p.extractGraphQLConstraints(field.Type, schema, &param)

		if field.DefaultValue != nil {
			param.DefaultValue = field.DefaultValue
		}

		baseName := p.getBaseTypeName(field.Type)
		if _, ok := schema.InputTypes[baseName]; ok {
			param.NestedParams = p.extractInputObjectFieldsWithVisited(baseName, schema, visited, depth+1)
		}

		params = append(params, param)
	}

	return params
}

func (p *Parser) mapGraphQLTypeRef(typeRef pkgGraphql.TypeRef, schema *pkgGraphql.GraphQLSchema) core.DataType {
	return p.mapGraphQLType(typeRef, schema)
}

func (p *Parser) getBaseTypeName(typeRef pkgGraphql.TypeRef) string {
	if typeRef.OfType != nil {
		return p.getBaseTypeName(*typeRef.OfType)
	}
	return typeRef.Name
}

func (p *Parser) formatTypeRef(typeRef pkgGraphql.TypeRef) string {
	result := ""

	if typeRef.IsList {
		result = "["
	}

	if typeRef.OfType != nil {
		result += p.formatTypeRef(*typeRef.OfType)
	} else {
		result += typeRef.Name
	}

	if typeRef.IsList {
		result += "]"
	}

	if typeRef.Required {
		result += "!"
	}

	return result
}

func ParseFromRawDefinition(rawDefinition []byte, baseURL string) ([]core.Operation, error) {
	parser := NewParser()

	tempDef := &db.APIDefinition{
		Type:          db.APIDefinitionTypeGraphQL,
		RawDefinition: rawDefinition,
		BaseURL:       baseURL,
	}

	return parser.Parse(tempDef)
}
