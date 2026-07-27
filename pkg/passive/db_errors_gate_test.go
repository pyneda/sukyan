package passive

import (
	"regexp"
	"regexp/syntax"
	"strings"
	"testing"
)

func TestRequiredLiteral(t *testing.T) {
	tests := []struct {
		pattern  string
		wantGate string
		wantFold bool
	}{
		{`\bORA-\d{5}`, "ORA-", false},
		{`(\W|\A)SQL Server.*Driver`, "SQL Server", false},
		{`\bdeadlock\b.*\bdetected\b`, "deadlock", false},
		{`(Microsoft|System)\.Data\.SQLite\.SQLiteException`, ".Data.SQLite.SQLiteException", false},
		{`(?i)Warning.*sybase.*`, "warning", true},
		{`\bdb2_\w+\(`, "db2_", false},
		{`valid MySQL result`, "valid MySQL result", false},
		// No provable literal: everything is alternated or optional.
		{`(foo|bar)`, "", false},
		{`a?b?c?`, "", false},
		{`.*`, "", false},
	}
	for _, tt := range tests {
		gate, fold := requiredLiteral(tt.pattern)
		if gate != tt.wantGate || fold != tt.wantFold {
			t.Errorf("requiredLiteral(%q) = (%q,%v), want (%q,%v)",
				tt.pattern, gate, fold, tt.wantGate, tt.wantFold)
		}
	}
}

// Soundness: a gate may never reject a string the pattern would have matched.
// For every gated pattern, synthesise strings that satisfy it and assert the
// gate is present in each. A false gate silently deletes a detector, so this
// sweep runs over the whole corpus, not a sample.
func TestGatesNeverRejectAMatch(t *testing.T) {
	gated := 0
	for _, p := range gatedDBMSErrors {
		if p.gate == "" {
			continue
		}
		gated++
		re, err := syntax.Parse(p.re.String(), syntax.Perl)
		if err != nil {
			t.Fatalf("%s: parse: %v", p.re, err)
		}
		for _, sample := range synthesizeMatches(re.Simplify(), 0) {
			if !p.re.MatchString(sample) {
				continue // generator approximation, not a real match
			}
			haystack := sample
			if p.fold {
				haystack = caseFold(sample)
			}
			if !strings.Contains(haystack, p.gate) {
				t.Errorf("gate %q (fold=%v) rejects %q which %s matches",
					p.gate, p.fold, sample, p.re)
			}
		}
	}
	if gated < 50 {
		t.Fatalf("only %d patterns gated; the extractor probably regressed", gated)
	}
	t.Logf("gated %d/%d patterns", gated, len(gatedDBMSErrors))
}

// The regexp package folds through unicode.SimpleFold, so a case-insensitive
// pattern matches the Kelvin sign and long s. The gate's folding must agree or it
// would reject a genuine match.
func TestCaseFoldMatchesRegexpFolding(t *testing.T) {
	re := regexp.MustCompile(`(?i)warning.*sybase`)
	for _, s := range []string{
		"Warning: sybase driver",
		"WARNING: SYBASE driver",
		"Warning: ſybaſe driver", // long s
		"WARNING: SYBASE Key",    // Kelvin sign elsewhere
	} {
		if !re.MatchString(s) {
			continue
		}
		if !strings.Contains(caseFold(s), "warning") {
			t.Errorf("caseFold(%q) drops a gate the regexp matches", s)
		}
	}
	if got := caseFold("ſK"); got != "sk" {
		t.Errorf("caseFold special runes = %q, want \"sk\"", got)
	}
}

// synthesizeMatches builds candidate strings for a parsed pattern, expanding
// every alternation branch so no branch escapes the soundness check.
func synthesizeMatches(re *syntax.Regexp, depth int) []string {
	if depth > 6 {
		return []string{""}
	}
	switch re.Op {
	case syntax.OpLiteral:
		return []string{string(re.Rune)}
	case syntax.OpCharClass:
		if len(re.Rune) >= 2 {
			return []string{string(re.Rune[0])}
		}
		return []string{"x"}
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		return []string{"x"}
	case syntax.OpCapture:
		return synthesizeMatches(re.Sub[0], depth+1)
	case syntax.OpStar, syntax.OpQuest:
		out := []string{""}
		return append(out, synthesizeMatches(re.Sub[0], depth+1)...)
	case syntax.OpPlus:
		return synthesizeMatches(re.Sub[0], depth+1)
	case syntax.OpRepeat:
		subs := synthesizeMatches(re.Sub[0], depth+1)
		out := make([]string, 0, len(subs))
		for _, s := range subs {
			out = append(out, strings.Repeat(s, max(re.Min, 1)))
		}
		return out
	case syntax.OpAlternate:
		out := []string{}
		for _, sub := range re.Sub {
			out = append(out, synthesizeMatches(sub, depth+1)...)
		}
		return out
	case syntax.OpConcat:
		out := []string{""}
		for _, sub := range re.Sub {
			parts := synthesizeMatches(sub, depth+1)
			next := make([]string, 0, len(out)*len(parts))
			for _, prefix := range out {
				for _, part := range parts {
					next = append(next, prefix+part)
				}
			}
			if len(next) > 64 {
				next = next[:64]
			}
			out = next
		}
		return out
	default:
		return []string{""}
	}
}
