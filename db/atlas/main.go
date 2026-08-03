package main

import (
	"fmt"
	"io"
	"os"

	"ariga.io/atlas-provider-gorm/gormschema"
	"github.com/pyneda/sukyan/db"
)

func main() {
	stmts, err := gormschema.New("postgres").Load(
		&db.Workspace{},
		&db.User{},
		&db.WorkerNode{},
		&db.Scan{},
		&db.ScanJob{},
		&db.PlaygroundCollection{},
		&db.PlaygroundSession{},
		&db.PlaygroundWsSession{},
		&db.PlaygroundWsRun{},
		&db.PlaygroundFuzzRun{},
		&db.PlaygroundWsFuzzRun{},
		&db.PlaygroundWsFuzzIteration{},
		&db.Task{},
		&db.TaskJob{},
		&db.History{},
		&db.WebSocketConnection{},
		&db.WebSocketMessage{},
		&db.JsonWebToken{},
		&db.WorkspaceCookie{},
		&db.StoredBrowserActions{},
		&db.Issue{},
		&db.OOBTest{},
		&db.OOBInteraction{},
		&db.BrowserEvent{},
		&db.SiteBehaviorResult{},
		&db.SiteBehaviorNotFoundSample{},
		&db.APIBehaviorResult{},
		&db.APIDefinition{},
		&db.APIEndpoint{},
		&db.APIDefinitionSecurityScheme{},
		&db.APIAuthConfig{},
		&db.APIAuthHeader{},
		&db.TokenRefreshConfig{},
		&db.APIScan{},
		&db.ScanAPIDefinition{},
		&db.ProxyService{},
		&db.MatcherPreset{},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load gorm schema: %v\n", err)
		os.Exit(1)
	}
	io.WriteString(os.Stdout, stmts)
	io.WriteString(os.Stdout, indexesGormCannotExpress)
}

// Indexes that exist in the database but have no representation in the GORM
// models, so `atlas migrate diff` would otherwise emit a DROP for them.
//
// The join table indexes cover the trailing column of a many2many composite
// primary key: GORM generates those tables itself, so there is no struct to
// carry an index tag, and without this the trailing foreign key is unindexed
// and every parent delete degrades into a sequential scan.
//
// The partial indexes back the "is there an active run" queries and cannot be
// expressed as a GORM tag at all.
const indexesGormCannotExpress = `
CREATE INDEX "idx_issue_requests_history_id" ON "issue_requests" ("history_id");
CREATE INDEX "idx_json_web_token_histories_history_id" ON "json_web_token_histories" ("history_id");
CREATE INDEX "idx_jwt_websocket_connections_connection_id" ON "json_web_token_websocket_connections" ("web_socket_connection_id");
CREATE INDEX "idx_api_scan_endpoints_api_endpoint_id" ON "api_scan_endpoints" ("api_endpoint_id");
CREATE INDEX "idx_playground_ws_runs_status_running" ON "playground_ws_runs" ("status") WHERE status = 'running';
CREATE INDEX "idx_playground_fuzz_runs_status_active" ON "playground_fuzz_runs" ("status") WHERE status IN ('pending', 'calibrating', 'running', 'paused');
`
