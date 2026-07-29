package actions

import (
	"encoding/json"

	"github.com/pyneda/sukyan/lib"
)

const (
	StepStatusOK      = "ok"
	StepStatusFailed  = "failed"
	StepStatusSkipped = "skipped"
)

// ScreenshotResult describes a screenshot captured by a screenshot action.
type ScreenshotResult struct {
	Selector   string `json:"selector"`
	Data       string `json:"data,omitempty"`
	OutputFile string `json:"output_file,omitempty"`
}

// EvaluationResult holds the structured return value of an evaluate action.
type EvaluationResult struct {
	Expression string          `json:"expression"`
	Value      json.RawMessage `json:"value,omitempty"`
	Type       string          `json:"type"`
}

// AssertionResult holds the verdict of an assert action.
type AssertionResult struct {
	Condition string `json:"condition"`
	Selector  string `json:"selector"`
	Expected  string `json:"expected,omitempty"`
	Actual    string `json:"actual,omitempty"`
	Passed    bool   `json:"passed"`
}

// ActionFailure identifies the action that ended a run.
type ActionFailure struct {
	StepIndex int    `json:"step_index"`
	Type      string `json:"type"`
	Message   string `json:"message"`
}

// ActionStepResult is the record of a single executed (or skipped) action.
type ActionStepResult struct {
	Index      int               `json:"index"`
	Type       string            `json:"type"`
	Target     string            `json:"target,omitempty"`
	Status     string            `json:"status"`
	DurationMs int64             `json:"duration_ms"`
	Error      string            `json:"error,omitempty"`
	Screenshot *ScreenshotResult `json:"screenshot,omitempty"`
	Evaluation *EvaluationResult `json:"evaluation,omitempty"`
	Assertion  *AssertionResult  `json:"assertion,omitempty"`
}

// ActionsExecutionResults is the full record of one action sequence run.
type ActionsExecutionResults struct {
	Succeeded   bool               `json:"succeeded"`
	Steps       []ActionStepResult `json:"steps"`
	Screenshots []ScreenshotResult `json:"screenshots"`
	Logs        []lib.LogEntry     `json:"logs"`
	Failure     *ActionFailure     `json:"failure,omitempty"`
	DurationMs  int64              `json:"duration_ms"`
}

// stepOutput carries the optional typed payload an action may produce.
type stepOutput struct {
	screenshot *ScreenshotResult
	evaluation *EvaluationResult
	assertion  *AssertionResult
}

// actionTarget returns the human-meaningful subject of an action, used as the
// step's Target: the URL for navigate, the selector otherwise.
func actionTarget(action Action) string {
	if action.Type == ActionNavigate {
		return action.URL
	}
	return action.Selector
}
