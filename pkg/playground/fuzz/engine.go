package fuzz

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mfonda/simhash"
	"github.com/projectdiscovery/rawhttp"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/manual"
	"github.com/pyneda/sukyan/pkg/playground/stream"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
)

// Hooks bundle the engine's outbound callbacks. Keeping them as function
// fields rather than an interface lets the api layer wire in whatever lambdas
// fit its testing needs without defining a mock type.
type Hooks struct {
	// Publish is invoked once per FuzzResult. Typically wraps the result in
	// a stream.Sequenced envelope and forwards to a broadcaster.
	Publish func(*FuzzResult)

	// UpdateProgress is called periodically (1s tick or every 100 results,
	// whichever first) with the latest progress counters. Implementations
	// typically persist these onto PlaygroundFuzzRun for crash-recovery and
	// post-run reporting; they MAY also republish over the stream.
	UpdateProgress func(sent, errs int)
}

// RunInput is the immutable per-run input to Engine.Run. The engine takes
// ownership of the resolved values; the caller should not mutate them after
// passing them in.
type RunInput struct {
	RunID               uint
	WorkspaceID         uint
	PlaygroundSessionID uint
	TargetURL           string
	RawRequest          string
	Mode                FuzzMode
	Positions           []FuzzerPosition
	Resolved            ResolvedPayloads
	Request             RequestOptions
	Execution           FuzzerExecutionOptions
	Strategy            ModeStrategy
	// Broadcaster the engine publishes results to. Nil disables streaming
	// (useful in tests that only care about persistence).
	Broadcaster *stream.Broadcaster
	// Baseline, when set, drives FuzzResult.BaselineMatch on each result.
	// Caller is expected to have run Calibrate before Run; the engine does
	// not invoke calibration itself.
	Baseline *RunBaseline
	// PauseGate, when set, is checked by workers before scheduling new work
	// (and on retry backoff). Nil disables pause semantics entirely — useful
	// in tests. The api layer wires this from fuzz.Registry.Gate(runID).
	PauseGate *PauseGate
	Hooks     Hooks
}

// RunOutcome is the terminal classification of a finished run. The engine
// returns this so the caller (api layer) can set the appropriate run status
// without duplicating the decision logic.
type RunOutcome struct {
	Status        db.PlaygroundFuzzRunStatus
	FailureReason string
	SentCount     int
	ErrorCount    int
	Duration      time.Duration
}

// Run executes the fuzz to completion (or until ctx is cancelled / a stop
// condition fires). Returns the outcome — the caller is responsible for
// persisting it onto PlaygroundFuzzRun and unregistering any cancel func.
//
// The engine is DB-free: it persists results via http_utils.ReadHttpResponseAndCreateHistory
// (which writes History rows tagged with the run id), but it never touches
// the run row itself. That separation makes the engine unit-testable.
func Run(ctx context.Context, input RunInput) RunOutcome {
	start := time.Now()
	parsedURL, err := url.Parse(input.TargetURL)
	if err != nil {
		return RunOutcome{
			Status:        db.FuzzRunFailed,
			FailureReason: fmt.Sprintf("invalid target url: %v", err),
		}
	}

	// If the caller passed a broadcaster but no Publish hook, default-wire
	// the publish path through it. Lets the api layer write `Broadcaster: bc`
	// and skip the boilerplate.
	if input.Hooks.Publish == nil && input.Broadcaster != nil {
		bc := input.Broadcaster
		runID := input.RunID
		input.Hooks.Publish = func(r *FuzzResult) {
			bc.Publish(&FuzzEvent{Type: FuzzEventResult, RunID: runID, Result: r, At: r.Ts})
		}
	}

	exec := input.Execution
	if exec.Concurrency <= 0 {
		exec.Concurrency = 30
	}
	if exec.RequestTimeoutSeconds <= 0 {
		exec.RequestTimeoutSeconds = 30
	}

	// Per-run rawhttp pipeline. MaxConnections / MaxPendingRequests derive
	// from Concurrency / PerHostConcurrency so users have one knob, not three.
	maxConns := exec.PerHostConcurrency
	if maxConns <= 0 {
		maxConns = exec.Concurrency
	}
	pipeOpts := rawhttp.PipelineOptions{
		Host:                parsedURL.Host,
		Timeout:             time.Duration(exec.RequestTimeoutSeconds) * time.Second,
		MaxConnections:      maxConns,
		MaxPendingRequests:  exec.Concurrency * 4,
		AutomaticHostHeader: input.Request.UpdateHostHeader,
	}
	// Pipeline is held via an atomic pointer so the scheduling goroutine can
	// swap it for a fresh one after a pause. The underlying rawhttp client
	// tears down its connection writer/reader goroutines after 10s of idle
	// (MaxIdleConnDuration, not configurable from outside the lib). A subsequent
	// request can race with that teardown and hang on the work channel; swapping
	// in a fresh pipeline post-pause sidesteps the race entirely.
	var pipelineHolder atomic.Pointer[rawhttp.PipelineClient]
	pipelineHolder.Store(rawhttp.NewPipelineClient(pipeOpts))

	// Rate limiter is a token bucket; nil means unlimited. Each retry
	// attempt acquires a token so retries don't sneak past the cap.
	var limiter *rate.Limiter
	if exec.RPS > 0 {
		limiter = rate.NewLimiter(rate.Limit(exec.RPS), exec.RPS)
	}

	// Per-host semaphore. For the typical single-target fuzz this is just
	// one semaphore; we keep the map so a future "fan out across N hosts"
	// mode comes for free.
	var hostSem *semaphore.Weighted
	if exec.PerHostConcurrency > 0 {
		hostSem = semaphore.NewWeighted(int64(exec.PerHostConcurrency))
	}

	// Derive an inner ctx so we can apply MaxDurationSeconds without
	// affecting the caller's ctx semantics elsewhere.
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()
	if exec.MaxDurationSeconds > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(runCtx, time.Duration(exec.MaxDurationSeconds)*time.Second)
		defer cancel()
	}

	var sent atomic.Int64
	var errs atomic.Int64

	// Progress flusher — periodic snapshots to the caller's Hooks. The
	// engine does NOT touch the DB; the api layer's UpdateProgress handler
	// does.
	flushDone := make(chan struct{})
	go func() {
		defer close(flushDone)
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		lastSent := int64(0)
		for {
			select {
			case <-runCtx.Done():
				return
			case <-tick.C:
				s := sent.Load()
				e := errs.Load()
				if s != lastSent {
					lastSent = s
					if input.Hooks.UpdateProgress != nil {
						input.Hooks.UpdateProgress(int(s), int(e))
					}
				}
			}
		}
	}()

	// Stop-on-error-rate watchdog. Triggers after at least 50 results so we
	// don't react to a flaky first request.
	stopReason := ""
	if exec.StopOnErrorRate > 0 {
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
					s := sent.Load()
					e := errs.Load()
					if s < 50 {
						continue
					}
					rate := float64(e) / float64(s)
					if rate > exec.StopOnErrorRate {
						stopReason = fmt.Sprintf("error rate %.2f exceeded threshold %.2f after %d requests", rate, exec.StopOnErrorRate, s)
						runCancel()
						return
					}
				}
			}
		}()
	}

	workers := pool.New().WithMaxGoroutines(exec.Concurrency)
	assignmentsCh := input.Strategy.Iterate(runCtx, input.Positions, input.Resolved)

	historyOptions := http_utils.HistoryCreationOptions{
		Source:              db.SourceFuzzer,
		WorkspaceID:         input.WorkspaceID,
		PlaygroundSessionID: input.PlaygroundSessionID,
		PlaygroundFuzzRunID: input.RunID,
		CreateNewBodyStream: true,
	}

	for assignment := range assignmentsCh {
		assignment := assignment // capture for goroutine
		// Rate-limit before scheduling so the limiter caps work-creation,
		// not just in-flight requests.
		if limiter != nil {
			if err := limiter.Wait(runCtx); err != nil {
				break // ctx cancelled
			}
		}
		// Pause gate: block scheduling new work while paused. In-flight
		// workers already past this point complete naturally — pause is
		// "stop scheduling," not "instant freeze." If we actually waited,
		// rebuild the rawhttp pipeline because its keep-alive writer may have
		// exited during the pause (see pipelineHolder comment above).
		if input.PauseGate != nil {
			waited, err := input.PauseGate.Wait(runCtx)
			if err != nil {
				break // ctx cancelled while waiting
			}
			if waited {
				pipelineHolder.Store(rawhttp.NewPipelineClient(pipeOpts))
			}
		}
		workers.Go(func() {
			// Per-host acquire/release. Released regardless of error path.
			if hostSem != nil {
				if err := hostSem.Acquire(runCtx, 1); err != nil {
					return
				}
				defer hostSem.Release(1)
			}
			// Jitter before sending — small random delay to stagger bursts.
			if exec.JitterMs > 0 {
				select {
				case <-time.After(time.Duration(rand.Intn(exec.JitterMs)) * time.Millisecond):
				case <-runCtx.Done():
					return
				}
			}
			result := doRequest(runCtx, doRequestInput{
				input:          input,
				assignment:     assignment,
				pipeline:       pipelineHolder.Load(),
				parsedURL:      parsedURL,
				historyOptions: historyOptions,
				limiter:        limiter,
			})
			sent.Add(1)
			if result != nil && result.Error != nil {
				errs.Add(1)
			}
			if result != nil && input.Hooks.Publish != nil {
				input.Hooks.Publish(result)
			}
		})
	}
	workers.Wait()

	// Stop the progress flusher and emit a final progress tick.
	runCancel()
	<-flushDone
	if input.Hooks.UpdateProgress != nil {
		input.Hooks.UpdateProgress(int(sent.Load()), int(errs.Load()))
	}

	outcome := RunOutcome{
		SentCount:  int(sent.Load()),
		ErrorCount: int(errs.Load()),
		Duration:   time.Since(start),
	}
	switch {
	case stopReason != "":
		outcome.Status = db.FuzzRunStoppedErrorRate
		outcome.FailureReason = stopReason
	case errors.Is(ctx.Err(), context.Canceled):
		outcome.Status = db.FuzzRunCancelled
		outcome.FailureReason = "user_cancelled"
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		outcome.Status = db.FuzzRunStoppedMaxDuration
		outcome.FailureReason = fmt.Sprintf("exceeded max_duration=%ds", exec.MaxDurationSeconds)
	default:
		outcome.Status = db.FuzzRunSucceeded
	}
	return outcome
}

// doRequestInput bundles the per-request inputs to keep doRequest's signature
// readable.
type doRequestInput struct {
	input          RunInput
	assignment     Assignment
	pipeline       *rawhttp.PipelineClient
	parsedURL      *url.URL
	historyOptions http_utils.HistoryCreationOptions
	limiter        *rate.Limiter
}

// doRequest issues one fuzzed request (with retry) and returns the
// projection. Returns nil if the request couldn't be built at all (parse
// error on the fuzzed raw HTTP — should be rare given the engine controls
// the substitution).
//
// Wrapped in panic-recovery: the upstream rawhttp + httputil.DumpResponse
// path can panic on a connection that closes mid-read (e.g. a cancel races
// the body read). Treat such panics as a failed request rather than letting
// them crash the whole process.
func doRequest(ctx context.Context, in doRequestInput) (out *FuzzResult) {
	defer func() {
		if r := recover(); r != nil {
			errStr := fmt.Sprintf("engine panic: %v", r)
			log.Warn().Interface("panic", r).Msg("fuzz: recovered from panic in doRequest")
			out = &FuzzResult{
				Index:         in.assignment.Index,
				PayloadValues: in.assignment.Payloads,
				Error:         &errStr,
				Ts:            time.Now(),
			}
		}
	}()
	return doRequestInner(ctx, in)
}

func doRequestInner(ctx context.Context, in doRequestInput) *FuzzResult {
	raw := ReplacePayloads(in.input.RawRequest, in.input.Positions, in.assignment.Payloads)
	parsedReq, err := manual.ParseRawRequest(raw, in.input.TargetURL)
	if err != nil {
		errStr := fmt.Sprintf("parse error: %v", err)
		return &FuzzResult{
			Index:         in.assignment.Index,
			PayloadValues: in.assignment.Payloads,
			Error:         &errStr,
			Ts:            time.Now(),
		}
	}

	exec := in.input.Execution
	maxAttempts := 1 + exec.Retries
	var (
		resp        *http.Response
		lastErr     error
		retryCount  int
		startedAt   time.Time
		durationMs  int
		respHeaders http.Header
	)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter; respect rate limiter on retries.
			backoff := time.Duration(50*(1<<attempt)) * time.Millisecond
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
			backoff += time.Duration(rand.Intn(50)) * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil
			}
			if in.limiter != nil {
				if err := in.limiter.Wait(ctx); err != nil {
					return nil
				}
			}
			// A long-paused run with a retry pending should respect the pause.
			// The scheduler swaps the pipeline on wake; this retry inside the
			// worker uses the same pipeline pointer it was given when scheduled,
			// which is fine — its DoRaw call below either succeeds against the
			// old connection (rare, only if the pause was short) or fails fast
			// with an error and lets the next retry round try again.
			if in.input.PauseGate != nil {
				if _, err := in.input.PauseGate.Wait(ctx); err != nil {
					return nil
				}
			}
			retryCount++
		}
		bodyReader := bytes.NewReader([]byte(parsedReq.Body))
		startedAt = time.Now()
		resp, lastErr = in.pipeline.DoRaw(parsedReq.Method, parsedReq.URL, parsedReq.URI, parsedReq.Headers, bodyReader)
		durationMs = int(time.Since(startedAt).Milliseconds())
		if lastErr != nil {
			continue // retry on network error
		}
		respHeaders = resp.Header
		if !shouldRetryStatus(resp.StatusCode, exec.RetryOn) {
			break
		}
		// retry-on-status: drain and close so the connection is reusable
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if attempt+1 < maxAttempts {
			lastErr = fmt.Errorf("status %d retryable", resp.StatusCode)
		}
	}
	if lastErr != nil && resp == nil {
		errStr := lastErr.Error()
		return &FuzzResult{
			Index:         in.assignment.Index,
			PayloadValues: in.assignment.Payloads,
			DurationMs:    durationMs,
			Error:         &errStr,
			RetryCount:    retryCount,
			Ts:            time.Now(),
		}
	}

	// Persist History row, tag with the run id. Compose the request URL from
	// the parsed target's scheme+host and the URI from the fuzzed raw request
	// — concatenating targetURL+URI would duplicate the path when targetURL
	// already carries one.
	reqURL := &url.URL{
		Scheme: in.parsedURL.Scheme,
		Host:   in.parsedURL.Host,
	}
	if parsed, err := url.Parse(parsedReq.URI); err == nil {
		reqURL.Path = parsed.Path
		reqURL.RawQuery = parsed.RawQuery
		reqURL.Fragment = parsed.Fragment
	}
	resp.Request = &http.Request{
		Method: parsedReq.Method,
		URL:    reqURL,
		Header: parsedReq.Headers,
		Body:   io.NopCloser(bytes.NewReader([]byte(parsedReq.Body))),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
	}
	// rawhttp's pipeline client leaves resp.Proto blank; backfill so the
	// persisted History row + UI detail panel show "HTTP/1.1" instead of "HTTP/0.0".
	if resp.Proto == "" {
		resp.Proto = "HTTP/1.1"
		resp.ProtoMajor = 1
		resp.ProtoMinor = 1
	}
	historyRow, err := http_utils.ReadHttpResponseAndCreateHistory(resp, in.historyOptions)
	if err != nil {
		log.Warn().Err(err).Str("url", reqURL.String()).Msg("fuzz: history persist failed")
		errStr := fmt.Sprintf("history persist: %v", err)
		return &FuzzResult{
			Index:         in.assignment.Index,
			PayloadValues: in.assignment.Payloads,
			DurationMs:    durationMs,
			Error:         &errStr,
			RetryCount:    retryCount,
			Ts:            time.Now(),
		}
	}
	// Run id is set on the History row via in.historyOptions.PlaygroundFuzzRunID
	// during persist — no separate attach step needed.

	body := historyRow.RawResponse
	wc, lc := countWordsLines(body)
	contentType := ""
	if respHeaders != nil {
		contentType = respHeaders.Get("Content-Type")
	}

	// Compute baseline match if the run has a baseline configured. We
	// recompute simhash here rather than passing it from the response loop
	// to keep doRequest's signature simple; the cost is negligible.
	baselineMatch := false
	if in.input.Baseline != nil && in.input.Baseline.Mode != AutoBaselineOff {
		bodyOnly := extractBodyForFingerprint(body)
		candidate := BaselineFingerprint{
			StatusCode:       historyRow.StatusCode,
			ResponseBodySize: historyRow.ResponseBodySize,
			WordCount:        wc,
			LineCount:        lc,
			BodySimhash:      simhash.Simhash(simhash.NewWordFeatureSet(bodyOnly)),
			ContentType:      contentType,
		}
		baselineMatch = IsBaselineMatch(in.input.Baseline, in.assignment.PositionIndex, candidate)
	}

	return &FuzzResult{
		HistoryID:           historyRow.ID,
		Index:               in.assignment.Index,
		StatusCode:          historyRow.StatusCode,
		Method:              parsedReq.Method,
		URL:                 historyRow.URL,
		ResponseBodySize:    historyRow.ResponseBodySize,
		ResponseContentType: contentType,
		DurationMs:          durationMs,
		PayloadValues:       in.assignment.Payloads,
		WordCount:           wc,
		LineCount:           lc,
		RetryCount:          retryCount,
		BaselineMatch:       baselineMatch,
		Ts:                  time.Now(),
	}
}

// extractBodyForFingerprint splits the body off a raw HTTP response. Mirrors
// the heuristic used by countWordsLines but returns bytes instead of counts.
func extractBodyForFingerprint(raw []byte) []byte {
	if idx := bytes.Index(raw, []byte("\r\n\r\n")); idx >= 0 {
		return raw[idx+4:]
	}
	if idx := bytes.Index(raw, []byte("\n\n")); idx >= 0 {
		return raw[idx+2:]
	}
	return raw
}

func shouldRetryStatus(status int, retryOn []int) bool {
	for _, s := range retryOn {
		if s == status {
			return true
		}
	}
	return false
}

// countWordsLines returns the word and line count of the response body — used
// by matchers. Whitespace-delimited words; \n-delimited lines (compatible
// with ffuf -mr / -ml semantics that pentesters expect).
func countWordsLines(raw []byte) (int, int) {
	if len(raw) == 0 {
		return 0, 0
	}
	// raw is the full raw HTTP response; split off the body. We don't need
	// to be precise — we strip headers by looking for the first blank line.
	body := raw
	if idx := bytes.Index(raw, []byte("\r\n\r\n")); idx >= 0 {
		body = raw[idx+4:]
	} else if idx := bytes.Index(raw, []byte("\n\n")); idx >= 0 {
		body = raw[idx+2:]
	}
	wc := len(strings.Fields(string(body)))
	lc := bytes.Count(body, []byte("\n"))
	if len(body) > 0 && body[len(body)-1] != '\n' {
		lc++ // count trailing line without newline
	}
	return wc, lc
}

// progressMu guards the shared progress counters used by the flusher when
// Hooks.UpdateProgress is invoked concurrently with later increments. The
// engine reads atomics for the counters themselves; this mutex serializes
// invocations of the user-supplied callback so the api layer doesn't have to
// worry about reentrancy.
var progressMu sync.Mutex

// invokeProgress is reserved for future use; UpdateProgress is currently
// invoked inline. Kept as a hook point to add backpressure if needed.
var _ = progressMu
