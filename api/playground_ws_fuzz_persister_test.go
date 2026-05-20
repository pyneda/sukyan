package api

import (
	"testing"

	"github.com/pyneda/sukyan/pkg/playground/wsfuzz"
)

// TestDBRunPersister_ImplementsInterface is a compile-time check that
// dbRunPersister satisfies wsfuzz.RunPersister.
func TestDBRunPersister_ImplementsInterface(t *testing.T) {
	var _ wsfuzz.RunPersister = (*dbRunPersister)(nil)
}
