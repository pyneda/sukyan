package wsfuzz

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
)

// ComputeFingerprint reduces a frame sequence + handshake status to a
// WsBaselineFingerprint. Sent frames are ignored (we only care about server
// output for baseline detection). Control frames (opcodes 8/9/10) never reach
// here because gorilla handles them in the read pump.
func ComputeFingerprint(frames []wsreplay.Frame, handshakeStatus int) WsBaselineFingerprint {
	fp := WsBaselineFingerprint{HandshakeStatus: handshakeStatus}
	for _, f := range frames {
		if f.Direction != "received" {
			continue
		}
		fp.PerFrame = append(fp.PerFrame, FrameSig{
			Opcode:      f.Opcode,
			SizeBytes:   len(f.Content),
			ContentHash: sha256Hex(f.Content),
		})
	}
	fp.FrameCount = len(fp.PerFrame)
	return fp
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// CompareFingerprint reports whether two fingerprints are an exact match
// (v1: no tolerance). Frame count must match; handshake status must match;
// every per-frame opcode/size/hash must match.
func CompareFingerprint(a, b WsBaselineFingerprint) bool {
	if a.FrameCount != b.FrameCount {
		return false
	}
	if a.HandshakeStatus != b.HandshakeStatus {
		return false
	}
	for i := range a.PerFrame {
		if a.PerFrame[i] != b.PerFrame[i] {
			return false
		}
	}
	return true
}

// CalibrateFromProbes folds N probe fingerprints into one baseline + warnings.
// Returns the canonical baseline (first probe) and a list of warning event
// names. Per spec:
//   - Probes disagree on FrameCount: baseline_disabled_count_disagreement;
//     caller is expected to force auto_baseline=off.
//   - Probes agree on count but differ on per-frame hashes:
//     baseline_partial_nondeterminism.
func CalibrateFromProbes(probes []WsBaselineFingerprint) (WsBaselineFingerprint, []string) {
	if len(probes) == 0 {
		return WsBaselineFingerprint{}, nil
	}
	first := probes[0]
	var warns []string

	countDisagree := false
	for _, p := range probes[1:] {
		if p.FrameCount != first.FrameCount {
			countDisagree = true
			break
		}
	}
	if countDisagree {
		warns = append(warns, "baseline_disabled_count_disagreement")
		return first, warns
	}

	for _, p := range probes[1:] {
		for i := range first.PerFrame {
			if first.PerFrame[i].ContentHash != p.PerFrame[i].ContentHash {
				warns = append(warns, "baseline_partial_nondeterminism")
				return first, warns
			}
		}
	}
	return first, warns
}
