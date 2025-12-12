package http_utils

import (
	"net/http"
	"testing"
)

func TestParseCSP(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[CSPDirective][]string
	}{
		{
			name:  "simple policy",
			input: "default-src 'self'",
			expected: map[CSPDirective][]string{
				DirectiveDefaultSrc: {"'self'"},
			},
		},
		{
			name:  "multiple directives",
			input: "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self'",
			expected: map[CSPDirective][]string{
				DirectiveDefaultSrc: {"'self'"},
				DirectiveScriptSrc:  {"'self'", "'unsafe-inline'"},
				DirectiveStyleSrc:   {"'self'"},
			},
		},
		{
			name:  "with hosts",
			input: "script-src 'self' https://example.com *.cdn.com",
			expected: map[CSPDirective][]string{
				DirectiveScriptSrc: {"'self'", "https://example.com", "*.cdn.com"},
			},
		},
		{
			name:  "with nonce",
			input: "script-src 'nonce-abc123' 'strict-dynamic'",
			expected: map[CSPDirective][]string{
				DirectiveScriptSrc: {"'nonce-abc123'", "'strict-dynamic'"},
			},
		},
		{
			name:     "empty policy",
			input:    "",
			expected: map[CSPDirective][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := ParseCSP(tt.input)
			if len(policy.Directives) != len(tt.expected) {
				t.Errorf("expected %d directives, got %d", len(tt.expected), len(policy.Directives))
			}
			for directive, expectedValues := range tt.expected {
				values, ok := policy.Directives[directive]
				if !ok {
					t.Errorf("missing directive %s", directive)
					continue
				}
				if len(values) != len(expectedValues) {
					t.Errorf("directive %s: expected %d values, got %d", directive, len(expectedValues), len(values))
				}
			}
		})
	}
}

func TestParseCSPFromHeaders(t *testing.T) {
	t.Run("CSP header", func(t *testing.T) {
		headers := http.Header{}
		headers.Set("Content-Security-Policy", "default-src 'self'")
		policy := ParseCSPFromHeaders(headers)
		if policy == nil {
			t.Fatal("expected policy, got nil")
		}
		if policy.ReportOnly {
			t.Error("expected ReportOnly to be false")
		}
	})

	t.Run("CSP report-only header", func(t *testing.T) {
		headers := http.Header{}
		headers.Set("Content-Security-Policy-Report-Only", "default-src 'self'")
		policy := ParseCSPFromHeaders(headers)
		if policy == nil {
			t.Fatal("expected policy, got nil")
		}
		if !policy.ReportOnly {
			t.Error("expected ReportOnly to be true")
		}
	})

	t.Run("no CSP header", func(t *testing.T) {
		headers := http.Header{}
		policy := ParseCSPFromHeaders(headers)
		if policy != nil {
			t.Error("expected nil policy")
		}
	})
}

func TestGetEffectiveDirective(t *testing.T) {
	policy := ParseCSP("default-src 'self'; script-src 'unsafe-inline'")

	t.Run("explicit directive", func(t *testing.T) {
		values := policy.GetEffectiveDirective(DirectiveScriptSrc)
		if len(values) != 1 || values[0] != "'unsafe-inline'" {
			t.Errorf("expected ['unsafe-inline'], got %v", values)
		}
	})

	t.Run("fallback to default-src", func(t *testing.T) {
		values := policy.GetEffectiveDirective(DirectiveImgSrc)
		if len(values) != 1 || values[0] != "'self'" {
			t.Errorf("expected ['self'], got %v", values)
		}
	})

	t.Run("script-src-elem fallback", func(t *testing.T) {
		values := policy.GetEffectiveDirective(DirectiveScriptSrcElem)
		if len(values) != 1 || values[0] != "'unsafe-inline'" {
			t.Errorf("expected ['unsafe-inline'], got %v", values)
		}
	})
}

func TestAllowsUnsafeInline(t *testing.T) {
	tests := []struct {
		name     string
		policy   string
		expected bool
	}{
		{"with unsafe-inline", "script-src 'unsafe-inline'", true},
		{"without unsafe-inline", "script-src 'self'", false},
		{"case insensitive", "script-src 'UNSAFE-INLINE'", true},
		{"fallback to default", "default-src 'unsafe-inline'", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := ParseCSP(tt.policy)
			if policy.AllowsUnsafeInline(DirectiveScriptSrc) != tt.expected {
				t.Errorf("expected %v", tt.expected)
			}
		})
	}
}

func TestAllowsUnsafeEval(t *testing.T) {
	tests := []struct {
		name     string
		policy   string
		expected bool
	}{
		{"with unsafe-eval", "script-src 'unsafe-eval'", true},
		{"without unsafe-eval", "script-src 'self'", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := ParseCSP(tt.policy)
			if policy.AllowsUnsafeEval(DirectiveScriptSrc) != tt.expected {
				t.Errorf("expected %v", tt.expected)
			}
		})
	}
}

func TestUsesNonces(t *testing.T) {
	tests := []struct {
		name     string
		policy   string
		expected bool
	}{
		{"with nonce", "script-src 'nonce-abc123DEF='", true},
		{"without nonce", "script-src 'self'", false},
		{"with hash only", "script-src 'sha256-abc123='", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := ParseCSP(tt.policy)
			if policy.UsesNonces(DirectiveScriptSrc) != tt.expected {
				t.Errorf("expected %v", tt.expected)
			}
		})
	}
}

func TestUsesHashes(t *testing.T) {
	tests := []struct {
		name     string
		policy   string
		expected bool
	}{
		{"with sha256", "script-src 'sha256-abc123DEF='", true},
		{"with sha384", "script-src 'sha384-abc123DEF='", true},
		{"with sha512", "script-src 'sha512-abc123DEF='", true},
		{"without hash", "script-src 'self'", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := ParseCSP(tt.policy)
			if policy.UsesHashes(DirectiveScriptSrc) != tt.expected {
				t.Errorf("expected %v", tt.expected)
			}
		})
	}
}

func TestGetAllowedHosts(t *testing.T) {
	policy := ParseCSP("script-src 'self' 'unsafe-inline' https://example.com *.cdn.com data:")
	hosts := policy.GetAllowedHosts(DirectiveScriptSrc)

	expected := []string{"https://example.com", "*.cdn.com"}
	if len(hosts) != len(expected) {
		t.Errorf("expected %d hosts, got %d: %v", len(expected), len(hosts), hosts)
	}
}

func TestAllowsHost(t *testing.T) {
	policy := ParseCSP("script-src 'self' https://example.com *.cdn.com")

	tests := []struct {
		host     string
		expected bool
	}{
		{"example.com", true},
		{"www.cdn.com", true},
		{"cdn.com", true},
		{"other.com", false},
		{"sub.sub.cdn.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if policy.AllowsHost(DirectiveScriptSrc, tt.host) != tt.expected {
				t.Errorf("AllowsHost(%s) = %v, expected %v", tt.host, !tt.expected, tt.expected)
			}
		})
	}
}

func TestAnalyzeWeaknesses(t *testing.T) {
	t.Run("unsafe-inline without nonce", func(t *testing.T) {
		policy := ParseCSP("script-src 'unsafe-inline'")
		weaknesses := policy.AnalyzeWeaknesses()
		found := false
		for _, w := range weaknesses {
			if w.Issue == "unsafe_inline" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected unsafe_inline weakness")
		}
	})

	t.Run("unsafe-inline with strict-dynamic is ok", func(t *testing.T) {
		policy := ParseCSP("script-src 'unsafe-inline' 'strict-dynamic' 'nonce-abc'")
		weaknesses := policy.AnalyzeWeaknesses()
		for _, w := range weaknesses {
			if w.Issue == "unsafe_inline" {
				t.Error("should not report unsafe_inline when strict-dynamic is present")
			}
		}
	})

	t.Run("missing base-uri", func(t *testing.T) {
		policy := ParseCSP("script-src 'self'")
		weaknesses := policy.AnalyzeWeaknesses()
		found := false
		for _, w := range weaknesses {
			if w.Issue == "missing_base_uri" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected missing_base_uri weakness")
		}
	})

	t.Run("bypassable CDN", func(t *testing.T) {
		policy := ParseCSP("script-src 'self' cdnjs.cloudflare.com")
		weaknesses := policy.AnalyzeWeaknesses()
		found := false
		for _, w := range weaknesses {
			if w.Issue == "bypassable_cdn" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected bypassable_cdn weakness")
		}
	})
}

func TestBlocksInlineScripts(t *testing.T) {
	tests := []struct {
		name     string
		policy   string
		expected bool
	}{
		{"no CSP", "", false},
		{"self only", "script-src 'self'", true},
		{"unsafe-inline", "script-src 'unsafe-inline'", false},
		{"unsafe-inline with nonce", "script-src 'unsafe-inline' 'nonce-abc'", true},
		{"unsafe-inline with strict-dynamic", "script-src 'unsafe-inline' 'strict-dynamic'", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := ParseCSP(tt.policy)
			if policy.BlocksInlineScripts() != tt.expected {
				t.Errorf("expected %v", tt.expected)
			}
		})
	}
}

func TestBlocksEval(t *testing.T) {
	tests := []struct {
		name     string
		policy   string
		expected bool
	}{
		{"no CSP", "", false},
		{"self only", "script-src 'self'", true},
		{"unsafe-eval", "script-src 'unsafe-eval'", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := ParseCSP(tt.policy)
			if policy.BlocksEval() != tt.expected {
				t.Errorf("expected %v", tt.expected)
			}
		})
	}
}
