package manual

import (
	"encoding/json"
	"testing"

	"github.com/pyneda/sukyan/pkg/browser/actions"
	"github.com/stretchr/testify/assert"
)

func TestBrowserReplayActionsResultsOmitsAbsentPhases(t *testing.T) {
	var empty BrowserReplayActionsResults
	raw, err := json.Marshal(empty)
	assert.NoError(t, err)
	assert.JSONEq(t, `{}`, string(raw), "absent phases must not serialize")

	withPre := BrowserReplayActionsResults{
		PreRequest: &actions.ActionsExecutionResults{Succeeded: true},
	}
	raw, err = json.Marshal(withPre)
	assert.NoError(t, err)

	var decoded map[string]any
	assert.NoError(t, json.Unmarshal(raw, &decoded))
	_, hasPre := decoded["pre_request"]
	_, hasPost := decoded["post_request"]
	assert.True(t, hasPre, "a phase that ran is present")
	assert.False(t, hasPost, "a phase that did not run is absent")
}

func TestNavigationFailureKeepsPreRequestResults(t *testing.T) {
	// A pre-request action (e.g. a login) can succeed before navigation itself
	// fails. That evidence - steps, screenshots, evaluations, assertions -
	// must survive, not be replaced by a bare zero-value ReplayResult.
	pre := &actions.ActionsExecutionResults{
		Succeeded: true,
		Steps: []actions.ActionStepResult{
			{Index: 0, Type: "fill", Status: actions.StepStatusOK},
			{Index: 1, Type: "click", Status: actions.StepStatusOK},
		},
	}
	browserActionsResults := BrowserReplayActionsResults{PreRequest: pre}

	result := navigationFailureResult(browserActionsResults)

	assert.Same(t, pre, result.BrowserActionsResults.PreRequest, "pre-request phase must survive a navigation failure")
	assert.Nil(t, result.BrowserActionsResults.PostRequest, "post-request never ran")
	assert.Nil(t, result.Result, "no history record exists when navigation itself failed")

	raw, err := json.Marshal(result.BrowserActionsResults)
	assert.NoError(t, err)
	assert.Contains(t, string(raw), `"pre_request"`)
	assert.Contains(t, string(raw), `"status":"ok"`)
}

func TestPostRequestFailureKeepsResults(t *testing.T) {
	// A post-request phase that failed must still be reported as a phase that
	// ran, carrying its failure, so the UI can mark the offending step.
	failed := &actions.ActionsExecutionResults{
		Succeeded: false,
		Failure:   &actions.ActionFailure{StepIndex: 2, Type: "assert", Message: "assertion failed"},
		Steps: []actions.ActionStepResult{
			{Index: 0, Type: "navigate", Status: actions.StepStatusOK},
			{Index: 1, Type: "click", Status: actions.StepStatusOK},
			{Index: 2, Type: "assert", Status: actions.StepStatusFailed, Error: "assertion failed"},
		},
	}
	result := ReplayResult{BrowserActionsResults: BrowserReplayActionsResults{PostRequest: failed}}

	raw, err := json.Marshal(result.BrowserActionsResults)
	assert.NoError(t, err)
	assert.Contains(t, string(raw), `"post_request"`)
	assert.NotContains(t, string(raw), `"pre_request"`)
	assert.Contains(t, string(raw), `"step_index":2`)
	assert.Contains(t, string(raw), `"status":"failed"`)
}
