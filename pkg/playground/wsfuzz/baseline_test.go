package wsfuzz

import (
	"strings"
	"testing"

	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
	"github.com/stretchr/testify/require"
)

func TestComputeFingerprint_DeterministicFrames(t *testing.T) {
	frames := []wsreplay.Frame{
		{Opcode: 1, Content: "hello", Direction: "received"},
		{Opcode: 1, Content: "world", Direction: "received"},
	}
	fp := ComputeFingerprint(frames, 101)
	require.Equal(t, 2, fp.FrameCount)
	require.Equal(t, 101, fp.HandshakeStatus)
	require.Len(t, fp.PerFrame, 2)
	require.Equal(t, 1, fp.PerFrame[0].Opcode)
	require.Equal(t, 5, fp.PerFrame[0].SizeBytes)
	require.True(t, strings.HasPrefix(fp.PerFrame[0].ContentHash, "sha256:"))
	require.NotEqual(t, fp.PerFrame[0].ContentHash, fp.PerFrame[1].ContentHash)
}

func TestComputeFingerprint_IgnoresSentFrames(t *testing.T) {
	frames := []wsreplay.Frame{
		{Opcode: 1, Content: "sent-me", Direction: "sent"},
		{Opcode: 1, Content: "got-this", Direction: "received"},
	}
	fp := ComputeFingerprint(frames, 101)
	require.Equal(t, 1, fp.FrameCount)
}

func TestCompareFingerprint_StrictMatch(t *testing.T) {
	frames := []wsreplay.Frame{{Opcode: 1, Content: "abc", Direction: "received"}}
	a := ComputeFingerprint(frames, 101)
	b := ComputeFingerprint(frames, 101)
	require.True(t, CompareFingerprint(a, b))
}

func TestCompareFingerprint_FrameCountMismatch(t *testing.T) {
	a := ComputeFingerprint([]wsreplay.Frame{{Opcode: 1, Content: "x", Direction: "received"}}, 101)
	b := ComputeFingerprint([]wsreplay.Frame{
		{Opcode: 1, Content: "x", Direction: "received"},
		{Opcode: 1, Content: "y", Direction: "received"},
	}, 101)
	require.False(t, CompareFingerprint(a, b))
}

func TestCompareFingerprint_ContentChange(t *testing.T) {
	a := ComputeFingerprint([]wsreplay.Frame{{Opcode: 1, Content: "abc", Direction: "received"}}, 101)
	b := ComputeFingerprint([]wsreplay.Frame{{Opcode: 1, Content: "xyz", Direction: "received"}}, 101)
	require.False(t, CompareFingerprint(a, b))
}

func TestCompareFingerprint_HandshakeStatusChange(t *testing.T) {
	frames := []wsreplay.Frame{{Opcode: 1, Content: "x", Direction: "received"}}
	a := ComputeFingerprint(frames, 101)
	b := ComputeFingerprint(frames, 200)
	require.False(t, CompareFingerprint(a, b))
}

func TestCalibrate_AgreeingProbes(t *testing.T) {
	probe1 := ComputeFingerprint([]wsreplay.Frame{{Opcode: 1, Content: "x", Direction: "received"}}, 101)
	probe2 := ComputeFingerprint([]wsreplay.Frame{{Opcode: 1, Content: "x", Direction: "received"}}, 101)
	out, warns := CalibrateFromProbes([]WsBaselineFingerprint{probe1, probe2})
	require.Empty(t, warns)
	require.Equal(t, probe1, out)
}

func TestCalibrate_FrameCountDisagreement(t *testing.T) {
	probe1 := ComputeFingerprint([]wsreplay.Frame{{Opcode: 1, Content: "x", Direction: "received"}}, 101)
	probe2 := ComputeFingerprint([]wsreplay.Frame{
		{Opcode: 1, Content: "x", Direction: "received"},
		{Opcode: 1, Content: "y", Direction: "received"},
	}, 101)
	_, warns := CalibrateFromProbes([]WsBaselineFingerprint{probe1, probe2})
	require.Contains(t, warns, "baseline_disabled_count_disagreement")
}

func TestCalibrate_PartialNondeterminism(t *testing.T) {
	probe1 := ComputeFingerprint([]wsreplay.Frame{{Opcode: 1, Content: "ts=1", Direction: "received"}}, 101)
	probe2 := ComputeFingerprint([]wsreplay.Frame{{Opcode: 1, Content: "ts=2", Direction: "received"}}, 101)
	_, warns := CalibrateFromProbes([]WsBaselineFingerprint{probe1, probe2})
	require.Contains(t, warns, "baseline_partial_nondeterminism")
}
