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
	OpEq          MatcherOperator = "eq"
	OpNeq         MatcherOperator = "neq"
	OpLt          MatcherOperator = "lt"
	OpLte         MatcherOperator = "lte"
	OpGt          MatcherOperator = "gt"
	OpGte         MatcherOperator = "gte"
	OpIn          MatcherOperator = "in"
	OpNotIn       MatcherOperator = "not_in"
	OpContains    MatcherOperator = "contains"
	OpNotContains MatcherOperator = "not_contains"
	OpRegex       MatcherOperator = "regex"
	OpNotRegex    MatcherOperator = "not_regex"
	OpExists      MatcherOperator = "exists"
	OpNotExists   MatcherOperator = "not_exists"
	// Extra operators used by WS string-typed fields.
	OpIsEmpty    MatcherOperator = "is_empty"
	OpIsNotEmpty MatcherOperator = "is_not_empty"
)

// MatcherMode controls how the rule set affects the result table.
type MatcherMode string

const (
	MatcherModeShow MatcherMode = "show" // hide rows that don't pass all rules
	MatcherModeHide MatcherMode = "hide" // hide rows that pass all rules
)

// MatcherDomain partitions the field/operator vocabulary so that HTTP fuzz
// and WS fuzz can share the matcher infrastructure without their field names
// colliding. Default zero value is DomainHTTP (back-compat with all existing
// callers in pkg/playground/fuzz and api/).
type MatcherDomain int

const (
	DomainHTTP   MatcherDomain = 0
	DomainWsFuzz MatcherDomain = 1
)

// WS fuzz matcher fields (registered under DomainWsFuzz).
const (
	FieldWsIterationStatus        MatcherField = "iteration.status"
	FieldWsIterationDurationMs    MatcherField = "iteration.duration_ms"
	FieldWsIterationBaselineMatch MatcherField = "iteration.baseline_match"
	FieldWsIterationPeerCloseCode MatcherField = "iteration.peer_close_code"
	FieldWsHandshakeStatus        MatcherField = "handshake.status"
	FieldWsHandshakeHeader        MatcherField = "handshake.header"
	FieldWsReceivedFrameCount     MatcherField = "received_frame_count"
	FieldWsTotalReceivedBytes     MatcherField = "total_received_bytes"
	FieldWsReceivedFrameAt        MatcherField = "received_frame_at"
	FieldWsStepReceivedFrame      MatcherField = "step.received_frame"
	FieldWsStepDurationMs         MatcherField = "step.duration_ms"
	FieldWsStepMatched            MatcherField = "step.matched"
	FieldWsVariable               MatcherField = "variables"
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
	// Domain selects the field/operator taxonomy. Zero value (DomainHTTP)
	// preserves back-compat for the HTTP fuzz call sites that predate this
	// refactor. Not serialised: the caller decides the domain by code path.
	Domain MatcherDomain `json:"-"`
	Mode   MatcherMode   `json:"mode"`
	Rules  []MatcherRule `json:"rules"`
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

var validOpsByDomain = map[MatcherDomain]map[MatcherField][]MatcherOperator{
	DomainHTTP: {
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
	},
	DomainWsFuzz: {
		FieldWsIterationStatus:        {OpEq, OpNeq, OpIn, OpNotIn},
		FieldWsIterationDurationMs:    {OpEq, OpNeq, OpLt, OpLte, OpGt, OpGte},
		FieldWsIterationBaselineMatch: {OpEq, OpNeq},
		FieldWsIterationPeerCloseCode: {OpEq, OpNeq, OpIn, OpNotIn},
		FieldWsHandshakeStatus:        {OpEq, OpNeq, OpLt, OpLte, OpGt, OpGte, OpIn, OpNotIn},
		FieldWsHandshakeHeader:        {OpContains, OpNotContains, OpRegex, OpNotRegex, OpEq, OpNeq, OpIsEmpty, OpIsNotEmpty},
		FieldWsReceivedFrameCount:     {OpEq, OpNeq, OpLt, OpLte, OpGt, OpGte},
		FieldWsTotalReceivedBytes:     {OpEq, OpNeq, OpLt, OpLte, OpGt, OpGte},
		FieldWsReceivedFrameAt:        {OpContains, OpNotContains, OpRegex, OpNotRegex, OpEq, OpNeq, OpIsEmpty, OpIsNotEmpty},
		FieldWsStepReceivedFrame:      {OpContains, OpNotContains, OpRegex, OpNotRegex, OpEq, OpNeq, OpIsEmpty, OpIsNotEmpty},
		FieldWsStepDurationMs:         {OpEq, OpNeq, OpLt, OpLte, OpGt, OpGte},
		FieldWsStepMatched:            {OpEq, OpNeq},
		FieldWsVariable:               {OpContains, OpNotContains, OpRegex, OpNotRegex, OpEq, OpNeq, OpIsEmpty, OpIsNotEmpty},
		FieldPayload:                  {OpContains, OpNotContains, OpRegex, OpNotRegex, OpEq, OpNeq},
		FieldError:                    {OpExists, OpNotExists, OpContains, OpNotContains},
	},
}

// ValidateRule checks operator validity for the given field within the given
// domain. Returns nil if the rule shape is acceptable; the value content is
// checked at eval time.
func ValidateRule(rule MatcherRule, domain MatcherDomain) error {
	byField, ok := validOpsByDomain[domain]
	if !ok {
		return fmt.Errorf("unknown matcher domain %d", domain)
	}
	valid, ok := byField[rule.Field]
	if !ok {
		return fmt.Errorf("unknown matcher field %q for domain %d", rule.Field, domain)
	}
	for _, op := range valid {
		if op == rule.Operator {
			return nil
		}
	}
	return fmt.Errorf("operator %q is not valid for field %q in domain %d", rule.Operator, rule.Field, domain)
}

// ValidateSet checks every rule in the set. Returns the first error.
func ValidateSet(set MatcherSet) error {
	if set.Mode != "" && set.Mode != MatcherModeShow && set.Mode != MatcherModeHide {
		return fmt.Errorf("invalid mode %q (want \"show\" or \"hide\")", set.Mode)
	}
	for i, r := range set.Rules {
		if err := ValidateRule(r, set.Domain); err != nil {
			return fmt.Errorf("rule %d: %w", i, err)
		}
	}
	return nil
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
