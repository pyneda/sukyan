package fuzz

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSentinelDiversity(t *testing.T) {
	// 6 sentinels should hit all three shapes at least once.
	lengths := map[int]int{}
	for i := 0; i < 9; i++ {
		s := generateSentinel(i)
		lengths[len(s)]++
	}
	// At least two distinct lengths (proves the shapes differ).
	require.Greater(t, len(lengths), 1, "expected at least 2 distinct sentinel lengths, got: %v", lengths)
}

func TestCalibrateAgainstStableServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "<html><body>Not Found</body></html>")
	}))
	defer srv.Close()

	raw := fmt.Sprintf("GET /?q=Q HTTP/1.1\r\nHost: %s\r\n\r\n", strings.TrimPrefix(srv.URL, "http://"))
	qIdx := strings.Index(raw, "Q")
	bl, err := Calibrate(context.Background(), CalibrateInput{
		TargetURL:  srv.URL,
		RawRequest: raw,
		Mode:       ModeSingle,
		Positions:  []FuzzerPosition{{Start: qIdx, End: qIdx + 1, OriginalValue: "Q"}},
		ProbeCount: 3,
	})
	require.NoError(t, err)
	require.Equal(t, AutoBaselineHide, bl.Mode)
	require.Len(t, bl.Fingerprints, 1)
	fp := bl.Fingerprints[0]
	require.False(t, fp.Disabled, "stable server should produce a usable baseline; warnings=%v", bl.Warnings)
	require.NotNil(t, fp.Fingerprint)
	require.Equal(t, 404, fp.Fingerprint.StatusCode)
}

func TestCalibrateRejectsInconsistentServer(t *testing.T) {
	// Server returns a random body each time → baselines should disable.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "random body %s here", r.URL.RawQuery)
	}))
	defer srv.Close()

	raw := fmt.Sprintf("GET /?q=Q HTTP/1.1\r\nHost: %s\r\n\r\n", strings.TrimPrefix(srv.URL, "http://"))
	qIdx := strings.Index(raw, "Q")
	bl, err := Calibrate(context.Background(), CalibrateInput{
		TargetURL:  srv.URL,
		RawRequest: raw,
		Mode:       ModeSingle,
		Positions:  []FuzzerPosition{{Start: qIdx, End: qIdx + 1, OriginalValue: "Q"}},
		ProbeCount: 3,
	})
	require.NoError(t, err)
	require.Len(t, bl.Fingerprints, 1)
	fp := bl.Fingerprints[0]
	require.True(t, fp.Disabled, "random server should disable baseline")
}

func TestCalibrateOffSkipsProbes(t *testing.T) {
	bl, err := Calibrate(context.Background(), CalibrateInput{BaselineMode: AutoBaselineOff})
	require.NoError(t, err)
	require.Equal(t, AutoBaselineOff, bl.Mode)
	require.Empty(t, bl.Fingerprints)
}

func TestIsBaselineMatchEqual(t *testing.T) {
	fp := BaselineFingerprint{
		StatusCode: 404, ResponseBodySize: 100, WordCount: 5, LineCount: 2,
		BodySimhash: 0xDEADBEEF, ContentType: "text/html",
	}
	rb := &RunBaseline{
		Mode:      AutoBaselineHide,
		Threshold: 3,
		Fingerprints: []PositionBaseline{
			{PositionIndex: 0, Fingerprint: &fp},
		},
	}
	require.True(t, IsBaselineMatch(rb, 0, fp), "identical fingerprint should match")
}

func TestIsBaselineMatchDifferentStatus(t *testing.T) {
	fp := BaselineFingerprint{StatusCode: 404, ResponseBodySize: 100, BodySimhash: 1}
	rb := &RunBaseline{
		Mode: AutoBaselineHide, Threshold: 3,
		Fingerprints: []PositionBaseline{{PositionIndex: 0, Fingerprint: &fp}},
	}
	different := fp
	different.StatusCode = 200
	require.False(t, IsBaselineMatch(rb, 0, different))
}

func TestIsBaselineMatchSimhashWithinThreshold(t *testing.T) {
	fp := BaselineFingerprint{StatusCode: 200, BodySimhash: 0xFF}
	// One bit different → distance = 1
	near := fp
	near.BodySimhash = 0xFE
	rb := &RunBaseline{Mode: AutoBaselineHide, Threshold: 3,
		Fingerprints: []PositionBaseline{{PositionIndex: -1, Fingerprint: &fp}}}
	require.True(t, IsBaselineMatch(rb, -1, near))
}

func TestIsBaselineMatchSimhashOutOfThreshold(t *testing.T) {
	fp := BaselineFingerprint{StatusCode: 200, BodySimhash: 0xFF}
	far := fp
	far.BodySimhash = 0x00 // 8 bits differ
	rb := &RunBaseline{Mode: AutoBaselineHide, Threshold: 3,
		Fingerprints: []PositionBaseline{{PositionIndex: -1, Fingerprint: &fp}}}
	require.False(t, IsBaselineMatch(rb, -1, far))
}

func TestIsBaselineMatchOffMode(t *testing.T) {
	fp := BaselineFingerprint{StatusCode: 200}
	rb := &RunBaseline{Mode: AutoBaselineOff,
		Fingerprints: []PositionBaseline{{Fingerprint: &fp}}}
	require.False(t, IsBaselineMatch(rb, 0, fp), "off mode should never match")
}

func TestIsBaselineMatchDisabledPosition(t *testing.T) {
	rb := &RunBaseline{Mode: AutoBaselineHide, Threshold: 3,
		Fingerprints: []PositionBaseline{{PositionIndex: 0, Disabled: true}}}
	require.False(t, IsBaselineMatch(rb, 0, BaselineFingerprint{}))
}
