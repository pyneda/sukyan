package payloads

import (
	"strings"

	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/web"
	"github.com/rs/zerolog/log"
)

// DOMXSSCSPFilterResult contains the result of CSP-aware payload filtering
type DOMXSSCSPFilterResult struct {
	Payloads             []DOMXSSPayload
	OriginalCount        int
	FilteredCount        int
	InlineScriptBlocked  int
	EvalBlocked          int
	DataURIBlocked       int
	JavascriptURIBlocked int
	BlocksInline         bool
	BlocksEval           bool
	BlocksJavascriptURI  bool
	AllowsData           bool
}

// GetCSPAwareDOMXSSPayloads filters DOM XSS payloads based on CSP policy
func GetCSPAwareDOMXSSPayloads(csp *http_utils.CSPPolicy) DOMXSSCSPFilterResult {
	allPayloads := GetDOMXSSPayloads()
	return FilterDOMXSSPayloadsByCSP(allPayloads, csp)
}

// GetCSPAwareDOMXSSPayloadsWithMarker filters DOM XSS payloads based on CSP policy using a specific marker
func GetCSPAwareDOMXSSPayloadsWithMarker(marker string, csp *http_utils.CSPPolicy) DOMXSSCSPFilterResult {
	allPayloads := GetDOMXSSPayloadsWithMarker(marker)
	return FilterDOMXSSPayloadsByCSP(allPayloads, csp)
}

// FilterDOMXSSPayloadsByCSP filters a list of DOM XSS payloads based on CSP policy
func FilterDOMXSSPayloadsByCSP(payloads []DOMXSSPayload, csp *http_utils.CSPPolicy) DOMXSSCSPFilterResult {
	result := DOMXSSCSPFilterResult{
		OriginalCount: len(payloads),
	}

	// If no CSP or report-only, return all payloads
	if csp == nil || csp.ReportOnly {
		result.Payloads = payloads
		result.FilteredCount = len(payloads)
		return result
	}

	// Determine CSP restrictions
	result.BlocksInline = csp.BlocksInlineScripts()
	result.BlocksEval = csp.BlocksEval()
	result.AllowsData = csp.AllowsData(http_utils.DirectiveScriptSrc)

	// Check if javascript: URIs are blocked
	// If CSP has script-src or default-src without 'unsafe-inline', javascript: URIs are blocked
	result.BlocksJavascriptURI = result.BlocksInline

	var filtered []DOMXSSPayload
	for _, payload := range payloads {
		// Check if this payload type is blocked by CSP
		if result.BlocksInline && isInlineScriptDOMXSSPayload(payload) {
			result.InlineScriptBlocked++
			continue
		}

		if result.BlocksEval && isEvalBasedDOMXSSPayload(payload) {
			result.EvalBlocked++
			continue
		}

		if !result.AllowsData && isDataURIDOMXSSPayload(payload) {
			result.DataURIBlocked++
			continue
		}

		if result.BlocksJavascriptURI && isJavascriptURIDOMXSSPayload(payload) {
			result.JavascriptURIBlocked++
			continue
		}

		filtered = append(filtered, payload)
	}

	result.Payloads = filtered
	result.FilteredCount = len(filtered)

	return result
}

// isInlineScriptDOMXSSPayload checks if a payload relies on inline script execution
// These are payloads that use <script> tags directly
func isInlineScriptDOMXSSPayload(payload DOMXSSPayload) bool {
	lower := strings.ToLower(payload.Value)

	// Check for <script> tags
	if strings.Contains(lower, "<script") {
		return true
	}

	return false
}

// isEvalBasedDOMXSSPayload checks if a payload relies on eval() or similar functions
// These target sinks like eval(), setTimeout(string), setInterval(string), Function()
func isEvalBasedDOMXSSPayload(payload DOMXSSPayload) bool {
	for _, sinkType := range payload.TargetSinks {
		if sinkType == web.SinkTypeJSExecution {
			// These are payloads designed for eval-like sinks
			// They typically don't have HTML tags
			lower := strings.ToLower(payload.Value)
			if !strings.Contains(lower, "<") {
				return true
			}
		}
	}
	return false
}

// isDataURIDOMXSSPayload checks if a payload uses data: URI scheme
func isDataURIDOMXSSPayload(payload DOMXSSPayload) bool {
	lower := strings.ToLower(payload.Value)
	return strings.Contains(lower, "data:")
}

// isJavascriptURIDOMXSSPayload checks if a payload uses javascript: URI scheme
func isJavascriptURIDOMXSSPayload(payload DOMXSSPayload) bool {
	lower := strings.ToLower(payload.Value)
	return strings.Contains(lower, "javascript:")
}

// LogCSPFilterStats logs the CSP filtering statistics
func (r *DOMXSSCSPFilterResult) LogCSPFilterStats(url string) {
	if r.OriginalCount == r.FilteredCount {
		return
	}

	logEvent := log.Info().
		Str("url", url).
		Int("original_payloads", r.OriginalCount).
		Int("filtered_payloads", r.FilteredCount).
		Bool("blocks_inline", r.BlocksInline).
		Bool("blocks_eval", r.BlocksEval).
		Bool("blocks_javascript_uri", r.BlocksJavascriptURI).
		Bool("allows_data", r.AllowsData)

	if r.InlineScriptBlocked > 0 {
		logEvent = logEvent.Int("inline_script_blocked", r.InlineScriptBlocked)
	}
	if r.EvalBlocked > 0 {
		logEvent = logEvent.Int("eval_blocked", r.EvalBlocked)
	}
	if r.DataURIBlocked > 0 {
		logEvent = logEvent.Int("data_uri_blocked", r.DataURIBlocked)
	}
	if r.JavascriptURIBlocked > 0 {
		logEvent = logEvent.Int("javascript_uri_blocked", r.JavascriptURIBlocked)
	}

	logEvent.Msg("DOM XSS payloads filtered by CSP")
}

// GetEffectivePayloadsForSinkType returns CSP-filtered payloads for a specific sink type
func GetEffectivePayloadsForSinkType(sinkType web.DOMXSSSinkType, csp *http_utils.CSPPolicy) []DOMXSSPayload {
	payloads := GetPayloadsForSinkType(sinkType)

	if csp == nil || csp.ReportOnly {
		return payloads
	}

	result := FilterDOMXSSPayloadsByCSP(payloads, csp)
	return result.Payloads
}
