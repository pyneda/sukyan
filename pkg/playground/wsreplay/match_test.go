package wsreplay

import "testing"

func TestMatchAny(t *testing.T) {
	if !Match(WaitForSpec{MatchType: MatchAny}, "anything") {
		t.Fatal("any should match anything")
	}
	if !Match(WaitForSpec{MatchType: MatchAny}, "") {
		t.Fatal("any should match empty too")
	}
}

func TestMatchContains(t *testing.T) {
	if !Match(WaitForSpec{MatchType: MatchContains, Pattern: "foo"}, "barfoobaz") {
		t.Fatal("expected contains to match")
	}
	if Match(WaitForSpec{MatchType: MatchContains, Pattern: "foo"}, "barbaz") {
		t.Fatal("expected contains to NOT match")
	}
}

func TestMatchRegex(t *testing.T) {
	if !Match(WaitForSpec{MatchType: MatchRegex, Pattern: `^\{.*"ok":true.*\}$`}, `{"ok":true,"id":1}`) {
		t.Fatal("regex should match")
	}
	if Match(WaitForSpec{MatchType: MatchRegex, Pattern: "^abc$"}, "abcdef") {
		t.Fatal("regex should not match")
	}
	if Match(WaitForSpec{MatchType: MatchRegex, Pattern: "[invalid("}, "anything") {
		t.Fatal("invalid regex should not match")
	}
}

func TestMatchJSONPath(t *testing.T) {
	payload := `{"a":{"b":42}}`
	if !Match(WaitForSpec{MatchType: MatchJSONPath, Pattern: "$.a.b"}, payload) {
		t.Fatal("json_path should find the key")
	}
	if Match(WaitForSpec{MatchType: MatchJSONPath, Pattern: "$.x"}, payload) {
		t.Fatal("json_path should not find missing key")
	}
	if Match(WaitForSpec{MatchType: MatchJSONPath, Pattern: "$.a"}, "not json") {
		t.Fatal("json_path should not match non-JSON payload")
	}
}

func TestMatchEdgeCases(t *testing.T) {
	// Empty pattern for contains matches everything (strings.Contains semantics).
	if !Match(WaitForSpec{MatchType: MatchContains, Pattern: ""}, "anything") {
		t.Fatal("contains with empty pattern should match")
	}

	// Empty regex compiles and matches everything.
	if !Match(WaitForSpec{MatchType: MatchRegex, Pattern: ""}, "anything") {
		t.Fatal("regex with empty pattern should match")
	}

	// Unknown match type returns false (default fallthrough).
	if Match(WaitForSpec{MatchType: "unknown_type"}, "anything") {
		t.Fatal("unknown match_type should not match")
	}

	// JSONPath finding a null value still counts as a match (presence-based).
	if !Match(WaitForSpec{MatchType: MatchJSONPath, Pattern: "$.a"}, `{"a":null}`) {
		t.Fatal("json_path on a null value should match (presence-based)")
	}
}
