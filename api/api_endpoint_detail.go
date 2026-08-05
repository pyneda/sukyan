package api

import (
	"encoding/json"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	pkgapi "github.com/pyneda/sukyan/pkg/api"
	"github.com/pyneda/sukyan/pkg/api/core"
	apigraphql "github.com/pyneda/sukyan/pkg/api/graphql"
	apiopenapi "github.com/pyneda/sukyan/pkg/api/openapi"
	"github.com/pyneda/sukyan/pkg/api/operationlink"
	apisoap "github.com/pyneda/sukyan/pkg/api/soap"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type endpointDetailResponse struct {
	Operation      core.Operation         `json:"operation"`
	ExampleRequest *pkgapi.ExampleRequest `json:"example_request,omitempty"`
	Backfilled     bool                   `json:"backfilled"`
	// ExampleError explains why example_request is absent. A definition can parse
	// while one operation still cannot produce a request, and a silently missing
	// panel reads as a bug.
	ExampleError string `json:"example_error,omitempty"`
}

// GetAPIEndpointDetail godoc
// @Summary Get full detail for one API operation
// @Description Returns the normalized operation plus a freshly built example request
// @Tags api-definitions
// @Produce json
// @Param id path string true "API Definition ID"
// @Param endpoint_id path string true "API Endpoint ID"
// @Param reveal query bool false "Return credentials unmasked"
// @Success 200 {object} endpointDetailResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/api-definitions/{id}/endpoints/{endpoint_id}/detail [get]
func GetAPIEndpointDetail(c fiber.Ctx) error {
	definitionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid definition ID format"))
	}
	endpointID, err := uuid.Parse(c.Params("endpoint_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid endpoint ID format"))
	}

	definition, err := db.Connection().GetAPIDefinitionByID(definitionID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(NewErrorResponse("API definition not found"))
	}

	endpoint, err := db.Connection().GetAPIEndpointByID(endpointID)
	if err != nil || endpoint.DefinitionID != definitionID {
		return c.Status(fiber.StatusNotFound).JSON(NewErrorResponse("API endpoint not found"))
	}

	backfilled := false
	if len(endpoint.OperationJSON) == 0 {
		if len(definition.RawDefinition) == 0 {
			return c.Status(fiber.StatusNotFound).JSON(NewErrorResponse(
				"This definition's source was not retained, so operation details cannot be reconstructed. Re-import it to see request details."))
		}
		if backfillErr := backfillDefinitionOperations(definition); backfillErr != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(NewErrorResponse(
				"The stored specification could not be parsed", backfillErr.Error()))
		}
		endpoint, err = db.Connection().GetAPIEndpointByID(endpointID)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(NewErrorResponse("API endpoint not found"))
		}
		if len(endpoint.OperationJSON) == 0 {
			return c.Status(fiber.StatusNotFound).JSON(NewErrorResponse(
				"This operation is no longer present in the stored specification. Re-import the definition."))
		}
		backfilled = true
	}

	var operation core.Operation
	if err := json.Unmarshal(endpoint.OperationJSON, &operation); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(NewErrorResponse("Stored operation is unreadable", err.Error()))
	}

	// The persisted operation carries whatever base URL its parser derived at import.
	// The definition's own base URL is editable and is what every scan uses, so it is
	// re-applied here rather than serving a stale origin.
	if definition.BaseURL != "" {
		operation.BaseURL = definition.BaseURL
	} else if definition.SourceURL != "" {
		operation.BaseURL = definition.SourceURL
	}

	var authConfig *db.APIAuthConfig
	if definition.AuthConfigID != nil {
		if resolved, authErr := db.Connection().GetAPIAuthConfigByIDWithRelations(*definition.AuthConfigID); authErr == nil {
			authConfig = resolved
		} else {
			log.Warn().Err(authErr).Str("definition_id", definitionID.String()).
				Msg("Could not load the definition's auth config for the example request")
		}
	}

	response := endpointDetailResponse{Operation: operation, Backfilled: backfilled}

	example, exampleErr := pkgapi.BuildExampleRequest(
		c.Context(), definition.Type, &operation, authConfig, fiber.Query[bool](c, "reveal", false),
	)
	if exampleErr != nil {
		response.ExampleError = exampleErr.Error()
	} else {
		response.ExampleRequest = example
	}

	return c.JSON(response)
}

// backfillDefinitionOperations parses the stored specification once and writes the
// normalized operation onto every endpoint of the definition.
//
// It is whole-definition rather than per-endpoint because the parse cost is per
// document: reconstructing one operation of a large specification costs the same as
// reconstructing all of them, so doing it per endpoint would repeat that cost for
// every row the user opens.
func backfillDefinitionOperations(definition *db.APIDefinition) error {
	endpoints, err := db.Connection().GetAPIEndpointsByDefinitionID(definition.ID)
	if err != nil {
		return err
	}

	var operations []core.Operation
	switch definition.Type {
	case db.APIDefinitionTypeGraphQL:
		operations, _, err = apigraphql.ParseFromRawDefinition(definition.RawDefinition, definition.BaseURL)
	case db.APIDefinitionTypeWSDL:
		operations, err = apisoap.ParseFromRawDefinition(definition.RawDefinition, definition.SourceURL)
	default:
		operations, err = apiopenapi.ParseFromRawDefinition(definition.RawDefinition)
	}
	if err != nil {
		return err
	}

	if operationlink.AttachOperationJSON(endpoints, operations, definition.Type) == 0 {
		return nil
	}

	return db.Connection().DB().Transaction(func(tx *gorm.DB) error {
		for _, endpoint := range endpoints {
			if len(endpoint.OperationJSON) == 0 {
				continue
			}
			if err := tx.Model(&db.APIEndpoint{}).Where("id = ?", endpoint.ID).
				Update("operation_json", endpoint.OperationJSON).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
