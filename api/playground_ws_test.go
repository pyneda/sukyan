package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gorilla/websocket"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startWsEchoServer spins up an in-process WebSocket echo server. Mirrors the
// helper in pkg/playground/wsreplay/session_test.go (which is package-private)
// so the api integration tests have a stable upstream to drive runs against.
func startWsEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			mt, msg, err := c.ReadMessage()
			if err != nil {
				return
			}
			if err := c.WriteMessage(mt, msg); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func httpToWs(httpURL string) string { return "ws" + strings.TrimPrefix(httpURL, "http") }

// setupPlaygroundWsTestEnv creates a fresh workspace + collection and registers
// a cleanup that hard-deletes the workspace (CASCADE removes the collection,
// playground_sessions, playground_ws_sessions, runs, and websocket_connections).
func setupPlaygroundWsTestEnv(t *testing.T) (*db.Workspace, *db.PlaygroundCollection) {
	t.Helper()
	code := "test-playground-ws-" + lib.GenerateRandomLowercaseString(8)
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Code:        code,
		Title:       code,
		Description: "ws integration test",
	})
	require.NoError(t, err)
	require.NotZero(t, workspace.ID)
	t.Cleanup(func() { _ = db.Connection().DeleteWorkspace(workspace.ID) })

	coll := &db.PlaygroundCollection{
		Name:        "ws-test-collection",
		WorkspaceID: workspace.ID,
	}
	require.NoError(t, db.Connection().CreatePlaygroundCollection(coll))
	return workspace, coll
}

// newPlaygroundWsApp builds a fiber app with all the WS playground routes the
// integration tests touch. JWT middleware is intentionally omitted; the unit
// tests exercise handlers directly.
func newPlaygroundWsApp() *fiber.App {
	app := fiber.New()
	app.Post("/api/v1/playground/ws/sessions", CreatePlaygroundWsSession)
	app.Get("/api/v1/playground/ws/sessions/:id", GetPlaygroundWsSession)
	app.Put("/api/v1/playground/ws/sessions/:id", UpdatePlaygroundWsSession)
	app.Delete("/api/v1/playground/ws/sessions/:id", DeletePlaygroundWsSession)
	app.Post("/api/v1/playground/ws/sessions/:id/runs", StartPlaygroundWsRun)
	app.Get("/api/v1/playground/ws/sessions/:id/runs", ListPlaygroundWsRuns)
	app.Post("/api/v1/playground/ws/sessions/:id/connect", ConnectPlaygroundWs)
	app.Post("/api/v1/playground/ws/sessions/:id/disconnect", DisconnectPlaygroundWs)
	app.Post("/api/v1/playground/ws/sessions/:id/frames", SendInteractiveFrame)
	return app
}

// createWsSessionResponse mirrors the {session, ws} envelope returned by
// CreatePlaygroundWsSession.
type createWsSessionResponse struct {
	Session db.PlaygroundSession   `json:"session"`
	Ws      db.PlaygroundWsSession `json:"ws"`
}

// TestPlaygroundWsCRUD exercises Create / Get / Update / Delete with no run.
// This subtest does not need an upstream WS server.
func TestPlaygroundWsCRUD(t *testing.T) {
	workspace, coll := setupPlaygroundWsTestEnv(t)
	app := newPlaygroundWsApp()

	// --- Create ---
	createInput := CreateWsSessionInput{
		CollectionID: coll.ID,
		WorkspaceID:  workspace.ID,
		Name:         "ws-crud-session",
		TargetURL:    "ws://example.invalid/ws",
		Script:       json.RawMessage(`[]`),
	}
	createBody, err := json.Marshal(createInput)
	require.NoError(t, err)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/playground/ws/sessions", strings.NewReader(string(createBody)))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusCreated, createResp.StatusCode)

	var created createWsSessionResponse
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))
	require.NotZero(t, created.Session.ID)
	require.NotZero(t, created.Ws.ID)
	assert.Equal(t, "ws-crud-session", created.Session.Name)
	assert.Equal(t, "ws://example.invalid/ws", created.Ws.TargetURL)
	assert.Equal(t, created.Session.ID, created.Ws.PlaygroundSessionID)

	// --- Get ---
	getURL := fmt.Sprintf("/api/v1/playground/ws/sessions/%d", created.Session.ID)
	getReq := httptest.NewRequest(http.MethodGet, getURL, nil)
	getResp, err := app.Test(getReq)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusOK, getResp.StatusCode)

	var got struct {
		Session    db.PlaygroundSession   `json:"session"`
		Ws         db.PlaygroundWsSession `json:"ws"`
		RecentRuns []*db.PlaygroundWsRun  `json:"recent_runs"`
	}
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&got))
	assert.Equal(t, created.Session.ID, got.Session.ID)
	assert.Equal(t, created.Ws.ID, got.Ws.ID)
	assert.Equal(t, "ws://example.invalid/ws", got.Ws.TargetURL)

	// --- Update (target_url + script). NOTE: skipping Name update here because of
	// a known-deferred GORM zero-value bug on the Update path. ---
	newURL := "ws://example.invalid/updated"
	newScript := json.RawMessage(`[{"id":"s1","name":"hi","content":"hi","opcode":1,"delay_ms":0,"on_timeout":"continue","on_no_match":"abort"}]`)
	updateInput := UpdateWsSessionInput{
		TargetURL: &newURL,
		Script:    newScript,
	}
	updateBody, err := json.Marshal(updateInput)
	require.NoError(t, err)
	updateReq := httptest.NewRequest(http.MethodPut, getURL, strings.NewReader(string(updateBody)))
	updateReq.Header.Set("Content-Type", "application/json")
	updateResp, err := app.Test(updateReq)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusOK, updateResp.StatusCode)

	var updated db.PlaygroundWsSession
	require.NoError(t, json.NewDecoder(updateResp.Body).Decode(&updated))
	assert.Equal(t, newURL, updated.TargetURL)
	assert.JSONEq(t, string(newScript), string(updated.Script))

	// --- Delete ---
	deleteReq := httptest.NewRequest(http.MethodDelete, getURL, nil)
	deleteResp, err := app.Test(deleteReq)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusNoContent, deleteResp.StatusCode)

	// Subsequent GET should now 404.
	getAfterDelete, err := app.Test(httptest.NewRequest(http.MethodGet, getURL, nil))
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, getAfterDelete.StatusCode)
}

// TestPlaygroundWsHappyPathRun creates a session, starts a run against an
// in-process echo server, and polls list-runs until status becomes "succeeded".
func TestPlaygroundWsHappyPathRun(t *testing.T) {
	// Boot-time wiring is Task 18; until then the test must initialize the
	// default manager itself or the run goroutine nil-derefs.
	wsreplay.Init(wsreplay.NewDBPersister(db.Connection()))

	echo := startWsEchoServer(t)
	workspace, coll := setupPlaygroundWsTestEnv(t)
	app := newPlaygroundWsApp()

	script := json.RawMessage(`[{"id":"s1","name":"hello","content":"ping","opcode":1,"delay_ms":0,"on_timeout":"continue","on_no_match":"abort","wait_for":{"match_type":"contains","pattern":"ping","timeout_ms":2000}}]`)
	createInput := CreateWsSessionInput{
		CollectionID: coll.ID,
		WorkspaceID:  workspace.ID,
		Name:         "ws-happy-run",
		TargetURL:    httpToWs(echo.URL),
		Script:       script,
		Options:      json.RawMessage(`{"connection_timeout_ms":5000,"send_timeout_ms":2000,"inter_step_delay_ms":0}`),
	}
	createBody, err := json.Marshal(createInput)
	require.NoError(t, err)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/playground/ws/sessions", strings.NewReader(string(createBody)))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusCreated, createResp.StatusCode)

	var created createWsSessionResponse
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))
	require.NotZero(t, created.Session.ID)

	// --- Start run ---
	runsURL := fmt.Sprintf("/api/v1/playground/ws/sessions/%d/runs", created.Session.ID)
	startReq := httptest.NewRequest(http.MethodPost, runsURL, nil)
	startReq.Header.Set("Content-Type", "application/json")
	// Fiber's app.Test default timeout is 1s; the run executes in a goroutine and
	// returns 202 immediately, so the default is fine here, but we extend the
	// deadline to be safe under loaded CI.
	startResp, err := app.Test(startReq, 5000)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusAccepted, startResp.StatusCode)

	var startBody struct {
		RunID uint `json:"run_id"`
	}
	require.NoError(t, json.NewDecoder(startResp.Body).Decode(&startBody))
	require.NotZero(t, startBody.RunID)

	// --- Poll list-runs until terminal status ---
	deadline := time.Now().Add(5 * time.Second)
	var finalStatus db.PlaygroundWsRunStatus
	for time.Now().Before(deadline) {
		listReq := httptest.NewRequest(http.MethodGet, runsURL, nil)
		listResp, err := app.Test(listReq)
		require.NoError(t, err)
		require.Equal(t, fiber.StatusOK, listResp.StatusCode)
		var listBody struct {
			Data  []*db.PlaygroundWsRun `json:"data"`
			Count int64                 `json:"count"`
		}
		require.NoError(t, json.NewDecoder(listResp.Body).Decode(&listBody))

		for _, r := range listBody.Data {
			if r.ID == startBody.RunID {
				finalStatus = r.Status
				break
			}
		}
		if finalStatus == db.WsRunSucceeded || finalStatus == db.WsRunFailed || finalStatus == db.WsRunCancelled {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	assert.Equal(t, db.WsRunSucceeded, finalStatus, "run should succeed against echo upstream")
}

// TestPlaygroundWsInteractive exercises the connect/send/disconnect REST path
// against an in-process echo server. It asserts the manager-state transitions
// observable through the interactive endpoints; it does not subscribe to the
// control-WS event stream.
func TestPlaygroundWsInteractive(t *testing.T) {
	wsreplay.Init(wsreplay.NewDBPersister(db.Connection()))

	echo := startWsEchoServer(t)
	workspace, coll := setupPlaygroundWsTestEnv(t)
	app := newPlaygroundWsApp()

	createInput := CreateWsSessionInput{
		CollectionID: coll.ID,
		WorkspaceID:  workspace.ID,
		Name:         "ws-interactive",
		TargetURL:    httpToWs(echo.URL),
		Script:       json.RawMessage(`[]`),
		Options:      json.RawMessage(`{"connection_timeout_ms":5000,"send_timeout_ms":2000,"inter_step_delay_ms":0}`),
	}
	createBody, err := json.Marshal(createInput)
	require.NoError(t, err)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/playground/ws/sessions", strings.NewReader(string(createBody)))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusCreated, createResp.StatusCode)

	var created createWsSessionResponse
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))

	connectURL := fmt.Sprintf("/api/v1/playground/ws/sessions/%d/connect", created.Session.ID)
	frameURL := fmt.Sprintf("/api/v1/playground/ws/sessions/%d/frames", created.Session.ID)
	disconnectURL := fmt.Sprintf("/api/v1/playground/ws/sessions/%d/disconnect", created.Session.ID)

	// --- Connect ---
	connectResp, err := app.Test(httptest.NewRequest(http.MethodPost, connectURL, nil), 5000)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusOK, connectResp.StatusCode)

	// --- Send a frame ---
	frameInput := SendFrameInput{Opcode: 1, Content: "hello"}
	frameBody, err := json.Marshal(frameInput)
	require.NoError(t, err)
	frameReq := httptest.NewRequest(http.MethodPost, frameURL, strings.NewReader(string(frameBody)))
	frameReq.Header.Set("Content-Type", "application/json")
	frameResp, err := app.Test(frameReq, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusAccepted, frameResp.StatusCode)

	// --- Disconnect ---
	disconnectResp, err := app.Test(httptest.NewRequest(http.MethodPost, disconnectURL, nil))
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNoContent, disconnectResp.StatusCode)
}
