package wsreplay

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pyneda/sukyan/pkg/playground/stream"
	"github.com/stretchr/testify/require"
)

// dialEchoSession boots an echo server + dials a fresh session against it.
// Returns the session for use in WalkScript tests; caller must Close it.
func dialEchoSession(t *testing.T, runID uint) *Session {
	t.Helper()
	echo := startEchoServer(t)
	b := stream.NewBroadcaster(256, 1000)
	persist := newFakePersister()
	sess, err := DialSession(context.Background(), SessionConfig{
		TargetURL:      wsURL(echo.URL),
		Instance:       RunInstance(runID),
		Persister:      persist,
		Events:         b,
		ConnectTimeout: 2 * time.Second,
		SendTimeout:    time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { sess.Close() })
	return sess
}

func TestWalkScript_VariableSubstitution_HappyPath(t *testing.T) {
	// Step 1 sends a token-bearing payload to the echo server, which echoes
	// it back. Step 1's Extract pulls "token" out via JSON path. Step 2's
	// Content references ${token} and the matched frame should contain the
	// substituted value (echoed back).
	sess := dialEchoSession(t, 100)
	b := stream.NewBroadcaster(256, 1000)
	script := []ScriptEntry{
		{
			ID: "auth", Content: `{"ok":true,"token":"sek-ret-xyz"}`, Opcode: 1,
			OnTimeout: PolicyAbort, OnNoMatch: PolicyAbort,
			WaitFor: &WaitForSpec{MatchType: MatchContains, Pattern: "token", TimeoutMs: 1000},
			Extract: []Extraction{{Name: "token", Method: ExtractMethodJSONPath, Group: "$.token"}},
		},
		{
			ID: "use", Content: `{"action":"query","auth":"${token}"}`, Opcode: 1,
			OnTimeout: PolicyAbort, OnNoMatch: PolicyAbort,
			WaitFor: &WaitForSpec{MatchType: MatchContains, Pattern: "sek-ret-xyz", TimeoutMs: 1000},
		},
	}
	res := WalkScript(context.Background(), sess, script, SessionOptions{}, b)
	require.Equal(t, "succeeded", res.Status, "failure_reason: %s", res.FailureReason)
}

func TestWalkScript_UndefinedVariable_AbortsBeforeSend(t *testing.T) {
	// The second step references ${not_set} which was never extracted. The
	// step must fail BEFORE sending so the literal `${not_set}` never hits
	// the wire.
	sess := dialEchoSession(t, 101)
	b := stream.NewBroadcaster(256, 1000)
	script := []ScriptEntry{
		{ID: "first", Content: "hello", Opcode: 1,
			OnTimeout: PolicyAbort, OnNoMatch: PolicyAbort,
			WaitFor: &WaitForSpec{MatchType: MatchAny, TimeoutMs: 500}},
		{ID: "second", Content: "auth=${not_set}", Opcode: 1,
			OnTimeout: PolicyAbort, OnNoMatch: PolicyAbort},
	}
	res := WalkScript(context.Background(), sess, script, SessionOptions{}, b)
	require.Equal(t, "failed", res.Status)
	require.Contains(t, res.FailureReason, "undefined variable")
	require.Contains(t, res.FailureReason, "${not_set}")
}

func TestWalkScript_UndefinedVariable_ListsMultipleNames(t *testing.T) {
	sess := dialEchoSession(t, 102)
	b := stream.NewBroadcaster(256, 1000)
	script := []ScriptEntry{
		{ID: "x", Content: "a=${a} b=${b}", Opcode: 1, OnTimeout: PolicyAbort, OnNoMatch: PolicyAbort},
	}
	res := WalkScript(context.Background(), sess, script, SessionOptions{}, b)
	require.Equal(t, "failed", res.Status)
	require.Contains(t, res.FailureReason, "undefined variables")
	require.Contains(t, res.FailureReason, "${a}")
	require.Contains(t, res.FailureReason, "${b}")
}

func TestWalkScript_ExtractWithoutWaitFor_FailsWithClearReason(t *testing.T) {
	// Extracting without a wait_for has no deterministic source frame; we
	// reject this at run-time instead of silently dropping the extract.
	sess := dialEchoSession(t, 103)
	b := stream.NewBroadcaster(256, 1000)
	script := []ScriptEntry{
		{ID: "x", Content: "hello", Opcode: 1, OnTimeout: PolicyAbort, OnNoMatch: PolicyAbort,
			Extract: []Extraction{{Name: "v", Method: ExtractMethodFull}}},
	}
	res := WalkScript(context.Background(), sess, script, SessionOptions{}, b)
	require.Equal(t, "failed", res.Status)
	require.True(t, strings.Contains(res.FailureReason, "extract requires wait_for"),
		"got %q", res.FailureReason)
}

func TestWalkScript_ExtractFailure_AbortsByDefault(t *testing.T) {
	// JSON path `$.absent` won't find anything in the echoed text frame.
	// Default OnFailure (empty string == treated as abort by Apply caller)
	// fails the run with a clear reason.
	sess := dialEchoSession(t, 104)
	b := stream.NewBroadcaster(256, 1000)
	script := []ScriptEntry{
		{ID: "x", Content: `{"present":1}`, Opcode: 1,
			OnTimeout: PolicyAbort, OnNoMatch: PolicyAbort,
			WaitFor: &WaitForSpec{MatchType: MatchAny, TimeoutMs: 1000},
			Extract: []Extraction{{Name: "v", Method: ExtractMethodJSONPath, Group: "$.absent"}}},
	}
	res := WalkScript(context.Background(), sess, script, SessionOptions{}, b)
	require.Equal(t, "failed", res.Status)
	require.Contains(t, res.FailureReason, "extract v failed")
}

func TestWalkScript_ExtractFailure_ContinuePolicy(t *testing.T) {
	// With OnFailure="continue", a missing path sets the var to "" and the
	// run proceeds. The follow-up step references ${v} which is now defined
	// (empty), so substitution succeeds and the step ships an empty value.
	sess := dialEchoSession(t, 105)
	b := stream.NewBroadcaster(256, 1000)
	script := []ScriptEntry{
		{ID: "extract", Content: `{"present":1}`, Opcode: 1,
			OnTimeout: PolicyAbort, OnNoMatch: PolicyAbort,
			WaitFor: &WaitForSpec{MatchType: MatchAny, TimeoutMs: 1000},
			Extract: []Extraction{{
				Name: "v", Method: ExtractMethodJSONPath, Group: "$.absent",
				OnFailure: PolicyContinue,
			}}},
		{ID: "use", Content: "v=[${v}]", Opcode: 1,
			OnTimeout: PolicyAbort, OnNoMatch: PolicyAbort,
			WaitFor: &WaitForSpec{MatchType: MatchContains, Pattern: "v=[]", TimeoutMs: 1000}},
	}
	res := WalkScript(context.Background(), sess, script, SessionOptions{}, b)
	require.Equal(t, "succeeded", res.Status, "failure_reason: %s", res.FailureReason)
}
