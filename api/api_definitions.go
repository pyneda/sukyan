package api

import (
	"encoding/json"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	pkgapi "github.com/pyneda/sukyan/pkg/api"
	"github.com/pyneda/sukyan/pkg/openapi"
	"github.com/pyneda/sukyan/pkg/scan/executor"
	"github.com/pyneda/sukyan/pkg/scan/manager"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
)

type CreateAPIDefinitionInput struct {
	WorkspaceID  uint       `json:"workspace_id" validate:"required"`
	Name         string     `json:"name" validate:"omitempty,max=255"`
	URL          string     `json:"url" validate:"omitempty,url"`
	Content      string     `json:"content" validate:"omitempty"`
	BaseURL      string     `json:"base_url" validate:"omitempty"`
	AuthConfigID *uuid.UUID `json:"auth_config_id" validate:"omitempty"`
	Type         string     `json:"type" validate:"omitempty,oneof=openapi graphql wsdl"`
}

type StartAPIDefinitionScanInput struct {
	EndpointIDs         []uuid.UUID          `json:"endpoint_ids" validate:"omitempty"`
	Mode                string               `json:"mode" validate:"omitempty,oneof=fast smart fuzz"`
	RunAPISpecificTests *bool                `json:"run_api_specific_tests"`
	RunStandardTests    *bool                `json:"run_standard_tests"`
	ServerSideChecks    *bool                `json:"server_side_checks"`
	ClientSideChecks    *bool                `json:"client_side_checks"`
	PassiveChecks       *bool                `json:"passive_checks"`
	AuthConfigID        *uuid.UUID           `json:"auth_config_id" validate:"omitempty"`
	SchemeAuthMap       map[string]uuid.UUID `json:"scheme_auth_map,omitempty"`
}

type UpdateAPIDefinitionInput struct {
	Name         *string    `json:"name" validate:"omitempty,max=255"`
	BaseURL      *string    `json:"base_url" validate:"omitempty"`
	AuthConfigID *uuid.UUID `json:"auth_config_id" validate:"omitempty"`
}

type UpdateAPIEndpointInput struct {
	Enabled *bool   `json:"enabled" validate:"omitempty"`
	Name    *string `json:"name" validate:"omitempty,max=255"`
}

type APIDefinitionResponse struct {
	db.APIDefinition
	Stats *db.APIDefinitionStats `json:"stats,omitempty"`
}

type APIDefinitionListResponse struct {
	Items []*db.APIDefinition `json:"items"`
	Count int64               `json:"count"`
}

type APIEndpointListResponse struct {
	Items []*db.APIEndpoint `json:"items"`
	Count int64             `json:"count"`
}

// ListAPIDefinitions godoc
// @Summary List API definitions
// @Description Returns a list of API definitions with optional filtering
// @Tags api-definitions
// @Accept json
// @Produce json
// @Param workspace_id query int false "Filter by workspace ID"
// @Param scan_id query int false "Filter by scan ID"
// @Param type query string false "Filter by type (openapi, graphql, wsdl)"
// @Param status query string false "Filter by status"
// @Param page query int false "Page number"
// @Param page_size query int false "Page size"
// @Success 200 {object} APIDefinitionListResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-definitions [get]
func ListAPIDefinitions(c *fiber.Ctx) error {
	filter := db.APIDefinitionFilter{
		WorkspaceID: uint(c.QueryInt("workspace_id", 0)),
		Pagination: db.Pagination{
			Page:     c.QueryInt("page", 1),
			PageSize: c.QueryInt("page_size", 20),
		},
	}

	if scanID := c.QueryInt("scan_id", 0); scanID > 0 {
		sid := uint(scanID)
		filter.ScanID = &sid
	}

	if typeFilter := c.Query("type"); typeFilter != "" {
		filter.Types = []db.APIDefinitionType{db.APIDefinitionType(typeFilter)}
	}

	if statusFilter := c.Query("status"); statusFilter != "" {
		filter.Statuses = []db.APIDefinitionStatus{db.APIDefinitionStatus(statusFilter)}
	}

	items, count, err := db.Connection().ListAPIDefinitions(filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(NewErrorResponse(err.Error()))
	}

	return c.JSON(APIDefinitionListResponse{Items: items, Count: count})
}

// GetAPIDefinition godoc
// @Summary Get API definition by ID
// @Description Returns a single API definition with its endpoints
// @Tags api-definitions
// @Accept json
// @Produce json
// @Param id path string true "API Definition ID"
// @Success 200 {object} APIDefinitionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-definitions/{id} [get]
func GetAPIDefinition(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid ID format"))
	}

	definition, err := db.Connection().GetAPIDefinitionByIDWithEndpoints(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(NewErrorResponse("API definition not found"))
	}

	stats, _ := db.Connection().GetAPIDefinitionStats(id)

	return c.JSON(APIDefinitionResponse{APIDefinition: *definition, Stats: stats})
}

// UpdateAPIDefinition godoc
// @Summary Update API definition
// @Description Updates an existing API definition
// @Tags api-definitions
// @Accept json
// @Produce json
// @Param id path string true "API Definition ID"
// @Param input body UpdateAPIDefinitionInput true "Update input"
// @Success 200 {object} db.APIDefinition
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-definitions/{id} [patch]
func UpdateAPIDefinition(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid ID format"))
	}

	var input UpdateAPIDefinitionInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid request body"))
	}

	validate := validator.New()
	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse(err.Error()))
	}

	definition, err := db.Connection().GetAPIDefinitionByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(NewErrorResponse("API definition not found"))
	}

	workspaceID := uint(c.QueryInt("workspace_id", 0))
	if workspaceID > 0 && definition.WorkspaceID != workspaceID {
		return c.Status(fiber.StatusForbidden).JSON(NewErrorResponse("API definition does not belong to the specified workspace"))
	}

	if input.Name != nil {
		definition.Name = *input.Name
	}
	if input.BaseURL != nil {
		definition.BaseURL = *input.BaseURL
	}
	if input.AuthConfigID != nil {
		definition.AuthConfigID = input.AuthConfigID
	}

	updated, err := db.Connection().UpdateAPIDefinition(definition)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(NewErrorResponse(err.Error()))
	}

	return c.JSON(updated)
}

// DeleteAPIDefinition godoc
// @Summary Delete API definition
// @Description Deletes an API definition and all its endpoints
// @Tags api-definitions
// @Accept json
// @Produce json
// @Param id path string true "API Definition ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-definitions/{id} [delete]
func DeleteAPIDefinition(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid ID format"))
	}

	definition, err := db.Connection().GetAPIDefinitionByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(NewErrorResponse("API definition not found"))
	}

	workspaceID := uint(c.QueryInt("workspace_id", 0))
	if workspaceID > 0 && definition.WorkspaceID != workspaceID {
		return c.Status(fiber.StatusForbidden).JSON(NewErrorResponse("API definition does not belong to the specified workspace"))
	}

	if err := db.Connection().DeleteAPIDefinition(id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(NewErrorResponse(err.Error()))
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// ListAPIEndpoints godoc
// @Summary List endpoints for an API definition
// @Description Returns a list of endpoints for a specific API definition
// @Tags api-definitions
// @Accept json
// @Produce json
// @Param id path string true "API Definition ID"
// @Param enabled query bool false "Filter by enabled status"
// @Param method query string false "Filter by HTTP method"
// @Param page query int false "Page number"
// @Param page_size query int false "Page size"
// @Success 200 {object} APIEndpointListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-definitions/{id}/endpoints [get]
func ListAPIEndpoints(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid ID format"))
	}

	filter := db.APIEndpointFilter{
		DefinitionID: &id,
		Pagination: db.Pagination{
			Page:     c.QueryInt("page", 1),
			PageSize: c.QueryInt("page_size", 50),
		},
	}

	if enabledStr := c.Query("enabled"); enabledStr != "" {
		enabled := enabledStr == "true"
		filter.Enabled = &enabled
	}

	if method := c.Query("method"); method != "" {
		filter.Methods = []string{method}
	}

	items, count, err := db.Connection().ListAPIEndpoints(filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(NewErrorResponse(err.Error()))
	}

	return c.JSON(APIEndpointListResponse{Items: items, Count: count})
}

// GetAPIEndpoint godoc
// @Summary Get API endpoint by ID
// @Description Returns a single API endpoint with its parameters
// @Tags api-definitions
// @Accept json
// @Produce json
// @Param id path string true "API Definition ID"
// @Param endpoint_id path string true "API Endpoint ID"
// @Success 200 {object} db.APIEndpoint
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-definitions/{id}/endpoints/{endpoint_id} [get]
func GetAPIEndpoint(c *fiber.Ctx) error {
	endpointID, err := uuid.Parse(c.Params("endpoint_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid endpoint ID format"))
	}

	endpoint, err := db.Connection().GetAPIEndpointByIDWithRelations(endpointID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(NewErrorResponse("API endpoint not found"))
	}

	return c.JSON(endpoint)
}

// UpdateAPIEndpoint godoc
// @Summary Update API endpoint
// @Description Updates an existing API endpoint
// @Tags api-definitions
// @Accept json
// @Produce json
// @Param id path string true "API Definition ID"
// @Param endpoint_id path string true "API Endpoint ID"
// @Param input body UpdateAPIEndpointInput true "Update input"
// @Success 200 {object} db.APIEndpoint
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-definitions/{id}/endpoints/{endpoint_id} [patch]
func UpdateAPIEndpoint(c *fiber.Ctx) error {
	endpointID, err := uuid.Parse(c.Params("endpoint_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid endpoint ID format"))
	}

	var input UpdateAPIEndpointInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid request body"))
	}

	endpoint, err := db.Connection().GetAPIEndpointByID(endpointID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(NewErrorResponse("API endpoint not found"))
	}

	definition, defErr := db.Connection().GetAPIDefinitionByID(endpoint.DefinitionID)
	if defErr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(NewErrorResponse("Failed to verify endpoint ownership"))
	}
	workspaceID := uint(c.QueryInt("workspace_id", 0))
	if workspaceID > 0 && definition.WorkspaceID != workspaceID {
		return c.Status(fiber.StatusForbidden).JSON(NewErrorResponse("API endpoint does not belong to the specified workspace"))
	}

	if input.Enabled != nil {
		endpoint.Enabled = *input.Enabled
	}
	if input.Name != nil {
		endpoint.Name = *input.Name
	}

	updated, err := db.Connection().UpdateAPIEndpoint(endpoint)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(NewErrorResponse(err.Error()))
	}

	return c.JSON(updated)
}

// ToggleAllEndpoints godoc
// @Summary Enable or disable all endpoints
// @Description Sets the enabled status for all endpoints in an API definition
// @Tags api-definitions
// @Accept json
// @Produce json
// @Param id path string true "API Definition ID"
// @Param enabled query bool true "Enable or disable all endpoints"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-definitions/{id}/endpoints/toggle-all [post]
func ToggleAllEndpoints(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid ID format"))
	}

	enabled := c.QueryBool("enabled", true)

	if err := db.Connection().SetAPIEndpointsEnabledByDefinition(id, enabled); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(NewErrorResponse(err.Error()))
	}

	return c.JSON(SuccessResponse{Message: "Endpoints updated"})
}

// CreateAPIDefinition godoc
// @Summary Create API definition
// @Description Creates a new API definition by parsing from URL or provided content
// @Tags api-definitions
// @Accept json
// @Produce json
// @Param input body CreateAPIDefinitionInput true "API definition input"
// @Success 201 {object} APIDefinitionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-definitions [post]
func CreateAPIDefinition(c *fiber.Ctx) error {
	var input CreateAPIDefinitionInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid request body"))
	}

	validate := validator.New()
	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse(err.Error()))
	}

	if input.URL == "" && input.Content == "" {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Either url or content is required"))
	}

	workspaceExists, _ := db.Connection().WorkspaceExists(input.WorkspaceID)
	if !workspaceExists {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid workspace"))
	}

	if input.AuthConfigID != nil {
		authConfig, err := db.Connection().GetAPIAuthConfigByID(*input.AuthConfigID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Auth config not found"))
		}
		if authConfig.WorkspaceID != input.WorkspaceID {
			return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Auth config does not belong to the specified workspace"))
		}
	}

	fetched, fetchErr := pkgapi.FetchAPIContent(input.URL, input.Content, input.Type)
	if fetchErr != nil {
		log.Error().Err(fetchErr).Str("url", input.URL).Msg("Failed to fetch API content")
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Failed to fetch API content", fetchErr.Error()))
	}

	if fetched.Type == "" {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Unable to detect API type. Please specify type parameter (openapi, graphql, wsdl)"))
	}

	opts := pkgapi.ImportOptions{
		WorkspaceID:  input.WorkspaceID,
		Name:         input.Name,
		SourceURL:    fetched.SourceURL,
		BaseURL:      input.BaseURL,
		Type:         string(fetched.Type),
		AuthConfigID: input.AuthConfigID,
	}

	definition, err := pkgapi.ImportAPIDefinition(fetched.Content, fetched.SourceURL, opts)
	if err != nil {
		log.Error().Err(err).Str("type", string(fetched.Type)).Msg("Failed to create API definition")
		return c.Status(fiber.StatusInternalServerError).JSON(NewErrorResponse("Failed to create API definition", err.Error()))
	}

	stats, _ := db.Connection().GetAPIDefinitionStats(definition.ID)

	return c.Status(fiber.StatusCreated).JSON(APIDefinitionResponse{
		APIDefinition: *definition,
		Stats:         stats,
	})
}


// StartAPIDefinitionScan godoc
// @Summary Start scan for API definition
// @Description Creates and starts a scan for the specified API definition
// @Tags api-definitions
// @Accept json
// @Produce json
// @Param id path string true "API Definition ID"
// @Param input body StartAPIDefinitionScanInput true "Scan configuration"
// @Success 200 {object} ActionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-definitions/{id}/scan [post]
func StartAPIDefinitionScan(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid ID format"))
	}

	var input StartAPIDefinitionScanInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid request body"))
	}

	validate := validator.New()
	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse(err.Error()))
	}

	definition, err := db.Connection().GetAPIDefinitionByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(NewErrorResponse("API definition not found"))
	}

	scanManager := GetScanManager()
	if scanManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(NewErrorResponse("Scan manager not available"))
	}

	mode := options.ScanModeSmart
	if input.Mode != "" {
		mode = options.NewScanMode(input.Mode)
	}

	serverSide := true
	if input.ServerSideChecks != nil {
		serverSide = *input.ServerSideChecks
	}

	clientSide := false
	if input.ClientSideChecks != nil {
		clientSide = *input.ClientSideChecks
	}

	passive := true
	if input.PassiveChecks != nil {
		passive = *input.PassiveChecks
	}

	runAPISpecificTests := true
	if input.RunAPISpecificTests != nil {
		runAPISpecificTests = *input.RunAPISpecificTests
	}

	runStandardTests := true
	if input.RunStandardTests != nil {
		runStandardTests = *input.RunStandardTests
	}

	auditCategories := options.AuditCategories{
		ServerSide: serverSide,
		ClientSide: clientSide,
		Passive:    passive,
	}

	baseURL := definition.BaseURL
	if baseURL == "" {
		baseURL = definition.SourceURL
	}

	scanOpts := options.FullScanOptions{
		Title:           "API Scan - " + definition.Name,
		StartURLs:       []string{baseURL},
		WorkspaceID:     definition.WorkspaceID,
		AuditCategories: auditCategories,
		Mode:            mode,
		APIScanOptions: options.FullScanAPIScanOptions{
			Enabled:             true,
			RunAPISpecificTests: runAPISpecificTests,
			RunStandardTests:    runStandardTests,
		},
		PagesPoolSize: 1,
		MaxRetries:    3,
	}

	scan, err := manager.CreateAdHocScanWithOptions(scanManager, scanOpts)
	if err != nil {
		log.Error().Err(err).Str("definition_id", id.String()).Msg("Failed to create scan")
		return c.Status(fiber.StatusInternalServerError).JSON(NewErrorResponse("Failed to create scan", err.Error()))
	}

	var endpoints []*db.APIEndpoint
	if len(input.EndpointIDs) > 0 {
		for _, endpointID := range input.EndpointIDs {
			endpoint, err := db.Connection().GetAPIEndpointByID(endpointID)
			if err == nil && endpoint.DefinitionID == definition.ID {
				endpoints = append(endpoints, endpoint)
			}
		}
	} else {
		endpoints, err = db.Connection().GetEnabledAPIEndpointsByDefinitionID(definition.ID)
		if err != nil {
			log.Warn().Err(err).Str("definition_id", id.String()).Msg("Failed to get endpoints")
		}
	}

	if len(endpoints) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("No endpoints to scan. Enable some endpoints or check endpoint IDs."))
	}

	apiScan := &db.APIScan{
		ScanID:              scan.ID,
		DefinitionID:        definition.ID,
		RunAPISpecificTests: runAPISpecificTests,
		RunStandardTests:    runStandardTests,
		TotalEndpoints:      len(endpoints),
	}

	apiScan, err = db.Connection().CreateAPIScan(apiScan)
	if err != nil {
		log.Error().Err(err).Str("definition_id", id.String()).Msg("Failed to create APIScan record")
		return c.Status(fiber.StatusInternalServerError).JSON(NewErrorResponse("Failed to create API scan record", err.Error()))
	}

	db.Connection().MarkAPIScanStarted(apiScan.ID)

	var schemeAuthMap map[string]uuid.UUID
	var authConfigID *uuid.UUID

	if len(input.SchemeAuthMap) > 0 {
		schemeAuthMap = input.SchemeAuthMap
	} else if input.AuthConfigID != nil {
		authConfigID = input.AuthConfigID
	} else {
		authConfigID = definition.AuthConfigID
	}

	jobsScheduled := 0
	for _, endpoint := range endpoints {
		if err := scheduleAPIScanJob(scan, definition, endpoint, apiScan, mode, auditCategories, runAPISpecificTests, runStandardTests, authConfigID, schemeAuthMap); err != nil {
			log.Warn().Err(err).Str("endpoint_id", endpoint.ID.String()).Msg("Failed to schedule API scan job")
			continue
		}
		jobsScheduled++
	}

	db.Connection().UpdateScanJobCounts(scan.ID)

	log.Info().
		Uint("scan_id", scan.ID).
		Str("definition_id", id.String()).
		Int("jobs_scheduled", jobsScheduled).
		Msg("API definition scan started")

	return c.JSON(fiber.Map{
		"message":        "API scan started",
		"scan_id":        scan.ID,
		"api_scan_id":    apiScan.ID.String(),
		"jobs_scheduled": jobsScheduled,
	})
}

func scheduleAPIScanJob(
	scan *db.Scan,
	definition *db.APIDefinition,
	endpoint *db.APIEndpoint,
	apiScan *db.APIScan,
	mode options.ScanMode,
	auditCategories options.AuditCategories,
	runAPISpecificTests bool,
	runStandardTests bool,
	authConfigID *uuid.UUID,
	schemeAuthMap map[string]uuid.UUID,
) error {
	baseURL := definition.BaseURL
	if baseURL == "" {
		baseURL = pkgapi.DeriveBaseURLFromSpecURL(definition.SourceURL)
	}

	fullURL := baseURL + endpoint.Path

	jobData := executor.APIScanJobData{
		DefinitionID:        definition.ID,
		EndpointID:          endpoint.ID,
		APIScanID:           apiScan.ID,
		Mode:                mode,
		AuditCategories:     auditCategories,
		RunAPISpecificTests: runAPISpecificTests,
		RunStandardTests:    runStandardTests,
		AuthConfigID:        authConfigID,
		SchemeAuthMap:       schemeAuthMap,
		MaxRetries:          3,
	}
	payload, _ := json.Marshal(jobData)

	method := endpoint.Method
	if method == "" {
		method = "GET"
	}

	job := &db.ScanJob{
		ScanID:      scan.ID,
		WorkspaceID: scan.WorkspaceID,
		Status:      db.ScanJobStatusPending,
		JobType:     db.ScanJobTypeAPIScan,
		Priority:    8,
		TargetHost:  extractHost(baseURL),
		URL:         fullURL,
		Method:      method,
		Payload:     payload,
	}

	_, err := db.Connection().CreateScanJob(job)
	return err
}

func extractHost(urlStr string) string {
	if urlStr == "" {
		return ""
	}
	parts := strings.Split(urlStr, "://")
	if len(parts) < 2 {
		return urlStr
	}
	hostPart := parts[1]
	if idx := strings.Index(hostPart, "/"); idx != -1 {
		hostPart = hostPart[:idx]
	}
	return hostPart
}



// GetAPIDefinitionSecuritySchemes godoc
// @Summary Get security schemes for an API definition
// @Description Returns the security schemes and global security requirements for an API definition
// @Tags api-definitions
// @Accept json
// @Produce json
// @Param id path string true "API Definition ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-definitions/{id}/security-schemes [get]
func GetAPIDefinitionSecuritySchemes(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid ID format"))
	}

	definition, err := db.Connection().GetAPIDefinitionByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(NewErrorResponse("API definition not found"))
	}

	schemes, err := db.Connection().GetAPIDefinitionSecuritySchemes(id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(NewErrorResponse(err.Error()))
	}

	// Lazy backfill: if no schemes in DB but we have the raw spec, parse and populate
	if len(schemes) == 0 && definition.Type == db.APIDefinitionTypeOpenAPI && len(definition.RawDefinition) > 0 {
		doc, parseErr := openapi.Parse(definition.RawDefinition)
		if parseErr == nil {
			specSchemes := doc.GetSecuritySchemes()
			if len(specSchemes) > 0 {
				var dbSchemes []*db.APIDefinitionSecurityScheme
				for _, s := range specSchemes {
					dbSchemes = append(dbSchemes, &db.APIDefinitionSecurityScheme{
						DefinitionID:     definition.ID,
						Name:             s.Name,
						Type:             s.Type,
						Scheme:           s.Scheme,
						In:               s.In,
						ParameterName:    s.ParameterName,
						BearerFormat:     s.BearerFormat,
						Description:      s.Description,
						OpenIDConnectURL: s.OpenIDConnectURL,
					})
				}
				if createErr := db.Connection().CreateAPIDefinitionSecuritySchemes(dbSchemes); createErr == nil {
					schemes = dbSchemes
				}
			}

			globalSec := doc.GetGlobalSecurityRequirements()
			if len(globalSec) > 0 {
				if globalSecJSON, marshalErr := json.Marshal(globalSec); marshalErr == nil {
					definition.GlobalSecurityJSON = globalSecJSON
					db.Connection().UpdateAPIDefinition(definition)
				}
			}
		}
	}

	var globalSecurity []openapi.SecurityRequirement
	if len(definition.GlobalSecurityJSON) > 0 {
		if err := json.Unmarshal(definition.GlobalSecurityJSON, &globalSecurity); err != nil {
			log.Warn().Err(err).Str("id", id.String()).Msg("Failed to unmarshal global security JSON")
		}
	}

	return c.JSON(fiber.Map{
		"security_schemes": schemes,
		"global_security":  globalSecurity,
	})
}

type ImportAndScanInput struct {
	WorkspaceID         uint                         `json:"workspace_id" validate:"required"`
	URL                 string                       `json:"url" validate:"omitempty,url"`
	Content             string                       `json:"content" validate:"omitempty"`
	Type                string                       `json:"type" validate:"omitempty,oneof=openapi graphql wsdl"`
	BaseURL             string                       `json:"base_url" validate:"omitempty"`
	Name                string                       `json:"name" validate:"omitempty,max=255"`
	AuthConfigID        *uuid.UUID                   `json:"auth_config_id" validate:"omitempty"`
	Mode                string                       `json:"mode" validate:"omitempty,oneof=fast smart fuzz"`
	RunAPISpecificTests *bool                        `json:"run_api_specific_tests"`
	RunStandardTests    *bool                        `json:"run_standard_tests"`
	RunSchemaTests      *bool                        `json:"run_schema_tests"`
	AuditCategories     *options.AuditCategories     `json:"audit_categories" validate:"omitempty"`
}

// ImportAndScanAPIDefinition godoc
// @Summary Import and scan API definition in one step
// @Description Imports an API definition and immediately starts a scan for it
// @Tags api-definitions
// @Accept json
// @Produce json
// @Param input body ImportAndScanInput true "Import and scan configuration"
// @Success 200 {object} ActionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-definitions/import-and-scan [post]
func ImportAndScanAPIDefinition(c *fiber.Ctx) error {
	var input ImportAndScanInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid request body"))
	}

	validate := validator.New()
	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse(err.Error()))
	}

	if input.URL == "" && input.Content == "" {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Either url or content is required"))
	}

	workspaceExists, _ := db.Connection().WorkspaceExists(input.WorkspaceID)
	if !workspaceExists {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid workspace"))
	}

	fetched, fetchErr := pkgapi.FetchAPIContent(input.URL, input.Content, input.Type)
	if fetchErr != nil {
		log.Error().Err(fetchErr).Str("url", input.URL).Msg("Failed to fetch API content")
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Failed to fetch API content", fetchErr.Error()))
	}

	if fetched.Type == "" {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Unable to detect API type. Please specify type parameter (openapi, graphql, wsdl)"))
	}

	importOpts := pkgapi.ImportOptions{
		WorkspaceID:  input.WorkspaceID,
		Name:         input.Name,
		SourceURL:    fetched.SourceURL,
		BaseURL:      input.BaseURL,
		Type:         string(fetched.Type),
		AuthConfigID: input.AuthConfigID,
	}

	definition, err := pkgapi.ImportAPIDefinition(fetched.Content, fetched.SourceURL, importOpts)
	if err != nil {
		log.Error().Err(err).Str("type", string(fetched.Type)).Msg("Failed to create API definition")
		return c.Status(fiber.StatusInternalServerError).JSON(NewErrorResponse("Failed to create API definition", err.Error()))
	}

	scanManager := GetScanManager()
	if scanManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(NewErrorResponse("Scan manager not available"))
	}

	mode := options.ScanModeSmart
	if input.Mode != "" {
		mode = options.NewScanMode(input.Mode)
	}

	auditCategories := options.AuditCategories{
		ServerSide: true,
		ClientSide: false,
		Passive:    true,
	}
	if input.AuditCategories != nil {
		auditCategories = *input.AuditCategories
	}

	runAPISpecificTests := true
	if input.RunAPISpecificTests != nil {
		runAPISpecificTests = *input.RunAPISpecificTests
	}

	runStandardTests := true
	if input.RunStandardTests != nil {
		runStandardTests = *input.RunStandardTests
	}

	runSchemaTests := true
	if input.RunSchemaTests != nil {
		runSchemaTests = *input.RunSchemaTests
	}

	baseURL := definition.BaseURL
	if baseURL == "" {
		baseURL = definition.SourceURL
	}

	scanOpts := options.FullScanOptions{
		Title:           "API Scan - " + definition.Name,
		StartURLs:       []string{},
		WorkspaceID:     definition.WorkspaceID,
		AuditCategories: auditCategories,
		Mode:            mode,
		APIScanOptions: options.FullScanAPIScanOptions{
			Enabled:             true,
			RunAPISpecificTests: runAPISpecificTests,
			RunStandardTests:    runStandardTests,
			RunSchemaTests:      runSchemaTests,
			DefinitionConfigs: []options.APIDefinitionScanConfig{
				{DefinitionID: definition.ID},
			},
		},
		PagesPoolSize: 1,
		MaxRetries:    3,
	}

	scan, err := scanManager.StartFullScan(scanOpts)
	if err != nil {
		log.Error().Err(err).Str("definition_id", definition.ID.String()).Msg("Failed to start scan")
		return c.Status(fiber.StatusInternalServerError).JSON(NewErrorResponse("Failed to start scan", err.Error()))
	}

	if err := db.Connection().LinkAPIDefinitionToScan(scan.ID, definition.ID); err != nil {
		log.Warn().Err(err).Uint("scan_id", scan.ID).Str("definition_id", definition.ID.String()).Msg("Failed to link definition to scan")
	}

	log.Info().
		Uint("scan_id", scan.ID).
		Str("definition_id", definition.ID.String()).
		Int("endpoint_count", definition.EndpointCount).
		Msg("API definition imported and scan started")

	return c.JSON(fiber.Map{
		"message":        "API definition imported and scan started",
		"scan_id":        scan.ID,
		"definition_id":  definition.ID.String(),
		"endpoint_count": definition.EndpointCount,
	})
}
