package api

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/pkg/graphql"
	"github.com/rs/zerolog/log"
)

// ParseGraphQLSchemaInput represents the input for parsing a GraphQL schema.
type ParseGraphQLSchemaInput struct {
	URL             string            `json:"url" validate:"required,url"`
	Headers         map[string]string `json:"headers,omitempty"`
	IncludeOptional bool              `json:"include_optional"`
	EnableFuzzing   bool              `json:"enable_fuzzing"`
	MaxDepth        int               `json:"max_depth,omitempty"`
}

// ParseGraphQLSchemaResponse represents the response from parsing a GraphQL schema.
type ParseGraphQLSchemaResponse struct {
	Endpoints []graphql.OperationEndpoint `json:"endpoints"`
	Schema    *GraphQLSchemaInfo          `json:"schema"`
	BaseURL   string                      `json:"base_url"`
	Count     int                         `json:"count"`
}

// GraphQLSchemaInfo contains summarized schema information for the response
type GraphQLSchemaInfo struct {
	QueryCount        int                             `json:"query_count"`
	MutationCount     int                             `json:"mutation_count"`
	SubscriptionCount int                             `json:"subscription_count"`
	TypeCount         int                             `json:"type_count"`
	EnumCount         int                             `json:"enum_count"`
	InputTypeCount    int                             `json:"input_type_count"`
	Types             map[string]graphql.TypeDef      `json:"types,omitempty"`
	Enums             map[string]graphql.EnumDef      `json:"enums,omitempty"`
	InputTypes        map[string]graphql.InputTypeDef `json:"input_types,omitempty"`
}

// ParseGraphQLFromIntrospectionInput represents input for parsing from raw introspection JSON
type ParseGraphQLFromIntrospectionInput struct {
	IntrospectionData json.RawMessage   `json:"introspection_data" validate:"required" swaggertype:"object"`
	BaseURL           string            `json:"base_url" validate:"required,url"`
	Headers           map[string]string `json:"headers,omitempty"`
	IncludeOptional   bool              `json:"include_optional"`
	EnableFuzzing     bool              `json:"enable_fuzzing"`
	MaxDepth          int               `json:"max_depth,omitempty"`
}

// ParseGraphQLSchema godoc
// @Summary Parse a GraphQL schema via introspection
// @Description Executes an introspection query against the provided GraphQL endpoint and generates test requests for all queries and mutations
// @Tags Playground
// @Accept json
// @Produce json
// @Param input body ParseGraphQLSchemaInput true "GraphQL endpoint URL and configuration"
// @Success 200 {object} ParseGraphQLSchemaResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/graphql/parse [post]
func ParseGraphQLSchema(c *fiber.Ctx) error {
	input := new(ParseGraphQLSchemaInput)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Cannot parse JSON",
			"message": err.Error(),
		})
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"message": err.Error(),
		})
	}

	// Create parser with custom headers
	parser := graphql.NewParser()
	if input.Headers != nil && len(input.Headers) > 0 {
		parser.WithHeaders(input.Headers)
	}

	// Parse schema via introspection
	schema, err := parser.ParseFromURL(input.URL)
	if err != nil {
		log.Error().Err(err).Str("url", input.URL).Msg("Failed to introspect GraphQL schema")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Failed to introspect GraphQL schema",
			"message": err.Error(),
		})
	}

	// Configure request generation
	config := graphql.GenerationConfig{
		BaseURL:               input.URL,
		IncludeOptionalParams: input.IncludeOptional,
		FuzzingEnabled:        input.EnableFuzzing,
		Headers:               input.Headers,
		MaxDepth:              input.MaxDepth,
	}

	if config.MaxDepth == 0 {
		config.MaxDepth = 3
	}

	// Generate requests
	generator := graphql.NewGenerator(schema, config)
	endpoints := generator.GenerateRequests()

	// Build schema info summary
	schemaInfo := &GraphQLSchemaInfo{
		QueryCount:        len(schema.Queries),
		MutationCount:     len(schema.Mutations),
		SubscriptionCount: len(schema.Subscriptions),
		TypeCount:         len(schema.Types),
		EnumCount:         len(schema.Enums),
		InputTypeCount:    len(schema.InputTypes),
		Types:             schema.Types,
		Enums:             schema.Enums,
		InputTypes:        schema.InputTypes,
	}

	response := ParseGraphQLSchemaResponse{
		Endpoints: endpoints,
		Schema:    schemaInfo,
		BaseURL:   input.URL,
		Count:     len(endpoints),
	}

	return c.Status(fiber.StatusOK).JSON(response)
}

// ParseGraphQLFromIntrospection godoc
// @Summary Parse a GraphQL schema from raw introspection data
// @Description Parses GraphQL schema from provided introspection JSON data (useful when introspection endpoint is not directly accessible)
// @Tags Playground
// @Accept json
// @Produce json
// @Param input body ParseGraphQLFromIntrospectionInput true "Introspection data and configuration"
// @Success 200 {object} ParseGraphQLSchemaResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/graphql/parse-introspection [post]
func ParseGraphQLFromIntrospection(c *fiber.Ctx) error {
	input := new(ParseGraphQLFromIntrospectionInput)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Cannot parse JSON",
			"message": err.Error(),
		})
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"message": err.Error(),
		})
	}

	// Parse schema from introspection JSON
	parser := graphql.NewParser()
	schema, err := parser.ParseFromJSON(input.IntrospectionData)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse GraphQL introspection data")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Failed to parse introspection data",
			"message": err.Error(),
		})
	}

	// Configure request generation
	config := graphql.GenerationConfig{
		BaseURL:               input.BaseURL,
		IncludeOptionalParams: input.IncludeOptional,
		FuzzingEnabled:        input.EnableFuzzing,
		Headers:               input.Headers,
		MaxDepth:              input.MaxDepth,
	}

	if config.MaxDepth == 0 {
		config.MaxDepth = 3
	}

	// Generate requests
	generator := graphql.NewGenerator(schema, config)
	endpoints := generator.GenerateRequests()

	// Build schema info summary
	schemaInfo := &GraphQLSchemaInfo{
		QueryCount:        len(schema.Queries),
		MutationCount:     len(schema.Mutations),
		SubscriptionCount: len(schema.Subscriptions),
		TypeCount:         len(schema.Types),
		EnumCount:         len(schema.Enums),
		InputTypeCount:    len(schema.InputTypes),
		Types:             schema.Types,
		Enums:             schema.Enums,
		InputTypes:        schema.InputTypes,
	}

	response := ParseGraphQLSchemaResponse{
		Endpoints: endpoints,
		Schema:    schemaInfo,
		BaseURL:   input.BaseURL,
		Count:     len(endpoints),
	}

	return c.Status(fiber.StatusOK).JSON(response)
}
