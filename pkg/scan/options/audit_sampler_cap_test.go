package options

import "testing"

// Request smuggling is a property of the frontend/origin pair, not of a URL, so
// the sampler must be able to bound how often it repeats per host.
func TestShouldRunCappedBoundsPerHost(t *testing.T) {
	s := NewAuditSampler(DefaultAuditSamplingConfig())

	allowed := 0
	for i := 0; i < 10; i++ {
		if s.ShouldRunCapped(AuditTypeRequestSmuggling, "http://example.com/page", 3) {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("allowed %d runs for one host, want 3", allowed)
	}
}

func TestShouldRunCappedCountsDistinctPathsOnSameHost(t *testing.T) {
	s := NewAuditSampler(DefaultAuditSamplingConfig())
	urls := []string{"http://example.com/a", "http://example.com/b", "http://example.com/c",
		"http://example.com/d", "http://example.com/e"}

	allowed := 0
	for _, u := range urls {
		if s.ShouldRunCapped(AuditTypeRequestSmuggling, u, 2) {
			allowed++
		}
	}
	if allowed != 2 {
		t.Fatalf("allowed %d runs across paths of one host, want 2", allowed)
	}
}

// Different hosts, and different ports on the same host, are separate targets.
func TestShouldRunCappedIsPerHostAndPort(t *testing.T) {
	s := NewAuditSampler(DefaultAuditSamplingConfig())
	for _, u := range []string{"http://a.com/", "http://b.com/", "http://a.com:8080/"} {
		if !s.ShouldRunCapped(AuditTypeRequestSmuggling, u, 1) {
			t.Fatalf("first run for %s was refused", u)
		}
	}
	if s.ShouldRunCapped(AuditTypeRequestSmuggling, "http://a.com/other", 1) {
		t.Fatalf("second run on a.com should be refused")
	}
}

func TestShouldRunCappedNonPositiveMeansUnlimited(t *testing.T) {
	s := NewAuditSampler(DefaultAuditSamplingConfig())
	for i := 0; i < 5; i++ {
		if !s.ShouldRunCapped(AuditTypeSNI, "http://example.com/", 0) {
			t.Fatalf("cap<=0 must not limit; refused at call %d", i)
		}
	}
}

// Capping one audit type must not consume another's budget.
func TestShouldRunCappedIsPerAuditType(t *testing.T) {
	s := NewAuditSampler(DefaultAuditSamplingConfig())
	if !s.ShouldRunCapped(AuditTypeRequestSmuggling, "http://example.com/", 1) {
		t.Fatal("first smuggling run refused")
	}
	if !s.ShouldRunCapped(AuditTypeHTTPVersions, "http://example.com/", 1) {
		t.Fatal("http-versions budget was consumed by the smuggling audit")
	}
}
