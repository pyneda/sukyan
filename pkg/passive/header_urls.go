package passive

import (
	"net/url"
	"regexp"
	"strings"
)

// linkHeaderURLRegex captures the URI-Reference inside the angle brackets of an
// RFC 8288 Link header field, e.g. `</resource>; rel="preload"`.
var linkHeaderURLRegex = regexp.MustCompile(`<([^>]+)>`)

// refreshURLRegex captures the target of a Refresh header or an equivalent
// meta refresh directive, e.g. `5; url=/next` or `0;URL='/next'`.
var refreshURLRegex = regexp.MustCompile(`(?i)url\s*=\s*['"]?([^'"\s;]+)`)

// extractURLsFromKnownHeaders parses response headers whose values encode URLs
// with a structured syntax the generic quoted-string extractor cannot handle
// (angle brackets, url= prefixes). Values are resolved against base.
func extractURLsFromKnownHeaders(headers map[string][]string, base *url.URL) ExtractedURLS {
	webURLs := make([]string, 0)
	nonWebURLs := make([]string, 0)

	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		absoluteURL, urlType, err := analyzeURL(raw, base)
		if err != nil {
			return
		}
		switch urlType {
		case "web":
			webURLs = append(webURLs, absoluteURL)
		case "non-web":
			nonWebURLs = append(nonWebURLs, absoluteURL)
		}
	}

	for name, values := range headers {
		switch {
		case strings.EqualFold(name, "Link"):
			for _, value := range values {
				for _, match := range linkHeaderURLRegex.FindAllStringSubmatch(value, -1) {
					add(match[1])
				}
			}
		case strings.EqualFold(name, "Refresh"):
			for _, value := range values {
				if match := refreshURLRegex.FindStringSubmatch(value); match != nil {
					add(match[1])
				}
			}
		}
	}

	return ExtractedURLS{Web: webURLs, NonWeb: nonWebURLs}
}
