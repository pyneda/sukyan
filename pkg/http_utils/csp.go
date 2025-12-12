package http_utils

import (
	"net/http"
	"regexp"
	"strings"
)

type CSPDirective string

const (
	DirectiveDefaultSrc     CSPDirective = "default-src"
	DirectiveScriptSrc      CSPDirective = "script-src"
	DirectiveStyleSrc       CSPDirective = "style-src"
	DirectiveImgSrc         CSPDirective = "img-src"
	DirectiveFontSrc        CSPDirective = "font-src"
	DirectiveConnectSrc     CSPDirective = "connect-src"
	DirectiveMediaSrc       CSPDirective = "media-src"
	DirectiveObjectSrc      CSPDirective = "object-src"
	DirectiveFrameSrc       CSPDirective = "frame-src"
	DirectiveChildSrc       CSPDirective = "child-src"
	DirectiveWorkerSrc      CSPDirective = "worker-src"
	DirectiveFrameAncestors CSPDirective = "frame-ancestors"
	DirectiveFormAction     CSPDirective = "form-action"
	DirectiveBaseURI        CSPDirective = "base-uri"
	DirectiveSandbox        CSPDirective = "sandbox"
	DirectiveReportURI      CSPDirective = "report-uri"
	DirectiveReportTo       CSPDirective = "report-to"
	DirectiveManifestSrc    CSPDirective = "manifest-src"
	DirectivePrefetchSrc    CSPDirective = "prefetch-src"
	DirectiveNavigateTo     CSPDirective = "navigate-to"
	DirectiveScriptSrcElem  CSPDirective = "script-src-elem"
	DirectiveScriptSrcAttr  CSPDirective = "script-src-attr"
	DirectiveStyleSrcElem   CSPDirective = "style-src-elem"
	DirectiveStyleSrcAttr   CSPDirective = "style-src-attr"
)

type CSPSourceValue string

const (
	SourceNone           CSPSourceValue = "'none'"
	SourceSelf           CSPSourceValue = "'self'"
	SourceUnsafeInline   CSPSourceValue = "'unsafe-inline'"
	SourceUnsafeEval     CSPSourceValue = "'unsafe-eval'"
	SourceUnsafeHashes   CSPSourceValue = "'unsafe-hashes'"
	SourceStrictDynamic  CSPSourceValue = "'strict-dynamic'"
	SourceWasmUnsafeEval CSPSourceValue = "'wasm-unsafe-eval'"
	SourceData           CSPSourceValue = "data:"
	SourceBlob           CSPSourceValue = "blob:"
	SourceMediastream    CSPSourceValue = "mediastream:"
	SourceFilesystem     CSPSourceValue = "filesystem:"
)

type CSPPolicy struct {
	Directives map[CSPDirective][]string
	ReportOnly bool
	Raw        string
}

type CSPWeakness struct {
	Directive   CSPDirective
	Issue       string
	Severity    string // "high", "medium", "low", "info"
	Exploitable bool
	Details     string
}

var noncePattern = regexp.MustCompile(`'nonce-[A-Za-z0-9+/=]+'`)
var hashPattern = regexp.MustCompile(`'sha(256|384|512)-[A-Za-z0-9+/=]+'`)

func ParseCSP(policyString string) *CSPPolicy {
	policy := &CSPPolicy{
		Directives: make(map[CSPDirective][]string),
		Raw:        policyString,
	}

	policyString = strings.TrimSpace(policyString)
	if policyString == "" {
		return policy
	}

	directives := strings.Split(policyString, ";")
	for _, directive := range directives {
		directive = strings.TrimSpace(directive)
		if directive == "" {
			continue
		}

		parts := strings.Fields(directive)
		if len(parts) == 0 {
			continue
		}

		directiveName := CSPDirective(strings.ToLower(parts[0]))
		var values []string
		if len(parts) > 1 {
			values = parts[1:]
		}
		policy.Directives[directiveName] = values
	}

	return policy
}

func ParseCSPFromHeaders(headers http.Header) *CSPPolicy {
	if csp := headers.Get("Content-Security-Policy"); csp != "" {
		policy := ParseCSP(csp)
		policy.ReportOnly = false
		return policy
	}

	if csp := headers.Get("Content-Security-Policy-Report-Only"); csp != "" {
		policy := ParseCSP(csp)
		policy.ReportOnly = true
		return policy
	}

	return nil
}

func (p *CSPPolicy) GetEffectiveDirective(directive CSPDirective) []string {
	if values, ok := p.Directives[directive]; ok {
		return values
	}

	fallbacks := map[CSPDirective]CSPDirective{
		DirectiveScriptSrcElem: DirectiveScriptSrc,
		DirectiveScriptSrcAttr: DirectiveScriptSrc,
		DirectiveStyleSrcElem:  DirectiveStyleSrc,
		DirectiveStyleSrcAttr:  DirectiveStyleSrc,
		DirectiveWorkerSrc:     DirectiveChildSrc,
		DirectiveFrameSrc:      DirectiveChildSrc,
		DirectiveChildSrc:      DirectiveDefaultSrc,
		DirectiveScriptSrc:     DirectiveDefaultSrc,
		DirectiveStyleSrc:      DirectiveDefaultSrc,
		DirectiveImgSrc:        DirectiveDefaultSrc,
		DirectiveFontSrc:       DirectiveDefaultSrc,
		DirectiveConnectSrc:    DirectiveDefaultSrc,
		DirectiveMediaSrc:      DirectiveDefaultSrc,
		DirectiveObjectSrc:     DirectiveDefaultSrc,
		DirectiveManifestSrc:   DirectiveDefaultSrc,
		DirectivePrefetchSrc:   DirectiveDefaultSrc,
	}

	if fallback, ok := fallbacks[directive]; ok {
		if values, ok := p.Directives[fallback]; ok {
			return values
		}
		if fallback != DirectiveDefaultSrc {
			return p.GetEffectiveDirective(fallback)
		}
	}

	return nil
}

func (p *CSPPolicy) HasDirective(directive CSPDirective) bool {
	_, ok := p.Directives[directive]
	return ok
}

func (p *CSPPolicy) AllowsUnsafeInline(directive CSPDirective) bool {
	values := p.GetEffectiveDirective(directive)
	return containsSource(values, SourceUnsafeInline)
}

func (p *CSPPolicy) AllowsUnsafeEval(directive CSPDirective) bool {
	values := p.GetEffectiveDirective(directive)
	return containsSource(values, SourceUnsafeEval)
}

func (p *CSPPolicy) AllowsData(directive CSPDirective) bool {
	values := p.GetEffectiveDirective(directive)
	return containsSource(values, SourceData)
}

func (p *CSPPolicy) AllowsBlob(directive CSPDirective) bool {
	values := p.GetEffectiveDirective(directive)
	return containsSource(values, SourceBlob)
}

func (p *CSPPolicy) HasStrictDynamic(directive CSPDirective) bool {
	values := p.GetEffectiveDirective(directive)
	return containsSource(values, SourceStrictDynamic)
}

func (p *CSPPolicy) UsesNonces(directive CSPDirective) bool {
	values := p.GetEffectiveDirective(directive)
	for _, v := range values {
		if noncePattern.MatchString(v) {
			return true
		}
	}
	return false
}

func (p *CSPPolicy) UsesHashes(directive CSPDirective) bool {
	values := p.GetEffectiveDirective(directive)
	for _, v := range values {
		if hashPattern.MatchString(v) {
			return true
		}
	}
	return false
}

func (p *CSPPolicy) GetAllowedHosts(directive CSPDirective) []string {
	values := p.GetEffectiveDirective(directive)
	var hosts []string
	for _, v := range values {
		lower := strings.ToLower(v)
		if strings.HasPrefix(lower, "'") {
			continue
		}
		if lower == "data:" || lower == "blob:" || lower == "mediastream:" || lower == "filesystem:" {
			continue
		}
		hosts = append(hosts, v)
	}
	return hosts
}

func (p *CSPPolicy) AllowsHost(directive CSPDirective, host string) bool {
	values := p.GetEffectiveDirective(directive)
	host = strings.ToLower(host)

	for _, v := range values {
		v = strings.ToLower(v)

		if v == "*" {
			return true
		}

		if v == "'self'" {
			continue
		}

		if strings.HasPrefix(v, "*.") {
			suffix := v[1:]
			if strings.HasSuffix(host, suffix) || host == v[2:] {
				return true
			}
		}

		if v == host {
			return true
		}

		vHost := extractHost(v)
		if vHost == host {
			return true
		}
	}
	return false
}

func (p *CSPPolicy) IsNone(directive CSPDirective) bool {
	values := p.GetEffectiveDirective(directive)
	return len(values) == 1 && strings.ToLower(values[0]) == "'none'"
}

func (p *CSPPolicy) AnalyzeWeaknesses() []CSPWeakness {
	var weaknesses []CSPWeakness

	if !p.HasDirective(DirectiveDefaultSrc) && !p.HasDirective(DirectiveScriptSrc) {
		weaknesses = append(weaknesses, CSPWeakness{
			Directive:   DirectiveScriptSrc,
			Issue:       "missing_script_src",
			Severity:    "high",
			Exploitable: true,
			Details:     "No script-src or default-src directive. Inline scripts and any source allowed.",
		})
	}

	if p.AllowsUnsafeInline(DirectiveScriptSrc) && !p.UsesNonces(DirectiveScriptSrc) && !p.UsesHashes(DirectiveScriptSrc) && !p.HasStrictDynamic(DirectiveScriptSrc) {
		weaknesses = append(weaknesses, CSPWeakness{
			Directive:   DirectiveScriptSrc,
			Issue:       "unsafe_inline",
			Severity:    "high",
			Exploitable: true,
			Details:     "unsafe-inline allows execution of inline scripts without nonce/hash protection.",
		})
	}

	if p.AllowsUnsafeEval(DirectiveScriptSrc) {
		weaknesses = append(weaknesses, CSPWeakness{
			Directive:   DirectiveScriptSrc,
			Issue:       "unsafe_eval",
			Severity:    "medium",
			Exploitable: true,
			Details:     "unsafe-eval allows eval(), Function(), setTimeout/setInterval with strings.",
		})
	}

	if !p.HasDirective(DirectiveBaseURI) {
		weaknesses = append(weaknesses, CSPWeakness{
			Directive:   DirectiveBaseURI,
			Issue:       "missing_base_uri",
			Severity:    "medium",
			Exploitable: true,
			Details:     "Missing base-uri allows <base> tag injection for relative URL hijacking.",
		})
	}

	if !p.HasDirective(DirectiveObjectSrc) && !p.HasDirective(DirectiveDefaultSrc) {
		weaknesses = append(weaknesses, CSPWeakness{
			Directive:   DirectiveObjectSrc,
			Issue:       "missing_object_src",
			Severity:    "medium",
			Exploitable: true,
			Details:     "Missing object-src allows plugin content (Flash, Java applets).",
		})
	}

	if p.AllowsData(DirectiveScriptSrc) {
		weaknesses = append(weaknesses, CSPWeakness{
			Directive:   DirectiveScriptSrc,
			Issue:       "data_uri_script",
			Severity:    "high",
			Exploitable: true,
			Details:     "data: URI in script-src allows data:text/html payloads.",
		})
	}

	scriptHosts := p.GetAllowedHosts(DirectiveScriptSrc)
	for _, host := range scriptHosts {
		if host == "*" {
			weaknesses = append(weaknesses, CSPWeakness{
				Directive:   DirectiveScriptSrc,
				Issue:       "wildcard_script_src",
				Severity:    "high",
				Exploitable: true,
				Details:     "Wildcard (*) in script-src allows scripts from any source.",
			})
			break
		}
		if isKnownBypassableCDN(host) {
			weaknesses = append(weaknesses, CSPWeakness{
				Directive:   DirectiveScriptSrc,
				Issue:       "bypassable_cdn",
				Severity:    "medium",
				Exploitable: true,
				Details:     "Host " + host + " is known to host JSONP endpoints or user content.",
			})
		}
	}

	if p.ReportOnly {
		weaknesses = append(weaknesses, CSPWeakness{
			Directive:   "",
			Issue:       "report_only",
			Severity:    "info",
			Exploitable: true,
			Details:     "Policy is report-only and does not block violations.",
		})
	}

	return weaknesses
}

func (p *CSPPolicy) BlocksInlineScripts() bool {
	if !p.HasDirective(DirectiveScriptSrc) && !p.HasDirective(DirectiveDefaultSrc) {
		return false
	}
	if p.AllowsUnsafeInline(DirectiveScriptSrc) {
		if !p.HasStrictDynamic(DirectiveScriptSrc) && !p.UsesNonces(DirectiveScriptSrc) && !p.UsesHashes(DirectiveScriptSrc) {
			return false
		}
	}
	return true
}

func (p *CSPPolicy) BlocksEval() bool {
	if !p.HasDirective(DirectiveScriptSrc) && !p.HasDirective(DirectiveDefaultSrc) {
		return false
	}
	return !p.AllowsUnsafeEval(DirectiveScriptSrc)
}

func containsSource(values []string, source CSPSourceValue) bool {
	target := strings.ToLower(string(source))
	for _, v := range values {
		if strings.ToLower(v) == target {
			return true
		}
	}
	return false
}

func extractHost(source string) string {
	s := strings.TrimPrefix(source, "https://")
	s = strings.TrimPrefix(s, "http://")
	if idx := strings.Index(s, "/"); idx != -1 {
		s = s[:idx]
	}
	if idx := strings.Index(s, ":"); idx != -1 {
		s = s[:idx]
	}
	return s
}

func isKnownBypassableCDN(host string) bool {
	bypassable := []string{
		"*.googleapis.com",
		"*.gstatic.com",
		"*.google.com",
		"*.cloudflare.com",
		"cdnjs.cloudflare.com",
		"cdn.jsdelivr.net",
		"*.jsdelivr.net",
		"unpkg.com",
		"*.unpkg.com",
		"ajax.googleapis.com",
		"*.akamaihd.net",
		"*.yandex.net",
		"*.yandex.ru",
		"*.baidu.com",
	}

	host = strings.ToLower(host)
	for _, pattern := range bypassable {
		if strings.HasPrefix(pattern, "*.") {
			suffix := pattern[1:]
			if strings.HasSuffix(host, suffix) || host == pattern[2:] {
				return true
			}
		} else if host == pattern {
			return true
		}
	}
	return false
}
