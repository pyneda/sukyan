package executor

import (
	"context"
	"testing"

	"github.com/pyneda/sukyan/db"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
)

// TestNewGraphQLAuditOptions_PropagatesScanMode guards D7: the API-level GraphQL
// audit must receive the scan's mode. batching.go (testBatchTiming) and depth.go
// (circular-fragment cases) only run when ScanMode == "fuzz"; if the executor omits
// it, the zero value ("") disables those checks in every mode, including fuzz.
func TestNewGraphQLAuditOptions_PropagatesScanMode(t *testing.T) {
	modes := []scan_options.ScanMode{
		scan_options.ScanModeFast,
		scan_options.ScanModeSmart,
		scan_options.ScanModeFuzz,
	}
	for _, mode := range modes {
		job := &db.ScanJob{
			BaseModel:   db.BaseModel{ID: 99},
			ScanID:      42,
			WorkspaceID: 7,
		}
		opts := newGraphQLAuditOptions(context.Background(), mode, job, nil)

		if opts.ScanMode != mode {
			t.Errorf("ScanMode not propagated: want %q, got %q", mode, opts.ScanMode)
		}
		if opts.WorkspaceID != 7 || opts.ScanID != 42 || opts.ScanJobID != 99 {
			t.Errorf("job identity not propagated: ws=%d scan=%d job=%d", opts.WorkspaceID, opts.ScanID, opts.ScanJobID)
		}
	}
}
