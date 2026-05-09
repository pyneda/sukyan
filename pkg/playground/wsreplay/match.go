package wsreplay

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/PaesslerAG/jsonpath"
)

// Match returns true if payload satisfies the wait_for spec.
// Returns false if the pattern is malformed (invalid regex) or the payload
// cannot be interpreted (non-JSON for json_path). Pattern validation is the
// caller's responsibility — typically done at script-save time via the
// frontend zod schema.
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
