package api

import (
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	pkgapi "github.com/pyneda/sukyan/pkg/api"
	"github.com/pyneda/sukyan/pkg/passive"
	"github.com/pyneda/sukyan/pkg/scan/manager"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
)

var validate = validator.New()

type ActiveScanInput struct {
	Items              []uint                        `json:"items" validate:"required,dive,min=0"`
	ScanID             *uint                         `json:"scan_id" validate:"omitempty,min=1"`
	WorkspaceID        uint                          `json:"workspace" validate:"omitempty,min=0"`
	TaskID             uint                          `json:"task" validate:"omitempty,min=0"`
	AuditCategories    *scan_options.AuditCategories `json:"audit_categories" validate:"omitempty"`
	Mode               string                        `json:"mode" validate:"omitempty,oneof=fast smart fuzz"`
	InsertionPoints    []string                      `json:"insertion_points" validate:"omitempty"`
	ExperimentalAudits bool                          `json:"experimental_audits"`
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

	scanManager := GetScanManager()
	if scanManager == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "Scan manager not available",
		})
	}

	var items []db.History
	var itemsWorkspaceID uint
	var err error

	if input.WorkspaceID > 0 {
		items, err = db.Connection().GetHistoriesByIDAndWorkspace(input.Items, input.WorkspaceID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Cannot get history items",
				Message: err.Error(),
			})
		}
		itemsWorkspaceID = input.WorkspaceID
	} else {
		items, err = db.Connection().GetHistoriesByID(input.Items)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Cannot get history items",
				Message: err.Error(),
			})
		}
		itemsWorkspaceID, err = manager.ValidateHistoryItemsWorkspace(items)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Workspace validation failed",
				Message: err.Error(),
			})
		}
	}

	if len(items) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "No valid history items found",
		})
	}

	auditCategories := scan_options.AuditCategories{
		Passive:    true,
		ServerSide: true,
		ClientSide: true,
	}
	if input.AuditCategories != nil {
		auditCategories = *input.AuditCategories
	}

	var scanID uint
	var scanEntity *db.Scan

	if input.ScanID != nil && *input.ScanID > 0 {
		scanEntity, err = manager.ValidateScanWorkspace(*input.ScanID, itemsWorkspaceID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Scan validation failed",
				Message: err.Error(),
			})
		}
		scanID = scanEntity.ID

	} else {
		if input.WorkspaceID > 0 && input.WorkspaceID != itemsWorkspaceID {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: "Workspace mismatch",
			})
		}

		mode := scan_options.ScanModeSmart
		if input.Mode != "" {
			mode = scan_options.NewScanMode(input.Mode)
		}

		insertionPoints := input.InsertionPoints
		if len(insertionPoints) == 0 {
			insertionPoints = scan_options.GetValidInsertionPoints()
		}

		opts := scan_options.FullScanOptions{
			Title:              "Ad-hoc Scan",
			StartURLs:          []string{items[0].URL},
			WorkspaceID:        itemsWorkspaceID,
			AuditCategories:    auditCategories,
			Mode:               mode,
			InsertionPoints:    insertionPoints,
			ExperimentalAudits: input.ExperimentalAudits,
			PagesPoolSize:      1,
			MaxRetries:         3,
		}

		scanEntity, err = manager.CreateAdHocScanWithOptions(scanManager, opts)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "Failed to create scan",
				Message: err.Error(),
			})
		}

		scanID = scanEntity.ID
	}

	// Fingerprint items for better scanning
	historyPtrs := make([]*db.History, len(items))
	for i := range items {
		historyPtrs[i] = &items[i]
	}
	fingerprints := passive.FingerprintHistoryItems(historyPtrs)
	fingerprintTags := passive.GetUniqueNucleiTags(fingerprints)

	// Build options - use scan config or input config
	opts := scan_options.HistoryItemScanOptions{
		WorkspaceID:     itemsWorkspaceID,
		TaskID:          input.TaskID,
		ScanID:          scanID,
		Fingerprints:    fingerprints,
		FingerprintTags: fingerprintTags,
	}

	opts.Mode = scanEntity.Options.Mode
	opts.InsertionPoints = scanEntity.Options.InsertionPoints
	opts.ExperimentalAudits = scanEntity.Options.ExperimentalAudits
	opts.AuditCategories = auditCategories
	opts.MaxRetries = scanEntity.Options.MaxRetries

	err = scanManager.ScheduleHistoryItemScan(scanID, itemsWorkspaceID, historyPtrs, opts)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to schedule jobs",
			Message: err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message":   "Active scan scheduled",
		"scan_id":   scanID,
		"job_count": len(items),
	})
}

// FullScanHandler godoc
// @Summary Submit URLs for full scanning
// @Description Receives a list of URLs and other parameters and schedules them for a full scan. Supports API-only scans with definition_ids or inline_imports.
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

	hasURLs := len(input.StartURLs) > 0
	hasAPIDefinitions := len(input.APIScanOptions.DefinitionIDs) > 0
	hasInlineImports := len(input.APIScanOptions.InlineImports) > 0

	if !hasURLs && !hasAPIDefinitions && !hasInlineImports {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "No scan targets provided",
			Message: "Either start_urls, api_scan_options.definition_ids, or api_scan_options.inline_imports must be provided",
		})
	}

	if hasAPIDefinitions {
		for _, defID := range input.APIScanOptions.DefinitionIDs {
			def, err := db.Connection().GetAPIDefinitionByID(defID)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
					Error:   "Invalid API definition",
					Message: "API definition " + defID.String() + " not found",
				})
			}
			if def.WorkspaceID != input.WorkspaceID {
				return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
					Error:   "API definition workspace mismatch",
					Message: "API definition " + defID.String() + " does not belong to the specified workspace",
				})
			}
		}
	}

	if hasInlineImports {
		for _, imp := range input.APIScanOptions.InlineImports {
			defID, err := processInlineAPIImport(imp, input.WorkspaceID)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
					Error:   "Failed to import API definition",
					Message: err.Error(),
				})
			}
			input.APIScanOptions.DefinitionIDs = append(input.APIScanOptions.DefinitionIDs, defID)
		}
		input.APIScanOptions.InlineImports = nil
	}

	if len(input.APIScanOptions.DefinitionIDs) > 0 {
		input.APIScanOptions.Enabled = true
	}

	if !input.AuditCategories.ServerSide && !input.AuditCategories.ClientSide && !input.AuditCategories.Passive && !input.AuditCategories.Discovery && !input.AuditCategories.WebSocket {
		log.Warn().Interface("input", input).Msg("Full scan request received without audit categories enabled, enabling all")
		input.AuditCategories.ServerSide = true
		input.AuditCategories.ClientSide = true
		input.AuditCategories.Passive = true
		input.AuditCategories.Discovery = true
		input.AuditCategories.WebSocket = true
		input.MaxRetries = 3
	}

	if input.Title == "" {
		if hasURLs {
			input.Title = "Full scan"
		} else {
			input.Title = "API scan"
		}
	}

	scanManager := GetScanManager()
	if scanManager == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Scan manager not available",
			Message: "The scan manager is not initialized",
		})
	}

	scan, err := scanManager.StartFullScan(*input)
	if err != nil {
		log.Error().Err(err).Interface("input", input).Msg("Failed to start full scan")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to start scan",
			Message: err.Error(),
		})
	}

	if len(input.APIScanOptions.DefinitionIDs) > 0 {
		if err := db.Connection().LinkAPIDefinitionsToScan(scan.ID, input.APIScanOptions.DefinitionIDs); err != nil {
			log.Error().Err(err).Uint("scan_id", scan.ID).Msg("Failed to link API definitions to scan")
		}
	}

	return c.JSON(fiber.Map{
		"message":            "Full scan scheduled",
		"scan_id":            scan.ID,
		"api_definitions":    len(input.APIScanOptions.DefinitionIDs),
	})
}

type ActiveWebSocketScanInput struct {
	Connections       []uint                        `json:"connections" validate:"required,dive,min=0"`
	ScanID            *uint                         `json:"scan_id" validate:"omitempty,min=1"`
	WorkspaceID       uint                          `json:"workspace_id" validate:"omitempty,min=0"`
	TaskID            uint                          `json:"task_id" validate:"omitempty,min=0"`
	AuditCategories   *scan_options.AuditCategories `json:"audit_categories" validate:"omitempty"`
	ReplayMessages    bool                          `json:"replay_messages"`
	ObservationWindow int                           `json:"observation_window" validate:"omitempty,min=0,max=120"`
	Concurrency       int                           `json:"concurrency" validate:"omitempty,min=1,max=100"`
	Mode              string                        `json:"mode" validate:"omitempty,oneof=fast smart fuzz"`
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

	scanManager := GetScanManager()
	if scanManager == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "Scan manager not available",
		})
	}

	var connections []db.WebSocketConnection
	var connWorkspaceID uint
	var err error

	if input.WorkspaceID > 0 {
		connections, err = db.Connection().GetWebSocketConnectionsByIDAndWorkspace(input.Connections, input.WorkspaceID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Cannot get WebSocket connections",
				Message: err.Error(),
			})
		}
		connWorkspaceID = input.WorkspaceID
	} else {
		connections, err = db.Connection().GetWebSocketConnectionsByID(input.Connections)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Cannot get WebSocket connections",
				Message: err.Error(),
			})
		}
		connWorkspaceID, err = manager.ValidateWebSocketConnectionsWorkspace(connections)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Workspace validation failed",
				Message: err.Error(),
			})
		}
	}

	if len(connections) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "No valid WebSocket connections found",
		})
	}

	auditCategories := scan_options.AuditCategories{
		Passive:    true,
		ServerSide: true,
		ClientSide: true,
		WebSocket:  true,
	}
	if input.AuditCategories != nil {
		auditCategories = *input.AuditCategories
		auditCategories.WebSocket = true
	}

	var scanID uint

	if input.ScanID != nil && *input.ScanID > 0 {
		_, err := manager.ValidateScanWorkspace(*input.ScanID, connWorkspaceID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Scan validation failed",
				Message: err.Error(),
			})
		}
		scanID = *input.ScanID

	} else {
		if input.WorkspaceID > 0 && input.WorkspaceID != connWorkspaceID {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: "Workspace mismatch",
			})
		}

		dummyURL := connections[0].URL

		scan, err := manager.CreateAdHocScan(
			scanManager,
			connWorkspaceID,
			"Ad-hoc WebSocket Scan",
			auditCategories,
			dummyURL,
		)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "Failed to create scan",
				Message: err.Error(),
			})
		}

		scanID = scan.ID
	}

	// Schedule WebSocket scan jobs
	err = scanManager.ScheduleWebSocketScan(c.Context(), scanID, input.Connections)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to schedule WebSocket scan",
			Message: err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message":   "Active WebSocket scan scheduled",
		"scan_id":   scanID,
		"job_count": len(connections),
	})
}

func processInlineAPIImport(imp scan_options.InlineAPIImport, workspaceID uint) (uuid.UUID, error) {
	fetched, err := pkgapi.FetchAPIContent(imp.URL, imp.Content, imp.Type)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to fetch API content: %w", err)
	}

	if fetched.Type == "" {
		return uuid.Nil, fmt.Errorf("unable to detect API type, please specify type parameter")
	}

	opts := pkgapi.ImportOptions{
		WorkspaceID:  workspaceID,
		Name:         imp.Name,
		SourceURL:    fetched.SourceURL,
		BaseURL:      imp.BaseURL,
		Type:         string(fetched.Type),
		AuthConfigID: imp.AuthConfigID,
	}

	definition, err := pkgapi.ImportAPIDefinition(fetched.Content, fetched.SourceURL, opts)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to create API definition: %w", err)
	}

	return definition.ID, nil
}
