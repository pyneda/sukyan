package wsfuzz

import (
	"testing"

	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
	"github.com/stretchr/testify/require"
)

func TestValidate_BlocksWithoutTargetURL(t *testing.T) {
	cfg := WsFuzzerConfig{
		Mode:   fuzz.ModeSingle,
		Script: []WsFuzzStep{{Role: RoleFuzz, Positions: []fuzz.FuzzerPosition{{Start: 0, End: 1}}}},
		SharedPayloads: &fuzz.FuzzerPayloadsGroup{Payloads: []string{"x"}},
	}
	_, errs := Validate(cfg)
	require.Contains(t, errs, "target_url is required")
}

func TestValidate_BlocksWithoutAnyPositions(t *testing.T) {
	cfg := WsFuzzerConfig{
		TargetURL: "ws://example.com/ws",
		Mode:      fuzz.ModeSingle,
		Script:    []WsFuzzStep{{Role: RoleSetup, Content: "{}"}},
	}
	_, errs := Validate(cfg)
	require.Contains(t, errs, "script has no insertion points (no fuzz step with positions)")
}

func TestValidate_BlocksFuzzStepWithZeroPositions(t *testing.T) {
	cfg := WsFuzzerConfig{
		TargetURL: "ws://example.com/ws",
		Mode:      fuzz.ModeSingle,
		Script: []WsFuzzStep{
			{Role: RoleFuzz, Content: "{}", Positions: nil},
		},
		SharedPayloads: &fuzz.FuzzerPayloadsGroup{Payloads: []string{"x"}},
	}
	_, errs := Validate(cfg)
	require.NotEmpty(t, errs)
}

func TestValidate_WarnsNoObservability(t *testing.T) {
	cfg := WsFuzzerConfig{
		TargetURL: "ws://example.com/ws",
		Mode:      fuzz.ModeSingle,
		Script: []WsFuzzStep{
			{Role: RoleFuzz, Content: "x", Positions: []fuzz.FuzzerPosition{{Start: 0, End: 1}}, WaitFor: nil},
		},
		SharedPayloads: &fuzz.FuzzerPayloadsGroup{Payloads: []string{"x"}},
	}
	warns, _ := Validate(cfg)
	require.Contains(t, warns, "script has no wait_for and no check step; matchers cannot fire")
}

func TestValidate_WarnsStepTimeoutsExceedBudget(t *testing.T) {
	cfg := WsFuzzerConfig{
		TargetURL:        "ws://example.com/ws",
		Mode:             fuzz.ModeSingle,
		ExecutionOptions: fuzz.FuzzerExecutionOptions{RequestTimeoutSeconds: 5},
		Script: []WsFuzzStep{
			{
				Role:      RoleFuzz,
				Content:   "x",
				Positions: []fuzz.FuzzerPosition{{Start: 0, End: 1}},
				DelayMs:   1000,
				WaitFor:   &wsreplay.WaitForSpec{MatchType: wsreplay.MatchAny, TimeoutMs: 10000},
			},
		},
		SharedPayloads: &fuzz.FuzzerPayloadsGroup{Payloads: []string{"x"}},
	}
	warns, _ := Validate(cfg)
	require.Contains(t, warns, "sum of step delays + wait_for timeouts exceeds request_timeout budget")
}

func TestValidate_WarnsConnectionTimeoutSwallowsBudget(t *testing.T) {
	cfg := WsFuzzerConfig{
		TargetURL:         "ws://example.com/ws",
		ConnectionTimeout: 10000,
		ExecutionOptions:  fuzz.FuzzerExecutionOptions{RequestTimeoutSeconds: 5},
		Mode:              fuzz.ModeSingle,
		Script:            []WsFuzzStep{{Role: RoleFuzz, Content: "x", Positions: []fuzz.FuzzerPosition{{Start: 0, End: 1}}}},
		SharedPayloads:    &fuzz.FuzzerPayloadsGroup{Payloads: []string{"x"}},
	}
	warns, _ := Validate(cfg)
	require.Contains(t, warns, "connection timeout consumes the entire iteration budget; no time left for steps")
}

func TestValidate_BlocksRunawayCount(t *testing.T) {
	cfg := WsFuzzerConfig{
		TargetURL: "ws://example.com/ws",
		Mode:      fuzz.ModeSingle,
		Script: []WsFuzzStep{
			{Role: RoleFuzz, Content: "x", Positions: []fuzz.FuzzerPosition{{Start: 0, End: 1}}},
		},
		SharedPayloads: &fuzz.FuzzerPayloadsGroup{Payloads: make([]string, 1_500_000)},
	}
	_, errs := Validate(cfg)
	require.Contains(t, errs, "planned iteration count exceeds limit of 1,000,000")
}

func TestValidate_BinaryJSONPathExtractionRejected(t *testing.T) {
	cfg := WsFuzzerConfig{
		TargetURL: "ws://example.com/ws",
		Mode:      fuzz.ModeSingle,
		Script: []WsFuzzStep{
			{
				Role: RoleFuzz, Opcode: 2, Content: "x",
				Positions: []fuzz.FuzzerPosition{{Start: 0, End: 1}},
				Extract: []Extraction{
					{Name: "v", Source: SourceLastReceivedFrame, Method: MethodJSONPath, GroupOrPath: "$.x"},
				},
			},
		},
		SharedPayloads: &fuzz.FuzzerPayloadsGroup{Payloads: []string{"x"}},
	}
	_, errs := Validate(cfg)
	require.NotEmpty(t, errs)
}
