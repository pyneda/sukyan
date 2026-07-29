package options

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestResolveCrawlOptionsFallsBackToDefaults(t *testing.T) {
	defaults := DefaultCrawlOptions()

	tests := []struct {
		name    string
		options *FullScanCrawlOptions
	}{
		{"nil options", nil},
		{"empty options", &FullScanCrawlOptions{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.options.Resolve(); !reflect.DeepEqual(got, defaults) {
				t.Errorf("Resolve() = %+v, want defaults %+v", got, defaults)
			}
		})
	}
}

// Explicit false and 0 are settings, not absence: false disables an interaction
// and MaxPagesWithSameParameters: 0 means unlimited. A non-pointer field could
// not tell them apart from "unset" and would silently restore the defaults.
func TestResolveCrawlOptionsKeepsZeroValuedOverrides(t *testing.T) {
	disabled := false
	unlimited := 0

	resolved := (&FullScanCrawlOptions{
		SubmitForms:                &disabled,
		ClickButtons:               &disabled,
		CaptureClientNavigation:    &disabled,
		WaitForStablePage:          &disabled,
		MaxPagesWithSameParameters: &unlimited,
		SeedPaths:                  []string{},
		ExcludeExtensions:          []string{},
	}).Resolve()

	if resolved.SubmitForms || resolved.ClickButtons || resolved.CaptureClientNavigation || resolved.WaitForStablePage {
		t.Errorf("explicit false overrides were lost: %+v", resolved)
	}
	if resolved.MaxPagesWithSameParameters != 0 {
		t.Errorf("MaxPagesWithSameParameters = %d, want 0", resolved.MaxPagesWithSameParameters)
	}
	if len(resolved.SeedPaths) != 0 {
		t.Errorf("SeedPaths = %v, want empty", resolved.SeedPaths)
	}
	if len(resolved.ExcludeExtensions) != 0 {
		t.Errorf("ExcludeExtensions = %v, want empty", resolved.ExcludeExtensions)
	}
}

func TestResolveCrawlOptionsAppliesOverrides(t *testing.T) {
	interaction := 45
	navigation := 30
	userAgent := "sukyan-engagement/1.0"
	once := true

	resolved := (&FullScanCrawlOptions{
		InteractionTimeoutSeconds: &interaction,
		NavigationTimeoutSeconds:  &navigation,
		SubmitEachFormOnce:        &once,
		UserAgent:                 &userAgent,
		SeedPaths:                 []string{"/robots.txt"},
	}).Resolve()

	if resolved.InteractionTimeout != 45*time.Second {
		t.Errorf("InteractionTimeout = %s, want 45s", resolved.InteractionTimeout)
	}
	if resolved.NavigationTimeout != 30*time.Second {
		t.Errorf("NavigationTimeout = %s, want 30s", resolved.NavigationTimeout)
	}
	if !resolved.SubmitEachFormOnce {
		t.Error("SubmitEachFormOnce = false, want true")
	}
	if resolved.UserAgent != userAgent {
		t.Errorf("UserAgent = %q, want %q", resolved.UserAgent, userAgent)
	}
	if !reflect.DeepEqual(resolved.SeedPaths, []string{"/robots.txt"}) {
		t.Errorf("SeedPaths = %v, want [/robots.txt]", resolved.SeedPaths)
	}

	// Untouched fields keep their defaults rather than becoming zero values.
	if resolved.PageSetupTimeout != DefaultCrawlOptions().PageSetupTimeout {
		t.Errorf("PageSetupTimeout = %s, want default", resolved.PageSetupTimeout)
	}
}

// Scans store their options as JSON (db/scan.go), so an override only survives if
// it round-trips. omitempty on a slice drops an explicitly empty one, which is why
// the slice fields carry no omitempty.
func TestCrawlOptionsSurviveJSONRoundTrip(t *testing.T) {
	disabled := false
	unlimited := 0

	original := &FullScanCrawlOptions{
		SubmitForms:                &disabled,
		MaxPagesWithSameParameters: &unlimited,
		SeedPaths:                  []string{},
	}

	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshalling crawl options: %v", err)
	}

	var decoded FullScanCrawlOptions
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshalling crawl options: %v", err)
	}

	resolved := decoded.Resolve()
	if resolved.SubmitForms {
		t.Errorf("SubmitForms reverted to the default after round trip: %s", encoded)
	}
	if resolved.MaxPagesWithSameParameters != 0 {
		t.Errorf("MaxPagesWithSameParameters reverted to the default after round trip: %s", encoded)
	}
	if len(resolved.SeedPaths) != 0 {
		t.Errorf("SeedPaths reverted to the default after round trip: %s", encoded)
	}
}

// A scan created before crawl options existed has no crawl_options key at all.
func TestScanOptionsWithoutCrawlOptionsResolveToDefaults(t *testing.T) {
	var scanOptions FullScanOptions
	if err := json.Unmarshal([]byte(`{"workspace_id":1,"max_depth":3}`), &scanOptions); err != nil {
		t.Fatalf("unmarshalling scan options: %v", err)
	}

	if scanOptions.CrawlOptions != nil {
		t.Fatalf("CrawlOptions = %+v, want nil", scanOptions.CrawlOptions)
	}
	if got := scanOptions.CrawlOptions.Resolve(); !reflect.DeepEqual(got, DefaultCrawlOptions()) {
		t.Errorf("Resolve() = %+v, want defaults", got)
	}
}
