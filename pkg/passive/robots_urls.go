package passive

import (
	"net/url"
	"strings"
)

// Robots holds what a robots.txt body declares, split by the role the file gives
// each value: Sitemaps are documents to read, Paths are locations to crawl.
type Robots struct {
	Paths    []string
	Sitemaps []string
}

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

// ParseRobotsTxt reads the Disallow, Allow and Sitemap directives of a robots.txt
// body and resolves the referenced locations against base. Wildcard patterns are
// reduced to their literal prefix ("/admin/*" yields "/admin/"), which keeps the
// directory they describe crawlable; root-only values are skipped as they are not
// navigable resources.
func ParseRobotsTxt(body string, base *url.URL) Robots {
	robots := Robots{Paths: []string{}, Sitemaps: []string{}}

	resolve := func(raw string) (string, bool) {
		absoluteURL, urlType, err := analyzeURL(raw, base)
		if err != nil || urlType != "web" {
			return "", false
		}
		return absoluteURL, true
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
		case "disallow", "allow":
			trimmed := trimPathPattern(value, robotsWildcards)
			if trimmed == "" {
				continue
			}
			if resolved, ok := resolve(trimmed); ok {
				robots.Paths = append(robots.Paths, resolved)
			}
		case "sitemap":
			if value == "" {
				continue
			}
			if resolved, ok := resolve(value); ok {
				robots.Sitemaps = append(robots.Sitemaps, resolved)
			}
		}
	}

	return robots
}

func extractURLsFromRobotsTxt(body string, base *url.URL) ExtractedURLS {
	robots := ParseRobotsTxt(body, base)
	return ExtractedURLS{
		Web:    mergeURLs(robots.Paths, robots.Sitemaps),
		NonWeb: []string{},
	}
}
