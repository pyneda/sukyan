package lib

import (
	"strings"

	"github.com/gosimple/slug"
	"github.com/sergi/go-diff/diffmatchpatch"
)

func Slugify(text string) string {
	return slug.Make(text)
}

func ComputeSimilarity(aBody, bBody []byte) float64 {
	aText := string(aBody)
	bText := string(bBody)

	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(aText, bText, false)

	distance := dmp.DiffLevenshtein(diffs)

	// Calculate the maximum possible distance
	maxLen := len(aText)
	if len(bText) > maxLen {
		maxLen = len(bText)
	}

	if maxLen == 0 {
		return 1.0 // Both strings are empty, so they are identical
	}

	// Compute similarity as (1 - (distance / maxLen))
	similarity := 1 - float64(distance)/float64(maxLen)
	return similarity
}

// ContainsAnySubstring checks if any string from the list appears as a substring
// in the original string
func ContainsAnySubstring(original string, substrings []string) bool {
	for _, s := range substrings {
		if strings.Contains(original, s) {
			return true
		}
	}
	return false
}

// ContainsAnySubstringIgnoreCase checks if any string from the list appears as a substring
// in the original string, ignoring case differences
func ContainsAnySubstringIgnoreCase(original string, substrings []string) bool {
	original = strings.ToLower(original)
	for _, s := range substrings {
		if strings.Contains(original, strings.ToLower(s)) {
			return true
		}
	}
	return false
}
