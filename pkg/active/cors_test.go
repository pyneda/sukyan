package active

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/stretchr/testify/require"
)

// The grading ladder. Every rung here maps to a case in the misconfig-exposure
// CORS matrix (testbeds/misconfig-exposure/core/cases/13-cors.js); the case id is
// named in each test so a testbed change that invalidates a rung is traceable.

func TestClassifyCORSProbe_ReflectedWithCredentialsIsHigh(t *testing.T) {
	// CORS-004 reflected-with-credentials
	got := classifyCORSProbe(
		corsResponse{},
		"https://evil.example",
		corsResponse{AllowOrigin: "https://evil.example", AllowCredentials: true},
	)
	require.Equal(t, corsSeverityHigh, got)
}

func TestClassifyCORSProbe_ReflectedWithoutCredentialsIsLow(t *testing.T) {
	// CORS-003 reflected-origin
	got := classifyCORSProbe(
		corsResponse{},
		"https://evil.example",
		corsResponse{AllowOrigin: "https://evil.example"},
	)
	require.Equal(t, corsSeverityLow, got)
}

func TestClassifyCORSProbe_NullOriginWithCredentialsIsMedium(t *testing.T) {
	// CORS-005 null-origin
	got := classifyCORSProbe(
		corsResponse{},
		"null",
		corsResponse{AllowOrigin: "null", AllowCredentials: true},
	)
	require.Equal(t, corsSeverityMedium, got)
}

func TestClassifyCORSProbe_WildcardWithoutCredentialsIsNotReported(t *testing.T) {
	// CORS-021 / CORS-022, both declared `trap`. `ACAO: *` with no credentials is
	// the correct public-API configuration and is the single largest source of
	// CORS false positives.
	got := classifyCORSProbe(
		corsResponse{AllowOrigin: "*"},
		"https://evil.example",
		corsResponse{AllowOrigin: "*"},
	)
	require.Equal(t, corsSeverityNone, got)
}

func TestClassifyCORSProbe_WildcardWithCredentialsIsLow(t *testing.T) {
	// CORS-002 wildcard-with-credentials. Browsers reject the combination, so it
	// is not directly exploitable, but the intent is visible and a proxy that
	// rewrites the wildcard to the request origin makes it exploitable.
	got := classifyCORSProbe(
		corsResponse{AllowOrigin: "*"},
		"https://evil.example",
		corsResponse{AllowOrigin: "*", AllowCredentials: true},
	)
	require.Equal(t, corsSeverityLow, got)
}

func TestClassifyCORSProbe_StaticAllowlistIsNotReported(t *testing.T) {
	// CORS-020 strict-allowlist. A fixed ACAO returned regardless of what was
	// sent is an allow-list, not reflection.
	got := classifyCORSProbe(
		corsResponse{},
		"https://evil.example",
		corsResponse{AllowOrigin: "https://app.vantageops.io", AllowCredentials: true},
	)
	require.Equal(t, corsSeverityNone, got)
}

func TestClassifyCORSProbe_BaselineAlreadyReturnedThatOriginIsNotReflection(t *testing.T) {
	// A server that hardcodes an ACAO returns it even with no Origin header. If
	// our probe origin happens to equal it, that is coincidence, not reflection.
	got := classifyCORSProbe(
		corsResponse{AllowOrigin: "https://evil.example", AllowCredentials: true},
		"https://evil.example",
		corsResponse{AllowOrigin: "https://evil.example", AllowCredentials: true},
	)
	require.Equal(t, corsSeverityNone, got)
}

func TestClassifyCORSProbe_SubstringMatchIsNotReflection(t *testing.T) {
	// Require string equality, never `contains`: an ACAO that merely contains the
	// origin we sent is a different origin.
	got := classifyCORSProbe(
		corsResponse{},
		"https://evil.example",
		corsResponse{AllowOrigin: "https://evil.example.attacker.net", AllowCredentials: true},
	)
	require.Equal(t, corsSeverityNone, got)
}

func TestClassifyCORSProbe_NoCORSHeadersIsNotReported(t *testing.T) {
	got := classifyCORSProbe(corsResponse{}, "https://evil.example", corsResponse{})
	require.Equal(t, corsSeverityNone, got)
}

// Probe generation. The handoff's bypass set derives candidate origins from the
// target host only, which was measured against the misconfig-exposure matrix to
// reach 1 of 6 allow-list cases. Origins harvested from response content reach
// 6 of 6, so both sources must be represented.

func bypassOriginStrings(probes []corsBypassOrigin) []string {
	out := make([]string, 0, len(probes))
	for _, p := range probes {
		out = append(out, p.Origin)
	}
	return out
}

func TestCORSBypassOrigins_IncludesNullAndHostDerivedVariants(t *testing.T) {
	origins := bypassOriginStrings(corsBypassOrigins("https", "app.example.com", nil))

	require.Contains(t, origins, "null")
	require.Contains(t, origins, "https://app.example.com.evil.example")
	require.Contains(t, origins, "https://evilapp.example.com")
}

func TestCORSBypassOrigins_IncludesHarvestedDomainVariants(t *testing.T) {
	// Harvesting yields both the full hostname and its registrable domain; the
	// dot-replacement variant is only derivable from the full hostname.
	origins := bypassOriginStrings(corsBypassOrigins("http", "127.0.0.1:21000", []string{"app.vantageops.io", "vantageops.io"}))

	// suffix-match bypass (endsWith): an attacker-registrable domain ending in
	// the trusted one. CORS-006.
	require.Contains(t, origins, "https://evilvantageops.io")
	// prefix-match bypass (startsWith): trusted domain as a prefix. CORS-007.
	require.Contains(t, origins, "https://vantageops.io.evil.example")
	// unescaped-dot regex bypass: any char where the dot was. CORS-008.
	require.Contains(t, origins, "https://appxvantageops.io")
	// any-subdomain-trusted. CORS-009.
	require.Contains(t, origins, "https://evil.vantageops.io")
	// plaintext-origin-trusted. CORS-010.
	require.Contains(t, origins, "http://vantageops.io")
}

func TestCORSBypassOrigins_AreBounded(t *testing.T) {
	var harvested []string
	for i := range 50 {
		harvested = append(harvested, fmt.Sprintf("brand%d.example", i))
	}

	origins := bypassOriginStrings(corsBypassOrigins("https", "target.local", harvested))

	require.LessOrEqual(t, len(origins), maxCORSBypassOrigins)
}

func TestCORSBypassOrigins_AreDeduplicated(t *testing.T) {
	origins := bypassOriginStrings(corsBypassOrigins("https", "vantageops.io", []string{"vantageops.io", "vantageops.io"}))

	seen := map[string]bool{}
	for _, o := range origins {
		require.False(t, seen[o], "duplicate origin %q", o)
		seen[o] = true
	}
}

// Domain harvesting. A real allow-list keys on the application's branded domain,
// which is not the host the scanner connects to.

func TestHarvestCORSDomains_FindsBrandedDomainInBody(t *testing.T) {
	body := `{"canary":"X","email":"maya.okonkwo@vantageops.io","workspace":"ws_8812"}`

	got := harvestCORSDomains(body, "127.0.0.1:21000")

	require.Contains(t, got, "vantageops.io")
}

func TestHarvestCORSDomains_ExcludesTargetHost(t *testing.T) {
	body := `<a href="https://app.example.com/x">self</a>`

	got := harvestCORSDomains(body, "app.example.com")

	require.NotContains(t, got, "app.example.com")
}

func TestHarvestCORSDomains_ExcludesThirdPartyNoise(t *testing.T) {
	body := `<script src="https://www.googletagmanager.com/gtag.js"></script>
	<link href="https://fonts.googleapis.com/css">
	<img src="https://cdn.jsdelivr.net/x.png">`

	got := harvestCORSDomains(body, "app.example.com")

	require.Empty(t, got)
}

func TestHarvestCORSDomains_IsBounded(t *testing.T) {
	var body strings.Builder
	for i := range 200 {
		fmt.Fprintf(&body, "https://host%d.example-brand%d.com ", i, i)
	}

	got := harvestCORSDomains(body.String(), "target.local")

	require.LessOrEqual(t, len(got), 2*maxHarvestedCORSDomains,
		"the budget caps organisations; each may contribute a hostname and its registrable domain")
}

// Orchestration. These exercise CORSScan end to end against servers that mirror
// the misconfig-exposure case shapes.

func newCORSHistory(t *testing.T, workspaceID uint, rawURL string) *db.History {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	require.NoError(t, err)

	result := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: workspaceID,
		},
	})
	require.NoError(t, result.Err)
	require.NotNil(t, result.History)
	return result.History
}

func corsIssueCount(t *testing.T) int64 {
	t.Helper()
	var count int64
	db.Connection().DB().Model(&db.Issue{}).Where("code = ?", string(db.CorsCode)).Count(&count)
	return count
}

func TestCORSScan_ReportsReflectedOriginWithCredentials(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "cors-reflect", Title: "cors-reflect"})
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Write([]byte(`{"balance": 100}`))
	}))
	defer srv.Close()

	cleanupIssues(t, db.CorsCode)

	CORSScan(newCORSHistory(t, workspace.ID, srv.URL+"/api/wallet/balance"), CORSScanOptions{
		ActiveModuleOptions: ActiveModuleOptions{WorkspaceID: workspace.ID, HTTPClient: http.DefaultClient},
	})

	require.Equal(t, int64(1), corsIssueCount(t), "expected one consolidated cors issue")

	var issue db.Issue
	require.NoError(t, db.Connection().DB().Where("code = ?", string(db.CorsCode)).First(&issue).Error)
	require.Equal(t, db.High, issue.Severity)
}

func TestCORSScan_DoesNotReportWildcardWithoutCredentials(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "cors-trap", Title: "cors-trap"})
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write([]byte(`public status feed`))
	}))
	defer srv.Close()

	cleanupIssues(t, db.CorsCode)

	CORSScan(newCORSHistory(t, workspace.ID, srv.URL+"/status/feed"), CORSScanOptions{
		ActiveModuleOptions: ActiveModuleOptions{WorkspaceID: workspace.ID, HTTPClient: http.DefaultClient},
	})

	require.Equal(t, int64(0), corsIssueCount(t), "ACAO:* without credentials is the correct public-API config")
}

func TestCORSScan_DoesNotReportStrictAllowlist(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "cors-safe", Title: "cors-safe"})
	require.NoError(t, err)

	var mu sync.Mutex
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		if r.Header.Get("Origin") == "https://app.trusted-brand.test" {
			w.Header().Set("Access-Control-Allow-Origin", "https://app.trusted-brand.test")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}
		w.Write([]byte(`{"ok":true,"support":"help@trusted-brand.test"}`))
	}))
	defer srv.Close()

	cleanupIssues(t, db.CorsCode)
	history := newCORSHistory(t, workspace.ID, srv.URL+"/api/account")

	mu.Lock()
	requests = 0
	mu.Unlock()

	CORSScan(history, CORSScanOptions{
		ActiveModuleOptions: ActiveModuleOptions{WorkspaceID: workspace.ID, HTTPClient: http.DefaultClient},
	})

	require.Equal(t, int64(0), corsIssueCount(t), "an exact-match allow-list must not be reported")

	// Worst case: no probe fires, so the whole bypass set is spent on this route.
	// This is the traffic ceiling for one route.
	mu.Lock()
	defer mu.Unlock()
	require.LessOrEqual(t, requests, maxCORSBypassOrigins+2, "bypass traffic must stay bounded")
	t.Logf("allow-listed route cost %d requests (baseline + arbitrary + bypass set)", requests)
}

func TestCORSScan_ReachesAllowlistBypassViaHarvestedDomain(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "cors-bypass", Title: "cors-bypass"})
	require.NoError(t, err)

	// Suffix-match allow-list keyed on a branded domain that is NOT the host being
	// scanned. Only reachable if the domain is harvested from the response body.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); strings.HasSuffix(origin, "trusted-brand.test") {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Write([]byte(`{"owner":"maya@trusted-brand.test"}`))
	}))
	defer srv.Close()

	cleanupIssues(t, db.CorsCode)

	CORSScan(newCORSHistory(t, workspace.ID, srv.URL+"/api/profile"), CORSScanOptions{
		ActiveModuleOptions: ActiveModuleOptions{WorkspaceID: workspace.ID, HTTPClient: http.DefaultClient},
	})

	require.Equal(t, int64(1), corsIssueCount(t), "harvested-domain bypass probe should have fired")
}

func TestCORSScan_ShortCircuitsBypassProbesWhenArbitraryOriginReflected(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "cors-short", Title: "cors-short"})
	require.NoError(t, err)

	var mu sync.Mutex
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cleanupIssues(t, db.CorsCode)
	history := newCORSHistory(t, workspace.ID, srv.URL+"/api/x")

	mu.Lock()
	requests = 0
	mu.Unlock()

	CORSScan(history, CORSScanOptions{
		ActiveModuleOptions: ActiveModuleOptions{WorkspaceID: workspace.ID, HTTPClient: http.DefaultClient},
	})

	mu.Lock()
	defer mu.Unlock()
	// baseline + arbitrary origin + preflight evidence. The bypass set carries no
	// information once an arbitrary origin is already reflected.
	require.LessOrEqual(t, requests, 3, "bypass probes must not run when the arbitrary origin is reflected")
}

func TestCORSScan_RouteGateSuppressesRepeatProbing(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "cors-gate", Title: "cors-gate"})
	require.NoError(t, err)

	var mu sync.Mutex
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cleanupIssues(t, db.CorsCode)
	history := newCORSHistory(t, workspace.ID, srv.URL+"/api/y")

	mu.Lock()
	requests = 0
	mu.Unlock()

	CORSScan(history, CORSScanOptions{
		ActiveModuleOptions: ActiveModuleOptions{WorkspaceID: workspace.ID, HTTPClient: http.DefaultClient},
		RouteGate:           func(string) bool { return false },
	})

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 0, requests, "a closed gate must send no probes at all")
}

// The route key decides what "probe once" means. A CORS policy is applied per
// route by the server, so it keys on the full path: many history items on one
// URL (the repeat-issue defect) collapse to a single probe, while genuinely
// distinct routes stay distinct. Truncating to a fixed number of path segments
// was measured to collapse all 15 misconfig-exposure cases, which share the
// /lab/cors/ parent, into a single probe.

func TestCORSRouteKey_CollapsesRepeatItemsOnOneURL(t *testing.T) {
	a := corsRouteKey("https://example.com/api/auth/login?user=a")
	b := corsRouteKey("https://example.com/api/auth/login?user=b")
	c := corsRouteKey("https://example.com/api/auth/login")

	require.Equal(t, a, b, "query strings do not change a CORS policy")
	require.Equal(t, a, c)
}

func TestCORSRouteKey_KeepsSiblingRoutesDistinct(t *testing.T) {
	login := corsRouteKey("https://example.com/api/auth/login")
	signup := corsRouteKey("https://example.com/api/auth/signup")

	require.NotEqual(t, login, signup, "sibling routes may carry different policies")
}

func TestCORSRouteKey_SeparatesHosts(t *testing.T) {
	a := corsRouteKey("https://example.com/api/auth/login")
	b := corsRouteKey("https://other.example.com/api/auth/login")

	require.NotEqual(t, a, b)
}

func TestCORSScan_DoesNotReplayStateChangingRequests(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "cors-post", Title: "cors-post"})
	require.NoError(t, err)

	var mu sync.Mutex
	var methods []string
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		methods = append(methods, r.Method)
		bodies = append(bodies, string(body))
		mu.Unlock()
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cleanupIssues(t, db.CorsCode)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/orders", strings.NewReader(`{"item":"gold-bar","qty":1}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	result := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: workspace.ID,
		},
	})
	require.NoError(t, result.Err)

	mu.Lock()
	methods = nil
	bodies = nil
	mu.Unlock()

	CORSScan(result.History, CORSScanOptions{
		ActiveModuleOptions: ActiveModuleOptions{WorkspaceID: workspace.ID, HTTPClient: http.DefaultClient},
	})

	mu.Lock()
	defer mu.Unlock()
	// Observing a CORS policy never requires re-submitting the order. Replaying the
	// original POST once per probe would place N orders on a real target.
	for i, m := range methods {
		require.NotEqual(t, http.MethodPost, m, "probe %d replayed the state-changing method", i)
		require.Empty(t, bodies[i], "probe %d replayed the request body", i)
	}
	require.NotEmpty(t, methods, "the audit should still probe the route")
}

// --- review fixes ---

func TestCORSScan_DoesNotFollowRedirectsToOtherHosts(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "cors-redir", Title: "cors-redir"})
	require.NoError(t, err)

	// A third-party identity provider with a permissive policy of its own.
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Write([]byte(`{}`))
	}))
	defer idp.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, idp.URL+"/authorize", http.StatusFound)
	}))
	defer target.Close()

	cleanupIssues(t, db.CorsCode)

	// The history item must be the scanned route itself, not the redirect target,
	// or the test exercises the helper rather than the module.
	req, err := http.NewRequest(http.MethodGet, target.URL+"/api/me", nil)
	require.NoError(t, err)
	result := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		Client:        http_utils.WithoutRedirects(http.DefaultClient),
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: workspace.ID,
		},
	})
	require.NoError(t, result.Err)
	require.Equal(t, 302, result.History.StatusCode)

	CORSScan(result.History, CORSScanOptions{
		ActiveModuleOptions: ActiveModuleOptions{WorkspaceID: workspace.ID, HTTPClient: http.DefaultClient},
	})

	require.Equal(t, int64(0), corsIssueCount(t),
		"a redirect target's CORS policy is not the scanned route's policy")
}

func TestCORSBypassOrigins_OmitsOwnOriginOnPlainHTTPTarget(t *testing.T) {
	origins := bypassOriginStrings(corsBypassOrigins("http", "app.internal", nil))

	require.NotContains(t, origins, "http://app.internal",
		"on an http:// target this is the page's own origin, not a cross-origin probe")
}

func TestCORSBypassOrigins_KeepsSchemeDowngradeOnHTTPSTarget(t *testing.T) {
	origins := bypassOriginStrings(corsBypassOrigins("https", "app.internal", nil))

	require.Contains(t, origins, "http://app.internal",
		"on an https:// target the plaintext origin is genuinely a different origin")
}

func TestHarvestCORSDomains_FindsBrandBehindThirdPartyScripts(t *testing.T) {
	// The shape of a real page: analytics and CDN tags precede any mention of the
	// application's own domain.
	body := `<script src="https://cdn.hotjar.com/hotjar.js"></script>
	<script src="https://cdn.segment.io/analytics.js"></script>
	<script src="https://widget.intercom.io/widget.js"></script>
	<a href="mailto:support@trusted-brand.test">support</a>`

	got := harvestCORSDomains(body, "127.0.0.1:21000")

	require.Contains(t, got, "trusted-brand.test",
		"third-party asset hosts must not crowd out the application's own domain")
}

func TestHarvestCORSDomains_BoundsTheBodyItScans(t *testing.T) {
	var body strings.Builder
	body.WriteString(strings.Repeat("x", maxCORSHarvestBytes+1))
	body.WriteString("https://late-brand.test/")

	got := harvestCORSDomains(body.String(), "target.local")

	require.NotContains(t, got, "late-brand.test",
		"content past the harvest budget must not be scanned at all")
}

func TestCORSScan_HostWideMiddlewareIsNotReportedOncePerRoute(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "cors-flood", Title: "cors-flood"})
	require.NoError(t, err)

	// One middleware, reflecting any origin, in front of every route.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cleanupIssues(t, db.CorsCode)

	claims := map[string]int{}
	reportGate := func(signature string) bool {
		claims[signature]++
		return claims[signature] <= 3
	}

	for i := range 40 {
		CORSScan(newCORSHistory(t, workspace.ID, fmt.Sprintf("%s/api/resource/%d", srv.URL, i)), CORSScanOptions{
			ActiveModuleOptions: ActiveModuleOptions{WorkspaceID: workspace.ID, HTTPClient: http.DefaultClient},
			ReportGate:          reportGate,
		})
	}

	require.Equal(t, int64(3), corsIssueCount(t),
		"40 routes behind one middleware is one misconfiguration, not 40 findings")
}

func TestCORSScan_ReleasesTheRouteWhenNothingWasDecided(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "cors-release", Title: "cors-release"})
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	history := newCORSHistory(t, workspace.ID, srv.URL+"/api/thing")
	srv.Close() // every probe from here on fails

	cleanupIssues(t, db.CorsCode)

	var released []string
	CORSScan(history, CORSScanOptions{
		ActiveModuleOptions: ActiveModuleOptions{WorkspaceID: workspace.ID, HTTPClient: http.DefaultClient},
		RouteGate:           func(string) bool { return true },
		RouteRelease:        func(route string) { released = append(released, route) },
	})

	require.Len(t, released, 1,
		"a route whose probes all failed must go back to the pool, not be retired for the scan")
}

func TestCORSScan_SweepGateBoundsAllowlistProbing(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "cors-sweep", Title: "cors-sweep"})
	require.NoError(t, err)

	var mu sync.Mutex
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.Write([]byte(`no cors here`))
	}))
	defer srv.Close()

	cleanupIssues(t, db.CorsCode)
	history := newCORSHistory(t, workspace.ID, srv.URL+"/api/plain")

	mu.Lock()
	requests = 0
	mu.Unlock()

	CORSScan(history, CORSScanOptions{
		ActiveModuleOptions: ActiveModuleOptions{WorkspaceID: workspace.ID, HTTPClient: http.DefaultClient},
		SweepGate:           func(string) bool { return false },
	})

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 2, requests,
		"with the host's sweep budget spent, only the baseline and arbitrary probes should be sent")
}
