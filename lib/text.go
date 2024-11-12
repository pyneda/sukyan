package lib

import (
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
