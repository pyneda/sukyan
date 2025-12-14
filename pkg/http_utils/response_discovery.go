package http_utils

import (
	"strings"
)

// DiscoverQueryParamsFromBody extracts query parameter names from an HTTP response body
// by looking for patterns like urlParams.get('name'), searchParams.get('name'), etc.
// This is useful for discovering which parameters a page's JavaScript accesses.
func DiscoverQueryParamsFromBody(body string) []string {
	paramSet := make(map[string]bool)

	// Patterns that indicate URLSearchParams parameter access
	patterns := []string{
		`.get('`,
		`.get("`,
		`.has('`,
		`.has("`,
		`.getAll('`,
		`.getAll("`,
	}

	for _, pattern := range patterns {
		idx := 0
		for {
			pos := strings.Index(body[idx:], pattern)
			if pos == -1 {
				break
			}
			pos += idx + len(pattern)
			// Find the closing quote
			quote := body[pos-1 : pos]
			endPos := strings.Index(body[pos:], quote)
			if endPos > 0 && endPos < 50 { // Reasonable param name length
				param := body[pos : pos+endPos]
				if IsValidParamName(param) {
					paramSet[param] = true
				}
			}
			idx = pos
		}
	}

	result := make([]string, 0, len(paramSet))
	for param := range paramSet {
		result = append(result, param)
	}
	return result
}

// DiscoverStorageKeysFromBody extracts storage key names from an HTTP response body
// by looking for patterns like localStorage.getItem('key'), sessionStorage.setItem('key'), etc.
// storageType should be either "localStorage" or "sessionStorage".
func DiscoverStorageKeysFromBody(body string, storageType string) []string {
	keySet := make(map[string]bool)

	// Method-based patterns: localStorage.getItem('key'), localStorage.setItem('key', value)
	getItemPatterns := []string{
		storageType + `.getItem('`,
		storageType + `.getItem("`,
		storageType + `.setItem('`,
		storageType + `.setItem("`,
	}

	for _, pattern := range getItemPatterns {
		idx := 0
		for {
			pos := strings.Index(body[idx:], pattern)
			if pos == -1 {
				break
			}
			pos += idx + len(pattern)
			// Find the closing quote
			quote := body[pos-1 : pos]
			endPos := strings.Index(body[pos:], quote)
			if endPos > 0 && endPos < 50 { // Reasonable key length
				key := body[pos : pos+endPos]
				if IsValidStorageKey(key) {
					keySet[key] = true
				}
			}
			idx = pos
		}
	}

	// Bracket notation patterns: localStorage['key'], localStorage["key"]
	bracketPatterns := []string{
		storageType + `['`,
		storageType + `["`,
	}

	for _, pattern := range bracketPatterns {
		idx := 0
		for {
			pos := strings.Index(body[idx:], pattern)
			if pos == -1 {
				break
			}
			pos += idx + len(pattern)
			quote := body[pos-1 : pos]
			endPos := strings.Index(body[pos:], quote)
			if endPos > 0 && endPos < 50 {
				key := body[pos : pos+endPos]
				if IsValidStorageKey(key) {
					keySet[key] = true
				}
			}
			idx = pos
		}
	}

	result := make([]string, 0, len(keySet))
	for k := range keySet {
		result = append(result, k)
	}
	return result
}

// IsValidParamName checks if a string looks like a valid query parameter name.
// Valid names contain only alphanumeric characters, underscores, hyphens, and dots.
// Maximum length is 50 characters.
func IsValidParamName(name string) bool {
	if len(name) == 0 || len(name) > 50 {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

// IsValidStorageKey checks if a string looks like a valid storage key name.
// Valid keys contain only alphanumeric characters, underscores, hyphens, and dots.
// Maximum length is 50 characters.
func IsValidStorageKey(name string) bool {
	if len(name) == 0 || len(name) > 50 {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

// CommonQueryParamFallbacks returns a list of commonly used query parameter names
// that can be used as fallback when no parameters are discovered from other sources.
var CommonQueryParamFallbacks = []string{
	"query", "q", "search", "s", "keyword",
	"url", "redirect", "return", "next", "callback",
	"page", "path", "id", "data", "input", "value",
}
