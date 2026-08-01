package active

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	defaultConnectionTimeout    = 10 * time.Second
	defaultRevalidationAttempts = 3
)

// smugglingIndicators are phrases that indicate a method-related error
var smugglingIndicators = []string{
	"invalid method",
	"not implemented",
	"bad request",
	"unknown method",
	"unsupported method",
	"method not allowed",
	"unrecognized method",
}

type RequestSmugglingAudit struct {
	Options              ActiveModuleOptions
	HistoryItem          *db.History
	ConnectionTimeout    time.Duration
	RevalidationAttempts int

	client *SmugglingClient
}

func (a *RequestSmugglingAudit) Run() {
	ctx := a.Options.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return
	default:
	}

	a.applyDefaults()
	a.client = NewSmugglingClient(a.ConnectionTimeout, http_utils.HistoryCreationOptions{
		Source:      db.SourceScanner,
		WorkspaceID: a.Options.WorkspaceID,
		TaskID:      a.Options.TaskID,
		TaskJobID:   a.Options.TaskJobID,
		ScanID:      a.Options.ScanID,
		ScanJobID:   a.Options.ScanJobID,
	})

	auditLog := log.With().
		Str("audit", "request-smuggling").
		Str("url", a.HistoryItem.URL).
		Uint("workspace", a.Options.WorkspaceID).
		Logger()

	auditLog.Info().Msg("Starting HTTP request smuggling audit")

	// Test all smuggling types using response-based detection
	a.testCLTE(ctx, auditLog)
	a.testTECL(ctx, auditLog)
	a.testTETE(ctx, auditLog)
	a.testCL0(ctx, auditLog)

	auditLog.Info().Msg("Completed HTTP request smuggling audit")
}

func (a *RequestSmugglingAudit) applyDefaults() {
	if a.ConnectionTimeout == 0 {
		a.ConnectionTimeout = defaultConnectionTimeout
	}
	if a.RevalidationAttempts == 0 {
		a.RevalidationAttempts = defaultRevalidationAttempts
	}
}

// detectSmugglingIndicators checks if the response contains evidence of a smuggled request being processed
func (a *RequestSmugglingAudit) detectSmugglingIndicators(response []byte, payload payloads.SmugglingPayload) (bool, string) {
	if len(response) == 0 {
		return false, ""
	}

	responseStr := string(response)
	responseLower := strings.ToLower(responseStr)
	statusCode := http_utils.ParseStatusCodeFromRawResponse(response)

	// Check for our specific markers in the response
	if payload.MethodMarker != "" && strings.Contains(responseStr, payload.MethodMarker) {
		return true, fmt.Sprintf("Method marker '%s' found in response", payload.MethodMarker)
	}
	if payload.PathMarker != "" && strings.Contains(responseStr, payload.PathMarker) {
		return true, fmt.Sprintf("Path marker '%s' found in response", payload.PathMarker)
	}

	// Check for method-related error status codes with indicator phrases
	if statusCode == 400 || statusCode == 405 || statusCode == 501 {
		for _, indicator := range smugglingIndicators {
			if strings.Contains(responseLower, indicator) {
				return true, fmt.Sprintf("Method error indicator '%s' found with status %d", indicator, statusCode)
			}
		}
	}

	return false, ""
}

// testCLTE tests for CL.TE request smuggling using response-based detection
func (a *RequestSmugglingAudit) testCLTE(ctx context.Context, auditLog zerolog.Logger) bool {
	select {
	case <-ctx.Done():
		return false
	default:
	}

	uc, err := lib.ParseURLComponents(a.HistoryItem.URL)
	if err != nil {
		auditLog.Debug().Err(err).Msg("Failed to parse URL for CL.TE test")
		return false
	}

	payload := payloads.GetCLTEPayload(uc.Host, uc.Path)
	followUp := payloads.BuildFollowUpRequest(uc.Host, uc.Path)

	resp, err := a.client.SendRawPipelined(ctx, uc.Host, uc.Port, uc.UseTLS, payload.RawRequest, followUp, "")
	if err != nil {
		auditLog.Debug().Err(err).Msg("CL.TE pipelined request failed")
		return false
	}

	// Check the second response for smuggling indicators
	found, reason := a.detectSmugglingIndicators(resp.SecondResponse, payload)
	if found {
		auditLog.Info().Str("reason", reason).Msg("Potential CL.TE detected, starting revalidation")

		vulnerable, confidence, details, revalidationHistories := a.revalidateResponseBased(
			ctx, auditLog, &uc, payloads.GetCLTEPayload, payload.Type)

		if vulnerable {
			a.reportIssue(resp.History, db.HttpRequestSmugglingClTeCode, payload, confidence, details, revalidationHistories)
			return true
		}
	}

	return false
}

// testTECL tests for TE.CL request smuggling using response-based detection
func (a *RequestSmugglingAudit) testTECL(ctx context.Context, auditLog zerolog.Logger) bool {
	select {
	case <-ctx.Done():
		return false
	default:
	}

	uc, err := lib.ParseURLComponents(a.HistoryItem.URL)
	if err != nil {
		auditLog.Debug().Err(err).Msg("Failed to parse URL for TE.CL test")
		return false
	}

	payload := payloads.GetTECLPayload(uc.Host, uc.Path)
	followUp := payloads.BuildFollowUpRequest(uc.Host, uc.Path)

	resp, err := a.client.SendRawPipelined(ctx, uc.Host, uc.Port, uc.UseTLS, payload.RawRequest, followUp, "")
	if err != nil {
		auditLog.Debug().Err(err).Msg("TE.CL pipelined request failed")
		return false
	}

	// Check the second response for smuggling indicators
	found, reason := a.detectSmugglingIndicators(resp.SecondResponse, payload)
	if found {
		auditLog.Info().Str("reason", reason).Msg("Potential TE.CL detected, starting revalidation")

		vulnerable, confidence, details, revalidationHistories := a.revalidateResponseBased(
			ctx, auditLog, &uc, payloads.GetTECLPayload, payload.Type)

		if vulnerable {
			a.reportIssue(resp.History, db.HttpRequestSmugglingTeClCode, payload, confidence, details, revalidationHistories)
			return true
		}
	}

	return false
}

// testTETE tests for TE.TE request smuggling with obfuscated Transfer-Encoding headers
func (a *RequestSmugglingAudit) testTETE(ctx context.Context, auditLog zerolog.Logger) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	uc, err := lib.ParseURLComponents(a.HistoryItem.URL)
	if err != nil {
		auditLog.Debug().Err(err).Msg("Failed to parse URL for TE.TE test")
		return
	}

	// Get payloads based on scan mode
	var payloadsToTest []payloads.SmugglingPayload
	if a.Options.ScanMode == scan_options.ScanModeFuzz {
		payloadsToTest = payloads.GetAllTETEPayloads(uc.Host, uc.Path)
	} else {
		payloadsToTest = payloads.GetTETEPayloads(uc.Host, uc.Path)
	}

	auditLog.Debug().Int("variants", len(payloadsToTest)).Msg("Testing TE.TE obfuscation variants")

	followUp := payloads.BuildFollowUpRequest(uc.Host, uc.Path)

	for _, payload := range payloadsToTest {
		select {
		case <-ctx.Done():
			return
		default:
		}

		resp, err := a.client.SendRawPipelined(ctx, uc.Host, uc.Port, uc.UseTLS, payload.RawRequest, followUp, "")
		if err != nil {
			continue
		}

		found, reason := a.detectSmugglingIndicators(resp.SecondResponse, payload)
		if found {
			auditLog.Info().
				Str("obfuscation", payload.TEObfuscation).
				Str("reason", reason).
				Msg("Potential TE.TE detected, starting revalidation")

			// Create a generator function for this specific obfuscation
			obf := payload.TEObfuscation
			generator := func(host, path string) payloads.SmugglingPayload {
				for _, o := range payloads.TEObfuscations {
					if o.Name == obf {
						return payloads.GetTETEPayload(host, path, o)
					}
				}
				return payloads.GetTETEPayload(host, path, payloads.EffectiveTEObfuscations[0])
			}

			vulnerable, confidence, details, revalidationHistories := a.revalidateResponseBased(
				ctx, auditLog, &uc, generator, payload.Type)

			if vulnerable {
				a.reportIssue(resp.History, db.HttpRequestSmugglingTeTeCode, payload, confidence, details, revalidationHistories)
				return
			}
		}
	}
}

// testCL0 tests for CL.0 request smuggling where the backend ignores Content-Length
func (a *RequestSmugglingAudit) testCL0(ctx context.Context, auditLog zerolog.Logger) bool {
	select {
	case <-ctx.Done():
		return false
	default:
	}

	uc, err := lib.ParseURLComponents(a.HistoryItem.URL)
	if err != nil {
		auditLog.Debug().Err(err).Msg("Failed to parse URL for CL.0 test")
		return false
	}

	payload := payloads.GetCL0Payload(uc.Host, uc.Path)
	followUp := payloads.BuildFollowUpRequest(uc.Host, uc.Path)

	resp, err := a.client.SendRawPipelined(ctx, uc.Host, uc.Port, uc.UseTLS, payload.RawRequest, followUp, payload.PathMarker)
	if err != nil {
		auditLog.Debug().Err(err).Msg("CL.0 pipelined request failed")
		return false
	}

	if resp.MarkerFound && !resp.MarkerInSecondResponse {
		auditLog.Debug().Str("location", resp.MarkerLocation).
			Msg("Marker echoed by the origin rather than smuggled, not a CL.0 desync")
		return false
	}

	if resp.MarkerInSecondResponse {
		auditLog.Info().Str("location", resp.MarkerLocation).Msg("Potential CL.0 detected, starting revalidation")

		vulnerable, confidence, details, revalidationHistories := a.revalidateResponseBased(
			ctx, auditLog, &uc, payloads.GetCL0Payload, payload.Type)

		if vulnerable {
			a.reportIssue(resp.History, db.HttpRequestSmugglingCl0Code, payload, confidence, details, revalidationHistories)
			return true
		}
	}

	return false
}

// payloadGenerator is a function type that generates a fresh smuggling payload
type payloadGenerator func(host, path string) payloads.SmugglingPayload

// revalidateResponseBased performs multiple attempts to confirm a smuggling vulnerability
func (a *RequestSmugglingAudit) revalidateResponseBased(
	ctx context.Context,
	auditLog zerolog.Logger,
	uc *lib.URLComponents,
	generator payloadGenerator,
	smugglingType payloads.SmugglingType,
) (bool, int, string, []*db.History) {
	var sb strings.Builder
	var revalidationHistories []*db.History
	successCount := 0

	sb.WriteString(fmt.Sprintf("Smuggling type: %s\n", smugglingType.String()))
	sb.WriteString("Detection method: Response-based marker detection\n\n")
	sb.WriteString(fmt.Sprintf("Revalidation performed with %d attempts:\n", a.RevalidationAttempts))

	followUp := payloads.BuildFollowUpRequest(uc.Host, uc.Path)

	for i := 1; i <= a.RevalidationAttempts; i++ {
		select {
		case <-ctx.Done():
			return false, 0, "Cancelled", revalidationHistories
		default:
		}

		if i > 1 {
			time.Sleep(500 * time.Millisecond)
		}

		// Generate fresh payload with new markers for each attempt
		freshPayload := generator(uc.Host, uc.Path)

		sb.WriteString(fmt.Sprintf("\n  Attempt %d:\n", i))
		if freshPayload.MethodMarker != "" {
			sb.WriteString(fmt.Sprintf("    Method marker: %s\n", freshPayload.MethodMarker))
		}
		if freshPayload.PathMarker != "" {
			sb.WriteString(fmt.Sprintf("    Path marker: %s\n", freshPayload.PathMarker))
		}

		// Determine which marker to use for SendRawPipelined
		marker := freshPayload.PathMarker
		if marker == "" {
			marker = freshPayload.MethodMarker
		}

		resp, err := a.client.SendRawPipelined(ctx, uc.Host, uc.Port, uc.UseTLS, freshPayload.RawRequest, followUp, marker)
		if resp != nil && resp.History != nil {
			revalidationHistories = append(revalidationHistories, resp.History)
		}

		if err != nil {
			sb.WriteString(fmt.Sprintf("    Result: Error - %v\n", err))
			continue
		}

		// Check for indicators in second response
		found, reason := a.detectSmugglingIndicators(resp.SecondResponse, freshPayload)

		// Also check if marker was found via SendRawPipelined's marker check. Only the
		// follow-up response counts: an echo in the first response repeats identically
		// on every attempt, so accepting it here turns revalidation into confirmation
		// of the scanner's own reflection.
		if !found && resp.MarkerInSecondResponse {
			found = true
			reason = fmt.Sprintf("Marker found in %s", resp.MarkerLocation)
		}

		if found {
			successCount++
			sb.WriteString(fmt.Sprintf("    Result: Confirmed - %s\n", reason))
		} else {
			sb.WriteString("    Result: Not detected\n")
		}
	}

	// Calculate confidence based on success rate
	// 3/3 = 95%, 2/3 = 85%, 1/3 = don't report
	var confidence int
	vulnerable := false

	if successCount >= a.RevalidationAttempts {
		confidence = 95
		vulnerable = true
	} else if successCount >= (a.RevalidationAttempts+1)/2 { // Majority (2/3 for 3 attempts)
		confidence = 85
		vulnerable = true
	}

	sb.WriteString(fmt.Sprintf("\nSummary: %d/%d attempts confirmed smuggling\n", successCount, a.RevalidationAttempts))

	auditLog.Debug().
		Int("success_count", successCount).
		Int("total_attempts", a.RevalidationAttempts).
		Int("confidence", confidence).
		Bool("vulnerable", vulnerable).
		Msg("Response-based revalidation complete")

	return vulnerable, confidence, sb.String(), revalidationHistories
}

func (a *RequestSmugglingAudit) reportIssue(
	history *db.History,
	code db.IssueCode,
	payload payloads.SmugglingPayload,
	confidence int,
	revalidationDetails string,
	revalidationHistories []*db.History,
) {
	var sb strings.Builder
	sb.WriteString("Detection method: Response-based marker detection\n\n")
	sb.WriteString("The scanner confirmed HTTP request smuggling by injecting a request with a unique ")
	sb.WriteString("invalid method marker into the connection buffer. When a follow-up request was sent ")
	sb.WriteString("on the same connection, the server returned an error indicating it processed the ")
	sb.WriteString("smuggled request, proving the frontend and backend disagree on request boundaries.\n\n")
	sb.WriteString(fmt.Sprintf("Payload description: %s\n", payload.Description))
	if payload.MethodMarker != "" {
		sb.WriteString(fmt.Sprintf("Method marker used: %s\n", payload.MethodMarker))
	}
	if payload.PathMarker != "" {
		sb.WriteString(fmt.Sprintf("Path marker used: %s\n", payload.PathMarker))
	}
	if payload.TEObfuscation != "" {
		sb.WriteString(fmt.Sprintf("TE obfuscation technique: %s\n", payload.TEObfuscation))
	}
	sb.WriteString("\nVerification details:\n")
	sb.WriteString(revalidationDetails)

	issue, err := db.CreateIssueFromHistoryAndTemplate(
		history,
		code,
		sb.String(),
		confidence,
		"",
		&a.Options.WorkspaceID,
		&a.Options.TaskID,
		&a.Options.TaskJobID,
		&a.Options.ScanID,
		&a.Options.ScanJobID,
	)

	if err != nil {
		log.Error().Err(err).Msg("Failed to create request smuggling issue")
		return
	}

	if err := issue.AppendHistories(revalidationHistories); err != nil {
		log.Warn().Err(err).Uint("issue_id", issue.ID).Int("history_count", len(revalidationHistories)).
			Msg("Failed to link revalidation histories to issue")
	}
}

// SmugglingClient handles raw TCP connections for smuggling detection
type SmugglingClient struct {
	timeout        time.Duration
	historyOptions http_utils.HistoryCreationOptions
}

func NewSmugglingClient(timeout time.Duration, historyOptions http_utils.HistoryCreationOptions) *SmugglingClient {
	return &SmugglingClient{
		timeout:        timeout,
		historyOptions: historyOptions,
	}
}

// SmugglingPipelinedResponse holds the result of a pipelined smuggling test
type SmugglingPipelinedResponse struct {
	FirstResponse  []byte
	SecondResponse []byte
	Duration       time.Duration
	MarkerFound    bool
	MarkerLocation string
	// MarkerInSecondResponse is the only marker signal that evidences a desync.
	// The marker appearing in the first response means the origin echoed the request
	// body back to us, which any body-echoing endpoint does and which leaves the
	// connection perfectly healthy. MarkerFound stays true there for diagnostics.
	MarkerInSecondResponse bool
	History                *db.History
}

func (c *SmugglingClient) createHistory(host string, port int, useTLS bool, rawRequest, rawResponse []byte) (*db.History, error) {
	scheme := "http"
	if useTLS {
		scheme = "https"
	}

	method, path, _ := lib.ParseRequestLine(rawRequest)

	var urlStr string
	if (useTLS && port == 443) || (!useTLS && port == 80) {
		urlStr = fmt.Sprintf("%s://%s%s", scheme, host, path)
	} else {
		urlStr = fmt.Sprintf("%s://%s:%d%s", scheme, host, port, path)
	}

	statusCode := 0
	if len(rawResponse) > 0 {
		statusCode = http_utils.ParseStatusCodeFromRawResponse(rawResponse)
	}

	record := &db.History{
		URL:         urlStr,
		Depth:       lib.CalculateURLDepth(urlStr),
		StatusCode:  statusCode,
		Method:      method,
		RawRequest:  rawRequest,
		RawResponse: rawResponse,
		Source:      db.SourceScanner,
		WorkspaceID: &c.historyOptions.WorkspaceID,
		TaskID:      lib.PtrIfNonZero(c.historyOptions.TaskID),
		ScanID:      lib.PtrIfNonZero(c.historyOptions.ScanID),
		ScanJobID:   lib.PtrIfNonZero(c.historyOptions.ScanJobID),
	}

	return db.Connection().CreateHistory(record)
}

// SendRawPipelined sends a smuggling payload followed by a follow-up request on the same
// connection. This is the core detection mechanism for all smuggling types.
func (c *SmugglingClient) SendRawPipelined(ctx context.Context, host string, port int, useTLS bool, smugglingPayload, followupRequest []byte, marker string) (*SmugglingPipelinedResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	address := fmt.Sprintf("%s:%d", host, port)

	dialer := &net.Dialer{
		Timeout: c.timeout,
	}

	var conn net.Conn
	var err error

	start := time.Now()

	if useTLS {
		var tcpConn net.Conn
		tcpConn, err = dialer.DialContext(ctx, "tcp", address)
		if err == nil {
			tlsConfig := &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         host,
			}
			tlsConn := tls.Client(tcpConn, tlsConfig)
			tlsConn.SetDeadline(time.Now().Add(c.timeout))
			if err = tlsConn.Handshake(); err != nil {
				tcpConn.Close()
			} else {
				conn = tlsConn
			}
		}
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}

	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

	// Set deadline for the entire pipelined exchange
	conn.SetDeadline(time.Now().Add(c.timeout * 2))

	// One reader for both responses: bytes belonging to the second response may
	// already have arrived in the same segment as the first, and must not be
	// discarded between reads.
	reader := bufio.NewReader(conn)

	// Step 1: Send the smuggling payload
	_, err = conn.Write(smugglingPayload)
	if err != nil {
		return nil, fmt.Errorf("write smuggling payload failed: %w", err)
	}

	// Step 2: Read first response
	firstResponse, err := readRawHTTPResponse(reader)
	if err != nil && len(firstResponse) == 0 {
		return nil, fmt.Errorf("read first response failed: %w", err)
	}

	// Step 3: Send follow-up request on the same connection
	_, err = conn.Write(followupRequest)
	if err != nil {
		// Connection closed, return what we have
		elapsed := time.Since(start)
		history, _ := c.createHistory(host, port, useTLS, smugglingPayload, firstResponse)
		return &SmugglingPipelinedResponse{
			FirstResponse: firstResponse,
			Duration:      elapsed,
			History:       history,
		}, nil
	}

	// Step 4: Read second response. A desynced origin may answer late, oddly, or
	// not at all, so a partial read is normal here and is kept as evidence.
	secondResponse, _ := readRawHTTPResponse(reader)

	elapsed := time.Since(start)

	// Check for marker in responses
	markerFound := false
	markerLocation := ""
	markerInSecond := false

	if marker != "" {
		if strings.Contains(string(firstResponse), marker) {
			markerFound = true
			markerLocation = "first response"
		}
		if strings.Contains(string(secondResponse), marker) {
			markerFound = true
			markerInSecond = true
			if markerLocation != "" {
				markerLocation = "both responses"
			} else {
				markerLocation = "second response"
			}
		}
	}

	// Combine requests/responses for history
	combinedRequest := append(smugglingPayload, followupRequest...)
	combinedResponse := append(firstResponse, secondResponse...)
	history, _ := c.createHistory(host, port, useTLS, combinedRequest, combinedResponse)

	return &SmugglingPipelinedResponse{
		FirstResponse:          firstResponse,
		SecondResponse:         secondResponse,
		Duration:               elapsed,
		MarkerFound:            markerFound,
		MarkerLocation:         markerLocation,
		MarkerInSecondResponse: markerInSecond,
		History:                history,
	}, nil
}

// maxSmugglingResponseBytes caps how much of a single response body is buffered,
// so a hostile or desynced origin cannot exhaust memory.
const maxSmugglingResponseBytes = 1 << 20 // 1 MiB

// readRawHTTPResponse reads exactly one complete HTTP/1.1 response message and
// returns its raw bytes.
//
// A single conn.Read() is not usable here. TCP carries no message boundaries, so
// a response split across segments leaks its tail into the following read, and
// two coalesced responses arrive as one. Either case silently breaks pipelined
// smuggling detection, where the evidence lives in the *second* response.
//
// Interim 1xx responses are consumed and discarded so the final response is what
// callers see. Whatever was read is returned alongside any error, since a
// truncated exchange is still evidence.
func readRawHTTPResponse(br *bufio.Reader) ([]byte, error) {
	var raw bytes.Buffer

	for {
		raw.Reset() // an interim response is not the message the caller wants

		statusLine, err := br.ReadString('\n')
		if err != nil {
			raw.WriteString(statusLine)
			return raw.Bytes(), err
		}
		raw.WriteString(statusLine)
		status := parseResponseStatusLine(statusLine)

		var contentLength int64 = -1
		chunked := false

		for {
			line, err := br.ReadString('\n')
			if err != nil {
				raw.WriteString(line)
				return raw.Bytes(), err
			}
			raw.WriteString(line)
			trimmed := strings.TrimRight(line, "\r\n")
			if trimmed == "" {
				break
			}
			idx := strings.Index(trimmed, ":")
			if idx <= 0 {
				continue
			}
			name := strings.ToLower(strings.TrimSpace(trimmed[:idx]))
			value := strings.TrimSpace(trimmed[idx+1:])
			switch name {
			case "content-length":
				if n, convErr := strconv.ParseInt(value, 10, 64); convErr == nil && n >= 0 {
					contentLength = n
				}
			case "transfer-encoding":
				if strings.Contains(strings.ToLower(value), "chunked") {
					chunked = true
				}
			}
		}

		// 1xx has no body and is followed by the real response on the same stream.
		if status >= 100 && status < 200 {
			continue
		}

		switch {
		case chunked:
			if err := copyChunkedBody(br, &raw); err != nil {
				return raw.Bytes(), err
			}
		case contentLength > 0:
			if contentLength > maxSmugglingResponseBytes {
				contentLength = maxSmugglingResponseBytes
			}
			if _, err := io.CopyN(&raw, br, contentLength); err != nil {
				return raw.Bytes(), err
			}
		case contentLength == 0 || status == 204 || status == 304:
			// Explicitly bodyless.
		default:
			// No framing headers: per RFC 7230 the body runs to connection close.
			if _, err := io.Copy(&raw, io.LimitReader(br, maxSmugglingResponseBytes)); err != nil {
				return raw.Bytes(), err
			}
		}

		return raw.Bytes(), nil
	}
}

// copyChunkedBody relays a chunked body, chunk headers and trailers included, so
// the caller keeps the bytes exactly as they appeared on the wire.
func copyChunkedBody(br *bufio.Reader, raw *bytes.Buffer) error {
	var total int64

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			raw.WriteString(line)
			return err
		}
		raw.WriteString(line)

		size := strings.TrimSpace(line)
		if i := strings.IndexByte(size, ';'); i >= 0 { // chunk extensions
			size = strings.TrimSpace(size[:i])
		}
		if size == "" {
			continue
		}

		n, err := strconv.ParseInt(size, 16, 64)
		if err != nil || n < 0 {
			return fmt.Errorf("invalid chunk size %q", size)
		}

		if n == 0 { // last chunk: consume trailers up to the blank line
			for {
				trailer, err := br.ReadString('\n')
				if err != nil {
					raw.WriteString(trailer)
					return err
				}
				raw.WriteString(trailer)
				if strings.TrimRight(trailer, "\r\n") == "" {
					return nil
				}
			}
		}

		total += n
		if total > maxSmugglingResponseBytes {
			return fmt.Errorf("chunked body exceeds %d bytes", maxSmugglingResponseBytes)
		}
		if _, err := io.CopyN(raw, br, n); err != nil {
			return err
		}
		crlf, err := br.ReadString('\n')
		if err != nil {
			raw.WriteString(crlf)
			return err
		}
		raw.WriteString(crlf)
	}
}

// parseResponseStatusLine extracts the numeric status from a status line,
// returning 0 when the line is not parseable.
func parseResponseStatusLine(line string) int {
	parts := strings.SplitN(strings.TrimSpace(line), " ", 3)
	if len(parts) < 2 {
		return 0
	}
	code, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}
	return code
}
