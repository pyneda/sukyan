package wsreplay

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubstituteVars_Basic(t *testing.T) {
	out, warn := SubstituteVars(`{"token":"${jwt}","x":1}`, map[string]string{"jwt": "abc.def.ghi"})
	require.Equal(t, `{"token":"abc.def.ghi","x":1}`, out)
	require.False(t, warn)
}

func TestSubstituteVars_MissingVarLeftAsIs(t *testing.T) {
	out, warn := SubstituteVars(`hello ${missing}`, map[string]string{})
	require.Equal(t, `hello ${missing}`, out)
	require.False(t, warn)
}

func TestSubstituteVars_SectionMarkerInValueWarns(t *testing.T) {
	out, warn := SubstituteVars(`x=${v}`, map[string]string{"v": "a§§b"})
	require.Equal(t, `x=a§§b`, out)
	require.True(t, warn, "substituted value containing §§ must surface a warning")
}

func TestSubstituteVars_MultipleVars(t *testing.T) {
	out, _ := SubstituteVars(`${a}/${b}/${a}`, map[string]string{"a": "X", "b": "Y"})
	require.Equal(t, `X/Y/X`, out)
}

func TestSubstituteVars_EmptyValue(t *testing.T) {
	out, _ := SubstituteVars(`x=${v};`, map[string]string{"v": ""})
	require.Equal(t, `x=;`, out)
}

func TestSubstituteVarsStrict_ReportsUndefinedRefs(t *testing.T) {
	// All known vars resolve — empty undefined slice signals strict mode is
	// happy to ship the content as-is.
	out, undef, warn := SubstituteVarsStrict(`a=${x} b=${y}`, map[string]string{"x": "1", "y": "2"})
	require.Equal(t, "a=1 b=2", out)
	require.False(t, warn)
	require.Empty(t, undef)
}

func TestSubstituteVarsStrict_CollectsMissingNames(t *testing.T) {
	out, undef, _ := SubstituteVarsStrict(`a=${x} b=${y} c=${z}`, map[string]string{"x": "1"})
	require.Equal(t, "a=1 b=${y} c=${z}", out)
	require.Equal(t, []string{"y", "z"}, undef)
}

func TestSubstituteVarsStrict_DedupesRepeatedMissingNames(t *testing.T) {
	// `${y}` is referenced twice but should appear only once in the
	// undefined list — the caller doesn't need to filter duplicates.
	_, undef, _ := SubstituteVarsStrict(`a=${y} b=${y} c=${z}`, map[string]string{})
	require.Equal(t, []string{"y", "z"}, undef)
}
