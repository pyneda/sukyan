package wsreplay

import "sync"

var (
	defaultManager     *Manager
	defaultManagerOnce sync.Once
)

// Default returns the process-wide singleton manager.
// Initialized lazily on first access; pass the persister via Init.
func Default() *Manager { return defaultManager }

// Init must be called once at server boot.
func Init(persister Persister) { defaultManagerOnce.Do(func() { defaultManager = NewManager(persister) }) }
