// Package wsfuzz is sukyan's WebSocket fuzzer engine. It owns:
//   - Per-iteration script execution (one fresh upstream socket per iteration)
//   - Mode-strategy iteration over insertion points scattered across multi-step scripts
//   - Variable extraction + substitution (run-scope and iteration-scope)
//   - Sequence-aware baseline calibration and per-iteration matching
//   - Pause/resume/cancel lifecycle + concurrency control + RPS limiting
//   - Live event stream via the fuzz package's broadcaster
//
// The package reuses pkg/playground/fuzz for mode strategies, payload resolution,
// the pause gate, the run registry, and matcher evaluation. It reuses
// pkg/playground/wsreplay for the upstream session (dial / Send / NextFrame).
//
// State lives on PlaygroundWsFuzzRun + PlaygroundWsFuzzIteration in the db
// package; this package never touches the database directly except through the
// Persister interface passed in by the API layer.
package wsfuzz
