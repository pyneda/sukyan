package api

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan"
	"github.com/pyneda/sukyan/pkg/scan/engine"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type PassiveScanInput struct {
	Items []uint `json:"items" validate:"required,dive,min=0"`
}

var validate = validator.New()

// PassiveScanHandler godoc
// @Summary Submit items for passive scanning
// @Description Receives a list of items and schedules them for passive scanning
// @Tags Scan
// @Accept  json
// @Produce  json
// @Param input body PassiveScanInput true "List of items"
// @Success 200 {object} ActionResponse
// @Failure 400 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scan/passive [post]
func PassiveScanHandler(c *fiber.Ctx) error {
	input := new(PassiveScanInput)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "Cannot parse JSON",
		})
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Validation failed",
			Message: err.Error(),
		})
	}

	items, err := db.Connection().GetHistoriesByID(input.Items)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Cannot get history items with provided IDs",
			Message: err.Error(),
		})
	}

	generators := c.Locals("generators").([]*generation.PayloadGenerator)
	interactionsManager := c.Locals("interactionsManager").(*integrations.InteractionsManager)
	e := engine.NewScanEngine(generators, viper.GetInt("scan.concurrency.passive"), viper.GetInt("scan.concurrency.active"), interactionsManager)

	for _, item := range items {
		// NOTE: By now, passive scans do not create task jobs, so we pass 0 as task ID
		options := scan_options.HistoryItemScanOptions{
			WorkspaceID: *item.WorkspaceID,
			TaskID:      0,
			AuditCategories: scan_options.AuditCategories{
				Passive: true,
			},
		}
		e.ScheduleHistoryItemScan(&item, engine.ScanJobTypePassive, options)
	}

	return c.JSON(ActionResponse{
		Message: "Passive scan scheduled",
	})
}

type ActiveScanInput struct {
	Items       []uint `json:"items" validate:"required,dive,min=0"`
	WorkspaceID uint   `json:"workspace" validate:"omitempty,min=0"`
	TaskID      uint   `json:"task" validate:"omitempty,min=0"`
}

// ActiveScanHandler godoc
// @Summary Submit items for active scanning
// @Description Receives a list of items and schedules them for active scanning. Either the workspace ID or task ID must be provided.
// @Tags Scan
// @Accept  json
// @Produce  json
// @Param input body ActiveScanInput true "Active scan items and configuration"
// @Success 200 {object} ActionResponse
// @Failure 400 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scan/active [post]
func ActiveScanHandler(c *fiber.Ctx) error {
	input := new(ActiveScanInput)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "Cannot parse JSON",
		})
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Validation failed",
			Message: err.Error(),
		})
	}

	// if !workspaceExists {
	// 	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
	// 		"error":   "Invalid workspace",
	// 		Message: "The provided workspace ID does not seem valid",
	// 	})
	// }

	taskExists, _ := db.Connection().TaskExists(input.TaskID)
	if !taskExists {
		workspaceExists, _ := db.Connection().WorkspaceExists(input.WorkspaceID)
		if !workspaceExists {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Invalid task",
				Message: "The provided task ID does not seem valid and we can't create a default one because the workspace ID is either not provided or invalid",
			})
		}

		task, err := db.Connection().GetOrCreateDefaultWorkspaceTask(input.WorkspaceID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Error creating default task",
				Message: "We couldnt create a default task for the provided workspace",
			})
		}
		input.TaskID = task.ID
	}

	items, err := db.Connection().GetHistoriesByID(input.Items)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Cannot get history items with provided IDs",
			Message: err.Error(),
		})
	}

	generators := c.Locals("generators").([]*generation.PayloadGenerator)
	interactionsManager := c.Locals("interactionsManager").(*integrations.InteractionsManager)
	e := engine.NewScanEngine(generators, viper.GetInt("scan.concurrency.passive"), viper.GetInt("scan.concurrency.active"), interactionsManager)

	for _, item := range items {
		// TODO: maybe should validate that the history item and task belongs to the same workspace
		options := scan_options.HistoryItemScanOptions{
			WorkspaceID:        *item.WorkspaceID,
			TaskID:             input.TaskID,
			InsertionPoints:    []string{"parameters", "urlpath", "body", "headers", "cookies", "json", "xml"},
			ExperimentalAudits: false,
			Mode:               scan_options.ScanModeSmart,
			AuditCategories: scan_options.AuditCategories{
				ServerSide: true,
				ClientSide: true,
				Passive:    true,
				WebSocket:  true,
			},
		}
		e.ScheduleHistoryItemScan(&item, engine.ScanJobTypeActive, options)
	}

	return c.JSON(ActionResponse{
		Message: "Active scan scheduled",
	})
}

// FullScanHandler godoc
// @Summary Submit URLs for full scanning
// @Description Receives a list of URLs and other parameters and schedules them for a full scan
// @Tags Scan
// @Accept  json
// @Produce  json
// @Param input body scan_options.FullScanOptions true "Configuration for full scan"
// @Success 200 {object} ActionResponse
// @Failure 400 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scan/full [post]
func FullScanHandler(c *fiber.Ctx) error {
	input := new(scan_options.FullScanOptions)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "Cannot parse JSON",
		})
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Validation failed",
			Message: err.Error(),
		})
	}

	workspaceExists, _ := db.Connection().WorkspaceExists(input.WorkspaceID)
	if !workspaceExists {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid workspace",
			Message: "The provided workspace ID does not seem valid",
		})
	}

	if !input.AuditCategories.ServerSide && !input.AuditCategories.ClientSide && !input.AuditCategories.Passive && !input.AuditCategories.Discovery && !input.AuditCategories.WebSocket {
		// return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
		// 	"error":   "Invalid audit categories",
		// 	Message: "At least one audit category must be enabled",
		// })
		log.Warn().Interface("input", input).Msg("Full scan request received witout audit categories enabled, enabling all")
		// NOTE: At a later stage, might be better to return the error above
		input.AuditCategories.ServerSide = true
		input.AuditCategories.ClientSide = true
		input.AuditCategories.Passive = true
		input.AuditCategories.Discovery = true
		input.AuditCategories.WebSocket = true
	}

	if input.Title == "" {
		input.Title = "Full scan"
	}

	generators := c.Locals("generators").([]*generation.PayloadGenerator)
	interactionsManager := c.Locals("interactionsManager").(*integrations.InteractionsManager)
	e := engine.NewScanEngine(generators, viper.GetInt("scan.concurrency.passive"), viper.GetInt("scan.concurrency.active"), interactionsManager)

	go e.FullScan(*input, false)

	return c.JSON(ActionResponse{
		Message: "Full scan scheduled",
	})
}

type ActiveWebSocketScanInput struct {
	Connections       []uint `json:"connections" validate:"required,dive,min=0"`
	WorkspaceID       uint   `json:"workspace_id" validate:"omitempty,min=0"`
	TaskID            uint   `json:"task_id" validate:"omitempty,min=0"`
	ReplayMessages    bool   `json:"replay_messages"`
	ObservationWindow int    `json:"observation_window" validate:"omitempty,min=0,max=120"`
	Concurrency       int    `json:"concurrency" validate:"omitempty,min=1,max=100"`
	Mode              string `json:"mode" validate:"omitempty,oneof=fast smart fuzz"`
}

// ActiveWebSocketScanHandler godoc
// @Summary Submit WebSocket connections for active scanning
// @Description Receives a list of WebSocket connection IDs and schedules them for active scanning. Either the workspace ID or task ID must be provided.
// @Tags Scan
// @Accept  json
// @Produce  json
// @Param input body ActiveWebSocketScanInput true "Active WebSocket scan connections and configuration"
// @Success 200 {object} ActionResponse
// @Failure 400 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scan/active/websocket [post]
func ActiveWebSocketScanHandler(c *fiber.Ctx) error {
	input := new(ActiveWebSocketScanInput)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "Cannot parse JSON",
		})
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Validation failed",
			Message: err.Error(),
		})
	}

	// Apply defaults
	if input.ObservationWindow == 0 {
		input.ObservationWindow = 10
	}

	if input.Concurrency == 0 {
		input.Concurrency = 5
	}

	mode := scan_options.ScanModeSmart
	if input.Mode != "" {
		mode = scan_options.NewScanMode(input.Mode)
	}

	taskExists, _ := db.Connection().TaskExists(input.TaskID)
	if !taskExists {
		workspaceExists, _ := db.Connection().WorkspaceExists(input.WorkspaceID)
		if !workspaceExists {
			log.Warn().Interface("input", input).Msg("Active WebSocket scan request received without a valid workspace ID")
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Invalid workspace",
				Message: "The provided workspace and/or task ID provided does not seem valid",
			})
		}

		task, err := db.Connection().GetOrCreateDefaultWorkspaceTask(input.WorkspaceID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Error creating default task",
				Message: "We couldnt create a default task for the provided workspace",
			})
		}
		input.TaskID = task.ID
	}

	connections := make([]db.WebSocketConnection, 0, len(input.Connections))
	for _, connID := range input.Connections {
		connection, err := db.Connection().GetWebSocketConnection(connID)
		if err != nil {
			log.Error().Err(err).Uint("connection_id", connID).Msg("Failed to get WebSocket connection")
			continue
		}
		connections = append(connections, *connection)
	}

	if len(connections) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Cannot get WebSocket connections with provided IDs",
			Message: "None of the provided connection IDs are valid",
		})
	}

	generators := c.Locals("generators").([]*generation.PayloadGenerator)
	interactionsManager := c.Locals("interactionsManager").(*integrations.InteractionsManager)
	e := engine.NewScanEngine(generators, viper.GetInt("scan.concurrency.passive"), viper.GetInt("scan.concurrency.active"), interactionsManager)

	workspaceID := input.WorkspaceID
	if workspaceID <= 0 {
		workspaceID = *connections[0].WorkspaceID
	}

	options := scan.WebSocketScanOptions{
		WorkspaceID:       workspaceID,
		TaskID:            input.TaskID,
		Mode:              mode,
		ReplayMessages:    input.ReplayMessages,
		ObservationWindow: time.Duration(input.ObservationWindow) * time.Second,
		Concurrency:       input.Concurrency,
	}
	e.EvaluateWebSocketConnections(connections, options)

	return c.JSON(ActionResponse{
		Message: "Active WebSocket connections scan scheduled",
	})
}
