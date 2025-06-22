package passive

import (
	"strings"
	"testing"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"gorm.io/datatypes"
)

func TestScanWebSocketMessage(t *testing.T) {
	// Create test workspace
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Title: "TestScanWebSocketMessage",
		Code:  "TestScanWebSocketMessage",
	})
	assert.NoError(t, err)

	// Create test task
	task, err := db.Connection().NewTask(workspace.ID, nil, "Test WebSocket Passive Scan", "running", db.TaskTypeScan)
	assert.NoError(t, err)

	// Create a test HTTP upgrade request
	upgradeRequest := &db.History{
		URL:         "wss://example.com/websocket",
		Method:      "GET",
		StatusCode:  101,
		RawRequest:  []byte("GET /websocket HTTP/1.1\r\nHost: example.com\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"),
		RawResponse: []byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"),
		WorkspaceID: &workspace.ID,
		TaskID:      &task.ID,
		Source:      "test",
	}
	upgradeRequest, err = db.Connection().CreateHistory(upgradeRequest)
	assert.NoError(t, err)

	// Create test WebSocket connection with test data
	connection := &db.WebSocketConnection{
		URL:              "wss://example.com/websocket",
		RequestHeaders:   datatypes.JSON(`{"Authorization": "Bearer test-token", "Host": "example.com"}`),
		ResponseHeaders:  datatypes.JSON(`{"Content-Type": "application/json"}`),
		StatusCode:       101,
		StatusText:       "Switching Protocols",
		WorkspaceID:      &workspace.ID,
		TaskID:           &task.ID,
		Source:           "test",
		UpgradeRequestID: &upgradeRequest.ID,
	}
	err = db.Connection().CreateWebSocketConnection(connection)
	assert.NoError(t, err)

	tests := []struct {
		name               string
		payloadData        string
		expectedIssueCount int
		expectedIssueCodes []db.IssueCode
		description        string
	}{
		{
			name:               "Empty payload",
			payloadData:        "",
			expectedIssueCount: 0,
			description:        "Should return no issues for empty payload",
		},
		{
			name:               "Normal message",
			payloadData:        `{"message": "hello world"}`,
			expectedIssueCount: 0,
			description:        "Should return no issues for normal JSON message",
		},
		{
			name:               "Database error in payload",
			payloadData:        `{"error": "ORA-00942: table or view does not exist"}`,
			expectedIssueCount: 1,
			expectedIssueCodes: []db.IssueCode{db.DatabaseErrorsCode},
			description:        "Should detect Oracle database error",
		},
		{
			name:               "JWT token in payload",
			payloadData:        `{"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"}`,
			expectedIssueCount: 3,
			expectedIssueCodes: []db.IssueCode{db.JwtDetectedCode, db.JwtWeakSigningSecretCode, db.SessionTokenInUrlCode},
			description:        "Should detect JWT token in message",
		},
		{
			name:               "Private IP in payload",
			payloadData:        `{"server": "192.168.1.100", "message": "Internal server communication"}`,
			expectedIssueCount: 1,
			expectedIssueCodes: []db.IssueCode{db.PrivateIpsCode},
			description:        "Should detect private IP address",
		},
		{
			name:               "API key in payload",
			payloadData:        `{"config": {"aws_key": "AKIAIOSFODNN7EXAMPLE"}}`,
			expectedIssueCount: 1,
			expectedIssueCodes: []db.IssueCode{db.ExposedApiCredentialsCode},
			description:        "Should detect AWS API key",
		},
		{
			name:               "Email address in payload",
			payloadData:        `{"contact": "admin@example.com", "message": "Please contact support"}`,
			expectedIssueCount: 1,
			expectedIssueCodes: []db.IssueCode{db.EmailAddressesCode},
			description:        "Should detect email address",
		},
		{
			name:               "Private key in payload",
			payloadData:        `{"key": "-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAKqVkA==\n-----END RSA PRIVATE KEY-----"}`,
			expectedIssueCount: 1,
			expectedIssueCodes: []db.IssueCode{db.PrivateKeysCode},
			description:        "Should detect RSA private key",
		},
		{
			name:               "Database connection string in payload",
			payloadData:        `{"db_config": "mongodb://user:password@localhost:27017/mydb"}`,
			expectedIssueCount: 1,
			expectedIssueCodes: []db.IssueCode{db.DbConnectionStringsCode},
			description:        "Should detect MongoDB connection string",
		},
		{
			name:               "Storage bucket in payload",
			payloadData:        `{"url": "https://my-bucket.s3.amazonaws.com/file.txt"}`,
			expectedIssueCount: 1,
			expectedIssueCodes: []db.IssueCode{db.StorageBucketDetectedCode},
			description:        "Should detect S3 bucket URL",
		},
		{
			name:               "Session token in payload",
			payloadData:        `{"session_token": "abc123def456ghi789jkl012"}`,
			expectedIssueCount: 1,
			expectedIssueCodes: []db.IssueCode{db.SessionTokenInUrlCode},
			description:        "Should detect session token",
		},
		{
			name:               "Multiple issues in one message",
			payloadData:        `{"email": "test@example.com", "server": "10.0.0.1", "jwt": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"}`,
			expectedIssueCount: 5,
			expectedIssueCodes: []db.IssueCode{
				db.EmailAddressesCode,
				db.PrivateIpsCode,
				db.JwtDetectedCode,
				db.JwtWeakSigningSecretCode,
				db.SessionTokenInUrlCode,
			},
			description: "Should detect multiple different issues in single message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test message
			message := &db.WebSocketMessage{
				ConnectionID: connection.ID,
				Opcode:       1,
				Mask:         false,
				PayloadData:  tt.payloadData,
				Timestamp:    time.Now(),
				Direction:    db.MessageReceived,
			}
			err := db.Connection().CreateWebSocketMessage(message)
			assert.NoError(t, err)

			// Run the scan
			issues := ScanWebSocketMessage(message, connection)

			// Verify results
			assert.Equal(t, tt.expectedIssueCount, len(issues), tt.description)

			if tt.expectedIssueCount > 0 {
				// Check that all expected issue codes are present
				foundCodes := make(map[db.IssueCode]bool)
				for _, issue := range issues {
					foundCodes[db.IssueCode(issue.Code)] = true
				}

				for _, expectedCode := range tt.expectedIssueCodes {
					assert.True(t, foundCodes[expectedCode], "Expected issue code %s not found", expectedCode)
				}

				// Verify issue fields are properly set
				for _, issue := range issues {
					assert.Equal(t, connection.URL, issue.URL)
					assert.Equal(t, workspace.ID, *issue.WorkspaceID)
					assert.Equal(t, task.ID, *issue.TaskID)
					assert.Equal(t, connection.ID, *issue.WebsocketConnectionID)
					assert.NotEmpty(t, issue.Details)
					assert.NotEmpty(t, issue.Title)
					assert.Greater(t, issue.Confidence, 0)
				}
			}
		})
	}
}

func TestScanWebSocketMessageEdgeCases(t *testing.T) {
	// Create test workspace
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Title: "TestScanWebSocketMessageEdgeCases",
		Code:  "TestScanWebSocketMessageEdgeCases",
	})
	assert.NoError(t, err)

	// Create test task
	task, err := db.Connection().NewTask(workspace.ID, nil, "Test WebSocket Edge Cases", "running", db.TaskTypeScan)
	assert.NoError(t, err)

	// Create test WebSocket connection
	connection := &db.WebSocketConnection{
		URL:             "ws://test.example.com/ws",
		RequestHeaders:  datatypes.JSON(`{}`),
		ResponseHeaders: datatypes.JSON(`{}`),
		StatusCode:      101,
		StatusText:      "Switching Protocols",
		WorkspaceID:     &workspace.ID,
		TaskID:          &task.ID,
		Source:          "test",
	}
	err = db.Connection().CreateWebSocketConnection(connection)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		payloadData string
		description string
	}{
		{
			name:        "Very large payload",
			payloadData: `{"data": "` + strings.Repeat("A", 10000) + `"}`,
			description: "Should handle large payloads without crashing",
		},
		{
			name:        "Special characters",
			payloadData: `{"message": "Testing special chars: \n\t\r\\ \"quotes\" 'apostrophes' éñ中文"}`,
			description: "Should handle special characters correctly",
		},
		{
			name:        "Invalid JSON",
			payloadData: `{"incomplete": "json"`,
			description: "Should handle malformed JSON gracefully",
		},
		{
			name:        "Binary-like data",
			payloadData: `{"data": "\u0001\u0002\u0003\ufffd\ufffe\ufffd"}`,
			description: "Should handle binary-like data without issues",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := &db.WebSocketMessage{
				ConnectionID: connection.ID,
				Opcode:       1,
				Mask:         false,
				PayloadData:  tt.payloadData,
				Timestamp:    time.Now(),
				Direction:    db.MessageReceived,
			}
			err := db.Connection().CreateWebSocketMessage(message)
			assert.NoError(t, err)

			// Should not panic or error
			assert.NotPanics(t, func() {
				issues := ScanWebSocketMessage(message, connection)
				// Issues may or may not be found, but function should complete
				assert.GreaterOrEqual(t, len(issues), 0)
			}, tt.description)
		})
	}
}
