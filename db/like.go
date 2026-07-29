package db

import "strings"

// likePatternEscaper escapes the three characters PostgreSQL's LIKE/ILIKE treats as
// pattern syntax rather than as literal text. Backslash goes first, otherwise the
// backslashes this escaper introduces would be escaped again.
var likePatternEscaper = strings.NewReplacer(
	`\`, `\\`,
	`%`, `\%`,
	`_`, `\_`,
)

// containsPattern turns a user-supplied search term into an ILIKE pattern that matches
// the term literally.
//
// Search terms reach these queries straight from a filter box, and a term is text the
// user expects to be matched as typed. Interpolating it raw means "50%" matches every
// row (the trailing % becomes "anything"), "v1_beta" quietly matches "v1-beta", and a
// lone "%" or "_" returns the whole table. The parameter binding already rules out
// injection; this is purely about the pattern metacharacters inside the bound value,
// which binding does not touch.
//
// PostgreSQL's default LIKE escape character is backslash, so no ESCAPE clause is
// needed alongside the escaped value.
func containsPattern(term string) string {
	return "%" + likePatternEscaper.Replace(term) + "%"
}
