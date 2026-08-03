package passive

import "strings"

// robotsWildcards are the two characters robots.txt treats as special; '?' is a
// literal there and starts a real query string.
const robotsWildcards = "*$"

// appLinkWildcards are the characters apple-app-site-association path patterns
// treat as special: '*' matches any substring, '?' any single character.
const appLinkWildcards = "*?"

// trimPathPattern reduces a path pattern to the longest literal prefix that is
// still a fetchable location. robots.txt and apple-app-site-association both
// express coverage with wildcards ("/admin/*", "/*.json$"), which are not URLs:
// requesting them verbatim only produces 404s while the directory they describe
// goes uncrawled. The empty string means the pattern carries no usable prefix.
func trimPathPattern(pattern, wildcards string) string {
	pattern = strings.TrimSpace(pattern)
	if idx := strings.IndexAny(pattern, wildcards); idx >= 0 {
		pattern = pattern[:idx]
	}
	pattern = strings.TrimSpace(pattern)
	if pattern == "/" {
		return ""
	}
	return pattern
}
