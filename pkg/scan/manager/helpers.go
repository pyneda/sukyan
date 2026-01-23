package manager

import (
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/scan/control"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
)

// CreateAdHocScan creates a minimal scan record for ad-hoc operations
// and sets it to "scanning" status so workers can claim jobs immediately
func CreateAdHocScan(
	scanManager *ScanManager,
	workspaceID uint,
	title string,
	auditCategories scan_options.AuditCategories,
	dummyStartURL string,
) (*db.Scan, error) {
	opts := scan_options.FullScanOptions{
		Title:           title,
		StartURLs:       []string{dummyStartURL},
		WorkspaceID:     workspaceID,
		AuditCategories: auditCategories,
		PagesPoolSize:   1,
		MaxRetries:      3,
	}

	return CreateAdHocScanWithOptions(scanManager, opts)
}

// CreateAdHocScanWithOptions creates an ad-hoc scan with full configuration options
// and sets it to "scanning" status so workers can claim jobs immediately
func CreateAdHocScanWithOptions(scanManager *ScanManager, opts scan_options.FullScanOptions) (*db.Scan, error) {
	scan, err := CreateScanRecord(db.Connection(), opts, false, db.ScanStatusScanning)
	if err != nil {
		return nil, err
	}

	scanManager.registry.Register(scan.ID, control.StateRunning)

	return scan, nil
}

// ValidateScanWorkspace validates that a scan exists and belongs to the expected workspace
func ValidateScanWorkspace(scanID uint, expectedWorkspaceID uint) (*db.Scan, error) {
	scan, err := db.Connection().GetScanByID(scanID)
	if err != nil {
		return nil, fmt.Errorf("scan not found: %w", err)
	}

	if scan.WorkspaceID != expectedWorkspaceID {
		return nil, fmt.Errorf("scan workspace mismatch: scan belongs to workspace %d, but items belong to workspace %d",
			scan.WorkspaceID, expectedWorkspaceID)
	}

	return scan, nil
}

// ValidateHistoryItemsWorkspace validates all history items belong to the same workspace
func ValidateHistoryItemsWorkspace(items []db.History) (uint, error) {
	if len(items) == 0 {
		return 0, fmt.Errorf("no items provided")
	}

	if items[0].WorkspaceID == nil {
		return 0, fmt.Errorf("history item missing workspace_id")
	}

	workspaceID := *items[0].WorkspaceID

	for i, item := range items {
		if item.WorkspaceID == nil {
			return 0, fmt.Errorf("history item %d missing workspace_id", i)
		}
		if *item.WorkspaceID != workspaceID {
			return 0, fmt.Errorf("history items belong to different workspaces: %d vs %d",
				workspaceID, *item.WorkspaceID)
		}
	}

	return workspaceID, nil
}

// ValidateWebSocketConnectionsWorkspace validates all connections belong to the same workspace
func ValidateWebSocketConnectionsWorkspace(connections []db.WebSocketConnection) (uint, error) {
	if len(connections) == 0 {
		return 0, fmt.Errorf("no connections provided")
	}

	if connections[0].WorkspaceID == nil {
		return 0, fmt.Errorf("connection missing workspace_id")
	}

	workspaceID := *connections[0].WorkspaceID

	for i, conn := range connections {
		if conn.WorkspaceID == nil {
			return 0, fmt.Errorf("connection %d missing workspace_id", i)
		}
		if *conn.WorkspaceID != workspaceID {
			return 0, fmt.Errorf("connections belong to different workspaces: %d vs %d",
				workspaceID, *conn.WorkspaceID)
		}
	}

	return workspaceID, nil
}
