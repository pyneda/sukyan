package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// CreatePlaygroundCollectionInput represents the input for creating a Playground Collection.
type CreatePlaygroundCollectionInput struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
	WorkspaceID uint   `json:"workspace_id" validate:"required,min=0"`
}

// CreatePlaygroundCollection godoc
// @Summary Create a new playground collection
// @Description Create a new playground collection
// @Tags Playground
// @Accept json
// @Produce json
// @Param input body CreatePlaygroundCollectionInput true "Create Playground Collection Input"
// @Success 201 {object} db.PlaygroundCollection
// @Failure 400 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/collections [post]
func CreatePlaygroundCollection(c *fiber.Ctx) error {
	input := new(CreatePlaygroundCollectionInput)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"message": err.Error(),
		})
	}

	workspaceExists, err := db.Connection.WorkspaceExists(input.WorkspaceID)
	if !workspaceExists || err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}

	collection := &db.PlaygroundCollection{
		Name:        input.Name,
		Description: input.Description,
		WorkspaceID: input.WorkspaceID,
	}

	if err := db.Connection.CreatePlaygroundCollection(collection); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to create Playground Collection",
			"message": err.Error(),
		})
	}
	return c.Status(fiber.StatusCreated).JSON(collection)
}

// CreatePlaygroundSessionInput represents the input for creating a Playground Session.
type CreatePlaygroundSessionInput struct {
	Name              string                   `json:"name" validate:"required"`
	Type              db.PlaygroundSessionType `json:"type"`
	OriginalRequestID uint                     `json:"original_request_id" validate:"omitempty,min=0"`
	CollectionID      uint                     `json:"collection_id" validate:"required,min=0"`
}

// CreatePlaygroundSession godoc
// @Summary Create a new playground session
// @Description Create a new playground session
// @Tags Playground
// @Accept json
// @Produce json
// @Param input body CreatePlaygroundSessionInput true "Create Playground Session Input"
// @Success 201 {object} db.PlaygroundSession
// @Failure 400 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/sessions [post]
func CreatePlaygroundSession(c *fiber.Ctx) error {
	input := new(CreatePlaygroundSessionInput)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"message": err.Error(),
		})
	}

	collection, err := db.Connection.GetPlaygroundCollection(input.CollectionID)
	if err != nil {
		log.Error().Err(err).Interface("input", input).Msg("Failed to retrieve Playground Collection")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid collection",
			"message": "The provided collection ID does not seem valid",
		})
	}

	// original request id should be validated - but probably is gonna be removed.

	session := &db.PlaygroundSession{
		Name: input.Name,
		Type: input.Type,
		// OriginalRequestID: &input.OriginalRequestID,
		CollectionID: input.CollectionID,
		WorkspaceID:  collection.WorkspaceID,
	}

	if err := db.Connection.CreatePlaygroundSession(session); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to create playground session",
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(session)
}

// ListPlaygroundCollections godoc
// @Summary List playground collections
// @Description List playground collections
// @Tags Playground
// @Accept json
// @Produce json
// @Param query query string false "Search by name or description"
// @Param workspace query uint true "Filter by workspace id"
// @Param sort_by query string false "Sort by field (id, name, description, workspace_id)"
// @Param sort_order query string false "Sort order (asc, desc)"
// @Param page query int false "Page number for pagination"
// @Param page_size query int false "Page size for pagination"
// @Success 200 {array} db.PlaygroundCollection
// @Failure 400 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/collections [get]
func ListPlaygroundCollections(c *fiber.Ctx) error {
	workspaceID, err := parseWorkspaceID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}

	filters := db.PlaygroundCollectionFilters{
		Query:       c.Query("query"),
		WorkspaceID: workspaceID,
		SortBy:      c.Query("sort_by"),
		SortOrder:   c.Query("sort_order"),
		Pagination: db.Pagination{
			Page:     c.QueryInt("page", 1),
			PageSize: c.QueryInt("page_size", 10),
		},
	}

	if err := validate.Struct(filters); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"message": err.Error(),
		})
	}

	collections, count, err := db.Connection.ListPlaygroundCollections(filters)
	if err != nil {
		log.Error().Err(err).Interface("filters", filters).Msg("Failed to retrieve Playground Collections")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to retrieve Playground Collections",
			"message": "There has been an error retrieving Playground Collections",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": collections, "count": count})
}

// ListPlaygroundSessions godoc
// @Summary List Playground Sessions
// @Description List Playground Sessions
// @Tags Playground
// @Accept json
// @Produce json
// @Param type query string false "Filter by session type (manual, fuzz)"
// @Param original_request_id query uint false "Filter by original request ID"
// @Param task query uint false "Filter by task ID"
// @Param collection query uint false "Filter by collection ID"
// @Param workspace query uint true "Filter by workspace ID"
// @Param query query string false "Search by name"
// @Param page query int false "Page number for pagination"
// @Param page_size query int false "Page size for pagination"
// @Param sort_by query string false "Sort by field (id, name, type, original_request_id, task, collection, workspace)"
// @Param sort_order query string false "Sort order (asc, desc)"
// @Success 200 {array} db.PlaygroundSession
// @Failure 400 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/playground/sessions [get]
func ListPlaygroundSessions(c *fiber.Ctx) error {
	workspaceID, err := parseWorkspaceID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}

	taskID, err := parseTaskID(c)
	if err != nil {
		if c.Query("task") != "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Invalid task",
				"message": "The provided task ID does not seem valid",
			})
		} else {
			taskID = uint(0)
		}
	}

	collectionID, err := parsePlaygroundCollectionID(c)
	if err != nil {
		if c.Query("collection") != "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Invalid collection",
				"message": "The provided collection ID does not seem valid",
			})
		} else {
			collectionID = uint(0)
		}
	}

	input := db.PlaygroundSessionFilters{
		Type:              db.PlaygroundSessionType(c.Query("type")),
		OriginalRequestID: uint(c.QueryInt("original_request_id", 0)),
		TaskID:            taskID,
		CollectionID:      collectionID,
		WorkspaceID:       workspaceID,
		Query:             c.Query("query"),
		Pagination: db.Pagination{
			Page:     c.QueryInt("page", 1),
			PageSize: c.QueryInt("page_size", 30),
		},
		SortBy:    c.Query("sort_by", "id"),
		SortOrder: c.Query("sort_order", "desc"),
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"message": err.Error(),
		})
	}

	sessions, count, err := db.Connection.ListPlaygroundSessions(input)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to retrieve Playground Sessions",
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data":  sessions,
		"count": count,
	})
}
