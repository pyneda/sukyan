package active

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	// maxHarvestedCORSDomains bounds how many distinct organisations (registrable
	// domains) are taken from a single response body. Each may contribute both the
	// full hostname and the registrable domain itself.
	maxHarvestedCORSDomains = 3
	// maxCORSHarvestBytes bounds how much of a response body is scanned for
	// candidate domains. The body is attacker-controlled, so without this a target
	// chooses how much memory and CPU the scanner spends per probed route.
	maxCORSHarvestBytes = 128 * 1024
	// maxCORSBypassOrigins bounds the allow-list bypass probe set so a
	// content-rich page cannot turn one route into unbounded traffic.
	maxCORSBypassOrigins = 24

	// corsAttackerDomain is the attacker-controlled domain appended or prepended
	// when building bypass origins.
	corsAttackerDomain = "evil.example"
)

type corsSeverity string

const (
	corsSeverityNone   corsSeverity = ""
	corsSeverityLow    corsSeverity = "Low"
	corsSeverityMedium corsSeverity = "Medium"
	corsSeverityHigh   corsSeverity = "High"
)

// Origin classes name the allow-list mistake a probe origin is designed to
// exercise. They appear in the issue details and key the per-host report budget.
const (
	corsClassArbitrary           = "arbitrary-origin"
	corsClassNull                = "null-origin"
	corsClassSuffix              = "suffix-match"
	corsClassPrefix              = "prefix-match"
	corsClassSubdomain           = "any-subdomain"
	corsClassSchemeDowngrade     = "plaintext-origin"
	corsClassRegexDot            = "unescaped-regex-dot"
	corsClassWildcardCredentials = "wildcard-with-credentials"
)

// corsBypassOrigin is one allow-list bypass probe origin and the mistake it tests.
type corsBypassOrigin struct {
	Origin string
	Class  string
}

// corsResponse holds the CORS-relevant headers of a single probe response.
type corsResponse struct {
	AllowOrigin      string
	AllowCredentials bool
}

// classifyCORSProbe grades one probe against the no-Origin baseline.
//
// The wildcard rungs are evaluated before the reflection rungs because `*` is a
// blanket permission rather than a per-origin decision: it is returned on the
// baseline too, so the static-baseline guard would otherwise swallow it.
func classifyCORSProbe(baseline corsResponse, sentOrigin string, got corsResponse) corsSeverity {
	if got.AllowOrigin == "" || sentOrigin == "" {
		return corsSeverityNone
	}

	if got.AllowOrigin == "*" {
		// `ACAO: *` without credentials is the correct configuration for a public
		// API. Reporting it is the largest source of CORS false positives.
		if got.AllowCredentials {
			return corsSeverityLow
		}
		return corsSeverityNone
	}

	// Equality, never `contains`: an ACAO that merely contains the origin we sent
	// is a different origin.
	if got.AllowOrigin != sentOrigin {
		return corsSeverityNone
	}

	// A server that hardcodes an ACAO returns it with no Origin header at all, so
	// a probe origin that happens to equal it is coincidence, not reflection.
	if baseline.AllowOrigin == got.AllowOrigin {
		return corsSeverityNone
	}

	if sentOrigin == "null" {
		if got.AllowCredentials {
			return corsSeverityMedium
		}
		return corsSeverityLow
	}

	if got.AllowCredentials {
		return corsSeverityHigh
	}
	return corsSeverityLow
}

// corsBypassOrigins builds the allow-list bypass probe set.
//
// Probes derived from the target host only defeat an allow-list that keys on the
// host being scanned. Real allow-lists key on the application's branded domain,
// which is why harvested domains are folded in here as well.
func corsBypassOrigins(targetScheme, targetHost string, harvested []string) []corsBypassOrigin {
	var origins []corsBypassOrigin
	seen := map[string]bool{}

	add := func(origin, class string) {
		if origin == "" || seen[origin] || len(origins) >= maxCORSBypassOrigins {
			return
		}
		seen[origin] = true
		origins = append(origins, corsBypassOrigin{Origin: origin, Class: class})
	}

	// A sandboxed iframe or a data: document sends Origin: null, so allowing it
	// grants access to any site that can create one.
	add("null", corsClassNull)

	targetHost = strings.ToLower(strings.TrimSpace(stripPort(targetHost)))
	for _, host := range append([]string{targetHost}, harvested...) {
		host = strings.ToLower(strings.TrimSpace(stripPort(host)))
		if host == "" {
			continue
		}
		// endsWith(trusted) — an attacker-registrable domain ending in the trusted one.
		add("https://evil"+host, corsClassSuffix)
		// startsWith(trusted) — the trusted domain as a prefix of an attacker domain.
		add("https://"+host+"."+corsAttackerDomain, corsClassPrefix)
		// Any-subdomain-trusted.
		add("https://evil."+host, corsClassSubdomain)
		// Plaintext origin trusted. On an http:// target its own host is the page's
		// own origin, so echoing it back is correct same-origin behaviour rather
		// than a finding.
		if !(host == targetHost && strings.EqualFold(targetScheme, "http")) {
			add("http://"+host, corsClassSchemeDowngrade)
		}
		// Unescaped dot in an allow-list regex: any character matches where the
		// dot was.
		if i := strings.Index(host, "."); i > 0 {
			add("https://"+host[:i]+"x"+host[i+1:], corsClassRegexDot)
		}
	}

	return origins
}

// corsFindingClass names the shape of the accepted origin. It separates genuinely
// different policies on one host (so they are reported independently) from one
// host-wide policy hit on many routes (so it is not reported many times), and it
// tells a triager which allow-list mistake was exercised.
func corsFindingClass(got corsResponse, originClass string) string {
	if got.AllowOrigin == "*" {
		return corsClassWildcardCredentials
	}
	return originClass
}

var (
	corsURLHostRegex   = regexp.MustCompile(`https?://([a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`)
	corsEmailHostRegex = regexp.MustCompile(`@([a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`)
)

// corsNoiseDomains are registrable domains that appear in almost every page and
// are never the application's own trusted origin.
var corsNoiseDomains = map[string]bool{
	"googletagmanager.com": true,
	"googleapis.com":       true,
	"google-analytics.com": true,
	"gstatic.com":          true,
	"jsdelivr.net":         true,
	"unpkg.com":            true,
	"cloudflare.com":       true,
	"cdnjs.com":            true,
	"bootstrapcdn.com":     true,
	"jquery.com":           true,
	"w3.org":               true,
	"schema.org":           true,
	"example.com":          true,
	"github.com":           true,
	"facebook.net":         true,
	"doubleclick.net":      true,
	"sentry.io":            true,
}

// harvestCORSDomains extracts candidate trusted domains from a response body.
// Both the full hostname and its registrable domain are returned: an allow-list
// may key on either, and the unescaped-dot bypass is only derivable from the
// full hostname.
func harvestCORSDomains(body string, targetHost string) []string {
	if len(body) > maxCORSHarvestBytes {
		body = body[:maxCORSHarvestBytes]
	}
	target := strings.ToLower(stripPort(targetHost))

	var out []string
	seen := map[string]bool{}
	orgs := map[string]bool{}

	consider := func(host string) {
		host = strings.ToLower(strings.Trim(host, "."))
		if host == "" || seen[host] {
			return
		}
		if net.ParseIP(host) != nil || !strings.Contains(host, ".") {
			return
		}
		// Anything sharing a suffix with the target is the target's own domain and
		// carries no information about a third-party allow-list entry.
		if host == target || strings.HasSuffix(target, "."+host) || strings.HasSuffix(host, "."+target) {
			return
		}
		org := registrableDomain(host)
		if corsNoiseDomains[org] {
			return
		}
		// The budget counts organisations, not hostnames: a CDN contributing both
		// cdn.example.net and example.net must not consume two of three slots and
		// crowd out the application's own domain.
		if !orgs[org] && len(orgs) >= maxHarvestedCORSDomains {
			return
		}
		orgs[org] = true

		seen[host] = true
		out = append(out, host)
		if org != host && !seen[org] {
			seen[org] = true
			out = append(out, org)
		}
	}

	// Email domains are considered first: an address written into the page is a far
	// stronger signal of the application's own domain than an asset host, and a page
	// that loads three analytics scripts before naming itself is the common case.
	for _, m := range corsEmailHostRegex.FindAllStringSubmatch(body, -1) {
		consider(m[1])
	}
	for _, m := range corsURLHostRegex.FindAllStringSubmatch(body, -1) {
		consider(m[1])
	}

	return out
}

// registrableDomain reduces a hostname to its last two labels. It is deliberately
// naive about multi-label public suffixes (.co.uk): over-reducing yields one extra
// probe origin, which is cheaper than carrying a public-suffix list.
func registrableDomain(host string) string {
	labels := strings.Split(strings.Trim(host, "."), ".")
	if len(labels) <= 2 {
		return host
	}
	return strings.Join(labels[len(labels)-2:], ".")
}

func stripPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

// CORSScanOptions configures the CORS misconfiguration probe.
type CORSScanOptions struct {
	ActiveModuleOptions

	// RouteGate, when set, is consulted before probing a route and returns true if
	// that route still needs probing. Callers use it to probe each route exactly
	// once per scan. Nil means always probe.
	RouteGate func(route string) bool

	// RouteRelease, when set, returns a claimed route to the pool because nothing
	// was decided about it. Without it a transient failure retires the route for
	// the remainder of the scan.
	RouteRelease func(route string)

	// SweepGate, when set, is consulted before spending the allow-list bypass
	// sweep on a host and returns true while that host still has sweep budget.
	// Nil means always sweep.
	SweepGate func(host string) bool

	// ReportGate, when set, is consulted before filing an issue and returns true
	// while this host still has budget for that policy signature. It keeps one
	// host-wide misconfiguration from being reported once per route.
	ReportGate func(signature string) bool
}

// corsProbe records one origin probe and the history row it produced.
type corsProbe struct {
	Origin   string
	Response corsResponse
	History  *db.History
}

// corsRouteKey identifies the route whose CORS policy is being probed. A server
// applies the policy per route, so the key is the full path: the many history
// items that share one URL collapse to a single probe, while sibling routes stay
// independent. Truncating to a fixed number of path segments would merge
// unrelated policies that happen to share a parent directory.
func corsRouteKey(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	path := strings.TrimRight(u.Path, "/")
	if path == "" {
		path = "/"
	}
	return u.Scheme + "://" + u.Host + path
}

// CORSScan probes a route for a permissive CORS policy.
//
// Sukyan's only other CORS signal is the DevTools relay in pkg/web, which fires
// when the browser BLOCKS a request; a permissive server produces no DevTools
// issue at all, so this is the only path that detects the misconfiguration.
func CORSScan(history *db.History, opts CORSScanOptions) {
	if history == nil {
		return
	}

	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return
	default:
	}

	route := corsRouteKey(history.URL)
	if opts.RouteGate != nil && !opts.RouteGate(route) {
		return
	}

	auditLog := log.With().Str("audit", "cors").Str("route", route).Uint("workspace", opts.WorkspaceID).Logger()

	// The claim is taken before any traffic so concurrent jobs on the same route
	// do not stampede, but a route that was never actually decided must go back:
	// otherwise one connection reset or a cancelled job silently retires the route
	// for the rest of the scan.
	decided := false
	defer func() {
		if !decided && opts.RouteRelease != nil {
			opts.RouteRelease(route)
		}
	}()

	client := opts.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}
	// A redirect target's CORS policy belongs to the redirect target, not to the
	// route being probed. Following one grades a third party's headers and files
	// the issue against a host that was never in scope.
	client = http_utils.WithoutRedirects(client)

	baseline, baselineBody := corsSendProbe(ctx, history, opts, client, "", "")
	if baseline == nil {
		auditLog.Debug().Msg("Baseline probe failed, skipping CORS audit")
		return
	}

	supporting := []*db.History{baseline.History}

	arbitrary, _ := corsSendProbe(ctx, history, opts, client, "https://"+corsAttackerDomain, "")
	if arbitrary == nil {
		return
	}
	supporting = append(supporting, arbitrary.History)

	hit := arbitrary
	hitClass := corsClassArbitrary
	severity := classifyCORSProbe(baseline.Response, arbitrary.Origin, arbitrary.Response)

	targetScheme, targetHost := "", ""
	if u, err := url.Parse(history.URL); err == nil {
		targetScheme, targetHost = u.Scheme, u.Host
	}

	// The allow-list bypass probes only carry information when there is an
	// allow-list to defeat. On a permissive target they all return the same answer.
	//
	// They cannot be gated on "the route returned some access-control-* header",
	// because an allow-list that rejects the origin returns no CORS headers at all
	// - that is exactly the case the sweep exists to find. So the budget is per
	// host instead: an allow-list is app-wide configuration, and sampling routes
	// finds it without paying the sweep on every route of a large site.
	if severity == corsSeverityNone {
		if opts.SweepGate != nil && !opts.SweepGate(targetHost) {
			auditLog.Debug().Str("host", targetHost).Msg("Allow-list bypass sweep budget for this host is spent, skipping sweep")
			decided = true
			return
		}
		harvested := harvestCORSDomains(baselineBody, targetHost)

		for _, candidate := range corsBypassOrigins(targetScheme, targetHost, harvested) {
			select {
			case <-ctx.Done():
				return
			default:
			}
			probe, _ := corsSendProbe(ctx, history, opts, client, candidate.Origin, "")
			if probe == nil {
				continue
			}
			supporting = append(supporting, probe.History)
			if s := classifyCORSProbe(baseline.Response, candidate.Origin, probe.Response); s != corsSeverityNone {
				hit, hitClass, severity = probe, candidate.Class, s
				break
			}
		}
	}

	decided = true

	if severity == corsSeverityNone {
		auditLog.Debug().Msg("No permissive CORS policy detected")
		return
	}

	class := corsFindingClass(hit.Response, hitClass)
	if opts.ReportGate != nil && !opts.ReportGate(corsSignature(targetHost, severity, class)) {
		auditLog.Info().
			Str("host", targetHost).
			Str("class", class).
			Str("severity", string(severity)).
			Msg("Suppressing duplicate CORS finding: this host already reported this policy on other routes")
		return
	}

	// A preflight is the more accurate evidence for a CORS finding and shows
	// whether the permissive policy also covers non-simple methods and headers.
	if preflight, _ := corsSendProbe(ctx, history, opts, client, hit.Origin, http.MethodOptions); preflight != nil {
		supporting = append(supporting, preflight.History)
	}

	corsReportIssue(hit, baseline, severity, class, supporting, opts, auditLog)
}

// corsSignature identifies one CORS policy on one host, so the same host-wide
// misconfiguration seen on many routes is recognised as one finding.
func corsSignature(host string, severity corsSeverity, class string) string {
	return host + "|" + string(severity) + "|" + class
}

// corsSendProbe issues one request with the given Origin. An empty origin sends
// no Origin header at all, which is the baseline.
//
// Probes never replay the history item's method or body. A CORS policy is served
// per route and is visible on a plain GET — casino returns its access-control-*
// headers on 405 and 404 responses alike — so replaying the original request
// would re-submit whatever it did, once per probe, for no extra signal.
func corsSendProbe(ctx context.Context, history *db.History, opts CORSScanOptions, client *http.Client, origin, method string) (*corsProbe, string) {
	req, err := http_utils.BuildRequestFromHistoryItem(history)
	if err != nil {
		return nil, ""
	}

	req.Method = http.MethodGet
	req.Body = nil
	req.GetBody = nil
	req.ContentLength = 0
	req.Header.Del("Content-Type")
	req.Header.Del("Content-Length")

	req.Header.Del("Origin")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if method == http.MethodOptions {
		req.Method = http.MethodOptions
		req.Header.Set("Access-Control-Request-Method", http.MethodPut)
		req.Header.Set("Access-Control-Request-Headers", "authorization")
	}
	req = req.WithContext(ctx)

	result := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		Client:        client,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:              db.SourceScanner,
			WorkspaceID:         opts.WorkspaceID,
			TaskID:              opts.TaskID,
			ScanID:              opts.ScanID,
			ScanJobID:           opts.ScanJobID,
			CreateNewBodyStream: false,
		},
	})
	if result.Err != nil || result.History == nil || result.Response == nil {
		return nil, ""
	}

	return &corsProbe{
		Origin:   origin,
		Response: corsResponseFromHeaders(result.Response.Header),
		History:  result.History,
	}, string(result.ResponseData.Body)
}

func corsResponseFromHeaders(h http.Header) corsResponse {
	return corsResponse{
		AllowOrigin:      strings.TrimSpace(h.Get("Access-Control-Allow-Origin")),
		AllowCredentials: strings.EqualFold(strings.TrimSpace(h.Get("Access-Control-Allow-Credentials")), "true"),
	}
}

func corsReportIssue(hit, baseline *corsProbe, severity corsSeverity, class string, supporting []*db.History, opts CORSScanOptions, auditLog zerolog.Logger) {
	baselineACAO := baseline.Response.AllowOrigin
	if baselineACAO == "" {
		baselineACAO = "(no access-control-* headers)"
	}

	details := fmt.Sprintf(`Accepted origin class: %s
Origin sent: %s
Access-Control-Allow-Origin returned: %s
Access-Control-Allow-Credentials: %t

Baseline (no Origin header) returned: %s
Probes sent: %d`,
		class,
		hit.Origin,
		hit.Response.AllowOrigin,
		hit.Response.AllowCredentials,
		baselineACAO,
		len(supporting),
	)

	confidence := 90
	if severity == corsSeverityLow {
		confidence = 70
	}

	issue, err := db.CreateIssueFromHistoryAndTemplate(
		hit.History,
		db.CorsCode,
		details,
		confidence,
		string(severity),
		&opts.WorkspaceID,
		&opts.TaskID,
		&opts.TaskJobID,
		&opts.ScanID,
		&opts.ScanJobID,
	)
	if err != nil {
		auditLog.Error().Err(err).Msg("Failed to create CORS issue")
		return
	}
	if issue.IsEmpty() {
		return
	}

	if err := issue.AppendHistories(supporting); err != nil {
		auditLog.Warn().Err(err).Uint("issue_id", issue.ID).Msg("Failed to link supporting CORS probes")
	}

	auditLog.Warn().
		Str("origin", hit.Origin).
		Str("severity", string(severity)).
		Msg("Permissive CORS policy detected")
}
