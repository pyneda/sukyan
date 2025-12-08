package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/pkg/wsdl"
	"github.com/rs/zerolog/log"
)

// ParseWSDLInput represents the input for parsing a WSDL specification.
type ParseWSDLInput struct {
	URL             string            `json:"url" validate:"required,url"`
	Headers         map[string]string `json:"headers,omitempty"`
	IncludeOptional bool              `json:"include_optional"`
	PreferSOAP12    bool              `json:"prefer_soap_12"`
}

// ParseWSDLResponse represents the response from parsing a WSDL specification.
type ParseWSDLResponse struct {
	Services        []wsdl.ServiceEndpoint `json:"services"`
	Schema          *WSDLSchemaInfo        `json:"schema"`
	TargetNamespace string                 `json:"target_namespace"`
	BaseURL         string                 `json:"base_url"`
	Count           int                    `json:"count"` // Total operation count
}

// WSDLSchemaInfo contains summarized schema information for the response
type WSDLSchemaInfo struct {
	ServiceCount   int `json:"service_count"`
	PortCount      int `json:"port_count"`
	OperationCount int `json:"operation_count"`
	MessageCount   int `json:"message_count"`
	PortTypeCount  int `json:"port_type_count"`
	BindingCount   int `json:"binding_count"`
	TypeCount      int `json:"type_count"`
}

// ParseWSDLFromBytesInput represents input for parsing from raw WSDL content
type ParseWSDLFromBytesInput struct {
	Content         string            `json:"content" validate:"required"`
	BaseURL         string            `json:"base_url" validate:"required,url"`
	Headers         map[string]string `json:"headers,omitempty"`
	IncludeOptional bool              `json:"include_optional"`
	PreferSOAP12    bool              `json:"prefer_soap_12"`
}

// ParseWSDL godoc
// @Summary Parse a WSDL specification from a URL
// @Description Fetches and parses a WSDL specification, resolving imports and generating SOAP requests for all operations
// @Tags Playground
// @Accept json
// @Produce json
// @Param input body ParseWSDLInput true "WSDL URL and configuration"
// @Success 200 {object} ParseWSDLResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/wsdl/parse [post]
func ParseWSDL(c *fiber.Ctx) error {
	input := new(ParseWSDLInput)

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
	parser := wsdl.NewParser()
	if input.Headers != nil && len(input.Headers) > 0 {
		parser.WithHeaders(input.Headers)
	}

	// Parse WSDL from URL
	doc, err := parser.ParseFromURL(input.URL)
	if err != nil {
		log.Error().Err(err).Str("url", input.URL).Msg("Failed to parse WSDL")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Failed to parse WSDL",
			"message": err.Error(),
		})
	}

	// Configure request generation
	config := wsdl.GenerationConfig{
		IncludeOptionalParams: input.IncludeOptional,
		Headers:               input.Headers,
		PreferSOAP12:          input.PreferSOAP12,
	}

	// Generate SOAP requests
	generator := wsdl.NewGenerator(doc, config)
	services, err := generator.GenerateRequests()
	if err != nil {
		log.Error().Err(err).Str("url", input.URL).Msg("Failed to generate SOAP requests")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to generate requests",
			"message": err.Error(),
		})
	}

	// Count total operations and ports
	opCount := 0
	portCount := 0
	for _, svc := range services {
		opCount += len(svc.Operations)
		portCount++
	}

	// Count types in registry
	typeCount := 0
	if doc.TypeRegistry != nil {
		typeCount = len(doc.TypeRegistry.ComplexTypes) + len(doc.TypeRegistry.SimpleTypes)
	}

	// Build schema info summary
	schemaInfo := &WSDLSchemaInfo{
		ServiceCount:   len(doc.Services),
		PortCount:      portCount,
		OperationCount: opCount,
		MessageCount:   len(doc.Messages),
		PortTypeCount:  len(doc.PortTypes),
		BindingCount:   len(doc.Bindings),
		TypeCount:      typeCount,
	}

	response := ParseWSDLResponse{
		Services:        services,
		Schema:          schemaInfo,
		TargetNamespace: doc.TargetNamespace,
		BaseURL:         input.URL,
		Count:           opCount,
	}

	return c.Status(fiber.StatusOK).JSON(response)
}

// ParseWSDLFromBytes godoc
// @Summary Parse a WSDL specification from raw content
// @Description Parses WSDL from provided XML content (useful when WSDL endpoint is not directly accessible)
// @Tags Playground
// @Accept json
// @Produce json
// @Param input body ParseWSDLFromBytesInput true "WSDL content and configuration"
// @Success 200 {object} ParseWSDLResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/wsdl/parse-content [post]
func ParseWSDLFromBytes(c *fiber.Ctx) error {
	input := new(ParseWSDLFromBytesInput)

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

	// Create parser with custom headers (for resolving imports)
	parser := wsdl.NewParser()
	if input.Headers != nil && len(input.Headers) > 0 {
		parser.WithHeaders(input.Headers)
	}

	// Parse WSDL from bytes
	doc, err := parser.ParseFromBytes([]byte(input.Content), input.BaseURL)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse WSDL content")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Failed to parse WSDL content",
			"message": err.Error(),
		})
	}

	// Configure request generation
	config := wsdl.GenerationConfig{
		BaseURL:               input.BaseURL,
		IncludeOptionalParams: input.IncludeOptional,
		Headers:               input.Headers,
		PreferSOAP12:          input.PreferSOAP12,
	}

	// Generate SOAP requests
	generator := wsdl.NewGenerator(doc, config)
	services, err := generator.GenerateRequests()
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate SOAP requests")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to generate requests",
			"message": err.Error(),
		})
	}

	// Count total operations and ports
	opCount := 0
	portCount := 0
	for _, svc := range services {
		opCount += len(svc.Operations)
		portCount++
	}

	// Count types in registry
	typeCount := 0
	if doc.TypeRegistry != nil {
		typeCount = len(doc.TypeRegistry.ComplexTypes) + len(doc.TypeRegistry.SimpleTypes)
	}

	// Build schema info summary
	schemaInfo := &WSDLSchemaInfo{
		ServiceCount:   len(doc.Services),
		PortCount:      portCount,
		OperationCount: opCount,
		MessageCount:   len(doc.Messages),
		PortTypeCount:  len(doc.PortTypes),
		BindingCount:   len(doc.Bindings),
		TypeCount:      typeCount,
	}

	response := ParseWSDLResponse{
		Services:        services,
		Schema:          schemaInfo,
		TargetNamespace: doc.TargetNamespace,
		BaseURL:         input.BaseURL,
		Count:           opCount,
	}

	return c.Status(fiber.StatusOK).JSON(response)
}
