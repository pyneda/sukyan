// Package fuzz is sukyan's HTTP fuzzer engine. It owns:
//   - Mode strategies (Single / All / Paired / Combinations) that produce
//     per-request payload assignments
//   - Payload resolution (processor chains, wordlist expansion)
//   - The Run loop: rate limiting, per-host concurrency, retries, progress
//   - Request-count preview (no I/O)
//
// State lives on PlaygroundFuzzRun in the db package; this package never
// touches the database directly except through small typed helpers passed in
// by the API layer.
package fuzz

import (
	"time"
)

// FuzzMode is the attack-mode taxonomy. Names are sukyan-native, not Burp's;
// see docs/superpowers/specs/2026-05-18-http-fuzzer-parity-decisions.md.
type FuzzMode string

const (
	// ModeSingle iterates one shared payload list, injecting into one position
	// at a time and leaving the other positions at their original value.
	// Request count = payloads × positions. (Burp: Sniper, Caido: Sequential.)
	ModeSingle FuzzMode = "single"

	// ModeAll iterates one shared payload list, injecting the same payload
	// into every position simultaneously.
	// Request count = payloads. (Burp: Battering Ram, Caido: All.)
	ModeAll FuzzMode = "all"

	// ModePaired walks per-position payload lists in lockstep by index.
	// Request count = min(list sizes). (Burp: Pitchfork, Caido: Parallel.)
	ModePaired FuzzMode = "paired"

	// ModeCombinations is the full cartesian product across per-position lists.
	// Request count = product of list sizes. (Burp: Cluster Bomb, Caido: Matrix.)
	ModeCombinations FuzzMode = "combinations"
)

// IsValid reports whether the value is one of the four known modes.
func (m FuzzMode) IsValid() bool {
	switch m {
	case ModeSingle, ModeAll, ModePaired, ModeCombinations:
		return true
	}
	return false
}

// FuzzerPayloadsGroup is one source of payloads for an insertion point.
// Either Payloads (inline) or Wordlist (filesystem) is non-empty; both may
// be present (concatenated). Processors are applied to each yielded payload
// in order.
type FuzzerPayloadsGroup struct {
	Payloads   []string `json:"payloads"`
	Type       string   `json:"type"`
	Processors []string `json:"processors,omitempty" validate:"omitempty,dive,oneof=base64encode base64decode urlencode urldecode sha1hash sha256hash md5hash" example:"base64encode"`
	Wordlist   string   `json:"wordlist,omitempty"`
}

// FuzzerPosition is one insertion point in the raw request, defined by a
// byte-offset range. PayloadGroups is only meaningful for Paired/Combinations
// modes; Single/All read payloads from the run-level SharedPayloads instead.
type FuzzerPosition struct {
	Start         int                   `json:"start"`
	End           int                   `json:"end"`
	OriginalValue string                `json:"originalValue"`
	PayloadGroups []FuzzerPayloadsGroup `json:"payload_groups,omitempty"`
}

// FuzzerExecutionOptions controls *how* the engine runs (concurrency, RPS,
// retries, timeouts). Separate from RequestOptions, which controls how each
// individual request is shaped.
type FuzzerExecutionOptions struct {
	Concurrency           int     `json:"concurrency" validate:"min=1,max=200"`
	RPS                   int     `json:"rps" validate:"min=0,max=1000"`
	PerHostConcurrency    int     `json:"per_host_concurrency" validate:"min=0,max=100"`
	RequestTimeoutSeconds int     `json:"request_timeout_seconds" validate:"min=1,max=300"`
	Retries               int     `json:"retries" validate:"min=0,max=10"`
	RetryOn               []int   `json:"retry_on,omitempty"`
	JitterMs              int     `json:"jitter_ms" validate:"min=0,max=10000"`
	MaxDurationSeconds    int     `json:"max_duration_seconds" validate:"min=0"`
	StopOnErrorRate       float64 `json:"stop_on_error_rate" validate:"min=0,max=1"`
}

// DefaultExecutionOptions returns the engine defaults (matches current
// hardcoded behaviour where applicable: concurrency=30, request_timeout=30s,
// everything else off).
func DefaultExecutionOptions() FuzzerExecutionOptions {
	return FuzzerExecutionOptions{
		Concurrency:           30,
		RequestTimeoutSeconds: 30,
	}
}

// RequestOptions shapes each outbound request. Kept here (not imported from
// pkg/manual) to break the dependency between the new fuzz engine and the
// legacy manual package, which holds replay-specific concerns.
type RequestOptions struct {
	FollowRedirects     bool `json:"follow_redirects"`
	MaxRedirects        int  `json:"max_redirects"`
	UpdateContentLength bool `json:"update_content_length"`
	UpdateHostHeader    bool `json:"update_host_header"`
}

// FuzzerConfig is the full configuration that gets snapshotted on
// PlaygroundFuzzRun.ConfigSnapshot at launch. It mirrors the API launch input
// minus the per-call URL / RawRequest / SessionID, so persisting it lets a
// session re-render its last config and a run preserve what it was launched
// with.
type FuzzerConfig struct {
	Mode      FuzzMode               `json:"mode"`
	Positions []FuzzerPosition       `json:"positions"`
	Shared    *FuzzerPayloadsGroup   `json:"shared,omitempty"`
	Request   RequestOptions         `json:"request"`
	Execution FuzzerExecutionOptions `json:"execution"`
}

// FuzzResult is the per-request projection published to the streaming
// broadcaster. Full request/response bytes live on the persisted History row
// and are fetched lazily by the UI on row click.
type FuzzResult struct {
	HistoryID           uint      `json:"history_id"`
	Index               int       `json:"index"`
	StatusCode          int       `json:"status_code"`
	Method              string    `json:"method"`
	URL                 string    `json:"url"`
	ResponseBodySize    int       `json:"response_body_size"`
	ResponseContentType string    `json:"response_content_type"`
	DurationMs          int       `json:"duration_ms"`
	PayloadValues       []string  `json:"payload_values"`
	WordCount           int       `json:"word_count"`
	LineCount           int       `json:"line_count"`
	Error               *string   `json:"error,omitempty"`
	RetryCount          int       `json:"retry_count"`
	BaselineMatch       bool      `json:"baseline_match,omitempty"`
	Ts                  time.Time `json:"ts"`
}
