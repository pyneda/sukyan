package payloads

import (
	"testing"

	"github.com/pyneda/sukyan/pkg/http_utils"
)

func TestFilterPayloadsByCSP(t *testing.T) {
	testPayloads := []PayloadInterface{
		XSSPayload{
			Value:      `<script>alert(1)</script>`,
			Categories: []XSSPayloadCategory{CategoryTagInjection},
		},
		XSSPayload{
			Value:      `<img src=x onerror=alert(1)>`,
			Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler},
		},
		XSSPayload{
			Value:      `" onclick=alert(1) "`,
			Categories: []XSSPayloadCategory{CategoryAttributeBreaking, CategoryEventHandler},
		},
		XSSPayload{
			Value:      `data:text/html,<script>alert(1)</script>`,
			Categories: []XSSPayloadCategory{CategoryURLScheme},
		},
	}

	t.Run("no CSP returns all payloads", func(t *testing.T) {
		result := filterPayloadsByCSP(testPayloads, nil)
		if len(result.Payloads) != len(testPayloads) {
			t.Errorf("expected %d payloads, got %d", len(testPayloads), len(result.Payloads))
		}
		if result.OriginalCount != len(testPayloads) {
			t.Errorf("expected OriginalCount %d, got %d", len(testPayloads), result.OriginalCount)
		}
	})

	t.Run("report-only CSP returns all payloads", func(t *testing.T) {
		csp := http_utils.ParseCSP("script-src 'self'")
		csp.ReportOnly = true
		result := filterPayloadsByCSP(testPayloads, csp)
		if len(result.Payloads) != len(testPayloads) {
			t.Errorf("expected %d payloads, got %d", len(testPayloads), len(result.Payloads))
		}
	})

	t.Run("CSP blocking inline scripts filters script tags", func(t *testing.T) {
		csp := http_utils.ParseCSP("script-src 'self'")
		result := filterPayloadsByCSP(testPayloads, csp)

		if result.InlineScriptBlocked == 0 {
			t.Error("expected at least one inline script blocked")
		}

		for _, p := range result.Payloads {
			xss, ok := p.(XSSPayload)
			if !ok {
				continue
			}
			if isInlineScriptPayload(xss) {
				t.Errorf("inline script payload should be filtered: %s", xss.Value)
			}
		}
	})

	t.Run("CSP blocking inline scripts keeps event handlers", func(t *testing.T) {
		csp := http_utils.ParseCSP("script-src 'self'")
		result := filterPayloadsByCSP(testPayloads, csp)

		hasEventHandler := false
		for _, p := range result.Payloads {
			xss, ok := p.(XSSPayload)
			if !ok {
				continue
			}
			if hasCategory(xss, CategoryEventHandler) {
				hasEventHandler = true
				break
			}
		}
		if !hasEventHandler {
			t.Error("event handler payloads should be kept")
		}
	})

	t.Run("CSP without data filters data URI payloads", func(t *testing.T) {
		csp := http_utils.ParseCSP("script-src 'self'")
		result := filterPayloadsByCSP(testPayloads, csp)

		if result.DataURIBlocked == 0 {
			t.Error("expected at least one data URI blocked")
		}

		for _, p := range result.Payloads {
			xss, ok := p.(XSSPayload)
			if !ok {
				continue
			}
			if isDataURIPayload(xss) {
				t.Errorf("data URI payload should be filtered: %s", xss.Value)
			}
		}
	})

	t.Run("CSP with data keeps data URI payloads", func(t *testing.T) {
		csp := http_utils.ParseCSP("script-src 'self' data:")
		result := filterPayloadsByCSP(testPayloads, csp)

		if result.DataURIBlocked != 0 {
			t.Errorf("expected no data URIs blocked, got %d", result.DataURIBlocked)
		}

		hasDataURI := false
		for _, p := range result.Payloads {
			xss, ok := p.(XSSPayload)
			if !ok {
				continue
			}
			if isDataURIPayload(xss) {
				hasDataURI = true
				break
			}
		}
		if !hasDataURI {
			t.Error("data URI payloads should be kept when CSP allows data:")
		}
	})

	t.Run("CSP with unsafe-inline keeps inline scripts", func(t *testing.T) {
		csp := http_utils.ParseCSP("script-src 'self' 'unsafe-inline'")
		result := filterPayloadsByCSP(testPayloads, csp)

		if result.InlineScriptBlocked != 0 {
			t.Errorf("expected no inline scripts blocked, got %d", result.InlineScriptBlocked)
		}

		hasInlineScript := false
		for _, p := range result.Payloads {
			xss, ok := p.(XSSPayload)
			if !ok {
				continue
			}
			if isInlineScriptPayload(xss) {
				hasInlineScript = true
				break
			}
		}
		if !hasInlineScript {
			t.Error("inline script payloads should be kept with unsafe-inline")
		}
	})

	t.Run("CSPFilterResult counts are accurate", func(t *testing.T) {
		csp := http_utils.ParseCSP("script-src 'self'")
		result := filterPayloadsByCSP(testPayloads, csp)

		expectedBlocked := result.InlineScriptBlocked + result.DataURIBlocked
		actualFiltered := result.OriginalCount - result.FilteredCount

		if expectedBlocked != actualFiltered {
			t.Errorf("count mismatch: blocked=%d (inline=%d, data=%d), filtered=%d",
				expectedBlocked, result.InlineScriptBlocked, result.DataURIBlocked, actualFiltered)
		}

		if result.BlocksInline != true {
			t.Error("expected BlocksInline to be true")
		}
		if result.AllowsData != false {
			t.Error("expected AllowsData to be false")
		}
	})
}

func TestIsInlineScriptPayload(t *testing.T) {
	tests := []struct {
		name     string
		payload  XSSPayload
		expected bool
	}{
		{
			name: "script tag",
			payload: XSSPayload{
				Value:      `<script>alert(1)</script>`,
				Categories: []XSSPayloadCategory{CategoryTagInjection},
			},
			expected: true,
		},
		{
			name: "event handler",
			payload: XSSPayload{
				Value:      `<img src=x onerror=alert(1)>`,
				Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler},
			},
			expected: false,
		},
		{
			name: "attribute breaking",
			payload: XSSPayload{
				Value:      `" onclick=alert(1)`,
				Categories: []XSSPayloadCategory{CategoryAttributeBreaking},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInlineScriptPayload(tt.payload)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsDataURIPayload(t *testing.T) {
	tests := []struct {
		name     string
		payload  XSSPayload
		expected bool
	}{
		{
			name:     "data URI",
			payload:  XSSPayload{Value: `data:text/html,<script>alert(1)</script>`},
			expected: true,
		},
		{
			name:     "javascript URI",
			payload:  XSSPayload{Value: `javascript:alert(1)`},
			expected: false,
		},
		{
			name:     "script tag",
			payload:  XSSPayload{Value: `<script>alert(1)</script>`},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDataURIPayload(tt.payload)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestHasCategory(t *testing.T) {
	payload := XSSPayload{
		Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler},
	}

	if !hasCategory(payload, CategoryTagInjection) {
		t.Error("should have CategoryTagInjection")
	}
	if !hasCategory(payload, CategoryEventHandler) {
		t.Error("should have CategoryEventHandler")
	}
	if hasCategory(payload, CategoryAttributeBreaking) {
		t.Error("should not have CategoryAttributeBreaking")
	}
}
