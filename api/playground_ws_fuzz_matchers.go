package api

import "github.com/gofiber/fiber/v2"

// wsFuzzMatcherField is the UI-facing metadata for one matcher field.
type wsFuzzMatcherField struct {
	Name      string   `json:"name"`
	Label     string   `json:"label"`
	Type      string   `json:"type"` // "string" | "int" | "bool"
	Operators []string `json:"operators"`
}

// wsFuzzMatcherFields enumerates the matcher field metadata for the WS fuzz
// UI. Kept here (not in pkg/playground/fuzz) because it's a UI-presentation
// concern — the matcher engine still validates server-side via the package's
// validOpsByDomain map.
var wsFuzzMatcherFields = []wsFuzzMatcherField{
	{Name: "iteration.status", Label: "Iteration status", Type: "string", Operators: []string{"eq", "neq", "in", "not_in"}},
	{Name: "iteration.duration_ms", Label: "Iteration duration (ms)", Type: "int", Operators: []string{"eq", "neq", "lt", "lte", "gt", "gte"}},
	{Name: "iteration.baseline_match", Label: "Baseline match", Type: "bool", Operators: []string{"eq", "neq"}},
	{Name: "iteration.peer_close_code", Label: "Peer close code", Type: "int", Operators: []string{"eq", "neq", "in", "not_in"}},
	{Name: "handshake.status", Label: "Handshake status", Type: "int", Operators: []string{"eq", "neq", "lt", "lte", "gt", "gte", "in", "not_in"}},
	{Name: "handshake.header", Label: "Handshake response header (parametric)", Type: "string", Operators: []string{"contains", "not_contains", "regex", "not_regex", "eq", "neq", "is_empty", "is_not_empty"}},
	{Name: "received_frame_count", Label: "Received frame count", Type: "int", Operators: []string{"eq", "neq", "lt", "lte", "gt", "gte"}},
	{Name: "total_received_bytes", Label: "Total received bytes", Type: "int", Operators: []string{"eq", "neq", "lt", "lte", "gt", "gte"}},
	{Name: "received_frame_at", Label: "Received frame at index N (parametric)", Type: "string", Operators: []string{"contains", "not_contains", "regex", "not_regex", "eq", "neq", "is_empty", "is_not_empty"}},
	{Name: "step.received_frame", Label: "Step K's first received frame (parametric)", Type: "string", Operators: []string{"contains", "not_contains", "regex", "not_regex", "eq", "neq", "is_empty", "is_not_empty"}},
	{Name: "step.duration_ms", Label: "Step K duration (ms, parametric)", Type: "int", Operators: []string{"eq", "neq", "lt", "lte", "gt", "gte"}},
	{Name: "step.matched", Label: "Step K wait_for matched (parametric)", Type: "bool", Operators: []string{"eq", "neq"}},
	{Name: "variables", Label: "Variables (any-match)", Type: "string", Operators: []string{"contains", "not_contains", "regex", "not_regex", "eq", "neq", "is_empty", "is_not_empty"}},
}

// GetWsFuzzMatcherFields returns the static matcher field metadata.
func GetWsFuzzMatcherFields(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"fields": wsFuzzMatcherFields})
}
