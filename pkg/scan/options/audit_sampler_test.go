package options

import (
	"testing"
)

func TestAuditSampler_ShouldRun(t *testing.T) {
	config := AuditSamplingConfig{
		RequestSmuggling: 3,
		SNI:              5,
		HTTPVersions:     3,
	}
	sampler := NewAuditSampler(config)

	testURL := "https://example.com/path"

	// Test Request Smuggling - should run on 1st, 4th, 7th calls (rate=3)
	expectedResults := []bool{true, false, false, true, false, false, true}
	for i, expected := range expectedResults {
		result := sampler.ShouldRun(AuditTypeRequestSmuggling, testURL)
		if result != expected {
			t.Errorf("Call %d for RequestSmuggling: expected %v, got %v", i+1, expected, result)
		}
	}

	// Reset and test SNI with rate=5
	sampler.Reset()
	expectedSNI := []bool{true, false, false, false, false, true, false, false, false, false, true}
	for i, expected := range expectedSNI {
		result := sampler.ShouldRun(AuditTypeSNI, testURL)
		if result != expected {
			t.Errorf("Call %d for SNI: expected %v, got %v", i+1, expected, result)
		}
	}
}

func TestAuditSampler_PerHostTracking(t *testing.T) {
	config := AuditSamplingConfig{
		RequestSmuggling: 3,
		SNI:              3,
		HTTPVersions:     3,
	}
	sampler := NewAuditSampler(config)

	// Different hosts should have independent counters
	host1 := "https://example.com/path"
	host2 := "https://other.com/path"

	// First call for each host should return true
	if !sampler.ShouldRun(AuditTypeRequestSmuggling, host1) {
		t.Error("First call for host1 should return true")
	}
	if !sampler.ShouldRun(AuditTypeRequestSmuggling, host2) {
		t.Error("First call for host2 should return true")
	}

	// Second and third calls for each host should return false
	if sampler.ShouldRun(AuditTypeRequestSmuggling, host1) {
		t.Error("Second call for host1 should return false")
	}
	if sampler.ShouldRun(AuditTypeRequestSmuggling, host2) {
		t.Error("Second call for host2 should return false")
	}
	if sampler.ShouldRun(AuditTypeRequestSmuggling, host1) {
		t.Error("Third call for host1 should return false")
	}
	if sampler.ShouldRun(AuditTypeRequestSmuggling, host2) {
		t.Error("Third call for host2 should return false")
	}

	// Fourth call for each host should return true
	if !sampler.ShouldRun(AuditTypeRequestSmuggling, host1) {
		t.Error("Fourth call for host1 should return true")
	}
	if !sampler.ShouldRun(AuditTypeRequestSmuggling, host2) {
		t.Error("Fourth call for host2 should return true")
	}
}

func TestAuditSampler_NoSampling(t *testing.T) {
	// Sampling rate of 0 or 1 means no sampling (always run)
	config := AuditSamplingConfig{
		RequestSmuggling: 0,
		SNI:              1,
		HTTPVersions:     3,
	}
	sampler := NewAuditSampler(config)

	testURL := "https://example.com/path"

	// Rate 0 should always return true
	for i := 0; i < 5; i++ {
		if !sampler.ShouldRun(AuditTypeRequestSmuggling, testURL) {
			t.Errorf("Call %d for RequestSmuggling (rate=0) should return true", i+1)
		}
	}

	// Rate 1 should always return true
	for i := 0; i < 5; i++ {
		if !sampler.ShouldRun(AuditTypeSNI, testURL) {
			t.Errorf("Call %d for SNI (rate=1) should return true", i+1)
		}
	}
}

func TestAuditSampler_InvalidURL(t *testing.T) {
	config := DefaultAuditSamplingConfig()
	sampler := NewAuditSampler(config)

	// Invalid URLs should default to allowing the audit
	invalidURL := "not-a-valid-url"
	if !sampler.ShouldRun(AuditTypeRequestSmuggling, invalidURL) {
		t.Error("Invalid URL should return true (allow audit)")
	}
}

func TestAuditSampler_GetStats(t *testing.T) {
	config := DefaultAuditSamplingConfig()
	sampler := NewAuditSampler(config)

	// Make some calls
	sampler.ShouldRun(AuditTypeRequestSmuggling, "https://example.com/a")
	sampler.ShouldRun(AuditTypeRequestSmuggling, "https://example.com/b")
	sampler.ShouldRun(AuditTypeSNI, "https://example.com/a")

	stats := sampler.GetStats()

	// Verify stats contain expected data
	if stats[AuditTypeRequestSmuggling]["example.com"] != 2 {
		t.Errorf("Expected 2 calls for RequestSmuggling on example.com, got %d",
			stats[AuditTypeRequestSmuggling]["example.com"])
	}
	if stats[AuditTypeSNI]["example.com"] != 1 {
		t.Errorf("Expected 1 call for SNI on example.com, got %d",
			stats[AuditTypeSNI]["example.com"])
	}
}

func TestDefaultAuditSamplingConfig(t *testing.T) {
	config := DefaultAuditSamplingConfig()

	// Verify defaults are sensible
	if config.RequestSmuggling < 1 {
		t.Error("RequestSmuggling sampling rate should be at least 1")
	}
	if config.SNI < 1 {
		t.Error("SNI sampling rate should be at least 1")
	}
	if config.HTTPVersions < 1 {
		t.Error("HTTPVersions sampling rate should be at least 1")
	}
}
