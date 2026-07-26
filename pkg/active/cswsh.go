package active

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
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

// CSWSHVerdict grades a cross-origin acceptance result by exploitability. Cross-
// origin handshake acceptance is only a session-hijack (CSWSH) when the socket
// authenticates via ambient credentials the browser attaches automatically. When
// there is no such evidence it is a permissive-origin hardening gap, not an
// exploitable vulnerability; when the socket is authenticated by a non-ambient
// token an attacker cannot forge, it is not a finding at all.
type CSWSHVerdict string

const (
	CSWSHVerdictNone             CSWSHVerdict = "none"
	CSWSHVerdictPermissiveOrigin CSWSHVerdict = "permissive_origin"
	CSWSHVerdictVulnerable       CSWSHVerdict = "vulnerable"
)

type CSWSHScanResult struct {
	Vulnerable       bool
	Verdict          CSWSHVerdict
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

	// PermissiveOriginGate, when set, is consulted before raising the low-severity
	// permissive-origin finding; it receives the target host and returns true if
	// the finding should be raised. Callers use it to deduplicate the note to once
	// per host per scan. Nil means always raise. It never gates the high-severity
	// CSWSH finding, which is always reported when detected.
	PermissiveOriginGate func(host string) bool
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

	closedAt := time.Now()
	wsConn := &db.WebSocketConnection{
		URL:             targetURL,
		RequestHeaders:  reqHeadersJSON,
		ResponseHeaders: respHeadersJSON,
		StatusCode:      statusCode,
		StatusText:      statusText,
		Source:          db.SourceScanner,
		ClosedAt:        &closedAt,
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

// sessionCookieKeywords are substrings whose presence in a cookie NAME marks it
// as a likely session/authentication credential (as opposed to an incidental
// analytics/consent/load-balancer cookie such as _ga, cookieconsent, or AWSALB).
var sessionCookieKeywords = []string{"sess", "sid", "auth", "token", "jwt", "sso", "login", "credential", "identity"}

// looksLikeSessionCookie reports whether a cookie name suggests a session or auth
// credential. It is deliberately a heuristic: distinguishing an authenticated
// session from an incidental cookie is not possible with certainty, but this
// filters the common non-session cookies that would otherwise be misread as
// hijackable ambient auth.
//
// The name is tokenized on non-alphanumeric boundaries so a short keyword like
// "sid" is matched only as a whole token, not mid-word - otherwise the common
// Imperva/Incapsula "visid_incap_*" tracking cookie (which contains "sid") would
// be misread as a session. Longer keywords ("sess", "auth", ...) still match as
// substrings; the whole name is also treated as a token for delimiterless names
// like "phpsessid".
// nonSessionCookieSubstrings mark anti-CSRF / anti-forgery cookies (Django
// csrftoken, Angular XSRF-TOKEN, ASP.NET __RequestVerificationToken): they carry
// a session-like keyword but are set for anonymous visitors and can't hijack a
// session, so they must not count as ambient auth.
var nonSessionCookieSubstrings = []string{"csrf", "xsrf", "requestverificationtoken", "antiforgery"}

func looksLikeSessionCookie(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	for _, marker := range nonSessionCookieSubstrings {
		if strings.Contains(n, marker) {
			return false
		}
	}
	tokens := strings.FieldsFunc(n, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	tokens = append(tokens, n)
	for _, tok := range tokens {
		for _, kw := range sessionCookieKeywords {
			if len(kw) <= 3 {
				if tok == kw {
					return true
				}
			} else if strings.Contains(tok, kw) {
				return true
			}
		}
	}
	return false
}

// cookieNamesFromRequestHeaders extracts cookie names from a Cookie request
// header (name1=v1; name2=v2).
func cookieNamesFromRequestHeaders(headers map[string][]string) []string {
	var names []string
	for key, values := range headers {
		if !strings.EqualFold(key, "Cookie") {
			continue
		}
		for _, v := range values {
			for _, pair := range strings.Split(v, ";") {
				if eq := strings.Index(pair, "="); eq > 0 {
					names = append(names, strings.TrimSpace(pair[:eq]))
				}
			}
		}
	}
	return names
}

// cookieNamesFromResponseHeaders extracts cookie names from Set-Cookie response
// headers (name=value; attributes...).
func cookieNamesFromResponseHeaders(headers map[string][]string) []string {
	var names []string
	for key, values := range headers {
		if !strings.EqualFold(key, "Set-Cookie") {
			continue
		}
		for _, v := range values {
			seg := v
			if semi := strings.Index(seg, ";"); semi >= 0 {
				seg = seg[:semi]
			}
			if eq := strings.Index(seg, "="); eq > 0 {
				names = append(names, strings.TrimSpace(seg[:eq]))
			}
		}
	}
	return names
}

// connectionHasAmbientAuth reports whether the captured handshake shows evidence
// of ambient (cookie-based) credentials — the precondition for CSWSH to be
// exploitable. It requires a SESSION-looking cookie (by name) on the upgrade
// request or a Set-Cookie on the response; a merely-present incidental cookie
// (analytics, consent, load-balancer affinity) is not treated as ambient auth,
// since there is no session to hijack. Authorization and bearer tokens are
// excluded entirely: they are not auto-attached cross-origin.
func connectionHasAmbientAuth(conn *db.WebSocketConnection) bool {
	if conn == nil {
		return false
	}
	if reqHeaders, err := conn.GetRequestHeadersAsMap(); err == nil {
		for _, name := range cookieNamesFromRequestHeaders(reqHeaders) {
			if looksLikeSessionCookie(name) {
				return true
			}
		}
	}
	if respHeaders, err := conn.GetResponseHeadersAsMap(); err == nil {
		for _, name := range cookieNamesFromResponseHeaders(respHeaders) {
			if looksLikeSessionCookie(name) {
				return true
			}
		}
	}
	return false
}

// tokenQueryParamNames are URL query parameter names specific enough to indicate
// a non-ambient bearer token in the connection URL. Deliberately excludes generic
// names like "sid" (Engine.IO/Socket.IO transport id), "key", and "auth", which
// match benign parameters and would wrongly suppress genuine findings.
var tokenQueryParamNames = map[string]bool{
	"token":         true,
	"access_token":  true,
	"accesstoken":   true,
	"auth_token":    true,
	"authtoken":     true,
	"authorization": true,
	"jwt":           true,
	"api_key":       true,
	"apikey":        true,
	"api-key":       true,
	"sessionid":     true,
	"session_id":    true,
	"session_token": true,
	"sessiontoken":  true,
}

var jwtLikeRegex = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{5,}\.eyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}`)

// connectionUsesTokenAuth reports whether the connection authenticates via a
// non-ambient bearer token carried in the URL query or the first client frame.
// Such a token cannot be forged by a cross-origin attacker page, so acceptance of
// a cross-origin handshake is not exploitable and is suppressed entirely.
func connectionUsesTokenAuth(conn *db.WebSocketConnection) bool {
	if conn == nil {
		return false
	}
	if u, err := url.Parse(conn.URL); err == nil {
		for name, values := range u.Query() {
			if !tokenQueryParamNames[strings.ToLower(name)] {
				continue
			}
			for _, v := range values {
				if len(strings.TrimSpace(v)) >= 8 {
					return true
				}
			}
		}
	}
	for _, msg := range conn.Messages {
		if msg.Direction != db.MessageSent {
			continue
		}
		return jwtLikeRegex.MatchString(msg.PayloadData)
	}
	return false
}

// classifyCSWSH grades a cross-origin acceptance result. Ambient-auth evidence
// takes precedence (a hijackable cookie session is exploitable even when a token
// is also present); an unauthenticated permissive origin is a hardening gap; a
// token-authenticated socket is not a finding.
func classifyCSWSH(conn *db.WebSocketConnection, crossOriginAccepted bool) CSWSHVerdict {
	if !crossOriginAccepted {
		return CSWSHVerdictNone
	}
	if connectionHasAmbientAuth(conn) {
		return CSWSHVerdictVulnerable
	}
	if connectionUsesTokenAuth(conn) {
		return CSWSHVerdictNone
	}
	return CSWSHVerdictPermissiveOrigin
}

// acceptedCrossOrigins returns the browser-reachable cross-origin types whose
// handshake was accepted (attacker, null, subdomain). The missing-Origin arm is
// excluded because a real browser always sends an Origin header.
func acceptedCrossOrigins(tests []CSWSHOriginTest) []string {
	var accepted []string
	for _, test := range tests {
		if test.OriginType == "same_origin" || test.OriginType == "missing" {
			continue
		}
		if test.HandshakeSuccess {
			accepted = append(accepted, test.OriginType)
		}
	}
	return accepted
}

func analyzeResults(baseline CSWSHOriginTest, tests []CSWSHOriginTest) (crossOriginAccepted bool, confidence int, details string) {
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
			// A real browser always sends an Origin header, so acceptance of a
			// missing Origin is not reachable as a browser-driven attack. Record
			// it for completeness but do not treat it as cross-origin acceptance.
			if test.OriginType == "missing" {
				sb.WriteString("  Result: ACCEPTED (informational; not browser-reachable, ignored)\n\n")
				continue
			}

			crossOriginAccepted = true
			sb.WriteString(fmt.Sprintf("  Result: ACCEPTED (Status %d)\n", test.ResponseStatusCode))
			if test.MessagesSent > 0 || test.MessagesReceived > 0 {
				sb.WriteString(fmt.Sprintf("  Messages: %d sent, %d received\n", test.MessagesSent, test.MessagesReceived))
			}

			typeConfidence := 0
			switch test.OriginType {
			case "attacker":
				typeConfidence = 90
			case "null":
				typeConfidence = 85
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

	if confidence > 100 {
		confidence = 100
	}

	return crossOriginAccepted, confidence, sb.String()
}

// buildVulnerableConclusion returns the interpretive section for a confirmed
// CSWSH finding: the socket carries ambient credentials and accepts cross-origin
// handshakes, so an attacker page can hijack the authenticated session.
func buildVulnerableConclusion(acceptedOrigins []string) string {
	var sb strings.Builder
	sb.WriteString("\nFINDING\n")
	sb.WriteString("=======\n\n")
	sb.WriteString(fmt.Sprintf("The WebSocket endpoint accepted connections from cross-origin sources: %s.\n",
		strings.Join(acceptedOrigins, ", ")))
	sb.WriteString("The captured handshake carries ambient (cookie-based) credentials, so a victim's ")
	sb.WriteString("browser would attach the session automatically on a cross-origin connection.\n\n")

	sb.WriteString("ATTACK SCENARIO\n")
	sb.WriteString("---------------\n\n")
	sb.WriteString("An attacker hosts a malicious webpage that opens a WebSocket to this endpoint. ")
	sb.WriteString("The victim's browser auto-includes the session cookie, letting the attacker:\n\n")
	sb.WriteString("  1. Read sensitive data transmitted over the WebSocket\n")
	sb.WriteString("  2. Send messages on behalf of the authenticated user\n")
	sb.WriteString("  3. Perform actions the user is authorized to do\n\n")

	sb.WriteString("VERIFICATION NOTES\n")
	sb.WriteString("------------------\n\n")
	sb.WriteString("Manual verification is recommended to confirm:\n\n")
	sb.WriteString("  - Sensitive or user-specific data is accessible through this WebSocket\n")
	sb.WriteString("  - Session cookies do not have SameSite=Strict/Lax, which would block the attack\n")
	return sb.String()
}

// buildPermissiveConclusion returns the interpretive section for the low-severity
// permissive-origin note: cross-origin handshakes are accepted but no ambient
// credentials were observed, so it is a hardening gap rather than an exploitable
// hijack.
func buildPermissiveConclusion(acceptedOrigins []string) string {
	var sb strings.Builder
	sb.WriteString("\nFINDING\n")
	sb.WriteString("=======\n\n")
	sb.WriteString(fmt.Sprintf("The WebSocket endpoint accepted connections from cross-origin sources: %s.\n",
		strings.Join(acceptedOrigins, ", ")))
	sb.WriteString("No ambient (cookie-based) authentication was observed on the captured connection, so this ")
	sb.WriteString("is reported as a hardening gap rather than an exploitable session hijack. It becomes ")
	sb.WriteString("CSWSH-exploitable if the endpoint relies on cookie-based session authentication.\n\n")

	sb.WriteString("VERIFICATION NOTES\n")
	sb.WriteString("------------------\n\n")
	sb.WriteString("Confirm whether this socket carries or will carry session-authenticated, user-specific ")
	sb.WriteString("data over cookies; if so, treat it as CSWSH.\n")
	return sb.String()
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

func reportCSWSHIssue(conn *db.WebSocketConnection, result *CSWSHScanResult, opts CSWSHScanOptions, code db.IssueCode) {
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
		Code:        code,
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

	crossOriginAccepted, confidence, details := analyzeResults(baseline, allTests)
	poc := generateCSWSHPOC(conn.URL, messagesToReplay)

	verdict := classifyCSWSH(conn, crossOriginAccepted)
	acceptedOrigins := acceptedCrossOrigins(allTests)

	switch verdict {
	case CSWSHVerdictVulnerable:
		details += buildVulnerableConclusion(acceptedOrigins)
	case CSWSHVerdictPermissiveOrigin:
		details += buildPermissiveConclusion(acceptedOrigins)
	}

	result := &CSWSHScanResult{
		Vulnerable:       verdict == CSWSHVerdictVulnerable,
		Verdict:          verdict,
		BaselineResult:   baseline,
		CrossOriginTests: allTests,
		Confidence:       confidence,
		Details:          details,
		POC:              poc,
	}

	switch verdict {
	case CSWSHVerdictVulnerable:
		taskLog.Warn().Int("confidence", confidence).Msg("CSWSH vulnerability detected (ambient auth present)")
		reportCSWSHIssue(conn, result, opts, db.WebsocketCswshCode)
	case CSWSHVerdictPermissiveOrigin:
		host, _ := lib.GetHostFromURL(conn.URL)
		if opts.PermissiveOriginGate == nil || opts.PermissiveOriginGate(host) {
			taskLog.Info().Int("confidence", confidence).Str("host", host).Msg("Permissive WebSocket origin (no ambient auth) - reporting low-severity note")
			reportCSWSHIssue(conn, result, opts, db.WebsocketPermissiveOriginCode)
		} else {
			taskLog.Debug().Str("host", host).Msg("Permissive origin already reported for host, skipping duplicate note")
		}
	default:
		taskLog.Info().Msg("No CSWSH vulnerability detected")
	}

	return result, nil
}
