package actions

import (
	"bytes"
	"encoding/json"

	"github.com/go-rod/rod/lib/proto"
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

// evaluationValueType reports the type name to surface for an evaluate
// action's result, derived from the *serialized value* rather than from
// Chrome's RemoteObject.Subtype.
//
// Do not "simplify" this to `if obj.Subtype != "" { return obj.Subtype }`.
// That looks obviously correct but is not: page.Eval (used by the
// ActionEvaluate case) requests CDP's `returnByValue: true`, and under that
// mode Chrome only special-cases the "null" subtype - it never reports
// "array"/"date"/"map"/"set"/etc. subtypes, those are only populated on the
// object-reference/preview path (`returnByValue: false`), which does not
// hand back a serialized value at all. This was verified empirically against
// go-rod v0.116.2 + Chromium 150.0.7871.46 (see task-2-report.md): evaluating
// `() => [1,2,3]` by value comes back as {Type: "object", Subtype: ""}, so
// trusting Subtype would report every array as a plain "object" and defeat
// the whole point of this field - letting the frontend label and shape the
// value it is rendering.
func evaluationValueType(raw json.RawMessage, obj *proto.RuntimeRemoteObject) string {
	// These remote object types carry no serializable value at all - there is
	// nothing in `raw` to inspect, so report the CDP type directly.
	if obj != nil {
		switch obj.Type {
		case proto.RuntimeRemoteObjectTypeUndefined,
			proto.RuntimeRemoteObjectTypeFunction,
			proto.RuntimeRemoteObjectTypeSymbol:
			return string(obj.Type)
		}
	}

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "undefined"
	}

	switch trimmed[0] {
	case '[':
		return "array"
	case '{':
		return "object"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		// digit or '-': a JSON number.
		return "number"
	}
}
