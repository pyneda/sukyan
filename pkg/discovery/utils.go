package discovery

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/pyneda/sukyan/db"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func setDefaultHeaders(req *http.Request, hasBody bool) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", DefaultUserAgent)
	}
	if req.Header.Get("Connection") == "" {
		req.Header.Set("Connection", "keep-alive")
	}
	if hasBody && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
}

// isHTMLResponse is a convenience wrapper around history.IsHTMLResponse()
// for use in discovery validation functions
func isHTMLResponse(history *db.History) bool {
	return history.IsHTMLResponse()
}

// looksLikeDirectoryListing checks if the response appears to be an actual directory listing
func looksLikeDirectoryListing(bodyStr string) bool {
	bodyLower := strings.ToLower(bodyStr)

	// Common directory listing indicators
	indicators := []string{
		"index of /",
		"index of",
		"directory listing",
		"<pre>",
		"[dir]",
		"[parentdir]",
		"parent directory",
		"<h1>index of",
	}

	matches := 0
	for _, indicator := range indicators {
		if strings.Contains(bodyLower, indicator) {
			matches++
		}
	}

	// Need multiple indicators to avoid false positives
	return matches >= 2
}

// validateTOMLContent checks if the content is valid TOML format
func validateTOMLContent(bodyStr string) bool {
	// TOML should have section headers [section] or key = value pairs
	lines := strings.Split(bodyStr, "\n")
	validLines := 0

	sectionRegex := regexp.MustCompile(`^\s*\[[^\]]+\]\s*$`)
	keyValueRegex := regexp.MustCompile(`^\s*[\w-]+\s*=\s*.+$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if sectionRegex.MatchString(line) || keyValueRegex.MatchString(line) {
			validLines++
		}
	}

	return validLines >= 2
}

// validateLockFileFormat checks if content looks like a lock file (JSON or YAML with hashes/versions)
func validateLockFileFormat(bodyStr string, isJSON bool) bool {
	if isJSON {
		// Should be valid JSON with lockfile-specific fields
		return strings.Contains(bodyStr, "\"version\"") &&
			(strings.Contains(bodyStr, "\"resolved\"") ||
				strings.Contains(bodyStr, "\"integrity\"") ||
				strings.Contains(bodyStr, "\"dependencies\"") ||
				strings.Contains(bodyStr, "\"packages\""))
	}

	// YAML lock files (like yarn.lock, pnpm-lock.yaml)
	return (strings.Contains(bodyStr, "version:") || strings.Contains(bodyStr, "resolution:")) &&
		(strings.Contains(bodyStr, "integrity:") || strings.Contains(bodyStr, "checksum:") || strings.Contains(bodyStr, "dependencies:"))
}
