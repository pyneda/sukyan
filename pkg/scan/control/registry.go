package control

import (
	"sync"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// Registry manages in-memory ScanControl instances.
// It provides centralized control over all active scans.
type Registry struct {
	mu       sync.RWMutex
	controls map[uint]*ScanControl
	dbConn   *db.DatabaseConnection
}

// NewRegistry creates a new control registry
func NewRegistry(dbConn *db.DatabaseConnection) *Registry {
	return &Registry{
		controls: make(map[uint]*ScanControl),
		dbConn:   dbConn,
	}
}

// Register creates and registers a new ScanControl for a scan
func (r *Registry) Register(scanID uint, state State) *ScanControl {
	r.mu.Lock()
	defer r.mu.Unlock()

	// If already exists, return existing
	if ctrl, exists := r.controls[scanID]; exists {
		return ctrl
	}

	ctrl := NewWithState(scanID, state)
	r.controls[scanID] = ctrl

	log.Debug().Uint("scan_id", scanID).Str("state", state.String()).Msg("Registered scan control")
	return ctrl
}

// Get returns the ScanControl for a scan, or nil if not found
func (r *Registry) Get(scanID uint) *ScanControl {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.controls[scanID]
}

// GetOrCreate returns existing ScanControl or creates a new one.
// When creating a new control, it checks the database for the scan's actual status.
func (r *Registry) GetOrCreate(scanID uint) *ScanControl {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ctrl, exists := r.controls[scanID]; exists {
		return ctrl
	}

	// Check database for scan status to create control with correct state
	var state State = StateRunning
	if r.dbConn != nil {
		scan, err := r.dbConn.GetScanByID(scanID)
		if err == nil {
			switch scan.Status {
			case db.ScanStatusPaused:
				state = StatePaused
			case db.ScanStatusCancelled, db.ScanStatusCompleted, db.ScanStatusFailed:
				state = StateCancelled
			default:
				state = StateRunning
			}
		}
	}

	ctrl := NewWithState(scanID, state)
	r.controls[scanID] = ctrl
	log.Debug().Uint("scan_id", scanID).Str("state", state.String()).Msg("Created new scan control from DB state")
	return ctrl
}

// Unregister removes a ScanControl from the registry
func (r *Registry) Unregister(scanID uint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.controls, scanID)
	log.Debug().Uint("scan_id", scanID).Msg("Unregistered scan control")
}

// SetPaused sets a scan to paused state
func (r *Registry) SetPaused(scanID uint) error {
	ctrl := r.Get(scanID)
	if ctrl == nil {
		return nil // Scan not being tracked
	}
	ctrl.SetPaused()
	log.Info().Uint("scan_id", scanID).Msg("Scan paused")
	return nil
}

// SetRunning sets a scan to running state
func (r *Registry) SetRunning(scanID uint) error {
	ctrl := r.Get(scanID)
	if ctrl == nil {
		return nil // Scan not being tracked
	}
	ctrl.SetRunning()
	log.Info().Uint("scan_id", scanID).Msg("Scan resumed")
	return nil
}

// SetCancelled sets a scan to cancelled state
func (r *Registry) SetCancelled(scanID uint) error {
	ctrl := r.Get(scanID)
	if ctrl == nil {
		return nil // Scan not being tracked
	}
	ctrl.SetCancelled()
	log.Info().Uint("scan_id", scanID).Msg("Scan cancelled")
	return nil
}

// RefreshFromDB syncs in-memory state with database state
// This should be called periodically or after a potential desync
func (r *Registry) RefreshFromDB() error {
	// Get all active and paused scans from DB
	activeScans, err := r.dbConn.GetActiveScans()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get active scans for refresh")
		return err
	}

	pausedScans, err := r.dbConn.GetPausedScans()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get paused scans for refresh")
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Track which scans should still be active
	activeScanIDs := make(map[uint]bool)

	// Update active scans
	for _, scan := range activeScans {
		activeScanIDs[scan.ID] = true
		if ctrl, exists := r.controls[scan.ID]; exists {
			if ctrl.IsPaused() {
				ctrl.SetRunning()
			}
		} else {
			r.controls[scan.ID] = New(scan.ID)
		}
	}

	// Update paused scans
	for _, scan := range pausedScans {
		activeScanIDs[scan.ID] = true
		if ctrl, exists := r.controls[scan.ID]; exists {
			if !ctrl.IsPaused() && !ctrl.IsCancelled() {
				ctrl.SetPaused()
			}
		} else {
			r.controls[scan.ID] = NewWithState(scan.ID, StatePaused)
		}
	}

	// Remove controls for scans that are no longer active or paused
	for scanID, ctrl := range r.controls {
		if !activeScanIDs[scanID] {
			// Scan is no longer active, cancel it and remove
			ctrl.SetCancelled()
			delete(r.controls, scanID)
			log.Debug().Uint("scan_id", scanID).Msg("Removed stale scan control during refresh")
		}
	}

	return nil
}

// StartPeriodicRefresh starts a goroutine that periodically refreshes from DB
func (r *Registry) StartPeriodicRefresh(interval time.Duration, stopCh <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				if err := r.RefreshFromDB(); err != nil {
					log.Warn().Err(err).Msg("Periodic refresh failed")
				}
			}
		}
	}()
}

// Count returns the number of tracked scans
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.controls)
}

// ListScanIDs returns all tracked scan IDs
func (r *Registry) ListScanIDs() []uint {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]uint, 0, len(r.controls))
	for id := range r.controls {
		ids = append(ids, id)
	}
	return ids
}

// StateMap returns a map of scan ID to state for all tracked scans
func (r *Registry) StateMap() map[uint]State {
	r.mu.RLock()
	defer r.mu.RUnlock()

	states := make(map[uint]State, len(r.controls))
	for id, ctrl := range r.controls {
		states[id] = ctrl.State()
	}
	return states
}

// RecoverFromDB initializes the registry from database state
// This should be called on startup to recover state after a restart
func (r *Registry) RecoverFromDB() error {
	log.Info().Msg("Recovering scan controls from database")

	interrupted, err := r.dbConn.GetInterruptedScans()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get interrupted scans")
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, scan := range interrupted {
		var state State
		switch scan.Status {
		case db.ScanStatusPaused:
			state = StatePaused
		case db.ScanStatusCrawling, db.ScanStatusScanning:
			state = StateRunning
		default:
			continue
		}

		r.controls[scan.ID] = NewWithState(scan.ID, state)
		log.Info().Uint("scan_id", scan.ID).Str("state", state.String()).Msg("Recovered scan control")
	}

	log.Info().Int("count", len(r.controls)).Msg("Scan control recovery complete")
	return nil
}
