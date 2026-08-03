package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// report.js names SukyanSyntax too, so only the IIFE's own binding proves the
// bundle itself was embedded.
const bundleMarker = "var SukyanSyntax"

func generateHTML(t *testing.T, options ReportOptions) string {
	t.Helper()
	options.Format = ReportFormatHTML
	var buf bytes.Buffer
	require.NoError(t, GenerateReport(options, &buf))
	return buf.String()
}

// The bundle is inlined verbatim into <script>, so any `<script`/`</script` in
// it steers the HTML parser out of script data and truncates or swallows the
// rest of the report. It is generated from sukyan-ui's grammars, so a change
// upstream is exactly how this would start being violated.
func TestSyntaxAssetsCannotEscapeTheirTag(t *testing.T) {
	script := strings.ToLower(mustAsset("syntax.js"))
	assert.NotContains(t, script, "<script")
	assert.NotContains(t, script, "</script")

	style := strings.ToLower(mustAsset("syntax.css"))
	assert.NotContains(t, style, "</style")
}

func TestSyntaxBundleExposesTokenize(t *testing.T) {
	script := mustAsset("syntax.js")
	assert.Contains(t, script, "SukyanSyntax")
	assert.Contains(t, script, "tokenize")

	style := mustAsset("syntax.css")
	assert.Contains(t, style, ".tok-header-name")
	assert.Contains(t, style, ".tok-status-error")
}

// Every role the tokeniser can emit needs a rule, or that token renders in the
// body colour and the highlighting silently loses a distinction.
func TestSyntaxStylesheetCoversEveryRoleTheBundleEmits(t *testing.T) {
	style := mustAsset("syntax.css")
	for _, role := range []string{
		"attr-name", "attr-value", "boolean", "builtin", "char", "class-name",
		"comment", "constant", "entity", "function", "header-name", "keyword",
		"method", "number", "operator", "prolog", "property", "punctuation",
		"section", "selector", "status-error", "status-info", "status-ok",
		"status-warn", "string", "symbol", "tag", "type", "variable",
	} {
		assert.Contains(t, style, ".tok-"+role+" {", "role %q has no rule", role)
	}
}

func TestSyntaxHighlightIsOnByDefault(t *testing.T) {
	content := generateHTML(t, ReportOptions{Title: "Default"})

	assert.Contains(t, content, bundleMarker, "the zero value must produce the report the UI shows")
	assert.Contains(t, content, "--syntax-header-name")
}

func TestSyntaxHighlightCanBeDisabled(t *testing.T) {
	on := generateHTML(t, ReportOptions{Title: "On"})
	off := generateHTML(t, ReportOptions{Title: "Off", DisableSyntaxHighlight: true})

	assert.NotContains(t, off, bundleMarker)
	assert.NotContains(t, off, "--syntax-header-name")
	assert.Less(t, len(off), len(on), "disabling must drop the assets, not just the markup")

	// The report still has to render; report.js falls back to plain text.
	assert.Contains(t, off, "__SUKYAN_REPORT__")
	assert.Contains(t, off, "code-body")
}

// Highlighting must not reintroduce a fetch at open time.
func TestHighlightedReportStaysSelfContained(t *testing.T) {
	content := generateHTML(t, ReportOptions{Title: "Self Contained"})

	for _, forbidden := range []string{"<script src=", "<link rel=\"stylesheet\"", "@import", "https://cdn."} {
		assert.NotContains(t, content, forbidden)
	}
}
