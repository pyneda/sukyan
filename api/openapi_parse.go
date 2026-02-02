package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/openapi"
	"github.com/rs/zerolog/log"
)

// ParseOpenAPISpecInput represents the input for parsing an OpenAPI specification.
type ParseOpenAPISpecInput struct {
	URL             string `json:"url" validate:"required,url"`
	BaseURL         string `json:"base_url" validate:"omitempty,url"`
	IncludeOptional bool   `json:"include_optional"`
	EnableFuzzing   bool   `json:"enable_fuzzing"`
}

// ParseOpenAPISpecResponse represents the response from parsing an OpenAPI specification.
type ParseOpenAPISpecResponse struct {
	Endpoints       []openapi.Endpoint            `json:"endpoints"`
	SecuritySchemes []openapi.SecurityScheme      `json:"security_schemes,omitempty"`
	GlobalSecurity  []openapi.SecurityRequirement `json:"global_security,omitempty"`
	BaseURL         string                        `json:"base_url"`
	Count           int                           `json:"count"`
}

// ParseOpenAPISpec godoc
// @Summary Parse an OpenAPI specification from a URL
// @Description Fetches and parses an OpenAPI specification from the provided URL, returning parsed endpoint data
// @Tags Playground
// @Accept json
// @Produce json
// @Param input body ParseOpenAPISpecInput true "OpenAPI Specification URL and configuration"
// @Success 200 {object} ParseOpenAPISpecResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/openapi/parse [post]
func ParseOpenAPISpec(c *fiber.Ctx) error {
	input := new(ParseOpenAPISpecInput)

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

	// Fetch the OpenAPI specification from the URL
	bodyBytes, err := http_utils.FetchOpenAPISpec(input.URL)
	if err != nil {
		log.Error().Err(err).Str("url", input.URL).Msg("Failed to fetch OpenAPI spec")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Failed to fetch OpenAPI spec",
			"message": err.Error(),
		})
	}

	// Parse the OpenAPI specification
	doc, err := openapi.Parse(bodyBytes)
	if err != nil {
		log.Error().Err(err).Str("url", input.URL).Msg("Failed to parse OpenAPI spec")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Failed to parse OpenAPI spec",
			"message": err.Error(),
		})
	}

	// Configure request generation
	config := openapi.GenerationConfig{
		BaseURL:               doc.BaseURL(),
		IncludeOptionalParams: input.IncludeOptional,
		FuzzingEnabled:        input.EnableFuzzing,
	}

	// Override base URL if provided
	if input.BaseURL != "" {
		config.BaseURL = input.BaseURL
	}

	// Generate requests from the parsed specification
	endpoints, err := openapi.GenerateRequests(doc, config)
	if err != nil {
		log.Error().Err(err).Str("url", input.URL).Msg("Failed to generate requests from OpenAPI spec")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to generate requests",
			"message": err.Error(),
		})
	}

	// Extract security information
	securitySchemes := doc.GetSecuritySchemes()
	globalSecurity := doc.GetGlobalSecurityRequirements()

	response := ParseOpenAPISpecResponse{
		Endpoints:       endpoints,
		SecuritySchemes: securitySchemes,
		GlobalSecurity:  globalSecurity,
		BaseURL:         config.BaseURL,
		Count:           len(endpoints),
	}

	return c.Status(fiber.StatusOK).JSON(response)
}

type ParseOpenAPISpecFromContentInput struct {
	Content         string `json:"content" validate:"required"`
	BaseURL         string `json:"base_url" validate:"omitempty,url"`
	IncludeOptional bool   `json:"include_optional"`
	EnableFuzzing   bool   `json:"enable_fuzzing"`
}

// ParseOpenAPISpecFromContent godoc
// @Summary Parse an OpenAPI specification from raw content
// @Description Parses OpenAPI from provided JSON/YAML content (useful when spec is not directly accessible via URL)
// @Tags Playground
// @Accept json
// @Produce json
// @Param input body ParseOpenAPISpecFromContentInput true "OpenAPI content and configuration"
// @Success 200 {object} ParseOpenAPISpecResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/openapi/parse-content [post]
func ParseOpenAPISpecFromContent(c *fiber.Ctx) error {
	input := new(ParseOpenAPISpecFromContentInput)

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

	doc, err := openapi.Parse([]byte(input.Content))
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse OpenAPI content")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Failed to parse OpenAPI content",
			"message": err.Error(),
		})
	}

	config := openapi.GenerationConfig{
		BaseURL:               doc.BaseURL(),
		IncludeOptionalParams: input.IncludeOptional,
		FuzzingEnabled:        input.EnableFuzzing,
	}

	if input.BaseURL != "" {
		config.BaseURL = input.BaseURL
	}

	endpoints, err := openapi.GenerateRequests(doc, config)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate requests from OpenAPI content")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to generate requests",
			"message": err.Error(),
		})
	}

	securitySchemes := doc.GetSecuritySchemes()
	globalSecurity := doc.GetGlobalSecurityRequirements()

	response := ParseOpenAPISpecResponse{
		Endpoints:       endpoints,
		SecuritySchemes: securitySchemes,
		GlobalSecurity:  globalSecurity,
		BaseURL:         config.BaseURL,
		Count:           len(endpoints),
	}

	return c.Status(fiber.StatusOK).JSON(response)
}
