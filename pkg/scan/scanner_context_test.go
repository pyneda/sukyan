package scan

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// historyPointingAt builds a minimal history whose revalidation request targets url.
func historyPointingAt(url string) *db.History {
	return &db.History{
		Method:     "GET",
		URL:        url,
		RawRequest: []byte("GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"),
	}
}

func newTimeBasedResult(server *httptest.Server, sleep string, observed time.Duration) TemplateScannerResult {
	return TemplateScannerResult{
		Original: historyPointingAt(server.URL),
		Result:   historyPointingAt(server.URL),
		Duration: observed,
		Payload: generation.Payload{
			IssueCode:          string(db.SqlInjectionCode),
			DetectionCondition: generation.Or,
			DetectionMethods: []generation.DetectionMethod{
				{TimeBased: &generation.TimeBasedDetectionMethod{Sleep: sleep, Confidence: 90}},
			},
		},
	}
}

func workspaceForScannerTest(t *testing.T) uint {
	t.Helper()
	ws, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Title:       "TestWorkspaceScannerCtx",
		Code:        "test-workspace-scanner-ctx-" + uuid.New().String(),
		Description: "Test workspace for scanner context cancellation",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.Connection().DeleteWorkspace(ws.ID) })
	return ws.ID
}

// TestEvaluateResult_FreesPromptlyOnCancellationDuringTimeBasedLoop reproduces the
// stall: the time-based revalidation loop sleeps for up to ~14 minutes and, before
// this fix, ignored context cancellation entirely, so a cancelled or timed-out job
// kept its worker blocked. A cooperative executor test (blockingExecutor) cannot
// catch this because it returns on ctx.Done() by construction; this drives the real
// evaluation path with a payload whose revalidation would otherwise sleep 30s.
func TestEvaluateResult_FreesPromptlyOnCancellationDuringTimeBasedLoop(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Sleep "1ns" makes every real revalidation duration count as "higher", which
	// drives the loop into its 30s-per-attempt sleep branch.
	result := newTimeBasedResult(server, "1ns", 5*time.Second)

	f := &TemplateScanner{WorkspaceID: workspaceForScannerTest(t)}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	type evalOutcome struct {
		details string
	}
	done := make(chan evalOutcome, 1)
	go func() {
		_, details, _, _, _ := f.EvaluateResult(ctx, result)
		done <- evalOutcome{details: details}
	}()

	select {
	case out := <-done:
		assert.NotEmpty(t, out.details, "partial evidence gathered before cancellation must be preserved, not discarded")
		assert.Less(t, atomic.LoadInt32(&requests), int32(14),
			"evaluation must exit early on cancellation, not run all revalidation attempts")
	case <-time.After(3 * time.Second):
		t.Fatal("EvaluateResult did not return promptly after cancellation: the time-based loop ignored the context")
	}
}

// TestEvaluateResult_TimeBasedLoopCompletesWhenNotCancelled proves the fix does not
// disturb a well-behaved evaluation: with no cancellation and fast responses the
// loop runs every attempt to completion, so its evidence is not truncated.
func TestEvaluateResult_TimeBasedLoopCompletesWhenNotCancelled(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Sleep "10s" is far above the real (sub-millisecond) response time, so no
	// attempt trips the sleep branch and the loop runs all seven attempts.
	result := newTimeBasedResult(server, "10s", 11*time.Second)

	f := &TemplateScanner{WorkspaceID: workspaceForScannerTest(t)}

	done := make(chan struct{}, 1)
	go func() {
		f.EvaluateResult(context.Background(), result)
		done <- struct{}{}
	}()

	select {
	case <-done:
		// Seven attempts (i = 1..7), each issuing an original and a payload request.
		assert.Equal(t, int32(14), atomic.LoadInt32(&requests),
			"a well-behaved evaluation must run every revalidation attempt")
	case <-time.After(8 * time.Second):
		t.Fatal("EvaluateResult did not complete a well-behaved time-based revalidation in time")
	}
}
