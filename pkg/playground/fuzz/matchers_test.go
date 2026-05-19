package fuzz

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func mkRule(field MatcherField, op MatcherOperator, value any) MatcherRule {
	raw, _ := json.Marshal(value)
	return MatcherRule{Field: field, Operator: op, Value: raw}
}

func TestValidateRuleAcceptsKnownCombos(t *testing.T) {
	require.NoError(t, ValidateRule(mkRule(FieldStatusCode, OpEq, 200), DomainHTTP))
	require.NoError(t, ValidateRule(mkRule(FieldResponseBody, OpContains, "needle"), DomainHTTP))
	require.NoError(t, ValidateRule(mkRule(FieldResponseHeaders, OpRegex, "X-.*"), DomainHTTP))
}

func TestValidateRuleRejectsUnknownField(t *testing.T) {
	require.Error(t, ValidateRule(mkRule("bogus", OpEq, 1), DomainHTTP))
}

func TestValidateRuleRejectsInvalidOperatorForField(t *testing.T) {
	require.Error(t, ValidateRule(mkRule(FieldStatusCode, OpContains, "x"), DomainHTTP))
	require.Error(t, ValidateRule(mkRule(FieldResponseBody, OpEq, "x"), DomainHTTP))
}

func TestValidateSetRejectsUnknownMode(t *testing.T) {
	require.Error(t, ValidateSet(MatcherSet{Mode: "wat"}))
}

func TestEvalServerSideContains(t *testing.T) {
	set := MatcherSet{Rules: []MatcherRule{mkRule(FieldResponseBody, OpContains, "secret")}}
	ok, err := set.EvalServerSide([]byte("...secret..."), "")
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = set.EvalServerSide([]byte("no match"), "")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestEvalServerSideRegex(t *testing.T) {
	set := MatcherSet{Rules: []MatcherRule{mkRule(FieldResponseBody, OpRegex, `error code: \d+`)}}
	ok, err := set.EvalServerSide([]byte("error code: 42"), "")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestEvalServerSideHeaders(t *testing.T) {
	set := MatcherSet{Rules: []MatcherRule{mkRule(FieldResponseHeaders, OpContains, "X-Powered-By: PHP")}}
	ok, err := set.EvalServerSide([]byte(""), "Content-Type: text/html\r\nX-Powered-By: PHP/7.4\r\n")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestEvalServerSideSkipsNonServerRules(t *testing.T) {
	set := MatcherSet{
		Rules: []MatcherRule{
			mkRule(FieldStatusCode, OpEq, 200),    // client-side, skipped
			mkRule(FieldResponseBody, OpContains, "x"), // server-side
		},
	}
	ok, err := set.EvalServerSide([]byte("contains x"), "")
	require.NoError(t, err)
	require.True(t, ok, "client-side rule must not affect server eval")
}

func TestIsServerSide(t *testing.T) {
	require.True(t, MatcherRule{Field: FieldResponseBody}.IsServerSide())
	require.True(t, MatcherRule{Field: FieldResponseHeaders}.IsServerSide())
	require.False(t, MatcherRule{Field: FieldStatusCode}.IsServerSide())
	require.False(t, MatcherRule{Field: FieldPayload}.IsServerSide())
}

func TestValidateRule_HTTPDomain(t *testing.T) {
	require.NoError(t, ValidateRule(mkRule(FieldStatusCode, OpEq, 200), DomainHTTP))
}

func TestValidateRule_WsFuzzDomain(t *testing.T) {
	require.NoError(t, ValidateRule(mkRule(FieldWsReceivedFrameCount, OpEq, 5), DomainWsFuzz))
	require.NoError(t, ValidateRule(mkRule(FieldWsHandshakeStatus, OpGte, 200), DomainWsFuzz))
	require.NoError(t, ValidateRule(mkRule(FieldWsStepReceivedFrame, OpRegex, "ok"), DomainWsFuzz))
}

func TestValidateRule_CrossDomainRejected(t *testing.T) {
	// HTTP field under WsFuzz domain — must reject
	require.Error(t, ValidateRule(mkRule(FieldStatusCode, OpEq, 200), DomainWsFuzz))
	// WS field under HTTP domain — must reject
	require.Error(t, ValidateRule(mkRule(FieldWsReceivedFrameCount, OpEq, 5), DomainHTTP))
}

func TestValidateSet_HonorsDomain(t *testing.T) {
	wsSet := MatcherSet{
		Domain: DomainWsFuzz,
		Rules:  []MatcherRule{mkRule(FieldWsReceivedFrameCount, OpEq, 1)},
	}
	require.NoError(t, ValidateSet(wsSet))

	httpDefaultSet := MatcherSet{
		Rules: []MatcherRule{mkRule(FieldStatusCode, OpEq, 200)},
	}
	require.NoError(t, ValidateSet(httpDefaultSet)) // domain zero == DomainHTTP
}
