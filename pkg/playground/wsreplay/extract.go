package wsreplay

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strconv"

	"github.com/PaesslerAG/jsonpath"
)

// ExtractMethod names how the receive-frame value is parsed to populate a
// run-scoped variable.
type ExtractMethod string

const (
	ExtractMethodRegexGroup ExtractMethod = "regex_group"
	ExtractMethodJSONPath   ExtractMethod = "json_path"
	ExtractMethodFull       ExtractMethod = "full"
)

// Extraction captures a value from the last frame received during this step
// into a run-scoped variable. WS Replay is single-execution (unlike WS Fuzz
// which iterates), so variables live for the entire run and are visible to
// every later step's ${name} substitution.
//
// On failure, the run terminates with failure_reason="extract <name> failed"
// unless OnFailure is "continue", in which case the variable is set to "" and
// the run proceeds. The strict default mirrors WS Fuzz's FallbackAbort policy
// — silent extraction failures cause downstream confusion.
type Extraction struct {
	Name      string        `json:"name"`
	Method    ExtractMethod `json:"method"`
	Pattern   string        `json:"pattern,omitempty"`
	Group     string        `json:"group_or_path,omitempty"`
	OnFailure string        `json:"on_failure,omitempty"` // "abort" (default) | "continue"
}

// Apply runs the extraction against `frame`. The boolean second return is
// true when a value was captured. The string is the captured value (always
// "" when the boolean is false). Callers decide what to do with failures
// based on the Extraction's OnFailure policy.
func (ext Extraction) Apply(frame Frame) (string, bool) {
	switch ext.Method {
	case ExtractMethodFull:
		if frame.Opcode == 2 {
			// Binary frames are base64-encoded to keep variables stringy.
			return base64.StdEncoding.EncodeToString([]byte(frame.Content)), true
		}
		return frame.Content, true
	case ExtractMethodRegexGroup:
		return extractRegexGroup(ext, frame.Content)
	case ExtractMethodJSONPath:
		if frame.Opcode == 2 {
			// JSON-path against binary content is almost certainly a user
			// error — fail explicitly so the run stops instead of silently
			// returning "".
			return "", false
		}
		return extractJSONPath(ext, frame.Content)
	}
	return "", false
}

func extractRegexGroup(ext Extraction, s string) (string, bool) {
	re, err := regexp.Compile(ext.Pattern)
	if err != nil {
		return "", false
	}
	m := re.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	groupIdx := 1
	if ext.Group != "" {
		if n, err := strconv.Atoi(ext.Group); err == nil {
			groupIdx = n
		}
	}
	if groupIdx < 0 || groupIdx >= len(m) {
		return "", false
	}
	return m[groupIdx], true
}

func extractJSONPath(ext Extraction, s string) (string, bool) {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return "", false
	}
	res, err := jsonpath.Get(ext.Group, v)
	if err != nil {
		return "", false
	}
	switch t := res.(type) {
	case string:
		return t, true
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64), true
	case bool:
		return strconv.FormatBool(t), true
	case nil:
		return "", true
	}
	b, _ := json.Marshal(res)
	return string(b), true
}
