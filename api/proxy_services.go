package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/proxy"
	"github.com/rs/zerolog/log"
)

// ProxyServiceCreateInput defines the acceptable input for creating a proxy service
type ProxyServiceCreateInput struct {
	Name                  string `json:"name" validate:"required"`
	Host                  string `json:"host" validate:"required"`
	Port                  int    `json:"port" validate:"required,min=1,max=65535"`
	Verbose               bool   `json:"verbose"`
	LogOutOfScopeRequests bool   `json:"log_out_of_scope_requests"`
}

// ProxyServiceUpdateInput defines the acceptable input for updating a proxy service
type ProxyServiceUpdateInput struct {
	Name                  *string `json:"name,omitempty"`
	Host                  *string `json:"host,omitempty"`
	Port                  *int    `json:"port,omitempty" validate:"omitempty,min=1,max=65535"`
	Verbose               *bool   `json:"verbose,omitempty"`
	LogOutOfScopeRequests *bool   `json:"log_out_of_scope_requests,omitempty"`
}

// ProxyServiceResponse extends ProxyService with runtime status
type ProxyServiceResponse struct {
	db.ProxyService
	Status *proxy.ProxyStatus `json:"status,omitempty"`
}

// CreateProxyService godoc
// @Summary Create a new proxy service
// @Description Creates a new proxy service for a workspace
// @Tags Proxy Services
// @Accept  json
// @Produce  json
// @Param workspaceId path string true "Workspace ID"
// @Param proxy_service body ProxyServiceCreateInput true "Proxy service to create"
// @Success 201 {object} db.ProxyService
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/workspaces/{workspaceId}/proxy-services [post]
func CreateProxyService(c *fiber.Ctx) error {
	workspaceID, err := parseUint(c.Params("workspaceId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid workspace ID"})
	}

	// Validate workspace exists
	exists, err := db.Connection().WorkspaceExists(workspaceID)
	if err != nil || !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Workspace not found"})
	}

	input := new(ProxyServiceCreateInput)
	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON"})
	}

	// Check if port is already in use
	existingProxy, _ := db.Connection().GetProxyServiceByPort(input.Port)
	if existingProxy != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Port is already in use by another proxy service"})
	}

	proxyService := &db.ProxyService{
		WorkspaceID:           &workspaceID,
		Name:                  input.Name,
		Host:                  input.Host,
		Port:                  input.Port,
		Verbose:               input.Verbose,
		LogOutOfScopeRequests: input.LogOutOfScopeRequests,
		Enabled:               false, // Created as disabled by default
	}

	createdProxy, err := db.Connection().CreateProxyService(proxyService)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create proxy service")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": createdProxy})
}

// ListProxyServices godoc
// @Summary List proxy services for a workspace
// @Description Retrieves all proxy services for a workspace with runtime status
// @Tags Proxy Services
// @Accept  json
// @Produce  json
// @Param workspaceId path string true "Workspace ID"
// @Success 200 {array} ProxyServiceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/workspaces/{workspaceId}/proxy-services [get]
func ListProxyServices(c *fiber.Ctx) error {
	workspaceID, err := parseUint(c.Params("workspaceId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid workspace ID"})
	}

	// Validate workspace exists
	exists, err := db.Connection().WorkspaceExists(workspaceID)
	if err != nil || !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Workspace not found"})
	}

	proxies, err := db.Connection().ListProxyServicesByWorkspace(workspaceID)
	if err != nil {
		log.Error().Err(err).Uint("workspace_id", workspaceID).Msg("Failed to list proxy services")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
	}

	// Enrich with runtime status
	proxyManager := c.Locals("proxyManager").(*proxy.ProxyManager)
	response := make([]ProxyServiceResponse, len(proxies))
	for i, p := range proxies {
		response[i] = ProxyServiceResponse{
			ProxyService: *p,
		}
		// Try to get runtime status (won't error if not running)
		if status, err := proxyManager.GetStatus(p.ID); err == nil {
			response[i].Status = status
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": response, "count": len(response)})
}

// GetProxyService godoc
// @Summary Get a proxy service by ID
// @Description Retrieves a proxy service with runtime status
// @Tags Proxy Services
// @Accept  json
// @Produce  json
// @Param id path string true "Proxy Service ID (UUID)"
// @Success 200 {object} ProxyServiceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/proxy-services/{id} [get]
func GetProxyService(c *fiber.Ctx) error {
	id, err := parseUUID(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid proxy service ID"})
	}

	proxyService, err := db.Connection().GetProxyServiceByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Proxy service not found"})
	}

	// Enrich with runtime status
	proxyManager := c.Locals("proxyManager").(*proxy.ProxyManager)
	response := ProxyServiceResponse{
		ProxyService: *proxyService,
	}
	if status, err := proxyManager.GetStatus(id); err == nil {
		response.Status = status
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": response})
}

// UpdateProxyService godoc
// @Summary Update a proxy service
// @Description Updates a proxy service and restarts it if running
// @Tags Proxy Services
// @Accept  json
// @Produce  json
// @Param id path string true "Proxy Service ID (UUID)"
// @Param proxy_service body ProxyServiceUpdateInput true "Proxy service updates"
// @Success 200 {object} db.ProxyService
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/proxy-services/{id} [patch]
func UpdateProxyService(c *fiber.Ctx) error {
	id, err := parseUUID(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid proxy service ID"})
	}

	// Check proxy exists
	existingProxy, err := db.Connection().GetProxyServiceByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Proxy service not found"})
	}

	input := new(ProxyServiceUpdateInput)
	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON"})
	}

	// Check if port is changing and if new port is available
	if input.Port != nil && *input.Port != existingProxy.Port {
		conflictingProxy, _ := db.Connection().GetProxyServiceByPort(*input.Port)
		if conflictingProxy != nil && conflictingProxy.ID != id {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Port is already in use by another proxy service"})
		}
	}

	// Build updates
	updates := &db.ProxyService{}
	if input.Name != nil {
		updates.Name = *input.Name
	}
	if input.Host != nil {
		updates.Host = *input.Host
	}
	if input.Port != nil {
		updates.Port = *input.Port
	}
	if input.Verbose != nil {
		updates.Verbose = *input.Verbose
	}
	if input.LogOutOfScopeRequests != nil {
		updates.LogOutOfScopeRequests = *input.LogOutOfScopeRequests
	}

	// Update in database
	if err := db.Connection().UpdateProxyService(id, updates); err != nil {
		log.Error().Err(err).Str("proxy_id", id.String()).Msg("Failed to update proxy service")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
	}

	// If proxy is running, restart it with new config
	proxyManager := c.Locals("proxyManager").(*proxy.ProxyManager)
	status, _ := proxyManager.GetStatus(id)
	if status != nil && status.Running {
		log.Info().Str("proxy_id", id.String()).Msg("Restarting proxy with updated configuration")
		if err := proxyManager.RestartProxy(c.Context(), id); err != nil {
			log.Error().Err(err).Str("proxy_id", id.String()).Msg("Failed to restart proxy after update")
			// Don't return error - update succeeded, restart failed
		}
	}

	// Fetch updated proxy
	updatedProxy, err := db.Connection().GetProxyServiceByID(id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": updatedProxy})
}

// DeleteProxyService godoc
// @Summary Delete a proxy service
// @Description Deletes a proxy service (stops it first if running)
// @Tags Proxy Services
// @Accept  json
// @Produce  json
// @Param id path string true "Proxy Service ID (UUID)"
// @Success 200 {object} map[string]interface{} "message": "Proxy service successfully deleted"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/proxy-services/{id} [delete]
func DeleteProxyService(c *fiber.Ctx) error {
	id, err := parseUUID(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid proxy service ID"})
	}

	// Check proxy exists
	_, err = db.Connection().GetProxyServiceByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Proxy service not found"})
	}

	// Stop proxy if running
	proxyManager := c.Locals("proxyManager").(*proxy.ProxyManager)
	status, _ := proxyManager.GetStatus(id)
	if status != nil && status.Running {
		log.Info().Str("proxy_id", id.String()).Msg("Stopping proxy before deletion")
		if err := proxyManager.StopProxy(id); err != nil {
			log.Error().Err(err).Str("proxy_id", id.String()).Msg("Failed to stop proxy before deletion")
			// Continue with deletion anyway
		}
	}

	// Delete from database
	if err := db.Connection().DeleteProxyService(id); err != nil {
		log.Error().Err(err).Str("proxy_id", id.String()).Msg("Failed to delete proxy service")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Proxy service successfully deleted"})
}

// StartProxyService godoc
// @Summary Start a proxy service
// @Description Starts a proxy service instance
// @Tags Proxy Services
// @Accept  json
// @Produce  json
// @Param id path string true "Proxy Service ID (UUID)"
// @Success 200 {object} ProxyServiceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/proxy-services/{id}/start [post]
func StartProxyService(c *fiber.Ctx) error {
	id, err := parseUUID(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid proxy service ID"})
	}

	proxyManager := c.Locals("proxyManager").(*proxy.ProxyManager)

	// Start the proxy
	if err := proxyManager.StartProxy(c.Context(), id); err != nil {
		log.Error().Err(err).Str("proxy_id", id.String()).Msg("Failed to start proxy service")

		// Check if it's a port conflict error
		if err.Error() == "proxy "+id.String()+" is already running" {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Proxy is already running"})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Enable in database
	if err := db.Connection().SetProxyServiceEnabled(id, true); err != nil {
		log.Error().Err(err).Str("proxy_id", id.String()).Msg("Failed to set proxy enabled status")
		// Proxy is already running, so don't return error
	}

	// Get updated status
	proxyService, _ := db.Connection().GetProxyServiceByID(id)
	response := ProxyServiceResponse{
		ProxyService: *proxyService,
	}
	if status, err := proxyManager.GetStatus(id); err == nil {
		response.Status = status
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": response})
}

// StopProxyService godoc
// @Summary Stop a proxy service
// @Description Stops a running proxy service instance
// @Tags Proxy Services
// @Accept  json
// @Produce  json
// @Param id path string true "Proxy Service ID (UUID)"
// @Success 200 {object} ProxyServiceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/proxy-services/{id}/stop [post]
func StopProxyService(c *fiber.Ctx) error {
	id, err := parseUUID(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid proxy service ID"})
	}

	proxyManager := c.Locals("proxyManager").(*proxy.ProxyManager)

	// Stop the proxy
	if err := proxyManager.StopProxy(id); err != nil {
		log.Error().Err(err).Str("proxy_id", id.String()).Msg("Failed to stop proxy service")

		if err.Error() == "proxy "+id.String()+" is not running" {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Proxy is not running"})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Disable in database
	if err := db.Connection().SetProxyServiceEnabled(id, false); err != nil {
		log.Error().Err(err).Str("proxy_id", id.String()).Msg("Failed to set proxy disabled status")
		// Proxy is already stopped, so don't return error
	}

	// Get updated status
	proxyService, _ := db.Connection().GetProxyServiceByID(id)
	response := ProxyServiceResponse{
		ProxyService: *proxyService,
	}
	if status, err := proxyManager.GetStatus(id); err == nil {
		response.Status = status
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": response})
}

// RestartProxyService godoc
// @Summary Restart a proxy service
// @Description Restarts a proxy service instance
// @Tags Proxy Services
// @Accept  json
// @Produce  json
// @Param id path string true "Proxy Service ID (UUID)"
// @Success 200 {object} ProxyServiceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/proxy-services/{id}/restart [post]
func RestartProxyService(c *fiber.Ctx) error {
	id, err := parseUUID(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid proxy service ID"})
	}

	proxyManager := c.Locals("proxyManager").(*proxy.ProxyManager)

	// Restart the proxy
	if err := proxyManager.RestartProxy(c.Context(), id); err != nil {
		log.Error().Err(err).Str("proxy_id", id.String()).Msg("Failed to restart proxy service")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Ensure enabled in database
	if err := db.Connection().SetProxyServiceEnabled(id, true); err != nil {
		log.Error().Err(err).Str("proxy_id", id.String()).Msg("Failed to set proxy enabled status")
		// Proxy is already running, so don't return error
	}

	// Get updated status
	proxyService, _ := db.Connection().GetProxyServiceByID(id)
	response := ProxyServiceResponse{
		ProxyService: *proxyService,
	}
	if status, err := proxyManager.GetStatus(id); err == nil {
		response.Status = status
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": response})
}
