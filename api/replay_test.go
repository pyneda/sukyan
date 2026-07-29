package api

import (
	"encoding/json"
	"testing"

	"github.com/pyneda/sukyan/pkg/browser/actions"
	"github.com/pyneda/sukyan/pkg/manual"
	"github.com/stretchr/testify/require"
)

// TestReplayErrorResponse_OmitsBrowserActionsResultsWhenAbsent guards against a
// ReplayErrorResponse for an error unrelated to browser actions (bad URL,
// validation failure, raw-mode replay) growing a misleading empty
// "browser_actions_results" key in the JSON body.
func TestReplayErrorResponse_OmitsBrowserActionsResultsWhenAbsent(t *testing.T) {
	response := ReplayErrorResponse{
		Error:   "Request Replay Failed",
		Message: "some non-browser-action related failure",
	}

	raw, err := json.Marshal(response)
	require.NoError(t, err)

	var asMap map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &asMap))

	require.NotContains(t, asMap, "browser_actions_results")
	require.Contains(t, asMap, "error")
	require.Contains(t, asMap, "message")
}

// TestReplayErrorResponse_IncludesBrowserActionsResultsWhenPopulated verifies
// that when a phase's results are attached, the field is present in the JSON
// body and round-trips the collected partial results.
func TestReplayErrorResponse_IncludesBrowserActionsResultsWhenPopulated(t *testing.T) {
	results := &manual.BrowserReplayActionsResults{
		PreRequest: &actions.ActionsExecutionResults{
			Succeeded: false,
			Failure: &actions.ActionFailure{
				StepIndex: 1,
				Message:   "element not found",
			},
		},
	}
	response := ReplayErrorResponse{
		Error:                 "Request Replay Failed",
		Message:               "pre-request action failed",
		BrowserActionsResults: results,
	}

	raw, err := json.Marshal(response)
	require.NoError(t, err)

	var asMap map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &asMap))
	require.Contains(t, asMap, "browser_actions_results")

	var decoded ReplayErrorResponse
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.NotNil(t, decoded.BrowserActionsResults)
	require.NotNil(t, decoded.BrowserActionsResults.PreRequest)
	require.False(t, decoded.BrowserActionsResults.PreRequest.Succeeded)
	require.NotNil(t, decoded.BrowserActionsResults.PreRequest.Failure)
	require.Equal(t, "element not found", decoded.BrowserActionsResults.PreRequest.Failure.Message)
	require.Nil(t, decoded.BrowserActionsResults.PostRequest)
}
