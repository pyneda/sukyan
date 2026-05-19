package wsfuzz

import (
	"encoding/json"
	"testing"

	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
	"github.com/stretchr/testify/require"
)

func TestEvaluateCheck_ReceivedFrameContains(t *testing.T) {
	ca := CheckAssertion{
		Logic: LogicAnd,
		Rules: []fuzz.MatcherRule{
			{Field: fuzz.FieldWsReceivedFrameAt, Operator: fuzz.OpContains, Value: json.RawMessage(`"banned"`)},
		},
	}
	frames := []wsreplay.Frame{{Opcode: 1, Content: "you are banned", Direction: "received"}}
	passed, _ := evaluateCheck(ca, frames, nil)
	require.True(t, passed)

	frames2 := []wsreplay.Frame{{Opcode: 1, Content: "all good", Direction: "received"}}
	passed2, _ := evaluateCheck(ca, frames2, nil)
	require.False(t, passed2)
}

func TestEvaluateCheck_ReceivedFrameCountGt(t *testing.T) {
	ca := CheckAssertion{
		Logic: LogicAnd,
		Rules: []fuzz.MatcherRule{
			{Field: fuzz.FieldWsReceivedFrameCount, Operator: fuzz.OpGt, Value: json.RawMessage(`1`)},
		},
	}
	frames := []wsreplay.Frame{{Direction: "received"}, {Direction: "received"}, {Direction: "received"}}
	passed, _ := evaluateCheck(ca, frames, nil)
	require.True(t, passed)
}

func TestEvaluateCheck_OrLogic(t *testing.T) {
	ca := CheckAssertion{
		Logic: LogicOr,
		Rules: []fuzz.MatcherRule{
			{Field: fuzz.FieldWsReceivedFrameCount, Operator: fuzz.OpEq, Value: json.RawMessage(`100`)},
			{Field: fuzz.FieldWsReceivedFrameCount, Operator: fuzz.OpGt, Value: json.RawMessage(`0`)},
		},
	}
	frames := []wsreplay.Frame{{Direction: "received"}}
	passed, _ := evaluateCheck(ca, frames, nil)
	require.True(t, passed)
}

func TestEvaluateCheck_Negate(t *testing.T) {
	ca := CheckAssertion{
		Logic:  LogicAnd,
		Negate: true,
		Rules: []fuzz.MatcherRule{
			{Field: fuzz.FieldWsReceivedFrameCount, Operator: fuzz.OpEq, Value: json.RawMessage(`0`)},
		},
	}
	frames := []wsreplay.Frame{{Direction: "received"}}
	passed, _ := evaluateCheck(ca, frames, nil)
	require.True(t, passed, "negated 'frame_count == 0' is true when frame_count != 0")
}

func TestEvaluateCheck_TotalReceivedBytesGte(t *testing.T) {
	ca := CheckAssertion{
		Logic: LogicAnd,
		Rules: []fuzz.MatcherRule{
			{Field: fuzz.FieldWsTotalReceivedBytes, Operator: fuzz.OpGte, Value: json.RawMessage(`10`)},
		},
	}
	frames := []wsreplay.Frame{
		{Content: "hello", Direction: "received"}, // 5 bytes
		{Content: "world", Direction: "received"}, // 5 bytes; total = 10
	}
	passed, _ := evaluateCheck(ca, frames, nil)
	require.True(t, passed)
}

func TestEvaluateCheck_RegexOnReceivedContent(t *testing.T) {
	ca := CheckAssertion{
		Logic: LogicAnd,
		Rules: []fuzz.MatcherRule{
			{Field: fuzz.FieldWsReceivedFrameAt, Operator: fuzz.OpRegex, Value: json.RawMessage(`"error code: \\d+"`)},
		},
	}
	frames := []wsreplay.Frame{{Content: "got error code: 42 from server", Direction: "received"}}
	passed, _ := evaluateCheck(ca, frames, nil)
	require.True(t, passed)
}

func TestEvaluateCheck_IsEmptyOnNoFrames(t *testing.T) {
	ca := CheckAssertion{
		Logic: LogicAnd,
		Rules: []fuzz.MatcherRule{
			{Field: fuzz.FieldWsReceivedFrameAt, Operator: fuzz.OpIsEmpty, Value: nil},
		},
	}
	passed, _ := evaluateCheck(ca, nil, nil)
	require.True(t, passed)
}

func TestEvaluateCheck_VariableEquals_AnyMatch(t *testing.T) {
	ca := CheckAssertion{
		Logic: LogicAnd,
		Rules: []fuzz.MatcherRule{
			{Field: fuzz.FieldWsVariable, Operator: fuzz.OpEq, Value: json.RawMessage(`"admin"`)},
		},
	}
	vars := map[string]string{"role": "admin", "uid": "42"}
	passed, _ := evaluateCheck(ca, nil, vars)
	require.True(t, passed, "v1: FieldWsVariable matches if ANY variable equals the value")
}

func TestEvaluateCheck_ResultsDetailsPopulated(t *testing.T) {
	ca := CheckAssertion{
		Logic: LogicAnd,
		Rules: []fuzz.MatcherRule{
			{Field: fuzz.FieldWsReceivedFrameCount, Operator: fuzz.OpEq, Value: json.RawMessage(`1`)},
		},
	}
	frames := []wsreplay.Frame{{Direction: "received"}}
	passed, details := evaluateCheck(ca, frames, nil)
	require.True(t, passed)
	require.Equal(t, LogicAnd, details.Logic)
	require.True(t, details.Passed)
	require.Len(t, details.Rules, 1)
	require.Equal(t, string(fuzz.FieldWsReceivedFrameCount), details.Rules[0].Field)
	require.Equal(t, "1", details.Rules[0].Actual)
	require.True(t, details.Rules[0].Passed)
}
