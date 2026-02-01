package active

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
)

type CSWSHOriginTest struct {
	Origin             string
	OriginType         string // "same_origin", "attacker", "null", "missing", "subdomain"
	HandshakeSuccess   bool
	ResponseStatusCode int
	ResponseHeaders    map[string][]string
	CloseCode          int
	CloseReason        string
	MessagesSent       int
	MessagesReceived   int
	ReceivedData       []string
	ErrorMessage       string
	Duration           time.Duration
}

type CSWSHScanResult struct {
	Vulnerable       bool
	BaselineResult   CSWSHOriginTest
	CrossOriginTests []CSWSHOriginTest
	Confidence       int
	Details          string
	POC              string
}

type CSWSHScanOptions struct {
	options.WebSocketScanOptions
	AttackerDomains   []string
	TestNullOrigin    bool
	TestMissingOrigin bool
	TestSubdomains    bool
	MessageTimeout    time.Duration
	ConnectionTimeout time.Duration
}

type originToTest struct {
	Origin string
	Type   string
}

func copyHeaders(h http.Header) map[string][]string {
	result := make(map[string][]string)
	for k, v := range h {
		result[k] = append([]string{}, v...)
	}
	return result
}

func generateSubdomainVariations(host string) []string {
	if host == "" {
		return nil
	}
	return []string{
		"https://attacker." + host,          // subdomain prepend
		"https://" + host + ".attacker.com", // suffix append
		"https://attacker-" + host,          // prefix with hyphen
		"https://api." + host,               // common internal subdomain
		"https://staging." + host,           // staging environment
	}
}

func buildOriginsToTest(targetURL string, opts CSWSHScanOptions) []originToTest {
	var origins []originToTest

	sameOrigin := lib.ExtractOrigin(targetURL)
	if sameOrigin != "" {
		origins = append(origins, originToTest{Origin: sameOrigin, Type: "same_origin"})
	}

	attackerDomains := opts.AttackerDomains
	if len(attackerDomains) == 0 {
		attackerDomains = []string{"https://cswsh-test.attacker.invalid"}
	}
	for _, domain := range attackerDomains {
		origins = append(origins, originToTest{Origin: domain, Type: "attacker"})
	}

	if opts.TestNullOrigin {
		origins = append(origins, originToTest{Origin: "null", Type: "null"})
	}

	if opts.TestMissingOrigin {
		origins = append(origins, originToTest{Origin: "", Type: "missing"})
	}

	if opts.TestSubdomains {
		host, _ := lib.GetHostFromURL(targetURL)
		for _, variation := range generateSubdomainVariations(host) {
			origins = append(origins, originToTest{Origin: variation, Type: "subdomain"})
		}
	}

	return origins
}

func exchangeMessages(conn *websocket.Conn, messagesToSend []db.WebSocketMessage, timeout time.Duration) (sent int, received int, data []string) {
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	for _, msg := range messagesToSend {
		msgType := websocket.TextMessage
		if msg.IsBinary {
			msgType = websocket.BinaryMessage
		}
		if err := conn.WriteMessage(msgType, []byte(msg.PayloadData)); err != nil {
			break
		}
		sent++
	}

	deadline := time.Now().Add(timeout)
	conn.SetReadDeadline(deadline)

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}
		received++
		if len(data) < 5 {
			truncated := string(message)
			if len(truncated) > 200 {
				truncated = truncated[:200] + "..."
			}
			data = append(data, truncated)
		}
		if received >= 10 {
			break
		}
	}

	return sent, received, data
}

func testOrigin(
	ctx context.Context,
	targetURL string,
	origin string,
	originType string,
	originalHeaders map[string][]string,
	messagesToReplay []db.WebSocketMessage,
	opts CSWSHScanOptions,
) CSWSHOriginTest {
	result := CSWSHOriginTest{
		Origin:     origin,
		OriginType: originType,
	}

	timeout := opts.ConnectionTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: timeout,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
	}

	reqHeaders := http.Header{}

	for key, values := range originalHeaders {
		if !http_utils.IsWebSocketProtocolHeader(key) && !strings.EqualFold(key, "Origin") {
			for _, v := range values {
				reqHeaders.Add(key, v)
			}
		}
	}

	switch originType {
	case "missing":
	case "null":
		reqHeaders.Set("Origin", "null")
	default:
		if origin != "" {
			reqHeaders.Set("Origin", origin)
		}
	}

	start := time.Now()
	conn, resp, err := dialer.DialContext(ctx, targetURL, reqHeaders)
	result.Duration = time.Since(start)

	if err != nil {
		result.HandshakeSuccess = false
		result.ErrorMessage = err.Error()
		if resp != nil {
			result.ResponseStatusCode = resp.StatusCode
			result.ResponseHeaders = copyHeaders(resp.Header)
		}
		storeTestConnection(targetURL, reqHeaders, resp, nil, opts)
		return result
	}
	defer conn.Close()

	result.HandshakeSuccess = true
	result.ResponseStatusCode = resp.StatusCode
	result.ResponseHeaders = copyHeaders(resp.Header)

	var sentMessages []db.WebSocketMessage
	var receivedMessages []db.WebSocketMessage

	if len(messagesToReplay) > 0 {
		sent, received, data := exchangeMessagesWithTracking(conn, messagesToReplay, opts.MessageTimeout, &sentMessages, &receivedMessages)
		result.MessagesSent = sent
		result.MessagesReceived = received
		result.ReceivedData = data
	}

	storeTestConnection(targetURL, reqHeaders, resp, append(sentMessages, receivedMessages...), opts)

	return result
}

func storeTestConnection(targetURL string, reqHeaders http.Header, resp *http.Response, messages []db.WebSocketMessage, opts CSWSHScanOptions) {
	reqHeadersJSON, _ := json.Marshal(reqHeaders)
	var respHeadersJSON []byte
	var statusCode int
	var statusText string
	if resp != nil {
		respHeadersJSON, _ = json.Marshal(resp.Header)
		statusCode = resp.StatusCode
		statusText = resp.Status
	}

	wsConn := &db.WebSocketConnection{
		URL:             targetURL,
		RequestHeaders:  reqHeadersJSON,
		ResponseHeaders: respHeadersJSON,
		StatusCode:      statusCode,
		StatusText:      statusText,
		Source:          db.SourceScanner,
		ClosedAt:        time.Now(),
	}

	if opts.WorkspaceID > 0 {
		wsConn.WorkspaceID = &opts.WorkspaceID
	}
	if opts.ScanID > 0 {
		wsConn.ScanID = &opts.ScanID
	}
	if opts.ScanJobID > 0 {
		wsConn.ScanJobID = &opts.ScanJobID
	}
	if opts.TaskID > 0 {
		wsConn.TaskID = &opts.TaskID
	}

	if err := db.Connection().CreateWebSocketConnection(wsConn); err != nil {
		log.Error().Err(err).Str("url", targetURL).Msg("Failed to store CSWSH test connection")
		return
	}

	for i := range messages {
		messages[i].ConnectionID = wsConn.ID
		if err := db.Connection().CreateWebSocketMessage(&messages[i]); err != nil {
			log.Error().Err(err).Uint("connection_id", wsConn.ID).Msg("Failed to store CSWSH test message")
		}
	}
}

func exchangeMessagesWithTracking(conn *websocket.Conn, messagesToSend []db.WebSocketMessage, timeout time.Duration, sentMessages, receivedMessages *[]db.WebSocketMessage) (sent int, received int, data []string) {
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	for _, msg := range messagesToSend {
		msgType := websocket.TextMessage
		if msg.IsBinary {
			msgType = websocket.BinaryMessage
		}
		if err := conn.WriteMessage(msgType, []byte(msg.PayloadData)); err != nil {
			break
		}
		sent++
		*sentMessages = append(*sentMessages, db.WebSocketMessage{
			Opcode:      float64(msgType),
			PayloadData: msg.PayloadData,
			IsBinary:    msg.IsBinary,
			Timestamp:   time.Now(),
			Direction:   db.MessageSent,
		})
	}

	deadline := time.Now().Add(timeout)
	conn.SetReadDeadline(deadline)

	for {
		msgType, message, err := conn.ReadMessage()
		if err != nil {
			break
		}
		received++
		payloadData := string(message)
		isBinary := msgType == websocket.BinaryMessage

		*receivedMessages = append(*receivedMessages, db.WebSocketMessage{
			Opcode:      float64(msgType),
			PayloadData: payloadData,
			IsBinary:    isBinary,
			Timestamp:   time.Now(),
			Direction:   db.MessageReceived,
		})

		if len(data) < 5 {
			truncated := payloadData
			if len(truncated) > 200 {
				truncated = truncated[:200] + "..."
			}
			data = append(data, truncated)
		}
		if received >= 10 {
			break
		}
	}

	return sent, received, data
}

func analyzeResults(baseline CSWSHOriginTest, tests []CSWSHOriginTest) (vulnerable bool, confidence int, details string) {
	var sb strings.Builder

	if !baseline.HandshakeSuccess {
		sb.WriteString("BASELINE TEST FAILED\n")
		sb.WriteString("====================\n\n")
		sb.WriteString("Same-origin WebSocket connection failed. ")
		sb.WriteString("The endpoint may require specific conditions or authentication.\n\n")
		sb.WriteString(fmt.Sprintf("Error: %s\n", baseline.ErrorMessage))
		return false, 0, sb.String()
	}

	sb.WriteString("TEST RESULTS\n")
	sb.WriteString("============\n\n")

	sb.WriteString("Baseline (Same-Origin)\n")
	sb.WriteString("-----------------------\n")
	sb.WriteString(fmt.Sprintf("  Origin: %s\n", baseline.Origin))
	sb.WriteString(fmt.Sprintf("  Status: %d\n", baseline.ResponseStatusCode))
	if baseline.MessagesReceived > 0 {
		sb.WriteString(fmt.Sprintf("  Messages: %d sent, %d received\n", baseline.MessagesSent, baseline.MessagesReceived))
	}
	sb.WriteString("\n")

	sb.WriteString("Cross-Origin Tests\n")
	sb.WriteString("------------------\n\n")

	var acceptedOrigins []string
	messageExchangeConfirmed := false

	for _, test := range tests {
		if test.OriginType == "same_origin" {
			continue
		}

		originLabel := test.OriginType
		switch test.OriginType {
		case "attacker":
			originLabel = "Arbitrary attacker domain"
		case "null":
			originLabel = "Null origin"
		case "missing":
			originLabel = "Missing Origin header"
		case "subdomain":
			originLabel = "Subdomain variation"
		}

		sb.WriteString(fmt.Sprintf("%s\n", originLabel))
		sb.WriteString(fmt.Sprintf("  Origin: %s\n", test.Origin))

		if test.HandshakeSuccess {
			vulnerable = true
			sb.WriteString(fmt.Sprintf("  Result: ACCEPTED (Status %d)\n", test.ResponseStatusCode))
			if test.MessagesSent > 0 || test.MessagesReceived > 0 {
				sb.WriteString(fmt.Sprintf("  Messages: %d sent, %d received\n", test.MessagesSent, test.MessagesReceived))
				if test.MessagesReceived > 0 {
					messageExchangeConfirmed = true
				}
			}
			acceptedOrigins = append(acceptedOrigins, test.OriginType)

			typeConfidence := 0
			switch test.OriginType {
			case "attacker":
				typeConfidence = 90
			case "null":
				typeConfidence = 85
			case "missing":
				typeConfidence = 80
			case "subdomain":
				typeConfidence = 75
			}

			if typeConfidence > confidence {
				confidence = typeConfidence
			}

			if test.MessagesReceived > 0 && confidence < 100 {
				confidence += 5
			}
		} else {
			sb.WriteString(fmt.Sprintf("  Result: REJECTED (%s)\n", test.ErrorMessage))
		}
		sb.WriteString("\n")
	}

	if vulnerable {
		sb.WriteString("FINDING\n")
		sb.WriteString("=======\n\n")
		sb.WriteString(fmt.Sprintf("The WebSocket endpoint accepted connections from cross-origin sources: %s.\n\n",
			strings.Join(acceptedOrigins, ", ")))

		if messageExchangeConfirmed {
			sb.WriteString("The scanner successfully connected and exchanged messages from a cross-origin context, ")
			sb.WriteString("confirming the WebSocket connection is fully functional.\n\n")
		} else {
			sb.WriteString("The handshake was accepted but no messages were exchanged during testing.\n\n")
		}

		sb.WriteString("ATTACK SCENARIO\n")
		sb.WriteString("---------------\n\n")
		sb.WriteString("An attacker can host a malicious webpage that establishes a WebSocket connection to this endpoint. ")
		sb.WriteString("If the application relies on cookie-based session authentication, the victim's browser will ")
		sb.WriteString("automatically include session cookies with the cross-origin WebSocket request, allowing the ")
		sb.WriteString("attacker to:\n\n")
		sb.WriteString("  1. Read sensitive data transmitted over the WebSocket\n")
		sb.WriteString("  2. Send messages on behalf of the authenticated user\n")
		sb.WriteString("  3. Perform actions the user is authorized to do\n\n")

		sb.WriteString("VERIFICATION NOTES\n")
		sb.WriteString("------------------\n\n")
		sb.WriteString("This detection is based on the WebSocket handshake being accepted from untrusted origins. ")
		sb.WriteString("Manual verification is recommended to confirm:\n\n")
		sb.WriteString("  - The endpoint uses cookie-based authentication (not token in URL or message)\n")
		sb.WriteString("  - Sensitive or user-specific data is accessible through this WebSocket\n")
		sb.WriteString("  - Session cookies do not have SameSite=Strict attribute which would block the attack\n\n")

		sb.WriteString("If the WebSocket uses token-based authentication passed in the connection URL or initial ")
		sb.WriteString("message rather than cookies, practical exploitability may be limited as the attacker ")
		sb.WriteString("would need to obtain a valid token through other means.\n")
	}

	if confidence > 100 {
		confidence = 100
	}

	return vulnerable, confidence, sb.String()
}

func generateCSWSHPOC(targetURL string, messages []db.WebSocketMessage) string {
	var msgJS strings.Builder
	msgJS.WriteString("[")
	for i, msg := range messages {
		if i > 0 {
			msgJS.WriteString(",")
		}
		// Escape backticks and backslashes for JS template literal
		escaped := strings.ReplaceAll(msg.PayloadData, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "`", "\\`")
		escaped = strings.ReplaceAll(escaped, "${", "\\${")
		msgJS.WriteString("`" + escaped + "`")
	}
	msgJS.WriteString("]")

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>CSWSH PoC</title></head>
<body>
<h1>CSWSH Proof of Concept</h1>
<p>Target: %s</p>
<div id="log" style="font-family: monospace; background: #f0f0f0; padding: 10px;"></div>
<script>
const ws = new WebSocket("%s");
const messages = %s;

ws.onopen = () => {
    log("Connected from origin: " + location.origin);
    messages.forEach((msg, i) => {
        ws.send(msg);
        log("Sent: " + msg.substring(0, 100));
    });
};

ws.onmessage = (e) => log("Received: " + String(e.data).substring(0, 100));
ws.onerror = (e) => log("Error occurred");
ws.onclose = (e) => log("Closed with code: " + e.code);

function log(msg) {
    const div = document.getElementById("log");
    div.innerHTML += "[" + new Date().toISOString() + "] " + msg + "<br>";
}
</script>
</body>
</html>`, targetURL, targetURL, msgJS.String())
}

func reportCSWSHIssue(conn *db.WebSocketConnection, result *CSWSHScanResult, opts CSWSHScanOptions) {
	var workspaceID, taskID, taskJobID, scanID, scanJobID *uint

	if opts.WorkspaceID > 0 {
		workspaceID = &opts.WorkspaceID
	}
	if opts.TaskID > 0 {
		taskID = &opts.TaskID
	}
	if opts.TaskJobID > 0 {
		taskJobID = &opts.TaskJobID
	}
	if opts.ScanID > 0 {
		scanID = &opts.ScanID
	}
	if opts.ScanJobID > 0 {
		scanJobID = &opts.ScanJobID
	}

	_, err := db.CreateWebSocketIssue(db.WebSocketIssueOptions{
		Connection:  conn,
		Code:        db.WebsocketCswshCode,
		Details:     result.Details,
		Confidence:  result.Confidence,
		WorkspaceID: workspaceID,
		TaskID:      taskID,
		TaskJobID:   taskJobID,
		ScanID:      scanID,
		ScanJobID:   scanJobID,
		POC:         result.POC,
		POCType:     "html",
	})

	if err != nil {
		log.Error().Err(err).Uint("connection_id", conn.ID).Msg("Failed to create CSWSH issue")
	}
}

func ScanForCSWSH(
	conn *db.WebSocketConnection,
	opts CSWSHScanOptions,
	interactionsManager *integrations.InteractionsManager,
) (*CSWSHScanResult, error) {
	ctx := context.Background()

	taskLog := log.With().
		Uint("connection_id", conn.ID).
		Str("url", conn.URL).
		Str("scan", "cswsh").
		Logger()

	taskLog.Info().Msg("Starting CSWSH scan")

	originalHeaders, err := conn.GetRequestHeadersAsMap()
	if err != nil {
		originalHeaders = make(map[string][]string)
	}

	var messagesToReplay []db.WebSocketMessage
	if opts.ReplayMessages {
		for _, msg := range conn.Messages {
			if msg.Direction == db.MessageSent {
				messagesToReplay = append(messagesToReplay, msg)
			}
		}
	}

	origins := buildOriginsToTest(conn.URL, opts)
	taskLog.Info().Int("origins_to_test", len(origins)).Msg("Testing origins for CSWSH")

	var baseline CSWSHOriginTest
	var allTests []CSWSHOriginTest
	attackerAccepted := false

	for _, o := range origins {
		if attackerAccepted && o.Type != "same_origin" && o.Type != "attacker" {
			taskLog.Debug().
				Str("origin", o.Origin).
				Str("type", o.Type).
				Msg("Skipping test - arbitrary cross-origin already accepted")
			continue
		}

		taskLog.Debug().Str("origin", o.Origin).Str("type", o.Type).Msg("Testing origin")

		result := testOrigin(ctx, conn.URL, o.Origin, o.Type, originalHeaders, messagesToReplay, opts)
		allTests = append(allTests, result)

		if o.Type == "same_origin" {
			baseline = result
		}

		if result.HandshakeSuccess {
			taskLog.Info().
				Str("origin", o.Origin).
				Str("type", o.Type).
				Int("status", result.ResponseStatusCode).
				Bool("messages_received", result.MessagesReceived > 0).
				Msg("Origin accepted WebSocket connection")

			if o.Type == "attacker" {
				attackerAccepted = true
			}
		} else {
			taskLog.Debug().
				Str("origin", o.Origin).
				Str("type", o.Type).
				Str("error", result.ErrorMessage).
				Msg("Origin rejected")
		}
	}

	vulnerable, confidence, details := analyzeResults(baseline, allTests)
	poc := generateCSWSHPOC(conn.URL, messagesToReplay)

	result := &CSWSHScanResult{
		Vulnerable:       vulnerable,
		BaselineResult:   baseline,
		CrossOriginTests: allTests,
		Confidence:       confidence,
		Details:          details,
		POC:              poc,
	}

	if vulnerable {
		taskLog.Warn().
			Int("confidence", confidence).
			Msg("CSWSH vulnerability detected")
		reportCSWSHIssue(conn, result, opts)
	} else {
		taskLog.Info().Msg("No CSWSH vulnerability detected")
	}

	return result, nil
}
