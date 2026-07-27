package active

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

type HeaderTest struct {
	HeaderName string
	Values     []string
}

var bypassIPs = []string{
	"127.0.0.1", // Standard loopback
	"localhost", // Localhost domain
	// "0.0.0.0",            // Non-routable meta-address
	"::1", // IPv6 loopback
	// "0000:0000:0000:0000:0000:0000:0000:0001",  // Full IPv6 loopback
	// "0:0:0:0:0:0:0:1",    // Shortened IPv6 loopback
	"127.0.1.1", // Alternative loopback in some systems
	// "10.0.0.0",           // Private IP address range (start)
	// "172.16.0.0",         // Another private IP address range (start)
	// "192.168.0.0",        // Another private IP address range (start)
	"0x7F000001", // Hex representation of 127.0.0.1
	"2130706433", // Decimal representation of 127.0.0.1
	"127.1",      // Short form of 127.0.0.1
}

var ipBasedHeaders = []HeaderTest{
	{"X-Original-URL", bypassIPs},
	{"X-Custom-IP-Authorization", bypassIPs},
	{"X-Forwarded-For", bypassIPs},
	{"X-Originally-Forwarded-For", bypassIPs},
	{"X-Originating-", bypassIPs},
	{"X-Originating-IP", bypassIPs},
	{"True-Client-IP", bypassIPs},
	{"X-WAP-Profile", bypassIPs},
	{"X-Arbitrary", bypassIPs},
	{"X-HTTP-DestinationURL", bypassIPs},
	{"X-Forwarded-Proto", bypassIPs},
	{"Destination", bypassIPs},
	{"X-Remote-IP", bypassIPs},
	{"X-Client-IP", bypassIPs},
	{"X-Host", bypassIPs},
	{"X-Forwarded-Host", bypassIPs},
	{"X-ProxyUser-Ip", bypassIPs},
	{"X-Real-IP", bypassIPs},
}

var bypassURLs = []string{
	"http://127.0.0.1",
	"https://127.0.0.1",
	"http://localhost",
	"https://localhost",
	"http://127.0.0.1:80",
	"https://127.0.0.1:443",
}

var urlBasedHeaders = []HeaderTest{
	{"X-Original-URL", bypassURLs},
	{"X-Forwarded-For", bypassURLs},
	{"X-Originating-", bypassURLs},
	{"X-Arbitrary", bypassURLs},
	{"X-HTTP-DestinationURL", bypassURLs},
	{"X-Forwarded-Proto", bypassURLs},
	{"X-Host", bypassURLs},
	{"X-Forwarded-Host", bypassURLs},
}

var bypassPorts = []string{"80", "443", "8000", "8080", "8443", "8888", "10443"}

var portBasedHeaders = []HeaderTest{
	{"X-Forwarded-Port", bypassPorts},
	{"X-Original-Port", bypassPorts},
	{"X-Real-Port", bypassPorts},
}
var bypassPaths = []string{
	"/",
	"/admin",
}

var pathBasedHeaders = []HeaderTest{
	{"X-Rewrite-URL", bypassPaths},
	{"X-Real-URL", bypassPaths},
}

func ForbiddenBypassScan(history *db.History, options ActiveModuleOptions) {
	auditLog := log.With().Str("audit", "bypass").Str("url", history.URL).Uint("workspace", options.WorkspaceID).Logger()

	// Check context cancellation
	if options.Ctx != nil {
		select {
		case <-options.Ctx.Done():
			auditLog.Debug().Msg("Context cancelled, skipping bypass scan")
			return
		default:
		}
	}

	if history.StatusCode != 401 && history.StatusCode != 403 {
		auditLog.Warn().Msg("Skipping auth bypass scan because the status code is not 401 or 403")
		return
	}
	if options.Concurrency == 0 {
		options.Concurrency = 5
	}
	client := options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	// Get context, defaulting to background if not provided
	ctx := options.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Create pool with context for cancellation support
	p := pool.New().WithMaxGoroutines(options.Concurrency)

	state := &bypassScanState{}
	// Deferred, not called at the end of the happy path: this function returns
	// early from ten places (context cancellation, parse failures), and findings
	// already confirmed by the control must not be discarded when a scan is
	// cancelled at its wall-clock cap.
	defer reportBypassFindings(history, state, options, auditLog)

	allHeaderTypes := [][]HeaderTest{ipBasedHeaders, urlBasedHeaders, portBasedHeaders, pathBasedHeaders}
	// header bypass checks
	for _, headers := range allHeaderTypes {
		// Check context before processing each header type
		select {
		case <-ctx.Done():
			auditLog.Info().Msg("Bypass scan cancelled")
			p.Wait()
			return
		default:
		}

		valueCombinations := flattenHeaders(headers)

		for _, combination := range valueCombinations {
			comb := combination
			p.Go(func() {
				// Check context before making request
				select {
				case <-ctx.Done():
					return
				default:
				}

				request, err := http_utils.BuildRequestFromHistoryItem(history)
				if err != nil {
					auditLog.Error().Err(err).Msg("Error creating the request")
					return
				}
				for header, value := range comb {
					request.Header.Set(header, value)
				}
				// Use context for HTTP request
				request = request.WithContext(ctx)
				sendRequestAndCheckBypass(ctx, client, request, history, options, auditLog, state)
			})
		}
	}

	// Check context before URL bypass checks
	select {
	case <-ctx.Done():
		auditLog.Info().Msg("Bypass scan cancelled before URL bypass checks")
		p.Wait()
		return
	default:
	}

	// url bypass checks
	bypassURLs, err := generateBypassURLs(history)
	if err != nil {
		auditLog.Error().Err(err).Msg("Error generating bypass URLs")
		return
	}

	for _, bypassURL := range bypassURLs {
		bURL := bypassURL
		p.Go(func() {
			// Check context before making request
			select {
			case <-ctx.Done():
				return
			default:
			}

			request, err := http_utils.BuildRequestFromHistoryItem(history)
			if err != nil {
				auditLog.Error().Err(err).Msgf("Error creating request for bypass URL: %s", bURL)
				return
			}
			parsed, err := url.Parse(bURL)
			if err != nil {
				auditLog.Error().Err(err).Msgf("Error parsing bypass URL: %s", bURL)
				return
			}
			request.URL = parsed
			// Use context for HTTP request
			request = request.WithContext(ctx)
			sendRequestAndCheckBypass(ctx, client, request, history, options, auditLog, state)
		})
	}
	p.Wait()
	auditLog.Info().Msg("Finished auth bypass scan")
}

// Flatten headers into a slice of individual header-value pairs
func flattenHeaders(headerTests []HeaderTest) []map[string]string {
	var flat []map[string]string
	for _, ht := range headerTests {
		for _, val := range ht.Values {
			flat = append(flat, map[string]string{ht.HeaderName: val})
		}
	}
	return flat
}

// Get the list of bypass URLs based on provided payloads.
func generateBypassURLs(history *db.History) ([]string, error) {
	originalURL, err := url.Parse(history.URL)
	if err != nil {
		return nil, err
	}
	urlPath := originalURL.Path

	if urlPath == "" {
		return nil, nil
	}

	segments := strings.Split(urlPath, "/")
	if len(segments) < 2 {
		return nil, nil
	}
	lastSegment := segments[len(segments)-1]
	basePath := strings.Join(segments[:len(segments)-1], "/")

	var pathPayloads = []string{
		"/%2e/" + lastSegment,
		lastSegment + "/./",
		"/." + lastSegment + "/./",
		lastSegment + "%20/",
		"/%20" + lastSegment + "%20/",
		lastSegment + "%09/",
		"/%09" + lastSegment + "%09/",
		lastSegment + "..;/",
		lastSegment + "?",
		lastSegment + "??",
		"/" + lastSegment + "//",
		lastSegment + "/",
		strings.ToUpper(lastSegment),
		lastSegment + "/.",
		"//" + lastSegment + "//",
		"/./" + lastSegment + "/..",
		";/" + lastSegment,
		".;/" + lastSegment,
		"//;//" + lastSegment,
	}

	var bypassURLs []string
	for _, payload := range pathPayloads {
		newURL := *originalURL
		newPath := basePath + payload
		newURL.Path = newPath
		bypassURLs = append(bypassURLs, newURL.String())
	}

	return bypassURLs, nil
}

// bypassFinding captures the evidence for a single header/path combination that
// appeared to bypass the 401/403, once the control has confirmed the baseline
// is still valid.
type bypassFinding struct {
	targetURL   string
	headersSent string
	history     *db.History
	confidence  int
}

// bypassScanState is shared across all goroutines scanning one history item:
// the baseline control (resolved at most once) and the findings collected so
// far, so they can be reported as a single consolidated issue at the end.
type bypassScanState struct {
	control  baselineControl
	mu       sync.Mutex
	findings []bypassFinding
}

func (s *bypassScanState) addFinding(f bypassFinding) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.findings = append(s.findings, f)
}

func sendRequestAndCheckBypass(ctx context.Context, client *http.Client, request *http.Request, original *db.History, options ActiveModuleOptions, auditLog zerolog.Logger, state *bypassScanState) {
	executionResult := http_utils.ExecuteRequest(request, http_utils.RequestExecutionOptions{
		Client:        client,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:              db.SourceScanner,
			WorkspaceID:         options.WorkspaceID,
			TaskID:              options.TaskID,
			ScanID:              options.ScanID,
			ScanJobID:           options.ScanJobID,
			CreateNewBodyStream: false,
		},
	})

	if executionResult.Err != nil {
		auditLog.Error().Err(executionResult.Err).Msg("Error during request")
		return
	}

	history := executionResult.History

	if history.StatusCode >= 200 && history.StatusCode < 400 {
		if options.SiteBehavior != nil && options.SiteBehavior.IsNotFound(history) {
			auditLog.Debug().Str("url", request.URL.String()).Msg("Bypass response matches site not-found behavior, skipping")
			return
		}

		// If the response URL domain differs from the original request domain,
		// the server redirected to a different host — not a real bypass
		originalHost := strings.ToLower(request.URL.Hostname())
		responseHost := strings.ToLower(history.URL)
		if parsedResponse, err := url.Parse(history.URL); err == nil {
			responseHost = strings.ToLower(parsedResponse.Hostname())
		}
		if originalHost != responseHost {
			auditLog.Debug().Str("original", originalHost).Str("response", responseHost).Msg("Response from different domain, not a bypass")
			return
		}

		// If the bypass response is identical to the homepage, the path
		// normalized to root (e.g. ./resource/..) — not a real bypass
		if options.SiteBehavior != nil && options.SiteBehavior.BaseURLSample != nil {
			if history.ResponseHash() == options.SiteBehavior.BaseURLSample.ResponseHash() {
				auditLog.Debug().Str("url", request.URL.String()).Msg("Bypass response matches homepage, path likely normalized")
				return
			}
		}

		// Crawled baselines go stale (e.g. the account they were captured
		// against gets created minutes later by the crawler itself), so
		// confirm the original request is still denied before trusting this
		// candidate as a real bypass.
		control, err := state.control.resolve(ctx, client, original, options)
		if err != nil {
			auditLog.Warn().Err(err).Str("url", request.URL.String()).Msg("Could not re-validate baseline with a control request, suppressing candidate bypass")
			return
		}
		if !isForbiddenStatus(control.StatusCode) {
			if controlIsOpen(control.StatusCode) {
				auditLog.Debug().Str("url", request.URL.String()).Int("control_status", control.StatusCode).Msg("Control request no longer returns 401/403, baseline is stale, skipping")
			} else {
				auditLog.Warn().Str("url", request.URL.String()).Int("control_status", control.StatusCode).Msg("Control request could not confirm the 401/403 baseline, suppressing candidate bypass")
			}
			return
		}

		confidence := 50
		if history.StatusCode >= 200 && history.StatusCode < 300 {
			confidence = 90
		}

		state.addFinding(bypassFinding{
			targetURL:   request.URL.String(),
			headersSent: http_utils.HeadersToString(request.Header),
			history:     history,
			confidence:  confidence,
		})
	}
}

// reportBypassFindings consolidates every technique that bypassed the 401/403
// into a single issue, using the highest-confidence finding as the primary
// evidence and attaching the rest as supporting histories.
func reportBypassFindings(original *db.History, state *bypassScanState, options ActiveModuleOptions, auditLog zerolog.Logger) {
	if len(state.findings) == 0 {
		return
	}

	bestIndex := 0
	for i, f := range state.findings {
		if f.confidence > state.findings[bestIndex].confidence {
			bestIndex = i
		}
	}
	best := state.findings[bestIndex]

	details := buildBypassDetails(original, state)

	issue, err := db.CreateIssueFromHistoryAndTemplate(
		best.history,
		db.ForbiddenBypassCode,
		details,
		best.confidence,
		"",
		&options.WorkspaceID, &options.TaskID, &options.TaskJobID, &options.ScanID, &options.ScanJobID,
	)
	if err != nil {
		auditLog.Error().Err(err).Msg("Failed to create forbidden bypass issue")
		return
	}

	var additional []*db.History
	for i, f := range state.findings {
		if i != bestIndex {
			additional = append(additional, f.history)
		}
	}
	if err := issue.AppendHistories(additional); err != nil {
		auditLog.Warn().Err(err).Uint("issue_id", issue.ID).Int("history_count", len(additional)).
			Msg("Failed to link additional bypass histories to issue")
	}

	auditLog.Info().Uint("issue_id", issue.ID).Int("techniques", len(state.findings)).Msg("Created consolidated forbidden bypass issue")
}

func buildBypassDetails(original *db.History, state *bypassScanState) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`Original Request:
	-	URL: %s
	-	Method: %s
	-	Status Code: %d
	-	Response Size: %d bytes

`, original.URL, original.Method, original.StatusCode, original.ResponseBodySize))

	if state.control.history != nil {
		sb.WriteString(fmt.Sprintf("Baseline re-validated with an unmodified request before reporting: still returned status %d, confirming the stored baseline is current.\n\n", state.control.history.StatusCode))
	}

	sb.WriteString(fmt.Sprintf("%d bypass technique(s) succeeded:\n\n", len(state.findings)))
	for i, f := range state.findings {
		sb.WriteString(fmt.Sprintf(`%d. Request to %s
Headers sent:
%s
Response received:
	-	Status Code: %d
	-	Response Size: %d bytes

`, i+1, f.targetURL, f.headersSent, f.history.StatusCode, f.history.ResponseBodySize))
	}

	return sb.String()
}
