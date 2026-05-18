package fuzz

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"math/bits"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mfonda/simhash"
	"github.com/projectdiscovery/rawhttp"
)

// AutoBaselineMode controls whether and how baseline matches are surfaced
// to the user. Stored on FuzzerExecutionOptions.AutoBaseline (added below).
type AutoBaselineMode string

const (
	AutoBaselineOff  AutoBaselineMode = "off"
	AutoBaselineHide AutoBaselineMode = "hide"  // default
	AutoBaselineFlag AutoBaselineMode = "flag"
)

// BaselineFingerprint captures the shape dimensions of a "boring" response.
// A live result matches if exact fields all agree AND body simhash is within
// the threshold (Hamming distance).
type BaselineFingerprint struct {
	StatusCode       int    `json:"status_code"`
	ResponseBodySize int    `json:"response_body_size"`
	WordCount        int    `json:"word_count"`
	LineCount        int    `json:"line_count"`
	BodySimhash      uint64 `json:"body_simhash"`
	ContentType      string `json:"content_type"`
}

// PositionBaseline is the baseline for a single position. PositionIndex = -1
// for All / Paired / Combinations modes (one shared baseline). Disabled =
// true means the probes were inconsistent and the auto-matcher won't apply
// for this position.
type PositionBaseline struct {
	PositionIndex  int                  `json:"position_index"`
	Fingerprint    *BaselineFingerprint `json:"fingerprint,omitempty"`
	Disabled       bool                 `json:"disabled"`
	DisabledReason string               `json:"disabled_reason,omitempty"`
}

// RunBaseline is what's persisted on PlaygroundFuzzRun.Baseline.
type RunBaseline struct {
	Mode         AutoBaselineMode   `json:"mode"`
	Threshold    int                `json:"threshold"`
	Fingerprints []PositionBaseline `json:"fingerprints"`
	Warnings     []string           `json:"warnings,omitempty"`
}

// CalibrateInput is the engine-internal input for the calibration phase.
type CalibrateInput struct {
	TargetURL     string
	RawRequest    string
	Mode          FuzzMode
	Positions     []FuzzerPosition
	ProbeCount    int
	Threshold     int
	BaselineMode  AutoBaselineMode
	RequestTimeoutSeconds int
}

// DefaultBaselineProbeCount is the probe count when caller passes 0.
const DefaultBaselineProbeCount = 3

// DefaultBaselineThreshold is the simhash Hamming distance cutoff: bodies
// within this many bit differences are considered "the same boring shape."
const DefaultBaselineThreshold = 3

// Calibrate runs the baseline probe phase. Returns a RunBaseline that the
// caller persists onto the run row, plus a per-position matcher rule the
// caller can append to the run's matcher set (for Hide mode).
//
// Calibrate is synchronous; the engine should run it in the goroutine that
// drives the run, before transitioning to Running.
func Calibrate(ctx context.Context, in CalibrateInput) (*RunBaseline, error) {
	if in.BaselineMode == "" {
		in.BaselineMode = AutoBaselineHide
	}
	if in.BaselineMode == AutoBaselineOff {
		return &RunBaseline{Mode: AutoBaselineOff}, nil
	}
	if in.ProbeCount <= 0 {
		in.ProbeCount = DefaultBaselineProbeCount
	}
	if in.Threshold <= 0 {
		in.Threshold = DefaultBaselineThreshold
	}
	if in.RequestTimeoutSeconds <= 0 {
		in.RequestTimeoutSeconds = 30
	}

	parsedURL, err := url.Parse(in.TargetURL)
	if err != nil {
		return nil, fmt.Errorf("calibrate: parse url: %w", err)
	}
	pipeline := rawhttp.NewPipelineClient(rawhttp.PipelineOptions{
		Host:               parsedURL.Host,
		Timeout:            time.Duration(in.RequestTimeoutSeconds) * time.Second,
		MaxConnections:     2,
		MaxPendingRequests: 10,
	})

	out := &RunBaseline{Mode: in.BaselineMode, Threshold: in.Threshold}

	// Single mode → per-position baselines; other modes → one shared baseline.
	if in.Mode == ModeSingle {
		for posIdx, pos := range in.Positions {
			pb, warn := probePosition(ctx, pipeline, in, []int{posIdx}, pos)
			if warn != "" {
				out.Warnings = append(out.Warnings, warn)
			}
			pb.PositionIndex = posIdx
			out.Fingerprints = append(out.Fingerprints, pb)
		}
	} else {
		// All positions filled with sentinels.
		positionIdxs := make([]int, len(in.Positions))
		for i := range in.Positions {
			positionIdxs[i] = i
		}
		pb, warn := probePosition(ctx, pipeline, in, positionIdxs, FuzzerPosition{})
		if warn != "" {
			out.Warnings = append(out.Warnings, warn)
		}
		pb.PositionIndex = -1
		out.Fingerprints = append(out.Fingerprints, pb)
	}

	return out, nil
}

// probePosition sends ProbeCount sentinels into the given target positions,
// returns the consolidated baseline (or a disabled marker + warning if the
// probes were inconsistent). For Single mode, targetIdxs has one element
// (the position being fuzzed); for other modes, all positions.
func probePosition(ctx context.Context, pipe *rawhttp.PipelineClient, in CalibrateInput, targetIdxs []int, label FuzzerPosition) (PositionBaseline, string) {
	fingerprints := make([]BaselineFingerprint, 0, in.ProbeCount)
	for i := 0; i < in.ProbeCount; i++ {
		sentinel := generateSentinel(i)
		payloads := make([]string, len(in.Positions))
		for j, p := range in.Positions {
			payloads[j] = p.OriginalValue
		}
		for _, idx := range targetIdxs {
			payloads[idx] = sentinel
		}
		fp, err := singleProbe(ctx, pipe, in, payloads)
		if err != nil {
			// Network / parse error during calibration is enough to disable
			// the baseline for safety — we'd rather show everything than
			// hide real results behind a stale fingerprint.
			return PositionBaseline{
				Disabled:       true,
				DisabledReason: fmt.Sprintf("probe %d failed: %v", i, err),
			}, fmt.Sprintf("baseline disabled for positions %v: %s", targetIdxs, err.Error())
		}
		fingerprints = append(fingerprints, fp)
	}

	// Consistency check: every probe should agree on the exact fields, and
	// pairwise simhash distance should fit within the threshold.
	if len(fingerprints) < 2 {
		// Single probe — not enough to verify, but use it.
		return PositionBaseline{Fingerprint: &fingerprints[0]}, ""
	}
	base := fingerprints[0]
	for i := 1; i < len(fingerprints); i++ {
		fp := fingerprints[i]
		if !exactMatch(base, fp) || hamming(base.BodySimhash, fp.BodySimhash) > uint(in.Threshold) {
			return PositionBaseline{
				Disabled:       true,
				DisabledReason: fmt.Sprintf("probes %d and %d returned inconsistent responses", 0, i),
			}, fmt.Sprintf("baseline disabled for positions %v: inconsistent probe responses (target may return random content)", targetIdxs)
		}
	}
	return PositionBaseline{Fingerprint: &base}, ""
}

// singleProbe issues one fuzzed request and returns its fingerprint.
func singleProbe(ctx context.Context, pipe *rawhttp.PipelineClient, in CalibrateInput, payloads []string) (BaselineFingerprint, error) {
	raw := ReplacePayloads(in.RawRequest, in.Positions, payloads)
	// Inline the rawhttp call rather than reuse engine.doRequest to keep
	// calibration's transport simple and avoid retry/baseline feedback loops.
	parsed, err := parseRawForCalibrate(raw, in.TargetURL)
	if err != nil {
		return BaselineFingerprint{}, fmt.Errorf("parse: %w", err)
	}
	body := bytes.NewReader([]byte(parsed.body))
	resp, err := pipe.DoRaw(parsed.method, parsed.targetURL, parsed.uri, parsed.headers, body)
	if err != nil {
		return BaselineFingerprint{}, fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	wc, lc := countWordsLines(bodyBytes)
	return BaselineFingerprint{
		StatusCode:       resp.StatusCode,
		ResponseBodySize: len(bodyBytes),
		WordCount:        wc,
		LineCount:        lc,
		BodySimhash:      simhash.Simhash(simhash.NewWordFeatureSet(bodyBytes)),
		ContentType:      resp.Header.Get("Content-Type"),
	}, nil
}

// generateSentinel returns a sentinel payload string for probe index i.
// Cycles through three different shapes (long alphanumeric, short, uuid) so
// length-sensitive or format-sensitive baselines surface as inconsistency.
func generateSentinel(i int) string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	switch i % 3 {
	case 0:
		return "z" + hex.EncodeToString(buf)[:16] // 17 char alphanumeric
	case 1:
		return hex.EncodeToString(buf[:4]) // 8 char
	default:
		b := buf
		return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	}
}

func exactMatch(a, b BaselineFingerprint) bool {
	return a.StatusCode == b.StatusCode &&
		a.ResponseBodySize == b.ResponseBodySize &&
		a.WordCount == b.WordCount &&
		a.LineCount == b.LineCount &&
		a.ContentType == b.ContentType
}

func hamming(a, b uint64) uint {
	return uint(bits.OnesCount64(a ^ b))
}

// IsBaselineMatch reports whether the given response shape matches a stored
// baseline within the configured threshold. The engine projection layer
// calls this once per result to compute FuzzResult.BaselineMatch.
//
// For Single mode the baseline is looked up by positionIndex; for other
// modes the single (-1) baseline applies. Returns false if no baseline is
// configured / the lookup fails / the baseline is disabled.
func IsBaselineMatch(rb *RunBaseline, positionIndex int, candidate BaselineFingerprint) bool {
	if rb == nil || rb.Mode == AutoBaselineOff {
		return false
	}
	var pb *PositionBaseline
	for i := range rb.Fingerprints {
		f := &rb.Fingerprints[i]
		if f.PositionIndex == positionIndex {
			pb = f
			break
		}
		// Single mode might miss the lookup if the engine isn't tracking the
		// position; fall back to the shared baseline if present.
		if f.PositionIndex == -1 {
			pb = f
		}
	}
	if pb == nil || pb.Disabled || pb.Fingerprint == nil {
		return false
	}
	fp := pb.Fingerprint
	if !exactMatch(*fp, candidate) {
		return false
	}
	return hamming(fp.BodySimhash, candidate.BodySimhash) <= uint(rb.Threshold)
}

// parsedRawRequest is a minimal shape for the calibration probe — we don't
// need the full manual.ParseRawRequest because we control the input.
type parsedRawRequest struct {
	method    string
	targetURL string
	uri       string
	headers   http.Header
	body      string
}

func parseRawForCalibrate(raw, targetURL string) (parsedRawRequest, error) {
	// Find request-line (first \r\n or \n).
	line := raw
	rest := ""
	if idx := strings.Index(raw, "\r\n"); idx >= 0 {
		line, rest = raw[:idx], raw[idx+2:]
	} else if idx := strings.Index(raw, "\n"); idx >= 0 {
		line, rest = raw[:idx], raw[idx+1:]
	}
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 2 {
		return parsedRawRequest{}, fmt.Errorf("invalid request line %q", line)
	}
	method, uri := parts[0], parts[1]

	// Parse headers + body.
	headers := http.Header{}
	body := ""
	if idx := strings.Index(rest, "\r\n\r\n"); idx >= 0 {
		headerBlock := rest[:idx]
		body = rest[idx+4:]
		for _, h := range strings.Split(headerBlock, "\r\n") {
			if c := strings.Index(h, ":"); c > 0 {
				headers.Add(strings.TrimSpace(h[:c]), strings.TrimSpace(h[c+1:]))
			}
		}
	}
	// strip URI's host portion if absolute; rawhttp wants origin-form for the URI param
	if u, err := url.Parse(uri); err == nil && u.Host != "" {
		uri = u.RequestURI()
	}
	return parsedRawRequest{
		method:    method,
		targetURL: targetURL,
		uri:       uri,
		headers:   headers,
		body:      body,
	}, nil
}
