package wsfuzz

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
