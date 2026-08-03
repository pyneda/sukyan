package workspace

// ArchiveFormatVersion is written into the manifest and checked on import. Bump
// it whenever the on-disk record shape changes in a way older readers cannot
// handle.
const ArchiveFormatVersion = 1

// tableSpec describes how one table is exported and re-inserted.
type tableSpec struct {
	// Name is the physical table name; it doubles as the record tag in the
	// archive stream.
	Name string
	// Query selects the rows belonging to a workspace. It takes the workspace
	// ID as its single parameter and must return whole rows of Name.
	Query string
	// IDColumns are bigint columns holding row identifiers, both this table's
	// own primary key and its foreign keys. Import shifts every one of them by
	// the same offset, which keeps all references internally consistent
	// without a per-row mapping table.
	IDColumns []string
	// UUIDPrimaryKey is the uuid column holding this table's own identity, if
	// it has one. Import always mints a fresh value for it.
	UUIDPrimaryKey string
	// UUIDReferences are uuid columns pointing at another table's identity.
	// Import rewrites them through the uuid map; a value that was never
	// imported resolves to NULL, which is what makes skipped tables safe.
	UUIDReferences []string
	// DeferredColumns are set aside on first insert and patched in a second
	// pass. They exist only to break foreign key cycles.
	DeferredColumns []string
	// SkipImport marks a table that is exported for completeness but cannot be
	// recreated in another deployment.
	SkipImport bool
	// SkipReason explains SkipImport in the manifest.
	SkipReason string
}

// orderedTables lists every table reachable from a workspace, in an order that
// satisfies the foreign key graph once DeferredColumns are excluded. The order
// and the deferred set are asserted against the live schema by
// TestTableOrderSatisfiesLiveForeignKeyGraph, so a schema change breaks a test
// rather than corrupting an import.
//
// histories and web_socket_connections are deliberately placed after everything
// they reference so that the two largest tables need no second-pass patching.
var orderedTables = []tableSpec{
	{
		Name:      "workspaces",
		Query:     `SELECT * FROM workspaces WHERE id = $1`,
		IDColumns: []string{"id"},
	},
	{
		Name:           "api_auth_configs",
		Query:          `SELECT * FROM api_auth_configs WHERE workspace_id = $1`,
		IDColumns:      []string{"workspace_id"},
		UUIDPrimaryKey: "id",
	},
	{
		Name:      "json_web_tokens",
		Query:     `SELECT * FROM json_web_tokens WHERE workspace_id = $1`,
		IDColumns: []string{"id", "workspace_id"},
	},
	{
		Name:      "matcher_presets",
		Query:     `SELECT * FROM matcher_presets WHERE workspace_id = $1`,
		IDColumns: []string{"id", "workspace_id"},
	},
	{
		Name:      "playground_collections",
		Query:     `SELECT * FROM playground_collections WHERE workspace_id = $1`,
		IDColumns: []string{"id", "workspace_id"},
	},
	{
		Name:           "proxy_services",
		Query:          `SELECT * FROM proxy_services WHERE workspace_id = $1`,
		IDColumns:      []string{"workspace_id"},
		UUIDPrimaryKey: "id",
		SkipImport:     true,
		SkipReason:     "port carries a globally unique index, so a proxy service belongs to the deployment that created it",
	},
	{
		Name:      "scans",
		Query:     `SELECT * FROM scans WHERE workspace_id = $1`,
		IDColumns: []string{"id", "workspace_id"},
	},
	{
		Name:      "stored_browser_actions",
		Query:     `SELECT * FROM stored_browser_actions WHERE workspace_id = $1`,
		IDColumns: []string{"id", "workspace_id"},
	},
	{
		Name:           "workspace_cookies",
		Query:          `SELECT * FROM workspace_cookies WHERE workspace_id = $1`,
		IDColumns:      []string{"workspace_id"},
		UUIDPrimaryKey: "id",
	},
	{
		Name: "api_auth_headers",
		Query: `SELECT h.* FROM api_auth_headers h
			JOIN api_auth_configs c ON c.id = h.auth_config_id WHERE c.workspace_id = $1`,
		UUIDPrimaryKey: "id",
		UUIDReferences: []string{"auth_config_id"},
	},
	{
		Name:            "api_definitions",
		Query:           `SELECT * FROM api_definitions WHERE workspace_id = $1`,
		IDColumns:       []string{"scan_id", "source_history_id", "workspace_id"},
		UUIDPrimaryKey:  "id",
		UUIDReferences:  []string{"auth_config_id"},
		DeferredColumns: []string{"source_history_id"},
	},
	{
		Name:            "playground_sessions",
		Query:           `SELECT * FROM playground_sessions WHERE workspace_id = $1`,
		IDColumns:       []string{"collection_id", "id", "original_request_id", "workspace_id"},
		DeferredColumns: []string{"original_request_id"},
	},
	{
		Name: "token_refresh_configs",
		Query: `SELECT t.* FROM token_refresh_configs t
			JOIN api_auth_configs c ON c.id = t.auth_config_id WHERE c.workspace_id = $1`,
		UUIDPrimaryKey: "id",
		UUIDReferences: []string{"auth_config_id"},
	},
	{
		Name: "api_definition_security_schemes",
		Query: `SELECT s.* FROM api_definition_security_schemes s
			JOIN api_definitions d ON d.id = s.definition_id WHERE d.workspace_id = $1`,
		UUIDPrimaryKey: "id",
		UUIDReferences: []string{"definition_id"},
	},
	{
		Name: "api_endpoints",
		Query: `SELECT e.* FROM api_endpoints e
			JOIN api_definitions d ON d.id = e.definition_id WHERE d.workspace_id = $1`,
		UUIDPrimaryKey: "id",
		UUIDReferences: []string{"definition_id"},
	},
	{
		Name: "api_scans",
		Query: `SELECT a.* FROM api_scans a
			JOIN scans s ON s.id = a.scan_id WHERE s.workspace_id = $1`,
		IDColumns:      []string{"scan_id"},
		UUIDPrimaryKey: "id",
		UUIDReferences: []string{"definition_id"},
	},
	{
		Name:      "playground_fuzz_runs",
		Query:     `SELECT * FROM playground_fuzz_runs WHERE workspace_id = $1`,
		IDColumns: []string{"id", "playground_session_id", "workspace_id"},
	},
	{
		Name: "playground_ws_fuzz_runs",
		Query: `SELECT r.* FROM playground_ws_fuzz_runs r
			JOIN playground_sessions s ON s.id = r.session_id WHERE s.workspace_id = $1`,
		IDColumns: []string{"id", "session_id"},
	},
	{
		Name: "scan_api_definitions",
		Query: `SELECT l.* FROM scan_api_definitions l
			JOIN scans s ON s.id = l.scan_id WHERE s.workspace_id = $1`,
		IDColumns:      []string{"scan_id"},
		UUIDReferences: []string{"api_definition_id"},
	},
	{
		Name:      "tasks",
		Query:     `SELECT * FROM tasks WHERE workspace_id = $1`,
		IDColumns: []string{"id", "playground_session_id", "workspace_id"},
	},
	{
		Name: "api_scan_endpoints",
		Query: `SELECT e.* FROM api_scan_endpoints e
			JOIN api_scans a ON a.id = e.api_scan_id
			JOIN scans s ON s.id = a.scan_id WHERE s.workspace_id = $1`,
		UUIDReferences: []string{"api_scan_id", "api_endpoint_id"},
	},
	{
		Name:            "scan_jobs",
		Query:           `SELECT * FROM scan_jobs WHERE workspace_id = $1`,
		IDColumns:       []string{"history_id", "id", "scan_id", "web_socket_connection_id", "workspace_id"},
		UUIDReferences:  []string{"api_definition_id", "api_endpoint_id"},
		DeferredColumns: []string{"history_id", "web_socket_connection_id"},
	},
	{
		Name:           "api_behavior_results",
		Query:          `SELECT * FROM api_behavior_results WHERE workspace_id = $1`,
		IDColumns:      []string{"scan_id", "scan_job_id", "workspace_id"},
		UUIDPrimaryKey: "id",
		UUIDReferences: []string{"definition_id"},
	},
	{
		Name:           "histories",
		Query:          `SELECT * FROM histories WHERE workspace_id = $1`,
		IDColumns:      []string{"id", "playground_fuzz_run_id", "playground_session_id", "scan_id", "scan_job_id", "task_id", "workspace_id"},
		UUIDReferences: []string{"api_definition_id", "api_endpoint_id", "proxy_service_id"},
	},
	{
		Name:           "browser_events",
		Query:          `SELECT * FROM browser_events WHERE workspace_id = $1`,
		IDColumns:      []string{"history_id", "scan_id", "scan_job_id", "task_id", "workspace_id"},
		UUIDPrimaryKey: "id",
	},
	{
		Name: "json_web_token_histories",
		Query: `SELECT l.* FROM json_web_token_histories l
			JOIN json_web_tokens t ON t.id = l.json_web_token_id WHERE t.workspace_id = $1`,
		IDColumns: []string{"history_id", "json_web_token_id"},
	},
	{
		Name:           "site_behavior_results",
		Query:          `SELECT * FROM site_behavior_results WHERE workspace_id = $1`,
		IDColumns:      []string{"base_url_sample_id", "scan_id", "scan_job_id", "workspace_id"},
		UUIDPrimaryKey: "id",
	},
	{
		Name:           "web_socket_connections",
		Query:          `SELECT * FROM web_socket_connections WHERE workspace_id = $1`,
		IDColumns:      []string{"id", "playground_session_id", "scan_id", "scan_job_id", "task_id", "upgrade_request_id", "workspace_id"},
		UUIDReferences: []string{"proxy_service_id"},
	},
	{
		Name: "json_web_token_websocket_connections",
		Query: `SELECT l.* FROM json_web_token_websocket_connections l
			JOIN json_web_tokens t ON t.id = l.json_web_token_id WHERE t.workspace_id = $1`,
		IDColumns: []string{"json_web_token_id", "web_socket_connection_id"},
	},
	{
		Name: "playground_ws_fuzz_iterations",
		Query: `SELECT i.* FROM playground_ws_fuzz_iterations i
			JOIN playground_ws_fuzz_runs r ON r.id = i.run_id
			JOIN playground_sessions s ON s.id = r.session_id WHERE s.workspace_id = $1`,
		IDColumns: []string{"id", "run_id", "web_socket_connection_id"},
	},
	{
		Name: "playground_ws_sessions",
		Query: `SELECT w.* FROM playground_ws_sessions w
			JOIN playground_sessions s ON s.id = w.playground_session_id WHERE s.workspace_id = $1`,
		IDColumns: []string{"id", "imported_from_connection_id", "playground_session_id"},
	},
	{
		Name: "site_behavior_not_found_samples",
		Query: `SELECT n.* FROM site_behavior_not_found_samples n
			JOIN site_behavior_results r ON r.id = n.site_behavior_result_id WHERE r.workspace_id = $1`,
		IDColumns:      []string{"history_id", "id"},
		UUIDReferences: []string{"site_behavior_result_id"},
	},
	{
		Name: "task_jobs",
		Query: `SELECT j.* FROM task_jobs j
			JOIN tasks t ON t.id = j.task_id WHERE t.workspace_id = $1`,
		IDColumns: []string{"history_id", "id", "task_id", "websocket_connection_id"},
	},
	{
		Name: "web_socket_messages",
		Query: `SELECT m.* FROM web_socket_messages m
			JOIN web_socket_connections c ON c.id = m.connection_id WHERE c.workspace_id = $1`,
		IDColumns: []string{"connection_id", "id"},
	},
	{
		Name:           "issues",
		Query:          `SELECT * FROM issues WHERE workspace_id = $1`,
		IDColumns:      []string{"id", "scan_id", "scan_job_id", "task_id", "task_job_id", "websocket_connection_id", "workspace_id"},
		UUIDReferences: []string{"api_definition_id", "api_endpoint_id"},
	},
	{
		Name: "playground_ws_runs",
		Query: `SELECT r.* FROM playground_ws_runs r
			JOIN playground_ws_sessions w ON w.id = r.playground_ws_session_id
			JOIN playground_sessions s ON s.id = w.playground_session_id WHERE s.workspace_id = $1`,
		IDColumns: []string{"id", "playground_ws_session_id", "web_socket_connection_id"},
	},
	{
		Name: "issue_requests",
		Query: `SELECT r.* FROM issue_requests r
			JOIN issues i ON i.id = r.issue_id WHERE i.workspace_id = $1`,
		IDColumns: []string{"history_id", "issue_id"},
	},
	{
		Name:      "oob_tests",
		Query:     `SELECT * FROM oob_tests WHERE workspace_id = $1`,
		IDColumns: []string{"history_id", "id", "issue_id", "scan_id", "scan_job_id", "task_id", "task_job_id", "workspace_id"},
	},
	{
		Name:      "oob_interactions",
		Query:     `SELECT * FROM oob_interactions WHERE workspace_id = $1`,
		IDColumns: []string{"id", "issue_id", "oob_test_id", "workspace_id"},
	},
}

func tableByName(name string) (tableSpec, bool) {
	for _, spec := range orderedTables {
		if spec.Name == name {
			return spec, true
		}
	}
	return tableSpec{}, false
}
