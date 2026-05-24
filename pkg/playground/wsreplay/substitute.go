package wsfuzz

import (
	"regexp"
	"strings"
)

var varRefRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// SubstituteVars expands ${name} references against the provided vars map.
// Unknown names are left as literal "${name}". Returns a warning bool that
// is true when any substituted value contained `§§` (which could be misread
// as a payload marker if it appeared in the original content; v1 does NOT
// re-run payload substitution after vars, so this is a heads-up not a defect).
func SubstituteVars(content string, vars map[string]string) (string, bool) {
	warn := false
	out := varRefRE.ReplaceAllStringFunc(content, func(match string) string {
		name := varRefRE.FindStringSubmatch(match)[1]
		val, ok := vars[name]
		if !ok {
			return match
		}
		if strings.Contains(val, "§§") {
			warn = true
		}
		return val
	})
	return out, warn
}
