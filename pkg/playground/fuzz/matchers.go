package fuzz

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// MatcherField names the projection field a rule evaluates against. Body and
// header matchers evaluate server-side (they need the full History row);
// everything else evaluates client-side against the streamed projection.
type MatcherField string

const (
	FieldStatusCode      MatcherField = "status_code"
	FieldResponseSize    MatcherField = "response_size"
	FieldWordCount       MatcherField = "word_count"
	FieldLineCount       MatcherField = "line_count"
	FieldDurationMs      MatcherField = "duration_ms"
	FieldResponseBody    MatcherField = "response_body"
	FieldResponseHeaders MatcherField = "response_headers"
	FieldPayload         MatcherField = "payload"
	FieldError           MatcherField = "error"
	FieldBaselineMatch   MatcherField = "baseline_match"
)

// MatcherOperator is the comparison op. Not every op is valid for every
// field; ValidateRule enforces the per-field op set.
type MatcherOperator string

const (
	OpEq           MatcherOperator = "eq"
	OpNeq          MatcherOperator = "neq"
	OpLt           MatcherOperator = "lt"
	OpLte          MatcherOperator = "lte"
	OpGt           MatcherOperator = "gt"
	OpGte          MatcherOperator = "gte"
	OpIn           MatcherOperator = "in"
	OpNotIn        MatcherOperator = "not_in"
	OpContains     MatcherOperator = "contains"
	OpNotContains  MatcherOperator = "not_contains"
	OpRegex        MatcherOperator = "regex"
	OpNotRegex     MatcherOperator = "not_regex"
	OpExists       MatcherOperator = "exists"
	OpNotExists    MatcherOperator = "not_exists"
)

// MatcherMode controls how the rule set affects the result table.
type MatcherMode string

const (
	MatcherModeShow MatcherMode = "show" // hide rows that don't pass all rules
	MatcherModeHide MatcherMode = "hide" // hide rows that pass all rules
)

// MatcherRule is one comparison.
type MatcherRule struct {
	Field    MatcherField    `json:"field"`
	Operator MatcherOperator `json:"operator"`
	Value    json.RawMessage `json:"value"`
}

// MatcherSet is a list of rules combined with implicit AND, plus the mode
// (show vs hide). An empty rules slice is a no-op (all rows visible).
type MatcherSet struct {
	Mode  MatcherMode    `json:"mode"`
	Rules []MatcherRule  `json:"rules"`
}

// IsServerSide reports whether the rule needs the persisted History row to
// evaluate (body/headers). The streaming engine ignores these; the api
// /match endpoint evaluates them in batch.
func (r MatcherRule) IsServerSide() bool {
	switch r.Field {
	case FieldResponseBody, FieldResponseHeaders:
		return true
	}
	return false
}

// ValidateRule checks operator validity for the given field. Returns nil if
// the rule shape is acceptable; the value content is checked at eval time.
func ValidateRule(rule MatcherRule) error {
	valid, ok := validOpsByField[rule.Field]
	if !ok {
		return fmt.Errorf("unknown matcher field %q", rule.Field)
	}
	for _, op := range valid {
		if op == rule.Operator {
			return nil
		}
	}
	return fmt.Errorf("operator %q is not valid for field %q", rule.Operator, rule.Field)
}

// ValidateSet checks every rule in the set. Returns the first error.
func ValidateSet(set MatcherSet) error {
	if set.Mode != "" && set.Mode != MatcherModeShow && set.Mode != MatcherModeHide {
		return fmt.Errorf("invalid mode %q (want \"show\" or \"hide\")", set.Mode)
	}
	for i, r := range set.Rules {
		if err := ValidateRule(r); err != nil {
			return fmt.Errorf("rule %d: %w", i, err)
		}
	}
	return nil
}

var validOpsByField = map[MatcherField][]MatcherOperator{
	FieldStatusCode:      {OpEq, OpNeq, OpLt, OpLte, OpGt, OpGte, OpIn, OpNotIn},
	FieldResponseSize:    {OpEq, OpNeq, OpLt, OpLte, OpGt, OpGte},
	FieldWordCount:       {OpEq, OpNeq, OpLt, OpLte, OpGt, OpGte},
	FieldLineCount:       {OpEq, OpNeq, OpLt, OpLte, OpGt, OpGte},
	FieldDurationMs:      {OpEq, OpNeq, OpLt, OpLte, OpGt, OpGte},
	FieldResponseBody:    {OpContains, OpNotContains, OpRegex, OpNotRegex},
	FieldResponseHeaders: {OpContains, OpNotContains, OpRegex, OpNotRegex},
	FieldPayload:         {OpContains, OpNotContains, OpRegex, OpNotRegex, OpEq, OpNeq},
	FieldError:           {OpExists, OpNotExists, OpContains, OpNotContains},
	FieldBaselineMatch:   {OpEq, OpNeq},
}

// EvalServerSide evaluates body/header rules against a single response. body
// is the response body bytes; headers is the rendered "Key: Value\r\n..."
// blob (same shape the matcher sees in the UI). Returns whether the row
// passes ALL server-side rules in the set.
//
// Non-server-side rules are skipped here (they're evaluated by the client).
// Empty set returns true.
func (s MatcherSet) EvalServerSide(body []byte, headers string) (bool, error) {
	for _, r := range s.Rules {
		if !r.IsServerSide() {
			continue
		}
		target := body
		if r.Field == FieldResponseHeaders {
			target = []byte(headers)
		}
		ok, err := evalStringRule(r, string(target))
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func evalStringRule(r MatcherRule, target string) (bool, error) {
	var needle string
	if err := json.Unmarshal(r.Value, &needle); err != nil {
		// allow non-string values to fall through as their JSON form
		needle = strings.Trim(string(r.Value), "\"")
	}
	switch r.Operator {
	case OpContains:
		return strings.Contains(target, needle), nil
	case OpNotContains:
		return !strings.Contains(target, needle), nil
	case OpRegex:
		re, err := regexp.Compile(needle)
		if err != nil {
			return false, fmt.Errorf("invalid regex: %w", err)
		}
		return re.MatchString(target), nil
	case OpNotRegex:
		re, err := regexp.Compile(needle)
		if err != nil {
			return false, fmt.Errorf("invalid regex: %w", err)
		}
		return !re.MatchString(target), nil
	}
	return false, fmt.Errorf("operator %q not supported for string field", r.Operator)
}
