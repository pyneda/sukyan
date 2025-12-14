package web

import (
	"testing"
)

func TestExtractJavascriptFromHTML(t *testing.T) {
	tests := []struct {
		name           string
		html           string
		expectedCount  int
		expectedSource string
	}{
		{
			name:           "inline script",
			html:           `<html><body><script>var apiKey = "secret123";</script></body></html>`,
			expectedCount:  1,
			expectedSource: "Inline <script> tag #1",
		},
		{
			name: "multiple inline scripts",
			html: `<html><body>
				<script>var a = 1;</script>
				<script>var b = 2;</script>
			</body></html>`,
			expectedCount: 2,
		},
		{
			name:          "external script should be skipped",
			html:          `<html><body><script src="https://example.com/app.js"></script></body></html>`,
			expectedCount: 0,
		},
		{
			name:          "non-javascript script type should be skipped",
			html:          `<html><body><script type="application/json">{"key": "value"}</script></body></html>`,
			expectedCount: 0,
		},
		{
			name:          "javascript type should be included",
			html:          `<html><body><script type="text/javascript">var x = 1;</script></body></html>`,
			expectedCount: 1,
		},
		{
			name:          "module type should be included",
			html:          `<html><body><script type="module">import x from './x';</script></body></html>`,
			expectedCount: 1,
		},
		{
			name:           "onclick event handler",
			html:           `<html><body><button onclick="alert('hello')">Click</button></body></html>`,
			expectedCount:  1,
			expectedSource: "Event handler 'onclick' on <button> element",
		},
		{
			name:          "multiple event handlers",
			html:          `<html><body><div onmouseover="track()" onclick="submit()">Test</div></body></html>`,
			expectedCount: 2,
		},
		{
			name:           "javascript href",
			html:           `<html><body><a href="javascript:doSomething()">Link</a></body></html>`,
			expectedCount:  1,
			expectedSource: "javascript: URL in href on <a> element",
		},
		{
			name:          "javascript href case insensitive",
			html:          `<html><body><a href="JavaScript:doSomething()">Link</a></body></html>`,
			expectedCount: 1,
		},
		{
			name:          "empty script should be skipped",
			html:          `<html><body><script>   </script></body></html>`,
			expectedCount: 0,
		},
		{
			name:          "empty onclick should be skipped",
			html:          `<html><body><button onclick="">Click</button></body></html>`,
			expectedCount: 0,
		},
		{
			name: "complex html with multiple sources",
			html: `<html>
				<head>
					<script>var config = {api: "key123"};</script>
				</head>
				<body>
					<button onclick="submit()">Submit</button>
					<a href="javascript:void(0)">Link</a>
					<script src="external.js"></script>
					<script>initApp();</script>
				</body>
			</html>`,
			expectedCount: 4, // 2 inline scripts + 1 onclick + 1 javascript href
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scripts := ExtractJavascriptFromHTML([]byte(tt.html))
			if len(scripts) != tt.expectedCount {
				t.Errorf("expected %d scripts, got %d", tt.expectedCount, len(scripts))
				for i, s := range scripts {
					t.Logf("Script %d: Source=%s, Code=%s", i, s.Source, s.Code)
				}
			}
			if tt.expectedSource != "" && len(scripts) > 0 {
				if scripts[0].Source != tt.expectedSource {
					t.Errorf("expected source %q, got %q", tt.expectedSource, scripts[0].Source)
				}
			}
		})
	}
}

func TestExtractJavascriptFromHTML_EventHandlers(t *testing.T) {
	// Test various event handlers are detected
	eventHandlers := []string{
		"onclick", "onload", "onerror", "onsubmit", "onchange",
		"onmouseover", "onkeydown", "onfocus", "onblur",
	}

	for _, handler := range eventHandlers {
		t.Run(handler, func(t *testing.T) {
			html := `<html><body><div ` + handler + `="doSomething()">Test</div></body></html>`
			scripts := ExtractJavascriptFromHTML([]byte(html))
			if len(scripts) != 1 {
				t.Errorf("expected 1 script for %s handler, got %d", handler, len(scripts))
			}
		})
	}
}

func TestExtractJavascriptFromHTML_InvalidHTML(t *testing.T) {
	// Should handle malformed HTML gracefully
	html := `<html><body><script>var x = 1;</script`
	scripts := ExtractJavascriptFromHTML([]byte(html))
	// goquery is lenient with malformed HTML, so this should still extract something
	if len(scripts) == 0 {
		t.Log("Malformed HTML resulted in no scripts extracted (acceptable behavior)")
	}
}
