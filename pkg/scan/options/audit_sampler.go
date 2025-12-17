package options

import (
	"net/url"
	"sync"
)

// AuditType represents the type of expensive audit that can be sampled.
type AuditType string

const (
	AuditTypeRequestSmuggling AuditType = "request_smuggling"
	AuditTypeSNI              AuditType = "sni"
	AuditTypeHTTPVersions     AuditType = "http_versions"
)

// AuditSamplingConfig defines the sampling rate for each audit type.
// A value of N means the audit runs on 1 out of every N history items per host.
// A value of 0 or 1 means no sampling (always run when conditions are met).
type AuditSamplingConfig struct {
	RequestSmuggling int `json:"request_smuggling"` // Default: 3 (run on 1/3 items per host)
	SNI              int `json:"sni"`               // Default: 10 (run on 1/10 items per host)
	HTTPVersions     int `json:"http_versions"`     // Default: 3 (run on 1/3 items per host)
}

// DefaultAuditSamplingConfig returns sensible default sampling rates.
func DefaultAuditSamplingConfig() AuditSamplingConfig {
	return AuditSamplingConfig{
		RequestSmuggling: 3,  // Run on ~33% of applicable items per host
		SNI:              10, // Run on ~20% of applicable items per host
		HTTPVersions:     3,  // Run on ~33% of applicable items per host
	}
}

// AuditSampler tracks per-host counters for certain audits and decides
// whether a specific audit should run based on sampling configuration.
// It is safe for concurrent use.
type AuditSampler struct {
	mu       sync.RWMutex
	config   AuditSamplingConfig
	counters map[AuditType]map[string]int // AuditType -> host -> counter
}

// NewAuditSampler creates a new AuditSampler with the given configuration.
func NewAuditSampler(config AuditSamplingConfig) *AuditSampler {
	return &AuditSampler{
		config: config,
		counters: map[AuditType]map[string]int{
			AuditTypeRequestSmuggling: make(map[string]int),
			AuditTypeSNI:              make(map[string]int),
			AuditTypeHTTPVersions:     make(map[string]int),
		},
	}
}

// ShouldRun determines if the specified audit should run for the given URL.
// It increments the internal counter and returns true based on sampling rate.
// For sampling rate N: returns true on every Nth call per host.
// A sampling rate of 0 or 1 means always return true (no sampling).
func (s *AuditSampler) ShouldRun(auditType AuditType, rawURL string) bool {
	host := extractHost(rawURL)
	if host == "" {
		// Can't determine host, default to allowing the audit
		return true
	}

	samplingRate := s.getSamplingRate(auditType)
	if samplingRate <= 1 {
		// No sampling, always run
		return true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Get or initialize the counter map for this audit type
	hostCounters, exists := s.counters[auditType]
	if !exists {
		hostCounters = make(map[string]int)
		s.counters[auditType] = hostCounters
	}

	// Increment counter for this host
	hostCounters[host]++
	count := hostCounters[host]

	// Run on every Nth item (1st, N+1th, 2N+1th, etc.)
	return (count-1)%samplingRate == 0
}

// GetStats returns the current counter state for debugging/monitoring.
func (s *AuditSampler) GetStats() map[AuditType]map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Deep copy to avoid races
	result := make(map[AuditType]map[string]int)
	for auditType, hostCounters := range s.counters {
		result[auditType] = make(map[string]int)
		for host, count := range hostCounters {
			result[auditType][host] = count
		}
	}
	return result
}

// Reset clears all counters. Useful for testing or starting a new scan phase.
func (s *AuditSampler) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for auditType := range s.counters {
		s.counters[auditType] = make(map[string]int)
	}
}

// getSamplingRate returns the sampling rate for the given audit type.
func (s *AuditSampler) getSamplingRate(auditType AuditType) int {
	switch auditType {
	case AuditTypeRequestSmuggling:
		return s.config.RequestSmuggling
	case AuditTypeSNI:
		return s.config.SNI
	case AuditTypeHTTPVersions:
		return s.config.HTTPVersions
	default:
		return 1 // Unknown type, don't sample
	}
}

// extractHost extracts the host from a URL string.
func extractHost(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}
