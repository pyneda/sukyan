package reflection

import (
	"testing"
)

func TestDetectContexts_HTMLText(t *testing.T) {
	body := `<html><body><p>Hello st4r7stest3nd world</p></body></html>`
	canary := "st4r7stest3nd"

	result := DetectContexts(body, canary)

	if len(result.Contexts) != 1 {
		t.Errorf("Expected 1 context, got %d", len(result.Contexts))
		return
	}

	ctx := result.Contexts[0]
	if ctx.Mode != ModeHTML {
		t.Errorf("Expected ModeHTML, got %s", ctx.Mode)
	}
	if ctx.QuoteState != QuoteNone {
		t.Errorf("Expected QuoteNone, got %s", ctx.QuoteState)
	}
}

func TestDetectContexts_ScriptDoubleQuote(t *testing.T) {
	body := `<html><script>var x = "st4r7stest3nd";</script></html>`
	canary := "st4r7stest3nd"

	result := DetectContexts(body, canary)

	if len(result.Contexts) != 1 {
		t.Errorf("Expected 1 context, got %d", len(result.Contexts))
		return
	}

	ctx := result.Contexts[0]
	if ctx.Mode != ModeScript {
		t.Errorf("Expected ModeScript, got %s", ctx.Mode)
	}
	if ctx.QuoteState != QuoteDouble {
		t.Errorf("Expected QuoteDouble, got %s", ctx.QuoteState)
	}
}

func TestDetectContexts_ScriptSingleQuote(t *testing.T) {
	body := `<html><script>var x = 'st4r7stest3nd';</script></html>`
	canary := "st4r7stest3nd"

	result := DetectContexts(body, canary)

	if len(result.Contexts) != 1 {
		t.Errorf("Expected 1 context, got %d", len(result.Contexts))
		return
	}

	ctx := result.Contexts[0]
	if ctx.Mode != ModeScript {
		t.Errorf("Expected ModeScript, got %s", ctx.Mode)
	}
	if ctx.QuoteState != QuoteSingle {
		t.Errorf("Expected QuoteSingle, got %s", ctx.QuoteState)
	}
}

func TestDetectContexts_ScriptBacktick(t *testing.T) {
	body := "<html><script>var x = `st4r7stest3nd`;</script></html>"
	canary := "st4r7stest3nd"

	result := DetectContexts(body, canary)

	if len(result.Contexts) != 1 {
		t.Errorf("Expected 1 context, got %d", len(result.Contexts))
		return
	}

	ctx := result.Contexts[0]
	if ctx.Mode != ModeScript {
		t.Errorf("Expected ModeScript, got %s", ctx.Mode)
	}
	if ctx.QuoteState != QuoteBacktick {
		t.Errorf("Expected QuoteBacktick, got %s", ctx.QuoteState)
	}
}

func TestDetectContexts_AttributeDoubleQuote(t *testing.T) {
	body := `<html><input value="st4r7stest3nd"></html>`
	canary := "st4r7stest3nd"

	result := DetectContexts(body, canary)

	if len(result.Contexts) != 1 {
		t.Errorf("Expected 1 context, got %d", len(result.Contexts))
		return
	}

	ctx := result.Contexts[0]
	if ctx.Mode != ModeAttribute {
		t.Errorf("Expected ModeAttribute, got %s", ctx.Mode)
	}
	if ctx.QuoteState != QuoteDouble {
		t.Errorf("Expected QuoteDouble, got %s", ctx.QuoteState)
	}

	// Check attribute details
	details, ok := result.AttributeDetails[ctx.Position]
	if !ok {
		t.Error("Expected attribute details")
		return
	}
	if details.Name != "value" {
		t.Errorf("Expected attribute name 'value', got '%s'", details.Name)
	}
	if details.Tag != "input" {
		t.Errorf("Expected tag 'input', got '%s'", details.Tag)
	}
}

func TestDetectContexts_Comment(t *testing.T) {
	body := `<html><!-- st4r7stest3nd --></html>`
	canary := "st4r7stest3nd"

	result := DetectContexts(body, canary)

	if len(result.Contexts) != 1 {
		t.Errorf("Expected 1 context, got %d", len(result.Contexts))
		return
	}

	ctx := result.Contexts[0]
	if ctx.Mode != ModeComment {
		t.Errorf("Expected ModeComment, got %s", ctx.Mode)
	}
}

func TestDetectContexts_CSS(t *testing.T) {
	body := `<html><style>.test { color: st4r7stest3nd; }</style></html>`
	canary := "st4r7stest3nd"

	result := DetectContexts(body, canary)

	if len(result.Contexts) != 1 {
		t.Errorf("Expected 1 context, got %d", len(result.Contexts))
		return
	}

	ctx := result.Contexts[0]
	if ctx.Mode != ModeCSS {
		t.Errorf("Expected ModeCSS, got %s", ctx.Mode)
	}
}

func TestDetectContexts_MultipleContexts(t *testing.T) {
	body := `<html>
		<p>st4r7stest3nd</p>
		<script>var x = "st4r7stest3nd";</script>
		<input value="st4r7stest3nd">
	</html>`
	canary := "st4r7stest3nd"

	result := DetectContexts(body, canary)

	if len(result.Contexts) != 3 {
		t.Errorf("Expected 3 contexts, got %d", len(result.Contexts))
		return
	}

	// Check that we have all three context types
	modes := make(map[ReflectionMode]bool)
	for _, ctx := range result.Contexts {
		modes[ctx.Mode] = true
	}

	if !modes[ModeHTML] {
		t.Error("Expected ModeHTML context")
	}
	if !modes[ModeScript] {
		t.Error("Expected ModeScript context")
	}
	if !modes[ModeAttribute] {
		t.Error("Expected ModeAttribute context")
	}
}

func TestDetectBadContexts_Textarea(t *testing.T) {
	body := `<html><textarea>st4r7stest3nd</textarea></html>`
	canary := "st4r7stest3nd"

	result := DetectContexts(body, canary)

	if len(result.BadContexts) != 1 {
		t.Errorf("Expected 1 bad context, got %d", len(result.BadContexts))
		return
	}

	if result.BadContexts[0].Tag != "textarea" {
		t.Errorf("Expected bad context tag 'textarea', got '%s'", result.BadContexts[0].Tag)
	}
}

func TestDetectBadContexts_Title(t *testing.T) {
	body := `<html><title>st4r7stest3nd</title></html>`
	canary := "st4r7stest3nd"

	result := DetectContexts(body, canary)

	if len(result.BadContexts) != 1 {
		t.Errorf("Expected 1 bad context, got %d", len(result.BadContexts))
		return
	}

	if result.BadContexts[0].Tag != "title" {
		t.Errorf("Expected bad context tag 'title', got '%s'", result.BadContexts[0].Tag)
	}
}

func TestDetectQuoteState(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected QuoteState
	}{
		{"empty", "", QuoteNone},
		{"no quotes", "var x = 1;", QuoteNone},
		{"in double quote", `var x = "hello `, QuoteDouble},
		{"closed double quote", `var x = "hello"`, QuoteNone},
		{"in single quote", `var x = 'hello `, QuoteSingle},
		{"closed single quote", `var x = 'hello'`, QuoteNone},
		{"in backtick", "var x = `hello ", QuoteBacktick},
		{"closed backtick", "var x = `hello`", QuoteNone},
		{"escaped double quote", `var x = "hello \"`, QuoteDouble},
		{"nested quotes", `var x = "it's fine"`, QuoteNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectQuoteState(tt.input)
			if result != tt.expected {
				t.Errorf("detectQuoteState(%q) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsInBadContext(t *testing.T) {
	badContexts := []BadContext{
		{Tag: "textarea", Start: 10, End: 50},
		{Tag: "title", Start: 100, End: 150},
	}

	tests := []struct {
		position int
		expected bool
	}{
		{5, false},
		{10, true},
		{30, true},
		{49, true},
		{50, false},
		{75, false},
		{100, true},
		{125, true},
		{150, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := IsInBadContext(tt.position, badContexts)
			isIn := result != nil
			if isIn != tt.expected {
				t.Errorf("IsInBadContext(%d) = %v, want %v", tt.position, isIn, tt.expected)
			}
		})
	}
}

func TestIsJSONResponse(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		contentType string
		expected    bool
	}{
		{"json content type", `{"key": "value"}`, "application/json", true},
		{"json body object", `{"key": "value"}`, "text/html", true},
		{"json body array", `[1, 2, 3]`, "text/html", true},
		{"html body", `<html></html>`, "text/html", false},
		{"empty body", ``, "text/html", false},
		{"whitespace json", `  {"key": "value"}  `, "text/html", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsJSONResponse(tt.body, tt.contentType)
			if result != tt.expected {
				t.Errorf("IsJSONResponse(%q, %q) = %v, want %v", tt.body, tt.contentType, result, tt.expected)
			}
		})
	}
}
