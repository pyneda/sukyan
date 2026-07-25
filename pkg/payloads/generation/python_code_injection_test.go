package generation

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/stretchr/testify/require"
)

// The Python OOB template is statement-based (import socket; ...), which a pure
// eval()/expression sink rejects with a SyntaxError, so eval(input) sinks that
// reflect the computed value went undetected. This response-based template must
// (a) exist, (b) be response-driven rather than OOB, and (c) predict exactly the
// string a Python eval of its payload would return, so the marker only appears
// when the expression actually executes - never from a plain echo of the payload.
func TestPythonCodeInjectionTemplateDetectsEvaluatedExpression(t *testing.T) {
	generators, err := LoadLocalGenerators()
	require.NoError(t, err)

	var gen *PayloadGenerator
	for _, g := range generators {
		if g.ID == "python-code-injection" {
			gen = g
			break
		}
	}
	require.NotNil(t, gen, "python-code-injection template must exist")
	require.Equal(t, "python_code_injection", gen.IssueCode)

	// Must be response-based, not OOB - an eval() sink issues no OOB callback.
	for _, dm := range gen.DetectionMethods {
		require.Nil(t, dm.OOBInteraction, "detection must not rely on OOB for an eval() sink")
		require.NotNil(t, dm.ResponseCondition, "detection must be a response_condition")
	}

	payloads, err := gen.BuildPayloads(integrations.InteractionsManager{})
	require.NoError(t, err)
	require.NotEmpty(t, payloads)

	// Payload shape: 'PREFIX'+str(V1*V2)+'SUFFIX' (single- or double-quoted).
	shape := regexp.MustCompile(`^['"](\w+)['"]\+str\((\d+)\*(\d+)\)\+['"](\w+)['"]$`)

	for _, p := range payloads {
		m := shape.FindStringSubmatch(p.Value)
		require.NotNil(t, m, "unexpected payload shape: %q", p.Value)
		prefix, suffix := m[1], m[4]
		v1, _ := strconv.Atoi(m[2])
		v2, _ := strconv.Atoi(m[3])

		// What Python's eval(payload) would actually return.
		evaluated := prefix + strconv.Itoa(v1*v2) + suffix

		require.Len(t, p.DetectionMethods, 1)
		marker := p.DetectionMethods[0].ResponseCondition.Contains
		require.NotEmpty(t, marker)

		// The detection marker must equal the evaluated result exactly, so a
		// response containing it proves the expression executed.
		require.Equal(t, evaluated, marker,
			"detection marker must match Python eval() output for payload %q", p.Value)

		// And a plain echo of the raw payload must NOT contain the marker,
		// otherwise a reflecting endpoint would be a false positive.
		require.False(t, strings.Contains(p.Value, marker),
			"raw payload %q must not contain the marker %q (would be an echo FP)", p.Value, marker)

		// The product must render as a plain integer, never scientific notation,
		// or it would never match the sink's numeric output.
		require.NotContains(t, marker, "e+", "product rendered in scientific notation: %q", marker)
	}
}
