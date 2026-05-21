package wsfuzz

import (
	"fmt"

	"github.com/pyneda/sukyan/pkg/playground/fuzz"
)

// MaxIterationCount is the upper bound on a single run's planned iterations.
// Above this, the validator blocks the run. The 1M figure is a safety net:
// realistic fuzz campaigns rarely exceed 100k; anything bigger likely
// indicates a misconfigured mode (e.g., accidental Cartesian explosion).
const MaxIterationCount = 1_000_000

// Validate returns user-facing warnings and blocking errors. errors empty means
// the run is launchable; warnings are advisory.
func Validate(cfg WsFuzzerConfig) (warnings []string, errors []string) {
	if cfg.TargetURL == "" {
		errors = append(errors, "target_url is required")
	}

	refs := BuildPositionRefs(cfg.Script)
	if len(refs) == 0 {
		errors = append(errors, "script has no insertion points (no fuzz step with positions)")
	}

	hasObservability := false
	for i, s := range cfg.Script {
		if s.Role == RoleFuzz && len(s.Positions) == 0 {
			errors = append(errors, fmt.Sprintf("step %d has role=fuzz but no positions (remove the step or add insertion points)", i))
		}
		if s.WaitFor != nil || s.Role == RoleCheck {
			hasObservability = true
		}
		for _, ext := range s.Extract {
			if s.Opcode == 2 && ext.Method == MethodJSONPath {
				errors = append(errors, fmt.Sprintf("step %d: json_path extraction is invalid on a binary frame", i))
			}
		}
	}
	if len(cfg.Script) > 0 && !hasObservability {
		warnings = append(warnings, "script has no wait_for and no check step; matchers cannot fire")
	}

	if cfg.PreIterationSetup != nil && cfg.PreIterationSetup.Kind != SetupNone && len(cfg.PreIterationSetup.Extract) == 0 {
		warnings = append(warnings, "pre-iteration setup has no extractions; consider removing it or capturing a variable")
	}

	if cfg.PreIterationSetup != nil && cfg.PreIterationSetup.Kind == SetupWsScript && len(cfg.PreIterationSetup.Steps) == 0 {
		errors = append(errors, "pre-iteration ws_script setup has no steps")
	}

	if cfg.ExecutionOptions.RequestTimeoutSeconds > 0 {
		budgetMs := cfg.ExecutionOptions.RequestTimeoutSeconds * 1000
		if cfg.ConnectionTimeout >= budgetMs {
			warnings = append(warnings, "connection timeout consumes the entire iteration budget; no time left for steps")
		}
		sum := 0
		for _, s := range cfg.Script {
			sum += s.DelayMs
			if s.WaitFor != nil {
				sum += s.WaitFor.TimeoutMs
			}
		}
		if sum > budgetMs {
			warnings = append(warnings, "sum of step delays + wait_for timeouts exceeds request_timeout budget")
		}
	}

	if cfg.Mode == fuzz.ModePaired || cfg.Mode == fuzz.ModeCombinations {
		for i, s := range cfg.Script {
			if s.Role != RoleFuzz {
				continue
			}
			for j, p := range s.Positions {
				if sumPayloadCount(p.PayloadGroups) == 0 {
					errors = append(errors, fmt.Sprintf("step %d position %d: payload list is empty", i, j))
				}
			}
		}
	} else {
		if cfg.SharedPayloads == nil || (len(cfg.SharedPayloads.Payloads) == 0 && cfg.SharedPayloads.Wordlist == "") {
			errors = append(errors, "shared payload list is empty (required for single/all modes)")
		}
	}

	iters := computePlannedCount(cfg.Mode, FlatPositions(refs), cfg.SharedPayloads)
	if iters > MaxIterationCount {
		errors = append(errors, fmt.Sprintf("planned iteration count exceeds limit of 1,000,000"))
	}

	return warnings, errors
}

func sumPayloadCount(groups []fuzz.FuzzerPayloadsGroup) int {
	total := 0
	for _, g := range groups {
		total += len(g.Payloads)
		if g.Wordlist != "" {
			total++ // treat wordlist as a non-empty signal
		}
	}
	return total
}
