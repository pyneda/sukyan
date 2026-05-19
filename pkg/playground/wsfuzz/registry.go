package wsfuzz

import "github.com/pyneda/sukyan/pkg/playground/fuzz"

// runRegistry is the wsfuzz-dedicated fuzz.Registry. Sharing fuzz.Default()
// with HTTP fuzz would let an HTTP run 42 and a WS run 42 share a pause gate
// (both registries are keyed by runID), so we keep a separate instance.
var runRegistry = fuzz.NewRegistry()

// Registry returns the wsfuzz package's dedicated run registry. Callers should
// use this for Register/Unregister/Pause/Resume/Cancel of wsfuzz runs only.
func Registry() *fuzz.Registry { return runRegistry }
