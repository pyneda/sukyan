package passive

import (
	"regexp"
	"strings"
)

var DOMXSSSourcePatterns = []*regexp.Regexp{
	// URL-based sources (various access patterns)
	regexp.MustCompile(`(?i)location\s*[.\[]`),
	regexp.MustCompile(`(?i)location\s*\[\s*["']`),
	regexp.MustCompile(`(?i)document\s*\.\s*(URL|documentURI|baseURI|referrer|cookie)`),
	regexp.MustCompile(`(?i)window\s*\.\s*name`),
	regexp.MustCompile(`(?i)(localStorage|sessionStorage)\s*[.\[]`),

	// Bracket notation obfuscation (e.g., window["location"]["hash"])
	regexp.MustCompile(`(?i)(window|document|self|top|parent)\s*\[\s*["'][^"']*["']\s*\]`),

	// postMessage source - very important for modern apps
	regexp.MustCompile(`(?i)addEventListener\s*\(\s*["']message["']`),
	regexp.MustCompile(`(?i)onmessage\s*=`),
	regexp.MustCompile(`(?i)\.on\s*\(\s*["']message["']`), // jQuery style

	// History API
	regexp.MustCompile(`(?i)history\s*\.\s*(state|pushState|replaceState)`),
	regexp.MustCompile(`(?i)popstate`),
	regexp.MustCompile(`(?i)hashchange`),

	// URL/URLSearchParams (modern APIs)
	regexp.MustCompile(`(?i)new\s+URL\s*\(`),
	regexp.MustCompile(`(?i)URLSearchParams`),

	// Form data
	regexp.MustCompile(`(?i)FormData\s*\(`),

	// Window hierarchy access
	regexp.MustCompile(`(?i)(opener|parent|top|frames)\s*[.\[]`),
}

var DOMXSSSinkPatterns = []*regexp.Regexp{
	// HTML execution sinks
	regexp.MustCompile(`(?i)\.innerHTML\s*=`),
	regexp.MustCompile(`(?i)\.outerHTML\s*=`),
	regexp.MustCompile(`(?i)document\.write(ln)?\s*\(`),
	regexp.MustCompile(`(?i)insertAdjacentHTML\s*\(`),
	regexp.MustCompile(`(?i)\.srcdoc\s*=`), // iframe srcdoc

	// JavaScript execution sinks
	regexp.MustCompile(`(?i)\beval\s*\(`),
	regexp.MustCompile(`(?i)(setTimeout|setInterval)\s*\(`),
	regexp.MustCompile(`(?i)new\s+Function\s*\(`),
	regexp.MustCompile(`(?i)execScript\s*\(`),

	// URL setters
	regexp.MustCompile(`(?i)location\s*(\.href)?\s*=`),
	regexp.MustCompile(`(?i)location\.(assign|replace)\s*\(`),
	regexp.MustCompile(`(?i)\.src\s*=`),
	regexp.MustCompile(`(?i)\.href\s*=`),
	regexp.MustCompile(`(?i)\.action\s*=`),
	regexp.MustCompile(`(?i)\.formAction\s*=`),

	// DOM manipulation that can execute code
	regexp.MustCompile(`(?i)\.(appendChild|insertBefore|replaceChild)\s*\(`),
	regexp.MustCompile(`(?i)\.setAttribute\s*\(\s*["'](src|href|action|formaction|data|srcdoc|on\w+)`),
	regexp.MustCompile(`(?i)\[\s*["']innerHTML["']\s*\]`), // Bracket notation

	// Event handler properties
	regexp.MustCompile(`(?i)\.(onclick|onerror|onload|onfocus|onblur|onmouseover|onmouseout|onkeydown|onkeyup|onchange|oninput)\s*=`),

	// Framework-specific dangerous patterns
	regexp.MustCompile(`(?i)dangerouslySetInnerHTML`), // React
	regexp.MustCompile(`(?i)v-html\s*=`),              // Vue
	regexp.MustCompile(`(?i)bypassSecurityTrust`),     // Angular
	regexp.MustCompile(`(?i)\[innerHTML\]\s*=`),       // Angular binding
	regexp.MustCompile(`(?i)ng-bind-html`),            // AngularJS
	regexp.MustCompile(`(?i)__html\s*:`),              // React dangerouslySetInnerHTML object

	// jQuery sinks
	regexp.MustCompile(`(?i)\$\s*\([^)]*\)\s*\.\s*(html|append|prepend|after|before|replaceWith|wrap|wrapAll)\s*\(`),
	regexp.MustCompile(`(?i)jQuery\s*\([^)]*\)\s*\.\s*(html|append|prepend|after|before|replaceWith|wrap|wrapAll)\s*\(`),
	regexp.MustCompile(`(?i)\.\s*globalEval\s*\(`),

	// Range API
	regexp.MustCompile(`(?i)createContextualFragment\s*\(`),

	// DOM Parser
	regexp.MustCompile(`(?i)DOMParser\s*\([^)]*\)\s*\.\s*parseFromString\s*\(`),
	regexp.MustCompile(`(?i)\.parseFromString\s*\(`),

	// Indirect eval patterns
	regexp.MustCompile(`(?i)\(\s*[01]\s*,\s*eval\s*\)\s*\(`), // (1, eval)("code")
	regexp.MustCompile(`(?i)window\s*\[\s*["']eval["']\s*\]`),
}

// AdvancedHasSourcesOrSinks performs pattern matching for DOM XSS
// source and sink patterns. It uses regex patterns that try to catch
// obfuscated code, modern framework patterns, and bracket notation access.
func AdvancedHasSourcesOrSinks(text string) (hasSources, hasSinks bool) {
	// Quick check - if text is too small, skip expensive regex
	if len(text) < 20 {
		return false, false
	}

	// Check for sources
	for _, pattern := range DOMXSSSourcePatterns {
		if pattern.MatchString(text) {
			hasSources = true
			break
		}
	}

	// Check for sinks
	for _, pattern := range DOMXSSSinkPatterns {
		if pattern.MatchString(text) {
			hasSinks = true
			break
		}
	}

	return
}

// HasDOMXSSIndicators checks for common DOM XSS indicators
func HasDOMXSSIndicators(text string) (hasSources, hasSinks bool) {
	return AdvancedHasSourcesOrSinks(text)
}

// FindDOMXSSSources returns all matching source patterns found in text
// Useful for detailed analysis and reporting
func FindDOMXSSSources(text string) []string {
	var matches []string
	seen := make(map[string]bool)

	for _, pattern := range DOMXSSSourcePatterns {
		found := pattern.FindAllString(text, 5) // Limit matches per pattern
		for _, match := range found {
			match = strings.TrimSpace(match)
			if !seen[match] {
				seen[match] = true
				matches = append(matches, match)
			}
		}
	}

	return matches
}

// FindDOMXSSSinks returns all matching sink patterns found in text
// Useful for detailed analysis and reporting
func FindDOMXSSSinks(text string) []string {
	var matches []string
	seen := make(map[string]bool)

	for _, pattern := range DOMXSSSinkPatterns {
		found := pattern.FindAllString(text, 5) // Limit matches per pattern
		for _, match := range found {
			match = strings.TrimSpace(match)
			if !seen[match] {
				seen[match] = true
				matches = append(matches, match)
			}
		}
	}

	return matches
}
