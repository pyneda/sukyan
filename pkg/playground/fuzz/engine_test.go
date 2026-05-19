package fuzz

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/require"
)

// startEchoServer returns a server that echoes the request path so we can
// verify our payloads landed where expected. The path is base64-encoded in
// the response body to keep it cleanly inspectable.
func startEchoServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "echo:%s", r.URL.RawQuery)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// setupFuzzTestSession ensures the fuzz run table exists and creates a
// throw-away workspace+collection+session. Returns the workspace/session ids
// + the in-memory db Persister hook for engine.Run.
func setupFuzzTestSession(t *testing.T) (workspaceID, sessionID uint) {
	t.Helper()
	conn := db.Connection()
	if !conn.DB().Migrator().HasTable(&db.PlaygroundFuzzRun{}) {
		require.NoError(t, conn.DB().Migrator().CreateTable(&db.PlaygroundFuzzRun{}))
	}
	ws, err := conn.GetOrCreateWorkspace(&db.Workspace{Title: "fuzz-test-" + t.Name(), Code: "fuzz_" + sanitizeName(t.Name())})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.DeleteWorkspace(ws.ID) })
	coll := &db.PlaygroundCollection{Name: "c", WorkspaceID: ws.ID}
	require.NoError(t, conn.CreatePlaygroundCollection(coll))
	sess := &db.PlaygroundSession{Name: "s", Type: db.FuzzType, WorkspaceID: ws.ID, CollectionID: coll.ID}
	require.NoError(t, conn.CreatePlaygroundSession(sess))
	return ws.ID, sess.ID
}

func sanitizeName(s string) string {
	return strings.NewReplacer("/", "_", " ", "_", "-", "_").Replace(s)
}

func TestEngineRunSmokeCombinations(t *testing.T) {
	srv := startEchoServer(t)
	wsID, sessID := setupFuzzTestSession(t)

	conn := db.Connection()
	run := &db.PlaygroundFuzzRun{
		PlaygroundSessionID: sessID,
		WorkspaceID:         wsID,
		ConfigSnapshot:      []byte(`{}`),
		Status:              db.FuzzRunPending,
	}
	require.NoError(t, conn.CreatePlaygroundFuzzRun(run))

	raw := fmt.Sprintf("GET /?a=A&b=B HTTP/1.1\r\nHost: %s\r\n\r\n", strings.TrimPrefix(srv.URL, "http://"))
	positions := []FuzzerPosition{
		{Start: strings.Index(raw, "A"), End: strings.Index(raw, "A") + 1, OriginalValue: "A"},
		{Start: strings.Index(raw, "B"), End: strings.Index(raw, "B") + 1, OriginalValue: "B"},
	}
	for i, p := range positions {
		t.Logf("position %d: %d-%d %q", i, p.Start, p.End, p.OriginalValue)
	}
	resolved := ResolvedPayloads{
		PerPosition: [][]string{
			{"X1", "X2", "X3"},
			{"Y1", "Y2", "Y3"},
		},
	}

	strategy, err := StrategyFor(ModeCombinations)
	require.NoError(t, err)

	var (
		results []*FuzzResult
		mu      sync.Mutex
	)
	hooks := Hooks{
		Publish: func(r *FuzzResult) {
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		},
	}

	outcome := Run(context.Background(), RunInput{
		RunID:               run.ID,
		WorkspaceID:         wsID,
		PlaygroundSessionID: sessID,
		TargetURL:           srv.URL,
		RawRequest:          raw,
		Mode:                ModeCombinations,
		Positions:           positions,
		Resolved:            resolved,
		Strategy:            strategy,
		Execution:           DefaultExecutionOptions(),
		Hooks:               hooks,
	})

	require.Equal(t, db.FuzzRunSucceeded, outcome.Status)
	require.Equal(t, 9, outcome.SentCount, "3 × 3 = 9 combinations")
	require.Len(t, results, 9)

	// Every result should have a HistoryID, OK status, the right payloads.
	seenCombos := map[string]bool{}
	for _, r := range results {
		require.NotZero(t, r.HistoryID, "expected persisted history row")
		require.Equal(t, http.StatusOK, r.StatusCode)
		require.Len(t, r.PayloadValues, 2)
		key := r.PayloadValues[0] + "|" + r.PayloadValues[1]
		require.False(t, seenCombos[key], "duplicate combo %q", key)
		seenCombos[key] = true
	}
	require.Len(t, seenCombos, 9)
}

func TestEngineRunCancellation(t *testing.T) {
	// Slow server that holds requests so we can cancel mid-flight.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wsID, sessID := setupFuzzTestSession(t)
	conn := db.Connection()
	run := &db.PlaygroundFuzzRun{
		PlaygroundSessionID: sessID,
		WorkspaceID:         wsID,
		ConfigSnapshot:      []byte(`{}`),
		Status:              db.FuzzRunPending,
	}
	require.NoError(t, conn.CreatePlaygroundFuzzRun(run))

	raw := fmt.Sprintf("GET /?q=Q HTTP/1.1\r\nHost: %s\r\n\r\n", strings.TrimPrefix(srv.URL, "http://"))
	positions := []FuzzerPosition{
		{Start: strings.Index(raw, "Q"), End: strings.Index(raw, "Q") + 1, OriginalValue: "Q"},
	}
	payloads := make([]string, 100)
	for i := range payloads {
		payloads[i] = fmt.Sprintf("p%d", i)
	}

	strategy, _ := StrategyFor(ModeSingle)
	exec := DefaultExecutionOptions()
	exec.Concurrency = 2

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	outcome := Run(ctx, RunInput{
		RunID:               run.ID,
		WorkspaceID:         wsID,
		PlaygroundSessionID: sessID,
		TargetURL:           srv.URL,
		RawRequest:          raw,
		Mode:                ModeSingle,
		Positions:           positions,
		Resolved:            ResolvedPayloads{Shared: payloads},
		Strategy:            strategy,
		Execution:           exec,
	})
	require.Equal(t, db.FuzzRunCancelled, outcome.Status)
	require.Less(t, outcome.SentCount, 100, "expected cancellation before all requests completed")
}

func TestEngineRunPauseHaltsScheduling(t *testing.T) {
	// Slow server so we can observe scheduling halt while paused.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wsID, sessID := setupFuzzTestSession(t)
	conn := db.Connection()
	run := &db.PlaygroundFuzzRun{
		PlaygroundSessionID: sessID,
		WorkspaceID:         wsID,
		ConfigSnapshot:      []byte(`{}`),
		Status:              db.FuzzRunPending,
	}
	require.NoError(t, conn.CreatePlaygroundFuzzRun(run))

	raw := fmt.Sprintf("GET /?q=Q HTTP/1.1\r\nHost: %s\r\n\r\n", strings.TrimPrefix(srv.URL, "http://"))
	positions := []FuzzerPosition{
		{Start: strings.Index(raw, "Q"), End: strings.Index(raw, "Q") + 1, OriginalValue: "Q"},
	}
	payloads := make([]string, 60)
	for i := range payloads {
		payloads[i] = fmt.Sprintf("p%d", i)
	}

	strategy, _ := StrategyFor(ModeSingle)
	exec := DefaultExecutionOptions()
	exec.Concurrency = 2

	gate := NewPauseGate()
	var sent atomic.Int64
	hooks := Hooks{
		Publish: func(r *FuzzResult) { sent.Add(1) },
	}

	// Pause shortly after start; verify sent count stays roughly stable; resume.
	go func() {
		time.Sleep(120 * time.Millisecond)
		gate.Pause()
		time.Sleep(400 * time.Millisecond)
		gate.Resume()
	}()

	outcome := Run(context.Background(), RunInput{
		RunID:               run.ID,
		WorkspaceID:         wsID,
		PlaygroundSessionID: sessID,
		TargetURL:           srv.URL,
		RawRequest:          raw,
		Mode:                ModeSingle,
		Positions:           positions,
		Resolved:            ResolvedPayloads{Shared: payloads},
		Strategy:            strategy,
		Execution:           exec,
		Hooks:               hooks,
		PauseGate:           gate,
	})
	require.Equal(t, db.FuzzRunSucceeded, outcome.Status)
	require.Equal(t, int64(60), sent.Load(), "all requests should eventually be sent after resume")
}

// TestEngineRunPauseResumeViaRegistry mirrors the production wiring: the gate
// lives in the global registry; engine fetches it via Default().Gate(runID);
// pause/resume happen via Default().Pause/Resume(runID), not on the local gate
// variable. Concurrency=1 + slow upstream + a long pause window — the
// regression seen in QA was that workers never resumed scheduling after the
// gate was released through the registry indirection.
func TestEngineRunPauseResumeViaRegistry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wsID, sessID := setupFuzzTestSession(t)
	conn := db.Connection()
	run := &db.PlaygroundFuzzRun{
		PlaygroundSessionID: sessID,
		WorkspaceID:         wsID,
		ConfigSnapshot:      []byte(`{}`),
		Status:              db.FuzzRunPending,
	}
	require.NoError(t, conn.CreatePlaygroundFuzzRun(run))

	raw := fmt.Sprintf("GET /?q=Q HTTP/1.1\r\nHost: %s\r\n\r\n", strings.TrimPrefix(srv.URL, "http://"))
	positions := []FuzzerPosition{
		{Start: strings.Index(raw, "Q"), End: strings.Index(raw, "Q") + 1, OriginalValue: "Q"},
	}
	payloads := make([]string, 40)
	for i := range payloads {
		payloads[i] = fmt.Sprintf("p%d", i)
	}

	strategy, _ := StrategyFor(ModeSingle)
	exec := DefaultExecutionOptions()
	exec.Concurrency = 1

	// Production wiring: registry holds the gate; engine receives it via Gate(id).
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	Default().Register(run.ID, cancel, nil)
	t.Cleanup(func() { Default().Unregister(run.ID) })

	var sent atomic.Int64
	hooks := Hooks{
		Publish: func(r *FuzzResult) { sent.Add(1) },
	}

	// Pause shortly after start, wait longer than any in-flight request,
	// resume via registry.
	go func() {
		time.Sleep(120 * time.Millisecond)
		ok := Default().Pause(run.ID)
		require.True(t, ok, "registry Pause should flip")
		time.Sleep(500 * time.Millisecond)
		ok = Default().Resume(run.ID)
		require.True(t, ok, "registry Resume should flip")
	}()

	outcome := Run(runCtx, RunInput{
		RunID:               run.ID,
		WorkspaceID:         wsID,
		PlaygroundSessionID: sessID,
		TargetURL:           srv.URL,
		RawRequest:          raw,
		Mode:                ModeSingle,
		Positions:           positions,
		Resolved:            ResolvedPayloads{Shared: payloads},
		Strategy:            strategy,
		Execution:           exec,
		Hooks:               hooks,
		PauseGate:           Default().Gate(run.ID),
	})
	require.Equal(t, db.FuzzRunSucceeded, outcome.Status)
	require.Equal(t, int64(40), sent.Load(), "all requests should eventually be sent after registry resume")
}

// TestEngineRunPauseLongerThanIdleConnTimeout exposes the production
// regression: a pause that lasts longer than rawhttp's MaxIdleConnDuration
// (10s default) causes the pipeline writer/reader goroutines to exit. When
// workers resume, DoRaw can race with channel teardown and hang.
func TestEngineRunPauseLongerThanIdleConnTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wsID, sessID := setupFuzzTestSession(t)
	conn := db.Connection()
	run := &db.PlaygroundFuzzRun{
		PlaygroundSessionID: sessID,
		WorkspaceID:         wsID,
		ConfigSnapshot:      []byte(`{}`),
		Status:              db.FuzzRunPending,
	}
	require.NoError(t, conn.CreatePlaygroundFuzzRun(run))

	raw := fmt.Sprintf("GET /?q=Q HTTP/1.1\r\nHost: %s\r\n\r\n", strings.TrimPrefix(srv.URL, "http://"))
	positions := []FuzzerPosition{
		{Start: strings.Index(raw, "Q"), End: strings.Index(raw, "Q") + 1, OriginalValue: "Q"},
	}
	payloads := make([]string, 30)
	for i := range payloads {
		payloads[i] = fmt.Sprintf("p%d", i)
	}

	strategy, _ := StrategyFor(ModeSingle)
	exec := DefaultExecutionOptions()
	exec.Concurrency = 1

	gate := NewPauseGate()
	var sent atomic.Int64
	hooks := Hooks{
		Publish: func(r *FuzzResult) { sent.Add(1) },
	}

	// Pause for 12s — exceeds rawhttp's 10s MaxIdleConnDuration so the
	// pipeline tears down. Verify workers still recover after resume.
	go func() {
		time.Sleep(200 * time.Millisecond)
		gate.Pause()
		time.Sleep(12 * time.Second)
		gate.Resume()
	}()

	done := make(chan struct{})
	var outcome RunOutcome
	go func() {
		outcome = Run(context.Background(), RunInput{
			RunID:               run.ID,
			WorkspaceID:         wsID,
			PlaygroundSessionID: sessID,
			TargetURL:           srv.URL,
			RawRequest:          raw,
			Mode:                ModeSingle,
			Positions:           positions,
			Resolved:            ResolvedPayloads{Shared: payloads},
			Strategy:            strategy,
			Execution:           exec,
			Hooks:               hooks,
			PauseGate:           gate,
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatalf("Run did not return after resume + 60s; sent=%d (pipeline hung after idle timeout)", sent.Load())
	}
	require.Equal(t, db.FuzzRunSucceeded, outcome.Status)
	require.Equal(t, int64(30), sent.Load(), "all requests should eventually be sent after long-pause resume")
}
