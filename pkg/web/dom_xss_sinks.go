package web

// DOMXSSSinkType categorizes DOM XSS sinks by their execution context
type DOMXSSSinkType int

const (
	SinkTypeHTMLExecution   DOMXSSSinkType = iota // innerHTML, document.write
	SinkTypeJSExecution                           // eval, setTimeout, Function
	SinkTypeURLSetter                             // location.href, location.assign
	SinkTypejQuery                                // $.html(), $.append()
	SinkTypeDOMManipulation                       // appendChild, insertBefore, etc.
	SinkTypeEventHandler                          // onclick, onerror, etc.
	SinkTypeFramework                             // React, Vue, Angular specific
)

// DOMXSSSink represents a potential DOM XSS sink
type DOMXSSSink struct {
	Name            string         // JavaScript function/property name
	Type            DOMXSSSinkType // Category of the sink
	AlwaysDangerous bool           // Always dangerous vs context-dependent (e.g., javascript: URLs)
	JSContext       bool           // Executes JavaScript directly (not via HTML parsing)
	Description     string         // Human-readable description
}

// DOMXSSSinks returns all supported DOM XSS sinks
func DOMXSSSinks() []DOMXSSSink {
	return []DOMXSSSink{
		// HTML Execution sinks - directly parse and render HTML
		{Name: "innerHTML", Type: SinkTypeHTMLExecution, AlwaysDangerous: true, JSContext: false, Description: "Sets inner HTML content"},
		{Name: "outerHTML", Type: SinkTypeHTMLExecution, AlwaysDangerous: true, JSContext: false, Description: "Replaces element with HTML"},
		{Name: "document.write", Type: SinkTypeHTMLExecution, AlwaysDangerous: true, JSContext: false, Description: "Writes HTML to document"},
		{Name: "document.writeln", Type: SinkTypeHTMLExecution, AlwaysDangerous: true, JSContext: false, Description: "Writes HTML line to document"},
		{Name: "insertAdjacentHTML", Type: SinkTypeHTMLExecution, AlwaysDangerous: true, JSContext: false, Description: "Inserts HTML at specified position"},

		// JavaScript Execution sinks - directly execute code
		{Name: "eval", Type: SinkTypeJSExecution, AlwaysDangerous: true, JSContext: true, Description: "Evaluates JavaScript code"},
		{Name: "setTimeout", Type: SinkTypeJSExecution, AlwaysDangerous: true, JSContext: true, Description: "Delayed code execution"},
		{Name: "setInterval", Type: SinkTypeJSExecution, AlwaysDangerous: true, JSContext: true, Description: "Repeated code execution"},
		{Name: "Function", Type: SinkTypeJSExecution, AlwaysDangerous: true, JSContext: true, Description: "Creates function from string"},
		{Name: "execScript", Type: SinkTypeJSExecution, AlwaysDangerous: true, JSContext: true, Description: "IE-specific script execution"},

		// URL Setter sinks - dangerous with javascript: URLs
		{Name: "location", Type: SinkTypeURLSetter, AlwaysDangerous: false, JSContext: true, Description: "Sets window location"},
		{Name: "location.href", Type: SinkTypeURLSetter, AlwaysDangerous: false, JSContext: true, Description: "Sets location href"},
		{Name: "location.assign", Type: SinkTypeURLSetter, AlwaysDangerous: false, JSContext: true, Description: "Navigates to URL"},
		{Name: "location.replace", Type: SinkTypeURLSetter, AlwaysDangerous: false, JSContext: true, Description: "Replaces current location"},
		{Name: "element.src", Type: SinkTypeURLSetter, AlwaysDangerous: false, JSContext: true, Description: "Sets element source URL"},
		{Name: "element.href", Type: SinkTypeURLSetter, AlwaysDangerous: false, JSContext: true, Description: "Sets element href"},

		// jQuery sinks - commonly used in web applications
		{Name: "$.html", Type: SinkTypejQuery, AlwaysDangerous: true, JSContext: false, Description: "jQuery html() method"},
		{Name: "$.append", Type: SinkTypejQuery, AlwaysDangerous: true, JSContext: false, Description: "jQuery append() method"},
		{Name: "$.prepend", Type: SinkTypejQuery, AlwaysDangerous: true, JSContext: false, Description: "jQuery prepend() method"},
		{Name: "$.after", Type: SinkTypejQuery, AlwaysDangerous: true, JSContext: false, Description: "jQuery after() method"},
		{Name: "$.before", Type: SinkTypejQuery, AlwaysDangerous: true, JSContext: false, Description: "jQuery before() method"},
		{Name: "$.replaceWith", Type: SinkTypejQuery, AlwaysDangerous: true, JSContext: false, Description: "jQuery replaceWith() method"},
		{Name: "$.wrapAll", Type: SinkTypejQuery, AlwaysDangerous: true, JSContext: false, Description: "jQuery wrapAll() method"},
		{Name: "$.wrap", Type: SinkTypejQuery, AlwaysDangerous: true, JSContext: false, Description: "jQuery wrap() method"},
		{Name: "$.globalEval", Type: SinkTypejQuery, AlwaysDangerous: true, JSContext: true, Description: "jQuery globalEval() method"},

		// DOM Manipulation sinks - can be dangerous with crafted elements
		{Name: "appendChild", Type: SinkTypeDOMManipulation, AlwaysDangerous: false, JSContext: false, Description: "Appends child node to element"},
		{Name: "insertBefore", Type: SinkTypeDOMManipulation, AlwaysDangerous: false, JSContext: false, Description: "Inserts node before reference node"},
		{Name: "replaceChild", Type: SinkTypeDOMManipulation, AlwaysDangerous: false, JSContext: false, Description: "Replaces child node with another"},
		{Name: "append", Type: SinkTypeDOMManipulation, AlwaysDangerous: false, JSContext: false, Description: "Modern append method"},
		{Name: "prepend", Type: SinkTypeDOMManipulation, AlwaysDangerous: false, JSContext: false, Description: "Modern prepend method"},
		{Name: "after", Type: SinkTypeDOMManipulation, AlwaysDangerous: false, JSContext: false, Description: "Inserts after element"},
		{Name: "before", Type: SinkTypeDOMManipulation, AlwaysDangerous: false, JSContext: false, Description: "Inserts before element"},
		{Name: "replaceWith", Type: SinkTypeDOMManipulation, AlwaysDangerous: false, JSContext: false, Description: "Replaces element"},
		{Name: "insertAdjacentElement", Type: SinkTypeDOMManipulation, AlwaysDangerous: false, JSContext: false, Description: "Inserts element at position"},
		{Name: "createContextualFragment", Type: SinkTypeDOMManipulation, AlwaysDangerous: true, JSContext: false, Description: "Creates fragment from HTML string"},

		// Event Handler property sinks - direct JavaScript execution
		{Name: "onclick", Type: SinkTypeEventHandler, AlwaysDangerous: true, JSContext: true, Description: "Click event handler property"},
		{Name: "onerror", Type: SinkTypeEventHandler, AlwaysDangerous: true, JSContext: true, Description: "Error event handler property"},
		{Name: "onload", Type: SinkTypeEventHandler, AlwaysDangerous: true, JSContext: true, Description: "Load event handler property"},
		{Name: "onfocus", Type: SinkTypeEventHandler, AlwaysDangerous: true, JSContext: true, Description: "Focus event handler property"},
		{Name: "onblur", Type: SinkTypeEventHandler, AlwaysDangerous: true, JSContext: true, Description: "Blur event handler property"},
		{Name: "onmouseover", Type: SinkTypeEventHandler, AlwaysDangerous: true, JSContext: true, Description: "Mouseover event handler property"},
		{Name: "onmouseout", Type: SinkTypeEventHandler, AlwaysDangerous: true, JSContext: true, Description: "Mouseout event handler property"},
		{Name: "onkeydown", Type: SinkTypeEventHandler, AlwaysDangerous: true, JSContext: true, Description: "Keydown event handler property"},
		{Name: "onkeyup", Type: SinkTypeEventHandler, AlwaysDangerous: true, JSContext: true, Description: "Keyup event handler property"},
		{Name: "onchange", Type: SinkTypeEventHandler, AlwaysDangerous: true, JSContext: true, Description: "Change event handler property"},
		{Name: "oninput", Type: SinkTypeEventHandler, AlwaysDangerous: true, JSContext: true, Description: "Input event handler property"},

		// setAttribute with dangerous attributes
		{Name: "setAttribute[onclick]", Type: SinkTypeEventHandler, AlwaysDangerous: true, JSContext: true, Description: "Sets onclick attribute"},
		{Name: "setAttribute[onerror]", Type: SinkTypeEventHandler, AlwaysDangerous: true, JSContext: true, Description: "Sets onerror attribute"},
		{Name: "setAttribute[src]", Type: SinkTypeURLSetter, AlwaysDangerous: false, JSContext: false, Description: "Sets src attribute"},
		{Name: "setAttribute[href]", Type: SinkTypeURLSetter, AlwaysDangerous: false, JSContext: false, Description: "Sets href attribute"},
		{Name: "setAttribute[srcdoc]", Type: SinkTypeHTMLExecution, AlwaysDangerous: true, JSContext: false, Description: "Sets iframe srcdoc"},
		{Name: "setAttribute[action]", Type: SinkTypeURLSetter, AlwaysDangerous: false, JSContext: false, Description: "Sets form action"},
		{Name: "setAttribute[formaction]", Type: SinkTypeURLSetter, AlwaysDangerous: false, JSContext: false, Description: "Sets formaction attribute"},

		// Framework-specific sinks
		{Name: "dangerouslySetInnerHTML", Type: SinkTypeFramework, AlwaysDangerous: true, JSContext: false, Description: "React dangerous HTML setter"},
		{Name: "v-html", Type: SinkTypeFramework, AlwaysDangerous: true, JSContext: false, Description: "Vue HTML directive"},
		{Name: "ng-bind-html", Type: SinkTypeFramework, AlwaysDangerous: true, JSContext: false, Description: "AngularJS HTML binding"},
		{Name: "[innerHTML]", Type: SinkTypeFramework, AlwaysDangerous: true, JSContext: false, Description: "Angular innerHTML binding"},
		{Name: "bypassSecurityTrustHtml", Type: SinkTypeFramework, AlwaysDangerous: true, JSContext: false, Description: "Angular security bypass"},
		{Name: "bypassSecurityTrustScript", Type: SinkTypeFramework, AlwaysDangerous: true, JSContext: true, Description: "Angular script security bypass"},
		{Name: "bypassSecurityTrustUrl", Type: SinkTypeFramework, AlwaysDangerous: true, JSContext: false, Description: "Angular URL security bypass"},

		// Additional HTML execution sinks
		{Name: "srcdoc", Type: SinkTypeHTMLExecution, AlwaysDangerous: true, JSContext: false, Description: "iframe srcdoc property"},
		{Name: "DOMParser.parseFromString", Type: SinkTypeHTMLExecution, AlwaysDangerous: false, JSContext: false, Description: "Parses string to DOM"},
	}
}

// String returns a string representation of the sink type
func (t DOMXSSSinkType) String() string {
	switch t {
	case SinkTypeHTMLExecution:
		return "HTML Execution"
	case SinkTypeJSExecution:
		return "JavaScript Execution"
	case SinkTypeURLSetter:
		return "URL Setter"
	case SinkTypejQuery:
		return "jQuery"
	case SinkTypeDOMManipulation:
		return "DOM Manipulation"
	case SinkTypeEventHandler:
		return "Event Handler"
	case SinkTypeFramework:
		return "Framework-specific"
	default:
		return "Unknown"
	}
}
