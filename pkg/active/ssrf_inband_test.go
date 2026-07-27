package active

import (
	"strings"
	"testing"
	"time"

	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/scan"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
)

func probeByName(t *testing.T, name string) ssrfProbe {
	t.Helper()
	for _, p := range append(append([]ssrfProbe{}, ssrfCoreProbes...), ssrfBypassProbes...) {
		if p.name == name {
			return p
		}
	}
	t.Fatalf("probe %q not found", name)
	return ssrfProbe{}
}

func TestSSRFProbeFired(t *testing.T) {
	aws := probeByName(t, "AWS instance metadata (IMDSv1)")
	file := probeByName(t, "Local file read via file:// scheme")
	azure := probeByName(t, "Azure instance metadata")

	tests := []struct {
		name     string
		probe    ssrfProbe
		response string
		baseline string
		want     bool
	}{
		{
			name:     "aws imds listing fetched through the sink",
			probe:    aws,
			response: `{"url":"http://169.254.169.254/latest/meta-data/","status":200,"body":"ami-id\nhostname\niam/\ninstance-id\n"}`,
			want:     true,
		},
		{
			name:     "single weak aws fingerprint is not enough",
			probe:    aws,
			response: `{"error":"could not reach instance-id service"}`,
			want:     false,
		},
		{
			name:     "sink merely echoes the injected url",
			probe:    aws,
			response: `{"url":"http://169.254.169.254/latest/meta-data/","error":"fetch failed"}`,
			want:     false,
		},
		{
			name:     "fingerprints already in the baseline are not payload-induced",
			probe:    aws,
			response: `{"body":"ami-id\ninstance-id\n"}`,
			baseline: `{"body":"ami-id\ninstance-id\n"}`,
			want:     false,
		},
		{
			name:     "etc passwd read via file scheme",
			probe:    file,
			response: `{"status":200,"body":"root:x:0:0:root:/root:/bin/bash\n"}`,
			want:     true,
		},
		{
			name:     "file probe does not fire on an error mentioning the path",
			probe:    file,
			response: `{"error":"file read: ENOENT: no such file or directory, open '/etc/passwd'"}`,
			want:     false,
		},
		{
			name:     "azure imds answered",
			probe:    azure,
			response: `{"compute":{"vmId":"x","subscriptionId":"y","azEnvironment":"AzurePublicCloud"}}`,
			want:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, got := ssrfProbeFired(tc.probe, tc.response, tc.baseline); got != tc.want {
				t.Errorf("ssrfProbeFired(%s) = %v, want %v", tc.probe.name, got, tc.want)
			}
		})
	}
}

// Every probe must be immune to the sink echoing the injected URL: no
// fingerprint may be a substring of its own payload.
func TestSSRFProbeIndicatorsAreEchoImmune(t *testing.T) {
	for _, probe := range append(append([]ssrfProbe{}, ssrfCoreProbes...), ssrfBypassProbes...) {
		if _, fired := ssrfProbeFired(probe, probe.payload, ""); fired {
			t.Errorf("probe %q fires on its own echoed payload", probe.name)
		}
		if probe.minHits > len(probe.indicators) {
			t.Errorf("probe %q needs %d hits but defines %d indicators", probe.name, probe.minHits, len(probe.indicators))
		}
	}
}

func TestIsURLLikeInsertionPoint(t *testing.T) {
	named := func(name string, valueType lib.DataType) scan.InsertionPoint {
		return scan.InsertionPoint{Type: scan.InsertionPointTypeParameter, Name: name, ValueType: valueType}
	}

	tests := []struct {
		name  string
		point scan.InsertionPoint
		mode  scan_options.ScanMode
		want  bool
	}{
		{"common name in smart mode", named("url", lib.TypeString), scan_options.ScanModeSmart, true},
		{"compound name in smart mode", named("passport_url", lib.TypeString), scan_options.ScanModeSmart, true},
		{"jwt header claim in smart mode", named("jku", lib.TypeString), scan_options.ScanModeSmart, true},
		{"url-valued unrelated name in smart mode", named("q", lib.TypeURL), scan_options.ScanModeSmart, true},
		{"unrelated name and value in smart mode", named("q", lib.TypeString), scan_options.ScanModeSmart, false},
		{"unrelated name in fuzz mode", named("q", lib.TypeString), scan_options.ScanModeFuzz, true},
		{"url-valued unrelated name in fast mode", named("q", lib.TypeURL), scan_options.ScanModeFast, false},
		{"common name in fast mode", named("src", lib.TypeString), scan_options.ScanModeFast, true},
		{
			"full body is never probed",
			scan.InsertionPoint{Type: scan.InsertionPointTypeFullBody, Name: "fullbody", ValueType: lib.TypeURL},
			scan_options.ScanModeFuzz,
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isURLLikeInsertionPoint(tc.point, tc.mode); got != tc.want {
				t.Errorf("isURLLikeInsertionPoint(%s, %s) = %v, want %v", tc.point.Name, tc.mode, got, tc.want)
			}
		})
	}
}

func TestCarriesEncodedURL(t *testing.T) {
	tests := map[string]bool{
		"http%3A%2F%2Finternal-api%3A8080%2Fflag": true,
		"https%3A%2F%2Fexample.com%2F":            true,
		"plain-segment":                           false,
		"https://example.com/":                    false, // already a URL; the ValueType check owns this
		"50%":                                     false,
		"report%202024":                           false,
	}
	for value, want := range tests {
		if got := carriesEncodedURL(value); got != want {
			t.Errorf("carriesEncodedURL(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestIsPortParameter(t *testing.T) {
	for name, want := range map[string]bool{
		"port": true, "target_port": true, "dstPort": true,
		"support": false, "export": false, "passport_url": false,
	} {
		if got := isPortParameter(name); got != want {
			t.Errorf("isPortParameter(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestSSRFTimingConfirmsConnect(t *testing.T) {
	ms := time.Millisecond

	tests := []struct {
		name       string
		refused    time.Duration
		unroutable time.Duration
		want       bool
	}{
		{"refused fast, unrouted stalls on the connect timeout", 12 * ms, 5000 * ms, true},
		{"filter rejects both without connecting", 11 * ms, 14 * ms, false},
		{"delta below the noise floor", 40 * ms, 900 * ms, false},
		{"uniformly slow endpoint fails the ratio guard", 2500 * ms, 5000 * ms, false},
		{"absolute delta met but ratio not", 1900 * ms, 4000 * ms, false},
		{"no measurement", 0, 5000 * ms, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ssrfTimingConfirmsConnect(tc.refused, tc.unroutable); got != tc.want {
				t.Errorf("ssrfTimingConfirmsConnect(%s, %s) = %v, want %v", tc.refused, tc.unroutable, got, tc.want)
			}
		})
	}
}

func TestTimingProbePayloads(t *testing.T) {
	hostPoint := scan.InsertionPoint{Type: scan.InsertionPointTypeParameter, Name: "host", Value: "example.com", ValueType: lib.TypeString}
	refused, unroutable := timingProbePayloads(hostPoint)
	if strings.Contains(refused, "://") || strings.Contains(unroutable, "://") {
		t.Errorf("host parameter must get bare host:port targets, got %q and %q", refused, unroutable)
	}

	urlPoint := scan.InsertionPoint{Type: scan.InsertionPointTypeParameter, Name: "url", Value: "https://example.com/", ValueType: lib.TypeURL}
	refused, unroutable = timingProbePayloads(urlPoint)
	if !strings.HasPrefix(refused, "http://") || !strings.HasPrefix(unroutable, "http://") {
		t.Errorf("url parameter must get URL targets, got %q and %q", refused, unroutable)
	}
}

// Budget guard: the audit runs on every URL-ish insertion point of every scanned
// item, so probe counts are a scan-cost decision, not an implementation detail.
func TestSSRFProbeBudgetPerMode(t *testing.T) {
	for mode, want := range map[scan_options.ScanMode]int{
		scan_options.ScanModeFast:  2,
		scan_options.ScanModeSmart: 5,
		scan_options.ScanModeFuzz:  9,
	} {
		if got := len(ssrfProbesForMode(mode)); got != want {
			t.Errorf("ssrfProbesForMode(%s) = %d probes, want %d", mode, got, want)
		}
	}
}
