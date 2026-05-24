package wsreplay

import (
	"regexp"
	"strings"
)

var varRefRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// SubstituteVars expands ${name} references against the provided vars map.
// Unknown names are left as literal "${name}" — callers that need to treat
// unknown references as an error (the safe default for production runs)
// should use SubstituteVarsStrict instead.
//
// Returns a warning bool that is true when any substituted value contained
// `§§` (which could be misread as a payload marker if it appeared in the
// original content; v1 does NOT re-run payload substitution after vars, so
// this is a heads-up not a defect).
func SubstituteVars(content string, vars map[string]string) (string, bool) {
	out, _, warn := substitute(content, vars)
	return out, warn
}

// SubstituteVarsStrict behaves like SubstituteVars but also returns the
// distinct, ordered list of `${name}` references that did not resolve. An
// empty slice means every reference resolved. The caller is responsible for
// deciding whether unresolved refs should fail the iteration — wsfuzz's
// engine and wsreplay's send path both treat a non-empty list as fatal so
// a typo'd var name doesn't silently ship the literal `${...}` to the peer.
func SubstituteVarsStrict(content string, vars map[string]string) (out string, undefined []string, warn bool) {
	return substitute(content, vars)
}

func substitute(content string, vars map[string]string) (string, []string, bool) {
	warn := false
	seenMissing := map[string]struct{}{}
	missing := make([]string, 0)
	out := varRefRE.ReplaceAllStringFunc(content, func(match string) string {
		name := varRefRE.FindStringSubmatch(match)[1]
		val, ok := vars[name]
		if !ok {
			if _, dup := seenMissing[name]; !dup {
				seenMissing[name] = struct{}{}
				missing = append(missing, name)
			}
			return match
		}
		if strings.Contains(val, "§§") {
			warn = true
		}
		return val
	})
	return out, missing, warn
}
