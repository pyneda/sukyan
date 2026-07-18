package passive

import (
	"net/url"
	"strings"
)

// isRobotsTxtURL reports whether the given URL points at a robots.txt file,
// whose Disallow/Allow/Sitemap directives encode paths the generic extractor
// cannot see (no quotes, no scheme).
func isRobotsTxtURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSuffix(parsed.Path, "/"), "/robots.txt")
}

// extractURLsFromRobotsTxt parses the Disallow, Allow and Sitemap directives of
// a robots.txt body and resolves the referenced paths against base. Wildcard and
// root-only path values are skipped as they are not navigable resources.
func extractURLsFromRobotsTxt(body string, base *url.URL) ExtractedURLS {
	webURLs := make([]string, 0)
	nonWebURLs := make([]string, 0)

	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || raw == "/" || strings.ContainsAny(raw, "*$") {
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

	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		if idx := strings.IndexByte(value, '#'); idx >= 0 {
			value = strings.TrimSpace(value[:idx])
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "disallow", "allow", "sitemap":
			add(value)
		}
	}

	return ExtractedURLS{Web: webURLs, NonWeb: nonWebURLs}
}
