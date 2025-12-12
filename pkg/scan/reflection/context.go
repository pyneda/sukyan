package reflection

import (
	"regexp"
	"sort"
	"strings"
)

// ReflectionMode represents where in the document structure the reflection occurs
type ReflectionMode string

const (
	ModeHTML      ReflectionMode = "html"      // Outside tags, in text content
	ModeScript    ReflectionMode = "script"    // Inside <script> tags
	ModeAttribute ReflectionMode = "attribute" // Inside a tag's attribute
	ModeComment   ReflectionMode = "comment"   // Inside HTML comments
	ModeCSS       ReflectionMode = "css"       // Inside <style> tags
	ModeJSON      ReflectionMode = "json"      // JSON response body
)

// QuoteState represents the quote context around the reflection
type QuoteState string

const (
	QuoteNone     QuoteState = "none"     // Not inside quotes
	QuoteDouble   QuoteState = "double"   // Inside "..."
	QuoteSingle   QuoteState = "single"   // Inside '...'
	QuoteBacktick QuoteState = "backtick" // Inside `...` (JS template literals)
)

// ReflectionContext combines mode and quote state
type ReflectionContext struct {
	Mode       ReflectionMode
	QuoteState QuoteState
	Position   int // Position in response where reflection was found
}

func (rc ReflectionContext) String() string {
	return string(rc.Mode) + "-" + string(rc.QuoteState)
}

// AttributeDetails captures information about attribute context reflections
type AttributeDetails struct {
	Tag       string // e.g., "input", "a", "script"
	Name      string // e.g., "value", "href", "onclick"
	ValueType string // "name" (attr name is payload), "value" (attr value contains payload), or "flag"
	Quote     string // Quote character used (", ', or empty)
	Value     string // Current attribute value
}

// BadContext represents a non-executable context where XSS won't work
type BadContext struct {
	Tag   string
	Start int
	End   int
}

// NonExecutableContextTags are HTML tags where script execution is blocked
var NonExecutableContextTags = []string{
	"style", "template", "textarea", "title",
	"noembed", "noscript",
}

// ContextDetectionResult holds all detected contexts for a canary
type ContextDetectionResult struct {
	Contexts         []ReflectionContext
	AttributeDetails map[int]AttributeDetails // Position -> details
	BadContexts      []BadContext
}

// DetectContexts analyzes the response body to find all reflection contexts for the canary
// This implements a state machine approach similar to Dalfox's Abstraction function
func DetectContexts(body string, canary string) ContextDetectionResult {
	result := ContextDetectionResult{
		AttributeDetails: make(map[int]AttributeDetails),
	}

	// First, detect bad contexts (non-executable areas)
	result.BadContexts = detectBadContexts(body, canary)

	// Remove comments for cleaner parsing (but track comment reflections separately)
	commentContexts := detectCommentContexts(body, canary)
	result.Contexts = append(result.Contexts, commentContexts...)

	cleanBody := removeComments(body)

	// Detect script contexts
	scriptContexts := detectScriptContexts(cleanBody, canary)
	result.Contexts = append(result.Contexts, scriptContexts...)

	// Detect CSS contexts
	cssContexts := detectCSSContexts(cleanBody, canary)
	result.Contexts = append(result.Contexts, cssContexts...)

	// Detect attribute contexts
	attrContexts, attrDetails := detectAttributeContexts(cleanBody, canary)
	result.Contexts = append(result.Contexts, attrContexts...)
	for pos, details := range attrDetails {
		result.AttributeDetails[pos] = details
	}

	// Detect HTML text contexts (remaining reflections)
	htmlContexts := detectHTMLTextContexts(cleanBody, canary, result.Contexts)
	result.Contexts = append(result.Contexts, htmlContexts...)

	// Sort contexts by position
	sort.Slice(result.Contexts, func(i, j int) bool {
		return result.Contexts[i].Position < result.Contexts[j].Position
	})

	return result
}

// detectBadContexts finds reflections inside non-executable contexts
func detectBadContexts(body string, canary string) []BadContext {
	var badContexts []BadContext
	lowerBody := strings.ToLower(body)

	for _, tag := range NonExecutableContextTags {
		pattern := regexp.MustCompile(`(?is)<` + tag + `[^>]*>.*?</` + tag + `>`)
		matches := pattern.FindAllStringIndex(lowerBody, -1)
		for _, match := range matches {
			segment := body[match[0]:match[1]]
			if strings.Contains(segment, canary) {
				badContexts = append(badContexts, BadContext{
					Tag:   tag,
					Start: match[0],
					End:   match[1],
				})
			}
		}
	}

	return badContexts
}

// detectCommentContexts finds reflections inside HTML comments
func detectCommentContexts(body string, canary string) []ReflectionContext {
	var contexts []ReflectionContext
	pattern := regexp.MustCompile(`<!--[\s\S]*?-->`)
	matches := pattern.FindAllStringIndex(body, -1)

	for _, match := range matches {
		comment := body[match[0]:match[1]]
		if idx := strings.Index(comment, canary); idx != -1 {
			contexts = append(contexts, ReflectionContext{
				Mode:       ModeComment,
				QuoteState: QuoteNone,
				Position:   match[0] + idx,
			})
		}
	}

	return contexts
}

// detectScriptContexts finds reflections inside <script> tags and determines quote state
func detectScriptContexts(body string, canary string) []ReflectionContext {
	var contexts []ReflectionContext
	pattern := regexp.MustCompile(`(?is)<script[^>]*>([\s\S]*?)</script>`)
	matches := pattern.FindAllStringSubmatchIndex(body, -1)

	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		scriptStart := match[2]
		scriptEnd := match[3]
		if scriptStart < 0 || scriptEnd < 0 || scriptStart >= scriptEnd {
			continue
		}
		scriptContent := body[scriptStart:scriptEnd]

		// Find canary positions within script
		offset := 0
		for {
			idx := strings.Index(scriptContent[offset:], canary)
			if idx == -1 {
				break
			}
			absolutePos := scriptStart + offset + idx
			quoteState := detectQuoteState(scriptContent[:offset+idx])

			contexts = append(contexts, ReflectionContext{
				Mode:       ModeScript,
				QuoteState: quoteState,
				Position:   absolutePos,
			})
			offset += idx + len(canary)
		}
	}

	return contexts
}

// detectCSSContexts finds reflections inside <style> tags
func detectCSSContexts(body string, canary string) []ReflectionContext {
	var contexts []ReflectionContext
	pattern := regexp.MustCompile(`(?is)<style[^>]*>([\s\S]*?)</style>`)
	matches := pattern.FindAllStringSubmatchIndex(body, -1)

	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		styleStart := match[2]
		styleEnd := match[3]
		if styleStart < 0 || styleEnd < 0 || styleStart >= styleEnd {
			continue
		}
		styleContent := body[styleStart:styleEnd]

		offset := 0
		for {
			idx := strings.Index(styleContent[offset:], canary)
			if idx == -1 {
				break
			}
			absolutePos := styleStart + offset + idx

			contexts = append(contexts, ReflectionContext{
				Mode:       ModeCSS,
				QuoteState: QuoteNone,
				Position:   absolutePos,
			})
			offset += idx + len(canary)
		}
	}

	return contexts
}

// detectAttributeContexts finds reflections inside HTML tag attributes
func detectAttributeContexts(body string, canary string) ([]ReflectionContext, map[int]AttributeDetails) {
	var contexts []ReflectionContext
	details := make(map[int]AttributeDetails)

	// Match tags containing the canary
	pattern := regexp.MustCompile(`<([a-zA-Z][a-zA-Z0-9]*)[^>]*` + regexp.QuoteMeta(canary) + `[^>]*>`)
	matches := pattern.FindAllStringSubmatchIndex(body, -1)

	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		tagStart := match[0]
		tagEnd := match[1]
		tagName := body[match[2]:match[3]]
		tagContent := body[tagStart:tagEnd]

		// Find the canary position within the tag
		canaryIdx := strings.Index(tagContent, canary)
		if canaryIdx == -1 {
			continue
		}
		absolutePos := tagStart + canaryIdx

		// Parse the tag to understand the attribute context
		attrDetail := parseAttributeContext(tagContent, canary, tagName)
		details[absolutePos] = attrDetail

		// Determine quote state from the attribute
		quoteState := QuoteNone
		if attrDetail.Quote == "\"" {
			quoteState = QuoteDouble
		} else if attrDetail.Quote == "'" {
			quoteState = QuoteSingle
		}

		contexts = append(contexts, ReflectionContext{
			Mode:       ModeAttribute,
			QuoteState: quoteState,
			Position:   absolutePos,
		})
	}

	return contexts, details
}

// parseAttributeContext extracts details about the attribute containing the canary
func parseAttributeContext(tagContent string, canary string, tagName string) AttributeDetails {
	details := AttributeDetails{
		Tag: strings.ToLower(tagName),
	}

	// Split tag content by whitespace to find attributes
	// This is a simplified parser - real HTML can be more complex
	parts := strings.Fields(tagContent)

	for _, part := range parts {
		if !strings.Contains(part, canary) {
			continue
		}

		// Check if it's in attribute name or value
		if strings.Contains(part, "=") {
			eqIdx := strings.Index(part, "=")
			attrName := part[:eqIdx]
			attrValue := part[eqIdx+1:]

			// Remove trailing > if present
			attrValue = strings.TrimSuffix(attrValue, ">")

			// Detect quote
			if len(attrValue) > 0 {
				firstChar := string(attrValue[0])
				if firstChar == "\"" || firstChar == "'" {
					details.Quote = firstChar
					// Remove surrounding quotes
					if len(attrValue) > 1 {
						attrValue = attrValue[1:]
						if lastIdx := strings.LastIndex(attrValue, firstChar); lastIdx != -1 {
							attrValue = attrValue[:lastIdx]
						}
					}
				}
			}

			if strings.Contains(attrName, canary) {
				details.ValueType = "name"
				details.Name = attrName
			} else {
				details.ValueType = "value"
				details.Name = strings.ToLower(attrName)
			}
			details.Value = attrValue
		} else {
			// Flag attribute (no value)
			details.ValueType = "flag"
			details.Name = strings.TrimSuffix(part, ">")
		}
		break
	}

	return details
}

// detectHTMLTextContexts finds reflections in plain HTML text (outside tags/scripts)
func detectHTMLTextContexts(body string, canary string, existingContexts []ReflectionContext) []ReflectionContext {
	var contexts []ReflectionContext

	// Build a set of already-found positions
	foundPositions := make(map[int]bool)
	for _, ctx := range existingContexts {
		foundPositions[ctx.Position] = true
	}

	// Find all occurrences of canary
	offset := 0
	for {
		idx := strings.Index(body[offset:], canary)
		if idx == -1 {
			break
		}
		absolutePos := offset + idx

		// If not already accounted for, it's HTML text
		if !foundPositions[absolutePos] {
			contexts = append(contexts, ReflectionContext{
				Mode:       ModeHTML,
				QuoteState: QuoteNone,
				Position:   absolutePos,
			})
		}
		offset = absolutePos + len(canary)
	}

	return contexts
}

// detectQuoteState determines what quote type we're inside based on preceding content
func detectQuoteState(preceding string) QuoteState {
	// Track quote state by scanning through the content
	var state QuoteState = QuoteNone
	inEscape := false

	for i := 0; i < len(preceding); i++ {
		char := preceding[i]

		if inEscape {
			inEscape = false
			continue
		}

		if char == '\\' {
			inEscape = true
			continue
		}

		switch char {
		case '"':
			if state == QuoteNone {
				state = QuoteDouble
			} else if state == QuoteDouble {
				state = QuoteNone
			}
		case '\'':
			if state == QuoteNone {
				state = QuoteSingle
			} else if state == QuoteSingle {
				state = QuoteNone
			}
		case '`':
			if state == QuoteNone {
				state = QuoteBacktick
			} else if state == QuoteBacktick {
				state = QuoteNone
			}
		}
	}

	return state
}

// removeComments removes HTML comments from the body for cleaner parsing
func removeComments(body string) string {
	pattern := regexp.MustCompile(`<!--[\s\S]*?-->`)
	return pattern.ReplaceAllString(body, "")
}

// IsInBadContext checks if a position falls within a non-executable context
func IsInBadContext(position int, badContexts []BadContext) *BadContext {
	for i := range badContexts {
		if position >= badContexts[i].Start && position < badContexts[i].End {
			return &badContexts[i]
		}
	}
	return nil
}

// IsJSONResponse checks if the response appears to be JSON
func IsJSONResponse(body string, contentType string) bool {
	// Check content type header
	if strings.Contains(strings.ToLower(contentType), "application/json") {
		return true
	}

	// Check if body starts with JSON structure
	trimmed := strings.TrimSpace(body)
	return (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"))
}
