package reflection

import (
	"net/http"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

// AnalysisOptions configures reflection analysis behavior
type AnalysisOptions struct {
	TestCharacterEfficiencies bool // Whether to test character encoding
	DetectBadContexts         bool // Whether to detect non-executable contexts
	Client                    *http.Client
	HistoryCreationOptions    http_utils.HistoryCreationOptions
}

// ReflectionAnalysis contains comprehensive reflection analysis results
type ReflectionAnalysis struct {
	// Core analysis
	IsReflected      bool
	ReflectionCount  int
	Contexts         []ReflectionContext
	AttributeDetails map[int]AttributeDetails // Position -> details
	BadContexts      []BadContext

	// Character efficiency (if enabled)
	CharEfficiencies []CharacterEfficiency

	// Quick access flags computed from CharEfficiencies
	CanInjectTags       bool // < and > pass through (efficiency >= 100)
	CanBreakDoubleQuote bool // " passes through
	CanBreakSingleQuote bool // ' passes through
	CanUseBackticks     bool // ` passes through
	CanCallFunctions    bool // ( and ) pass through
	CanUseSlash         bool // / passes through
	CanUseEquals        bool // = passes through
	CanUseSemicolon     bool // ; passes through
	CanEscape           bool // \ passes through

	// Context summary flags
	HasHTMLContext      bool
	HasScriptContext    bool
	HasAttributeContext bool
	HasCommentContext   bool
	HasCSSContext       bool
	IsInBadContext      bool // At least one reflection is in a non-executable context
}

// AnalyzeReflection performs comprehensive reflection analysis for an insertion point
func AnalyzeReflection(
	originalItem *db.History,
	insertionPoint InsertionPointInfo,
	options AnalysisOptions,
) (*ReflectionAnalysis, error) {
	client := options.Client
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	result := &ReflectionAnalysis{
		AttributeDetails: make(map[int]AttributeDetails),
	}

	// Step 1: Send a request with a canary to detect reflection
	canary := CanaryPrefix + "test" + CanarySuffix
	responseBody, err := sendCanaryRequest(originalItem, insertionPoint, canary, client, options.HistoryCreationOptions)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to send canary request for reflection analysis")
		return result, err
	}

	// Step 2: Detect all reflection contexts
	contextResult := DetectContexts(responseBody, canary)
	result.Contexts = contextResult.Contexts
	result.AttributeDetails = contextResult.AttributeDetails
	result.BadContexts = contextResult.BadContexts
	result.ReflectionCount = len(result.Contexts)
	result.IsReflected = result.ReflectionCount > 0

	if !result.IsReflected {
		log.Debug().Str("insertionPoint", insertionPoint.Name).Msg("No reflection detected")
		return result, nil
	}

	// Step 3: Compute context summary flags
	computeContextFlags(result)

	// Step 4: Check if any reflection is in a bad context
	for _, ctx := range result.Contexts {
		if IsInBadContext(ctx.Position, result.BadContexts) != nil {
			result.IsInBadContext = true
			break
		}
	}

	// Step 5: Test character efficiencies if enabled
	if options.TestCharacterEfficiencies {
		result.CharEfficiencies = AnalyzeCharacterEfficiencies(
			originalItem,
			insertionPoint,
			client,
			options.HistoryCreationOptions,
		)

		// Compute quick access flags
		flags := ComputeEfficiencyFlags(result.CharEfficiencies)
		result.CanInjectTags = flags.CanInjectTags
		result.CanBreakDoubleQuote = flags.CanBreakDoubleQuote
		result.CanBreakSingleQuote = flags.CanBreakSingleQuote
		result.CanUseBackticks = flags.CanUseBackticks
		result.CanCallFunctions = flags.CanCallFunctions
		result.CanUseSlash = flags.CanUseSlash
		result.CanUseEquals = flags.CanUseEquals
		result.CanUseSemicolon = flags.CanUseSemicolon
		result.CanEscape = flags.CanEscape
	}

	log.Debug().
		Str("insertionPoint", insertionPoint.Name).
		Int("reflectionCount", result.ReflectionCount).
		Bool("hasHTML", result.HasHTMLContext).
		Bool("hasScript", result.HasScriptContext).
		Bool("hasAttribute", result.HasAttributeContext).
		Bool("inBadContext", result.IsInBadContext).
		Msg("Reflection analysis complete")

	return result, nil
}

// sendCanaryRequest sends a request with the canary and returns the response body
func sendCanaryRequest(
	originalItem *db.History,
	insertionPoint InsertionPointInfo,
	canary string,
	client *http.Client,
	historyOptions http_utils.HistoryCreationOptions,
) (string, error) {
	req, err := buildTestRequest(originalItem, insertionPoint, canary)
	if err != nil {
		return "", err
	}

	execResult := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		Client:                 client,
		CreateHistory:          false, // Don't pollute history with analysis requests
		HistoryCreationOptions: historyOptions,
	})

	if execResult.Err != nil {
		return "", execResult.Err
	}

	return string(execResult.ResponseData.Body), nil
}

// computeContextFlags sets boolean flags for each detected context type
func computeContextFlags(result *ReflectionAnalysis) {
	for _, ctx := range result.Contexts {
		switch ctx.Mode {
		case ModeHTML:
			result.HasHTMLContext = true
		case ModeScript:
			result.HasScriptContext = true
		case ModeAttribute:
			result.HasAttributeContext = true
		case ModeComment:
			result.HasCommentContext = true
		case ModeCSS:
			result.HasCSSContext = true
		}
	}
}

// GetBestContext returns the most exploitable context for XSS
// Priority: script > attribute (with event handler potential) > html > comment > css
func (ra *ReflectionAnalysis) GetBestContext() *ReflectionContext {
	if len(ra.Contexts) == 0 {
		return nil
	}

	// Priority order
	var scriptCtx, attrCtx, htmlCtx, commentCtx, cssCtx *ReflectionContext

	for i := range ra.Contexts {
		ctx := &ra.Contexts[i]

		// Skip if in bad context
		if IsInBadContext(ctx.Position, ra.BadContexts) != nil {
			continue
		}

		switch ctx.Mode {
		case ModeScript:
			if scriptCtx == nil {
				scriptCtx = ctx
			}
		case ModeAttribute:
			if attrCtx == nil {
				attrCtx = ctx
			}
		case ModeHTML:
			if htmlCtx == nil {
				htmlCtx = ctx
			}
		case ModeComment:
			if commentCtx == nil {
				commentCtx = ctx
			}
		case ModeCSS:
			if cssCtx == nil {
				cssCtx = ctx
			}
		}
	}

	// Return in priority order
	if scriptCtx != nil {
		return scriptCtx
	}
	if attrCtx != nil {
		return attrCtx
	}
	if htmlCtx != nil {
		return htmlCtx
	}
	if commentCtx != nil {
		return commentCtx
	}
	return cssCtx
}

// GetAttributeContextDetails returns details for a specific attribute context
func (ra *ReflectionAnalysis) GetAttributeContextDetails(position int) *AttributeDetails {
	if details, ok := ra.AttributeDetails[position]; ok {
		return &details
	}
	return nil
}

// IsEventHandlerAttribute checks if an attribute context is an event handler
func (ra *ReflectionAnalysis) IsEventHandlerAttribute(position int) bool {
	details := ra.GetAttributeContextDetails(position)
	if details == nil {
		return false
	}
	return isEventHandler(details.Name)
}

// IsDangerousAttribute checks if an attribute can lead to XSS
func (ra *ReflectionAnalysis) IsDangerousAttribute(position int) bool {
	details := ra.GetAttributeContextDetails(position)
	if details == nil {
		return false
	}
	return isDangerousAttribute(details.Name)
}

// isEventHandler checks if an attribute name is an event handler
func isEventHandler(name string) bool {
	eventHandlers := []string{
		"onclick", "ondblclick", "onmousedown", "onmouseup", "onmouseover",
		"onmousemove", "onmouseout", "onmouseenter", "onmouseleave",
		"onkeydown", "onkeyup", "onkeypress",
		"onfocus", "onblur", "onchange", "oninput", "onsubmit", "onreset",
		"onload", "onerror", "onunload", "onbeforeunload",
		"onscroll", "onresize",
		"ondrag", "ondragstart", "ondragend", "ondragenter", "ondragleave", "ondragover", "ondrop",
		"oncopy", "oncut", "onpaste",
		"oncontextmenu", "onwheel",
		"ontouchstart", "ontouchend", "ontouchmove", "ontouchcancel",
		"onanimationstart", "onanimationend", "onanimationiteration",
		"ontransitionend",
		"onpointerdown", "onpointerup", "onpointermove", "onpointerenter", "onpointerleave",
	}

	for _, handler := range eventHandlers {
		if name == handler {
			return true
		}
	}
	return false
}

// isDangerousAttribute checks if an attribute can be exploited for XSS
func isDangerousAttribute(name string) bool {
	dangerousAttrs := []string{
		// URL-based
		"href", "src", "action", "formaction", "data", "poster", "srcset",
		// Code injection
		"srcdoc", "style",
		// Meta
		"content",
	}

	// Event handlers are always dangerous
	if isEventHandler(name) {
		return true
	}

	for _, attr := range dangerousAttrs {
		if name == attr {
			return true
		}
	}
	return false
}

// SuggestBreakers returns suggested breaker strings based on context and character efficiencies
func (ra *ReflectionAnalysis) SuggestBreakers() []string {
	var breakers []string

	for _, ctx := range ra.Contexts {
		// Skip bad contexts
		if IsInBadContext(ctx.Position, ra.BadContexts) != nil {
			continue
		}

		switch ctx.Mode {
		case ModeScript:
			breakers = append(breakers, ra.suggestScriptBreakers(ctx)...)
		case ModeAttribute:
			breakers = append(breakers, ra.suggestAttributeBreakers(ctx)...)
		case ModeHTML:
			breakers = append(breakers, ra.suggestHTMLBreakers()...)
		case ModeComment:
			breakers = append(breakers, ra.suggestCommentBreakers()...)
		}
	}

	return unique(breakers)
}

func (ra *ReflectionAnalysis) suggestScriptBreakers(ctx ReflectionContext) []string {
	var breakers []string

	switch ctx.QuoteState {
	case QuoteDouble:
		if ra.CanBreakDoubleQuote {
			breakers = append(breakers, `"`, `";`, `"-`)
		}
		if ra.CanEscape && ra.CanBreakDoubleQuote {
			breakers = append(breakers, `\"`, `\";`)
		}
	case QuoteSingle:
		if ra.CanBreakSingleQuote {
			breakers = append(breakers, `'`, `';`, `'-`)
		}
		if ra.CanEscape && ra.CanBreakSingleQuote {
			breakers = append(breakers, `\'`, `\';`)
		}
	case QuoteBacktick:
		if ra.CanUseBackticks {
			breakers = append(breakers, "`", "${", "}")
		}
	case QuoteNone:
		if ra.CanUseSemicolon {
			breakers = append(breakers, ";")
		}
		if ra.CanInjectTags {
			breakers = append(breakers, "</script>")
		}
	}

	return breakers
}

func (ra *ReflectionAnalysis) suggestAttributeBreakers(ctx ReflectionContext) []string {
	var breakers []string

	switch ctx.QuoteState {
	case QuoteDouble:
		if ra.CanBreakDoubleQuote {
			breakers = append(breakers, `"`, `" `, `">`)
		}
	case QuoteSingle:
		if ra.CanBreakSingleQuote {
			breakers = append(breakers, `'`, `' `, `'>`)
		}
	case QuoteNone:
		breakers = append(breakers, " ", ">")
	}

	return breakers
}

func (ra *ReflectionAnalysis) suggestHTMLBreakers() []string {
	var breakers []string

	if ra.CanInjectTags {
		breakers = append(breakers, "<", ">", "<script>", "<img ", "<svg ")
	}

	return breakers
}

func (ra *ReflectionAnalysis) suggestCommentBreakers() []string {
	return []string{"-->", "--!>"}
}

// unique removes duplicate strings from a slice
func unique(items []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}
