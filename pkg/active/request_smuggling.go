package active

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
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

	if resp.MarkerFound {
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

		// Also check if marker was found via SendRawPipelined's marker check
		if !found && resp.MarkerFound {
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

	// Link all revalidation request histories to the issue
	if len(revalidationHistories) > 0 {
		validHistories := make([]*db.History, 0, len(revalidationHistories))
		for _, h := range revalidationHistories {
			if h != nil {
				validHistories = append(validHistories, h)
			}
		}
		if len(validHistories) > 0 {
			if err := db.Connection().DB().Model(&issue).Association("Requests").Append(validHistories); err != nil {
				log.Warn().Err(err).Uint("issue_id", issue.ID).Int("history_count", len(validHistories)).
					Msg("Failed to link revalidation histories to issue")
			}
		}
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
	History        *db.History
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

	// Step 1: Send the smuggling payload
	_, err = conn.Write(smugglingPayload)
	if err != nil {
		return nil, fmt.Errorf("write smuggling payload failed: %w", err)
	}

	// Step 2: Read first response
	firstBuf := make([]byte, 16384)
	n1, err := conn.Read(firstBuf)
	if err != nil && n1 == 0 {
		return nil, fmt.Errorf("read first response failed: %w", err)
	}
	firstResponse := firstBuf[:n1]

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

	// Step 4: Read second response
	secondBuf := make([]byte, 16384)
	n2, _ := conn.Read(secondBuf) // Ignore error - may get EOF
	secondResponse := secondBuf[:n2]

	elapsed := time.Since(start)

	// Check for marker in responses
	markerFound := false
	markerLocation := ""

	if marker != "" {
		if strings.Contains(string(firstResponse), marker) {
			markerFound = true
			markerLocation = "first response"
		}
		if strings.Contains(string(secondResponse), marker) {
			markerFound = true
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
		FirstResponse:  firstResponse,
		SecondResponse: secondResponse,
		Duration:       elapsed,
		MarkerFound:    markerFound,
		MarkerLocation: markerLocation,
		History:        history,
	}, nil
}
