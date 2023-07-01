package passive

import "regexp"

func compilePatterns(patterns ...string) []*regexp.Regexp {
	reSlice := make([]*regexp.Regexp, len(patterns))
	for i, pattern := range patterns {
		reSlice[i] = regexp.MustCompile(pattern)
	}
	return reSlice
}
