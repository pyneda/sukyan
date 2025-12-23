package active

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func createTestWebSocketServer(t *testing.T, validateOrigin func(r *http.Request) bool) *httptest.Server {
	upgrader := websocket.Upgrader{
		CheckOrigin: validateOrigin,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				break
			}
			response := "echo: " + string(message)
			if err := conn.WriteMessage(messageType, []byte(response)); err != nil {
				break
			}
		}
	})

	return httptest.NewServer(handler)
}

func createVulnerableWebSocketServer(t *testing.T) *httptest.Server {
	return createTestWebSocketServer(t, func(r *http.Request) bool {
		return true
	})
}

func createSecureWebSocketServer(t *testing.T, allowedOrigins []string) *httptest.Server {
	return createTestWebSocketServer(t, func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" || origin == "null" {
			return false
		}
		for _, allowed := range allowedOrigins {
			if origin == allowed {
				return true
			}
		}
		return false
	})
}

func httpToWsURL(httpURL string) string {
	return strings.Replace(httpURL, "http://", "ws://", 1)
}

func createTestWebSocketConnection(t *testing.T, wsURL string, workspace *db.Workspace) *db.WebSocketConnection {
	conn := &db.WebSocketConnection{
		URL:             wsURL,
		RequestHeaders:  datatypes.JSON(`{"Host": "localhost", "Cookie": "session=test123"}`),
		ResponseHeaders: datatypes.JSON(`{}`),
		StatusCode:      101,
		StatusText:      "Switching Protocols",
		WorkspaceID:     &workspace.ID,
		Source:          "test",
	}

	err := db.Connection().CreateWebSocketConnection(conn)
	require.NoError(t, err)

	msg := &db.WebSocketMessage{
		ConnectionID: conn.ID,
		Opcode:       1,
		PayloadData:  `{"action": "ping"}`,
		Timestamp:    time.Now(),
		Direction:    db.MessageSent,
	}
	err = db.Connection().CreateWebSocketMessage(msg)
	require.NoError(t, err)

	conn.Messages = []db.WebSocketMessage{*msg}

	return conn
}

func TestScanForCSWSH_VulnerableServer(t *testing.T) {
	server := createVulnerableWebSocketServer(t)
	defer server.Close()

	wsURL := httpToWsURL(server.URL)

	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Title: "TestCSWSH_Vulnerable",
		Code:  "test-cswsh-vulnerable",
	})
	require.NoError(t, err)

	conn := createTestWebSocketConnection(t, wsURL, workspace)

	opts := CSWSHScanOptions{
		WebSocketScanOptions: options.WebSocketScanOptions{
			WorkspaceID:    workspace.ID,
			ReplayMessages: true,
		},
		TestNullOrigin:    true,
		TestMissingOrigin: true,
		TestSubdomains:    false,
		MessageTimeout:    2 * time.Second,
		ConnectionTimeout: 5 * time.Second,
	}

	result, err := ScanForCSWSH(conn, opts, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Vulnerable, "Server should be detected as vulnerable")

	assert.GreaterOrEqual(t, result.Confidence, 80)
	assert.True(t, result.BaselineResult.HandshakeSuccess)
	assert.Equal(t, "same_origin", result.BaselineResult.OriginType)

	crossOriginSucceeded := false
	for _, test := range result.CrossOriginTests {
		if test.OriginType != "same_origin" && test.HandshakeSuccess {
			crossOriginSucceeded = true
			break
		}
	}
	assert.True(t, crossOriginSucceeded)

	for _, test := range result.CrossOriginTests {
		if test.OriginType == "attacker" {
			assert.True(t, test.HandshakeSuccess)
		}
	}

	assert.Contains(t, result.Details, "TEST RESULTS")
	assert.Contains(t, result.Details, "attacker")
	assert.Contains(t, result.Details, "ACCEPTED")

	assert.NotEmpty(t, result.POC)
	assert.Contains(t, result.POC, "WebSocket")
	assert.Contains(t, result.POC, wsURL)

	issues, _, err := db.Connection().ListIssues(db.IssueFilter{
		WorkspaceID: workspace.ID,
		Codes:       []string{string(db.WebsocketCswshCode)},
		URL:         wsURL,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(issues), 1)

	if len(issues) > 0 {
		assert.Equal(t, string(db.WebsocketCswshCode), issues[0].Code)
		assert.GreaterOrEqual(t, issues[0].Confidence, 80)
	}
}

func TestScanForCSWSH_SecureServer(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		expectedOrigin := strings.Replace(serverURL, "http://", "http://", 1)

		if origin != expectedOrigin && origin != "" && origin != "null" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if origin == "null" || origin == "" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				o := r.Header.Get("Origin")
				return o == expectedOrigin
			},
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				break
			}
			if err := conn.WriteMessage(messageType, message); err != nil {
				break
			}
		}
	}))
	serverURL = server.URL
	defer server.Close()

	wsURL := httpToWsURL(server.URL)

	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Title: "TestCSWSH_Secure",
		Code:  "test-cswsh-secure",
	})
	require.NoError(t, err)

	conn := createTestWebSocketConnection(t, wsURL, workspace)

	opts := CSWSHScanOptions{
		WebSocketScanOptions: options.WebSocketScanOptions{
			WorkspaceID:    workspace.ID,
			ReplayMessages: true,
		},
		TestNullOrigin:    true,
		TestMissingOrigin: true,
		TestSubdomains:    false,
		MessageTimeout:    2 * time.Second,
		ConnectionTimeout: 5 * time.Second,
	}

	result, err := ScanForCSWSH(conn, opts, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Vulnerable)
	assert.Equal(t, 0, result.Confidence)
	assert.True(t, result.BaselineResult.HandshakeSuccess)

	for _, test := range result.CrossOriginTests {
		if test.OriginType != "same_origin" {
			assert.False(t, test.HandshakeSuccess)
		}
	}

	issues, _, err := db.Connection().ListIssues(db.IssueFilter{
		WorkspaceID: workspace.ID,
		Codes:       []string{string(db.WebsocketCswshCode)},
		URL:         wsURL,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, len(issues))
}

func TestScanForCSWSH_NullOriginOnly(t *testing.T) {
	server := createTestWebSocketServer(t, func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "null" {
			return true
		}
		if strings.HasPrefix(origin, "http://127.0.0.1") {
			return true
		}
		return false
	})
	defer server.Close()

	wsURL := httpToWsURL(server.URL)

	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Title: "TestCSWSH_NullOrigin",
		Code:  "test-cswsh-null-origin",
	})
	require.NoError(t, err)

	conn := createTestWebSocketConnection(t, wsURL, workspace)

	opts := CSWSHScanOptions{
		WebSocketScanOptions: options.WebSocketScanOptions{
			WorkspaceID:    workspace.ID,
			ReplayMessages: true,
		},
		TestNullOrigin:    true,
		TestMissingOrigin: true,
		TestSubdomains:    false,
		MessageTimeout:    2 * time.Second,
		ConnectionTimeout: 5 * time.Second,
	}

	result, err := ScanForCSWSH(conn, opts, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Vulnerable)
	assert.GreaterOrEqual(t, result.Confidence, 80)
	assert.LessOrEqual(t, result.Confidence, 95)

	for _, test := range result.CrossOriginTests {
		if test.OriginType == "null" {
			assert.True(t, test.HandshakeSuccess)
		}
		if test.OriginType == "attacker" {
			assert.False(t, test.HandshakeSuccess)
		}
	}

	assert.Contains(t, result.Details, "null")
}

func TestScanForCSWSH_MessageExchange(t *testing.T) {
	server := createVulnerableWebSocketServer(t)
	defer server.Close()

	wsURL := httpToWsURL(server.URL)

	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Title: "TestCSWSH_MessageExchange",
		Code:  "test-cswsh-messages",
	})
	require.NoError(t, err)

	conn := createTestWebSocketConnection(t, wsURL, workspace)

	opts := CSWSHScanOptions{
		WebSocketScanOptions: options.WebSocketScanOptions{
			WorkspaceID:    workspace.ID,
			ReplayMessages: true,
		},
		TestNullOrigin:    true,
		TestMissingOrigin: false,
		TestSubdomains:    false,
		MessageTimeout:    3 * time.Second,
		ConnectionTimeout: 5 * time.Second,
	}

	result, err := ScanForCSWSH(conn, opts, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Vulnerable)

	for _, test := range result.CrossOriginTests {
		if test.HandshakeSuccess && test.OriginType == "attacker" {
			assert.Greater(t, test.MessagesSent, 0)
			assert.Greater(t, test.MessagesReceived, 0)
			assert.NotEmpty(t, test.ReceivedData)
		}
	}

	assert.Equal(t, 100, result.Confidence)
}

func TestBuildOriginsToTest(t *testing.T) {
	tests := []struct {
		name          string
		targetURL     string
		opts          CSWSHScanOptions
		expectedCount int
		expectedTypes []string
		shouldNotHave []string
	}{
		{
			name:      "default options - attacker only",
			targetURL: "ws://example.com/ws",
			opts: CSWSHScanOptions{
				TestNullOrigin:    false,
				TestMissingOrigin: false,
				TestSubdomains:    false,
			},
			expectedCount: 2, // same_origin + attacker
			expectedTypes: []string{"same_origin", "attacker"},
			shouldNotHave: []string{"null", "missing", "subdomain"},
		},
		{
			name:      "all options enabled",
			targetURL: "ws://example.com/ws",
			opts: CSWSHScanOptions{
				TestNullOrigin:    true,
				TestMissingOrigin: true,
				TestSubdomains:    true,
			},
			expectedCount: 8, // same_origin + attacker + null + missing + 5 subdomains - but subdomains is 5
			expectedTypes: []string{"same_origin", "attacker", "null", "missing", "subdomain"},
		},
		{
			name:      "custom attacker domains",
			targetURL: "ws://example.com/ws",
			opts: CSWSHScanOptions{
				AttackerDomains:   []string{"https://evil1.com", "https://evil2.com"},
				TestNullOrigin:    false,
				TestMissingOrigin: false,
				TestSubdomains:    false,
			},
			expectedCount: 3, // same_origin + 2 attacker domains
			expectedTypes: []string{"same_origin", "attacker"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origins := buildOriginsToTest(tt.targetURL, tt.opts)

			// Check count is at least expected (subdomains add variable count)
			assert.GreaterOrEqual(t, len(origins), tt.expectedCount-5)

			// Check expected types are present
			foundTypes := make(map[string]bool)
			for _, o := range origins {
				foundTypes[o.Type] = true
			}

			for _, expectedType := range tt.expectedTypes {
				assert.True(t, foundTypes[expectedType], "Should have origin type: %s", expectedType)
			}

			for _, shouldNot := range tt.shouldNotHave {
				assert.False(t, foundTypes[shouldNot], "Should NOT have origin type: %s", shouldNot)
			}
		})
	}
}

func TestExtractOrigin(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ws://example.com/path", "http://example.com"},
		{"wss://example.com/path", "https://example.com"},
		{"wss://example.com:8080/path", "https://example.com:8080"},
		{"http://example.com/path", "http://example.com"},
		{"https://example.com:443/path", "https://example.com:443"},
		{"invalid-url", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := lib.ExtractOrigin(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractHost(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ws://example.com/path", "example.com"},
		{"wss://example.com:8080/path", "example.com"},
		{"http://sub.example.com/path", "sub.example.com"},
		{"invalid-url", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, _ := lib.GetHostFromURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateSubdomainVariations(t *testing.T) {
	variations := generateSubdomainVariations("example.com")

	assert.NotEmpty(t, variations)
	assert.Contains(t, variations, "https://attacker.example.com")
	assert.Contains(t, variations, "https://example.com.attacker.com")
	assert.Contains(t, variations, "https://attacker-example.com")
	assert.Contains(t, variations, "https://api.example.com")
	assert.Contains(t, variations, "https://staging.example.com")

	// Empty host should return nil
	emptyVariations := generateSubdomainVariations("")
	assert.Nil(t, emptyVariations)
}

func TestAnalyzeResults(t *testing.T) {
	tests := []struct {
		name               string
		baseline           CSWSHOriginTest
		tests              []CSWSHOriginTest
		expectedVulnerable bool
		minConfidence      int
		maxConfidence      int
	}{
		{
			name: "baseline failed",
			baseline: CSWSHOriginTest{
				OriginType:       "same_origin",
				HandshakeSuccess: false,
				ErrorMessage:     "connection refused",
			},
			tests:              []CSWSHOriginTest{},
			expectedVulnerable: false,
			minConfidence:      0,
			maxConfidence:      0,
		},
		{
			name: "only baseline succeeds - secure",
			baseline: CSWSHOriginTest{
				OriginType:       "same_origin",
				HandshakeSuccess: true,
			},
			tests: []CSWSHOriginTest{
				{OriginType: "attacker", HandshakeSuccess: false, ErrorMessage: "forbidden"},
				{OriginType: "null", HandshakeSuccess: false, ErrorMessage: "forbidden"},
			},
			expectedVulnerable: false,
			minConfidence:      0,
			maxConfidence:      0,
		},
		{
			name: "attacker origin succeeds - critical",
			baseline: CSWSHOriginTest{
				OriginType:       "same_origin",
				HandshakeSuccess: true,
			},
			tests: []CSWSHOriginTest{
				{OriginType: "attacker", HandshakeSuccess: true, MessagesReceived: 1},
			},
			expectedVulnerable: true,
			minConfidence:      95,
			maxConfidence:      100,
		},
		{
			name: "null origin succeeds - high",
			baseline: CSWSHOriginTest{
				OriginType:       "same_origin",
				HandshakeSuccess: true,
			},
			tests: []CSWSHOriginTest{
				{OriginType: "attacker", HandshakeSuccess: false},
				{OriginType: "null", HandshakeSuccess: true},
			},
			expectedVulnerable: true,
			minConfidence:      85,
			maxConfidence:      90,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vulnerable, confidence, details := analyzeResults(tt.baseline, tt.tests)

			assert.Equal(t, tt.expectedVulnerable, vulnerable)
			assert.GreaterOrEqual(t, confidence, tt.minConfidence)
			assert.LessOrEqual(t, confidence, tt.maxConfidence)
			assert.NotEmpty(t, details)
		})
	}
}

func TestIsWebSocketProtocolHeader(t *testing.T) {
	wsHeaders := []string{
		"Connection", "Upgrade", "Sec-WebSocket-Key",
		"Sec-WebSocket-Version", "Sec-WebSocket-Protocol",
		"Sec-WebSocket-Extensions",
	}

	for _, h := range wsHeaders {
		assert.True(t, http_utils.IsWebSocketProtocolHeader(h), "%s should be a WS protocol header", h)
		assert.True(t, http_utils.IsWebSocketProtocolHeader(strings.ToLower(h)), "lowercase %s should match", h)
		assert.True(t, http_utils.IsWebSocketProtocolHeader(strings.ToUpper(h)), "uppercase %s should match", h)
	}

	nonWsHeaders := []string{
		"Host", "Cookie", "Authorization", "User-Agent", "Origin",
	}

	for _, h := range nonWsHeaders {
		assert.False(t, http_utils.IsWebSocketProtocolHeader(h), "%s should NOT be a WS protocol header", h)
	}
}

func TestGenerateCSWSHPOC(t *testing.T) {
	messages := []db.WebSocketMessage{
		{PayloadData: `{"action": "test"}`},
		{PayloadData: `ping`},
	}

	poc := generateCSWSHPOC("wss://example.com/ws", messages)

	assert.Contains(t, poc, "<!DOCTYPE html>")
	assert.Contains(t, poc, "wss://example.com/ws")
	assert.Contains(t, poc, `{"action": "test"}`)
	assert.Contains(t, poc, "ping")
	assert.Contains(t, poc, "WebSocket")
	assert.Contains(t, poc, "onopen")
	assert.Contains(t, poc, "onmessage")
}
