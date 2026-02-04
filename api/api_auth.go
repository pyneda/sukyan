package api

import (
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/scan/auth"
	"github.com/rs/zerolog/log"
)

type CreateAPIAuthConfigInput struct {
	WorkspaceID    uint              `json:"workspace_id" validate:"required"`
	Name           string            `json:"name" validate:"required,max=255"`
	Type           db.APIAuthType    `json:"type" validate:"required,oneof=none basic bearer api_key oauth2"`
	Username       string            `json:"username" validate:"omitempty,max=255"`
	Password       string            `json:"password" validate:"omitempty,max=500"`
	Token          string            `json:"token" validate:"omitempty"`
	TokenPrefix    string            `json:"token_prefix" validate:"omitempty,max=50"`
	APIKeyName     string            `json:"api_key_name" validate:"omitempty,max=255"`
	APIKeyValue    string            `json:"api_key_value" validate:"omitempty"`
	APIKeyLocation db.APIKeyLocation `json:"api_key_location" validate:"omitempty,oneof=header query cookie"`
	CustomHeaders      []CustomHeaderInput      `json:"custom_headers" validate:"omitempty,dive"`
	TokenRefreshConfig *TokenRefreshConfigInput `json:"token_refresh_config" validate:"omitempty"`
}

type UpdateAPIAuthConfigInput struct {
	Name           *string              `json:"name" validate:"omitempty,max=255"`
	Type           *db.APIAuthType      `json:"type" validate:"omitempty,oneof=none basic bearer api_key oauth2"`
	Username       *string              `json:"username" validate:"omitempty,max=255"`
	Password       *string              `json:"password" validate:"omitempty,max=500"`
	Token          *string              `json:"token" validate:"omitempty"`
	TokenPrefix    *string              `json:"token_prefix" validate:"omitempty,max=50"`
	APIKeyName     *string              `json:"api_key_name" validate:"omitempty,max=255"`
	APIKeyValue    *string              `json:"api_key_value" validate:"omitempty"`
	APIKeyLocation *db.APIKeyLocation   `json:"api_key_location" validate:"omitempty,oneof=header query cookie"`
	CustomHeaders            []CustomHeaderInput      `json:"custom_headers" validate:"omitempty,dive"`
	TokenRefreshConfig       *TokenRefreshConfigInput `json:"token_refresh_config,omitempty"`
	RemoveTokenRefreshConfig *bool                    `json:"remove_token_refresh_config,omitempty"`
}

type CustomHeaderInput struct {
	HeaderName  string `json:"header_name" validate:"required,max=255"`
	HeaderValue string `json:"header_value" validate:"required"`
}

type TokenRefreshConfigInput struct {
	RequestURL         string            `json:"request_url" validate:"required,url"`
	RequestMethod      string            `json:"request_method" validate:"required,oneof=GET POST PUT"`
	RequestHeaders     map[string]string `json:"request_headers" validate:"omitempty"`
	RequestBody        string            `json:"request_body" validate:"omitempty"`
	RequestContentType string            `json:"request_content_type" validate:"omitempty,max=100"`
	IntervalSeconds    int               `json:"interval_seconds" validate:"required,min=1"`
	ExtractionSource   string            `json:"extraction_source" validate:"required,oneof=body_jsonpath response_header"`
	ExtractionValue    string            `json:"extraction_value" validate:"required"`
}

type APIAuthConfigListResponse struct {
	Items []*db.APIAuthConfig `json:"items"`
	Count int64               `json:"count"`
}

// ListAPIAuthConfigs godoc
// @Summary List API auth configurations
// @Description Returns a list of API authentication configurations
// @Tags api-auth
// @Accept json
// @Produce json
// @Param workspace_id query int true "Workspace ID"
// @Param page query int false "Page number"
// @Param page_size query int false "Page size"
// @Success 200 {object} APIAuthConfigListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-auth-configs [get]
func ListAPIAuthConfigs(c *fiber.Ctx) error {
	workspaceID := uint(c.QueryInt("workspace_id", 0))
	if workspaceID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "workspace_id is required"})
	}

	filter := db.APIAuthConfigFilter{
		WorkspaceID: workspaceID,
		Pagination: db.Pagination{
			Page:     c.QueryInt("page", 1),
			PageSize: c.QueryInt("page_size", 20),
		},
	}

	items, count, err := db.Connection().ListAPIAuthConfigs(filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
	}

	return c.JSON(APIAuthConfigListResponse{Items: items, Count: count})
}

// GetAPIAuthConfig godoc
// @Summary Get API auth configuration by ID
// @Description Returns a single API authentication configuration
// @Tags api-auth
// @Accept json
// @Produce json
// @Param id path string true "Auth Config ID"
// @Success 200 {object} db.APIAuthConfig
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-auth-configs/{id} [get]
func GetAPIAuthConfig(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid ID format"})
	}

	config, err := db.Connection().GetAPIAuthConfigByIDWithRelations(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Auth config not found"})
	}

	return c.JSON(config)
}

// CreateAPIAuthConfig godoc
// @Summary Create API auth configuration
// @Description Creates a new API authentication configuration
// @Tags api-auth
// @Accept json
// @Produce json
// @Param input body CreateAPIAuthConfigInput true "Auth config input"
// @Success 201 {object} db.APIAuthConfig
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-auth-configs [post]
func CreateAPIAuthConfig(c *fiber.Ctx) error {
	var input CreateAPIAuthConfigInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid request body"})
	}

	validate := validator.New()
	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: err.Error()})
	}

	config := &db.APIAuthConfig{
		WorkspaceID:    input.WorkspaceID,
		Name:           input.Name,
		Type:           input.Type,
		Username:       input.Username,
		Password:       input.Password,
		Token:          input.Token,
		TokenPrefix:    input.TokenPrefix,
		APIKeyName:     input.APIKeyName,
		APIKeyValue:    input.APIKeyValue,
		APIKeyLocation: input.APIKeyLocation,
	}

	created, err := db.Connection().CreateAPIAuthConfig(config)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
	}

	if len(input.CustomHeaders) > 0 {
		headers := make([]*db.APIAuthHeader, 0, len(input.CustomHeaders))
		for _, h := range input.CustomHeaders {
			headers = append(headers, &db.APIAuthHeader{
				AuthConfigID: created.ID,
				HeaderName:   h.HeaderName,
				HeaderValue:  h.HeaderValue,
			})
		}
		if err := db.Connection().CreateAPIAuthHeaders(headers); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
		}
	}

	if input.TokenRefreshConfig != nil {
		refreshConfig := &db.TokenRefreshConfig{
			AuthConfigID:       created.ID,
			RequestURL:         input.TokenRefreshConfig.RequestURL,
			RequestMethod:      input.TokenRefreshConfig.RequestMethod,
			RequestHeaders:     input.TokenRefreshConfig.RequestHeaders,
			RequestBody:        input.TokenRefreshConfig.RequestBody,
			RequestContentType: input.TokenRefreshConfig.RequestContentType,
			IntervalSeconds:    input.TokenRefreshConfig.IntervalSeconds,
			ExtractionSource:   db.TokenExtractionSource(input.TokenRefreshConfig.ExtractionSource),
			ExtractionValue:    input.TokenRefreshConfig.ExtractionValue,
		}
		if _, err := db.Connection().CreateTokenRefreshConfig(refreshConfig); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
		}
	}

	created, _ = db.Connection().GetAPIAuthConfigByIDWithRelations(created.ID)

	return c.Status(fiber.StatusCreated).JSON(created)
}

// UpdateAPIAuthConfig godoc
// @Summary Update API auth configuration
// @Description Updates an existing API authentication configuration
// @Tags api-auth
// @Accept json
// @Produce json
// @Param id path string true "Auth Config ID"
// @Param input body UpdateAPIAuthConfigInput true "Update input"
// @Success 200 {object} db.APIAuthConfig
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-auth-configs/{id} [patch]
func UpdateAPIAuthConfig(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid ID format"})
	}

	var input UpdateAPIAuthConfigInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid request body"})
	}

	validate := validator.New()
	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: err.Error()})
	}

	config, err := db.Connection().GetAPIAuthConfigByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Auth config not found"})
	}

	workspaceID := uint(c.QueryInt("workspace_id", 0))
	if workspaceID > 0 && config.WorkspaceID != workspaceID {
		return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
			Error: "Auth config does not belong to the specified workspace",
		})
	}

	if input.Name != nil {
		config.Name = *input.Name
	}
	if input.Type != nil {
		config.Type = *input.Type
	}
	if input.Username != nil {
		config.Username = *input.Username
	}
	if input.Password != nil {
		config.Password = *input.Password
	}
	if input.Token != nil {
		config.Token = *input.Token
	}
	if input.TokenPrefix != nil {
		config.TokenPrefix = *input.TokenPrefix
	}
	if input.APIKeyName != nil {
		config.APIKeyName = *input.APIKeyName
	}
	if input.APIKeyValue != nil {
		config.APIKeyValue = *input.APIKeyValue
	}
	if input.APIKeyLocation != nil {
		config.APIKeyLocation = *input.APIKeyLocation
	}

	updated, err := db.Connection().UpdateAPIAuthConfig(config)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
	}

	if input.CustomHeaders != nil {
		if err := db.Connection().DeleteAPIAuthHeadersByConfigID(id); err != nil {
			log.Error().Err(err).Str("config_id", id.String()).Msg("Failed to delete existing auth headers")
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Failed to update custom headers"})
		}
		if len(input.CustomHeaders) > 0 {
			headers := make([]*db.APIAuthHeader, 0, len(input.CustomHeaders))
			for _, h := range input.CustomHeaders {
				headers = append(headers, &db.APIAuthHeader{
					AuthConfigID: id,
					HeaderName:   h.HeaderName,
					HeaderValue:  h.HeaderValue,
				})
			}
			if err := db.Connection().CreateAPIAuthHeaders(headers); err != nil {
				log.Error().Err(err).Str("config_id", id.String()).Msg("Failed to create auth headers")
				return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Failed to create custom headers"})
			}
		}
	}

	if input.RemoveTokenRefreshConfig != nil && *input.RemoveTokenRefreshConfig {
		if err := db.Connection().DeleteTokenRefreshConfigByAuthConfigID(id); err != nil {
			log.Error().Err(err).Str("config_id", id.String()).Msg("Failed to delete token refresh config")
		}
	} else if input.TokenRefreshConfig != nil {
		db.Connection().DeleteTokenRefreshConfigByAuthConfigID(id)
		refreshConfig := &db.TokenRefreshConfig{
			AuthConfigID:       id,
			RequestURL:         input.TokenRefreshConfig.RequestURL,
			RequestMethod:      input.TokenRefreshConfig.RequestMethod,
			RequestHeaders:     input.TokenRefreshConfig.RequestHeaders,
			RequestBody:        input.TokenRefreshConfig.RequestBody,
			RequestContentType: input.TokenRefreshConfig.RequestContentType,
			IntervalSeconds:    input.TokenRefreshConfig.IntervalSeconds,
			ExtractionSource:   db.TokenExtractionSource(input.TokenRefreshConfig.ExtractionSource),
			ExtractionValue:    input.TokenRefreshConfig.ExtractionValue,
		}
		if _, err := db.Connection().CreateTokenRefreshConfig(refreshConfig); err != nil {
			log.Error().Err(err).Str("config_id", id.String()).Msg("Failed to create token refresh config")
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Failed to update token refresh config"})
		}
	}

	updated, _ = db.Connection().GetAPIAuthConfigByIDWithRelations(updated.ID)

	return c.JSON(updated)
}

// DeleteAPIAuthConfig godoc
// @Summary Delete API auth configuration
// @Description Deletes an API authentication configuration
// @Tags api-auth
// @Accept json
// @Produce json
// @Param id path string true "Auth Config ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-auth-configs/{id} [delete]
func DeleteAPIAuthConfig(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid ID format"})
	}

	config, err := db.Connection().GetAPIAuthConfigByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Auth config not found"})
	}

	workspaceID := uint(c.QueryInt("workspace_id", 0))
	if workspaceID > 0 && config.WorkspaceID != workspaceID {
		return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
			Error: "Auth config does not belong to the specified workspace",
		})
	}

	if err := db.Connection().DeleteAPIAuthConfig(id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// TestTokenRefresh godoc
// @Summary Test token refresh configuration
// @Description Executes a token refresh request and returns the result
// @Tags api-auth
// @Accept json
// @Produce json
// @Param id path string true "Auth Config ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-auth-configs/{id}/test-refresh [post]
func TestTokenRefresh(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid ID format"})
	}

	config, err := db.Connection().GetAPIAuthConfigByIDWithRelations(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Auth config not found"})
	}

	if config.TokenRefreshConfig == nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "No token refresh configuration found"})
	}

	tokenManager := auth.NewTokenManager(db.Connection())
	token, err := tokenManager.ExecuteRefresh(config.TokenRefreshConfig)
	if err != nil {
		return c.JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	displayToken := token
	if len(displayToken) > 20 {
		displayToken = displayToken[:20] + "..."
	}

	return c.JSON(fiber.Map{
		"success": true,
		"token":   displayToken,
	})
}
