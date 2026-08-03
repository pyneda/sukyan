-- atlas:txmode none

-- Create index "idx_api_behavior_results_definition_id" to table: "api_behavior_results"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_api_behavior_results_definition_id" ON "api_behavior_results" ("definition_id");
-- Create index "idx_api_scan_endpoints_api_endpoint_id" to table: "api_scan_endpoints"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_api_scan_endpoints_api_endpoint_id" ON "api_scan_endpoints" ("api_endpoint_id");
-- Create index "idx_issue_requests_history_id" to table: "issue_requests"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_issue_requests_history_id" ON "issue_requests" ("history_id");
-- Create index "idx_json_web_token_histories_history_id" to table: "json_web_token_histories"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_json_web_token_histories_history_id" ON "json_web_token_histories" ("history_id");
-- Create index "idx_jwt_websocket_connections_connection_id" to table: "json_web_token_websocket_connections"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_jwt_websocket_connections_connection_id" ON "json_web_token_websocket_connections" ("web_socket_connection_id");
-- Create index "idx_json_web_tokens_workspace_id" to table: "json_web_tokens"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_json_web_tokens_workspace_id" ON "json_web_tokens" ("workspace_id");
-- Create index "idx_oob_interactions_issue_id" to table: "oob_interactions"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_oob_interactions_issue_id" ON "oob_interactions" ("issue_id");
-- Create index "idx_oob_interactions_oob_test_id" to table: "oob_interactions"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_oob_interactions_oob_test_id" ON "oob_interactions" ("oob_test_id");
-- Create index "idx_oob_interactions_workspace_id" to table: "oob_interactions"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_oob_interactions_workspace_id" ON "oob_interactions" ("workspace_id");
-- Create index "idx_oob_tests_history_id" to table: "oob_tests"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_oob_tests_history_id" ON "oob_tests" ("history_id");
-- Create index "idx_oob_tests_workspace_id" to table: "oob_tests"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_oob_tests_workspace_id" ON "oob_tests" ("workspace_id");
-- Create index "idx_playground_sessions_collection_id" to table: "playground_sessions"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_playground_sessions_collection_id" ON "playground_sessions" ("collection_id");
-- Create index "idx_playground_sessions_original_request_id" to table: "playground_sessions"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_playground_sessions_original_request_id" ON "playground_sessions" ("original_request_id");
-- Create index "idx_playground_ws_fuzz_iterations_web_socket_connection_id" to table: "playground_ws_fuzz_iterations"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_playground_ws_fuzz_iterations_web_socket_connection_id" ON "playground_ws_fuzz_iterations" ("web_socket_connection_id");
-- Create index "idx_scan_api_definitions_api_definition_id" to table: "scan_api_definitions"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_scan_api_definitions_api_definition_id" ON "scan_api_definitions" ("api_definition_id");
-- Create index "idx_task_jobs_history_id" to table: "task_jobs"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_task_jobs_history_id" ON "task_jobs" ("history_id");
-- Create index "idx_task_jobs_task_id" to table: "task_jobs"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_task_jobs_task_id" ON "task_jobs" ("task_id");
-- Create index "idx_web_socket_connections_workspace_id" to table: "web_socket_connections"
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_web_socket_connections_workspace_id" ON "web_socket_connections" ("workspace_id");
