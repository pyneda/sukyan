package wsfuzz

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
)

// evaluateCheck evaluates a CheckAssertion against the iteration's collected
// frames + variables. Returns (passed, details). The details capture each
// rule's outcome for the per-iteration "Check assertions" UI tab.
//
// v1 limitations (documented):
//   - FieldWsReceivedFrameAt is treated as "concatenated received frames" —
//     operator semantics apply to the combined buffer. Per-index N selection
//     is future work (requires extending MatcherRule to carry a parametric arg).
//   - FieldWsVariable matches if ANY variable in scope satisfies the operator.
//     Per-name binding is future work.
func evaluateCheck(ca CheckAssertion, frames []wsreplay.Frame, vars map[string]string) (bool, checkResultEntry) {
	out := checkResultEntry{Logic: ca.Logic}
	results := make([]bool, 0, len(ca.Rules))
	for _, r := range ca.Rules {
		actual, ruleOK := evalRule(r, frames, vars)
		out.Rules = append(out.Rules, checkRuleOutcome{
			Field:    string(r.Field),
			Operator: string(r.Operator),
			Value:    string(r.Value),
			Actual:   actual,
			Passed:   ruleOK,
		})
		results = append(results, ruleOK)
	}
	combined := combineResults(ca.Logic, results)
	if ca.Negate {
		combined = !combined
	}
	out.Passed = combined
	return combined, out
}

func combineResults(logic AssertionLogic, results []bool) bool {
	switch logic {
	case LogicOr:
		for _, r := range results {
			if r {
				return true
			}
		}
		return false
	case LogicAnd, "":
		for _, r := range results {
			if !r {
				return false
			}
		}
		return true
	}
	return false
}

func evalRule(r fuzz.MatcherRule, frames []wsreplay.Frame, vars map[string]string) (actual string, passed bool) {
	switch r.Field {
	case fuzz.FieldWsReceivedFrameCount:
		n := countReceived(frames)
		actual = strconv.Itoa(n)
		return actual, opIntCompare(n, r)

	case fuzz.FieldWsTotalReceivedBytes:
		total := 0
		for _, f := range frames {
			if f.Direction == "received" {
				total += len(f.Content)
			}
		}
		actual = strconv.Itoa(total)
		return actual, opIntCompare(total, r)

	case fuzz.FieldWsReceivedFrameAt:
		// v1: concatenate all received frames into one buffer.
		var sb strings.Builder
		for _, f := range frames {
			if f.Direction == "received" {
				sb.WriteString(f.Content)
				sb.WriteByte('\n')
			}
		}
		actual = strings.TrimRight(sb.String(), "\n")
		return actual, opStringCompare(actual, r)

	case fuzz.FieldWsVariable:
		// v1: ANY variable matches.
		for _, v := range vars {
			if opStringCompare(v, r) {
				return v, true
			}
		}
		return "", false
	}
	return "", false
}

func countReceived(frames []wsreplay.Frame) int {
	n := 0
	for _, f := range frames {
		if f.Direction == "received" {
			n++
		}
	}
	return n
}

func opIntCompare(actual int, r fuzz.MatcherRule) bool {
	want, ok := parseIntValue(r.Value)
	if !ok {
		return false
	}
	switch r.Operator {
	case fuzz.OpEq:
		return actual == want
	case fuzz.OpNeq:
		return actual != want
	case fuzz.OpLt:
		return actual < want
	case fuzz.OpLte:
		return actual <= want
	case fuzz.OpGt:
		return actual > want
	case fuzz.OpGte:
		return actual >= want
	}
	return false
}

func opStringCompare(actual string, r fuzz.MatcherRule) bool {
	if r.Operator == fuzz.OpIsEmpty {
		return actual == ""
	}
	if r.Operator == fuzz.OpIsNotEmpty {
		return actual != ""
	}
	want, _ := parseStringValue(r.Value)
	switch r.Operator {
	case fuzz.OpEq:
		return actual == want
	case fuzz.OpNeq:
		return actual != want
	case fuzz.OpContains:
		return strings.Contains(actual, want)
	case fuzz.OpNotContains:
		return !strings.Contains(actual, want)
	case fuzz.OpRegex:
		re, err := regexp.Compile(want)
		return err == nil && re.MatchString(actual)
	case fuzz.OpNotRegex:
		re, err := regexp.Compile(want)
		return err == nil && !re.MatchString(actual)
	}
	return false
}

func parseIntValue(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		v, err := strconv.Atoi(s)
		return v, err == nil
	}
	return 0, false
}

func parseStringValue(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, true
	}
	// Fall back to the raw token (e.g., a number written without quotes).
	return strings.Trim(string(raw), `"`), true
}
