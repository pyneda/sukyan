package wsreplay

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/PaesslerAG/jsonpath"
)

// Match returns true if `payload` satisfies the wait_for spec. Any unparseable inputs return false.
func Match(spec WaitForSpec, payload string) bool {
	switch spec.MatchType {
	case MatchAny:
		return true
	case MatchContains:
		return strings.Contains(payload, spec.Pattern)
	case MatchRegex:
		re, err := regexp.Compile(spec.Pattern)
		if err != nil {
			return false
		}
		return re.MatchString(payload)
	case MatchJSONPath:
		var v any
		if err := json.Unmarshal([]byte(payload), &v); err != nil {
			return false
		}
		_, err := jsonpath.Get(spec.Pattern, v)
		return err == nil
	}
	return false
}
