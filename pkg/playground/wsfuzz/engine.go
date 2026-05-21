package wsfuzz

import (
	"context"
	"encoding/json"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/pyneda/sukyan/pkg/playground/stream"
	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
	"golang.org/x/time/rate"
)

// EngineDeps is everything Run needs from the outside.
type EngineDeps struct {
	Persister    RunPersister
	Broadcaster  *stream.Broadcaster
	Dial         func(ctx context.Context, cfg wsreplay.SessionConfig) (SessionHandle, error)
	HTTPRespRef  *HTTPResponseRef  // optional; from PreSetup http_request kind
	RunScopeVars map[string]string // optional; from PreSetup extractions
}

// Run executes a full wsfuzz run end-to-end. Blocks until terminal state.
// Status transitions are persisted; events are published to the broadcaster.
func Run(
	ctx context.Context,
	runID uint,
	cfg WsFuzzerConfig,
	deps EngineDeps,
) error {
	if deps.Dial == nil {
		return wrapErr("EngineDeps.Dial is required")
	}
	if deps.Persister == nil {
		return wrapErr("EngineDeps.Persister is required")
	}

	// Per-run context lets the registry's Cancel terminate us. If max_duration
	// is set, also enforce that as a hard wall-clock cap.
	var runCtx context.Context
	var cancel context.CancelFunc
	if md := cfg.ExecutionOptions.MaxDurationSeconds; md > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(md)*time.Second)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()
	Registry().Register(runID, cancel, deps.Broadcaster)
	defer Registry().Unregister(runID)
	gate := Registry().Gate(runID)

	startTime := time.Now()
	_ = deps.Persister.UpdateRunStartedAt(runID, startTime)

	publish := func(ev *WsFuzzEvent) {
		ev.RunID = runID
		ev.Ts = time.Now()
		if deps.Broadcaster != nil {
			deps.Broadcaster.Publish(ev)
		}
	}
	setStatus := func(from, to, reason string) {
		_ = deps.Persister.UpdateRunStatus(runID, to, reason)
		publish(&WsFuzzEvent{Type: EventStatus, Status: &WsFuzzStatus{From: from, To: to, Reason: reason}})
	}

	// Run pre-iteration setup ONCE (e.g. http_request to obtain an auth token).
	// Resulting variables become run-scope vars visible to every iteration.
	if cfg.PreIterationSetup != nil && cfg.PreIterationSetup.Kind != SetupNone && cfg.PreIterationSetup.Kind != "" {
		preVars, preResp, err := runPreSetup(runCtx, cfg.PreIterationSetup, cfg, deps.Dial)
		if err != nil {
			log.Warn().Err(err).Uint("run_id", runID).Msg("wsfuzz: pre-iteration setup failed")
			// If any extraction has abort policy, the run can't proceed without its var.
			for _, ext := range cfg.PreIterationSetup.Extract {
				if ext.FallbackPolicy == FallbackAbort {
					reason := "pre-iteration setup failed: " + err.Error()
					setStatus("pending", "failed", reason)
					publish(&WsFuzzEvent{Type: EventDone, Done: &WsFuzzDone{Status: "failed", FailureReason: reason, FinishedAt: time.Now()}})
					if deps.Broadcaster != nil {
						deps.Broadcaster.Close()
					}
					return wrapErr(reason)
				}
			}
		}
		if deps.RunScopeVars == nil {
			deps.RunScopeVars = map[string]string{}
		}
		for k, v := range preVars {
			deps.RunScopeVars[k] = v
		}
		if preResp != nil && deps.HTTPRespRef == nil {
			deps.HTTPRespRef = preResp
		}
	}

	// 1. Build position refs and resolve payloads (once per run).
	refs := BuildPositionRefs(cfg.Script)
	flat := FlatPositions(refs)
	resolved := fuzz.Resolve(cfg.Mode, flat, cfg.SharedPayloads)
	strategy, sErr := fuzz.StrategyFor(cfg.Mode)
	if sErr != nil {
		setStatus("pending", "failed", sErr.Error())
		publish(&WsFuzzEvent{Type: EventDone, Done: &WsFuzzDone{Status: "failed", FailureReason: sErr.Error(), FinishedAt: time.Now()}})
		if deps.Broadcaster != nil {
			deps.Broadcaster.Close()
		}
		return sErr
	}

	plannedCount, _ := strategy.RequestCount(flat, resolved)

	// 2. Calibrate baseline (if enabled). Probes use original payload values
	// — each position's OriginalValue (already captured at insertion-point time).
	var baseline *WsBaselineFingerprint
	if cfg.AutoBaseline != fuzz.AutoBaselineOff && cfg.AutoBaseline != "" {
		setStatus("pending", "calibrating", "")
		publish(&WsFuzzEvent{Type: EventCalibrating})
		probeCount := cfg.ExecutionOptions.BaselineProbeCount
		if probeCount == 0 {
			probeCount = 3
		}
		origPayloads := make([]string, len(refs))
		for i, r := range refs {
			origPayloads[i] = r.Position.OriginalValue
		}
		probes := make([]WsBaselineFingerprint, 0, probeCount)
		for p := 0; p < probeCount; p++ {
			if runCtx.Err() != nil {
				break
			}
			res, _ := RunIteration(runCtx, cfg, -1-p, refs, origPayloads, deps.RunScopeVars, nil, IterationDeps{
				Dial:        deps.Dial,
				HTTPRespRef: deps.HTTPRespRef,
			})
			// We don't have access to the raw frame list from the iteration result
			// (the per-iteration WsIterationResult only carries a summary). For v1
			// we can only seed FrameCount=0 placeholder if the iteration succeeded;
			// real baseline content fingerprinting requires future work to surface
			// frames from RunIteration. See spec §3 "calibration".
			probes = append(probes, WsBaselineFingerprint{
				FrameCount:      0, // unknown without frame list
				HandshakeStatus: res.HandshakeStatusCode,
			})
		}
		fp, warns := CalibrateFromProbes(probes)
		for _, w := range warns {
			publish(&WsFuzzEvent{Type: EventWarning, Warning: &WsFuzzWarning{Code: w}})
		}
		if !contains(warns, "baseline_disabled_count_disagreement") {
			baseline = &fp
			b, _ := json.Marshal(fp)
			_ = deps.Persister.UpdateRunBaseline(runID, b)
			publish(&WsFuzzEvent{Type: EventBaseline, Baseline: &fp})
		}
	}

	// 3. Transition to running.
	setStatus("calibrating", "running", "")

	// 4. Worker pool.
	concurrency := cfg.ExecutionOptions.Concurrency
	if concurrency <= 0 {
		concurrency = 20
	}
	wp := pool.New().WithMaxGoroutines(concurrency)

	var sent, errs, findings int64
	var inFlight int64

	// 5. Progress flusher.
	stopProgress := make(chan struct{})
	var flushWG sync.WaitGroup
	flushWG.Add(1)
	go func() {
		defer flushWG.Done()
		tick := time.NewTicker(1 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stopProgress:
				return
			case <-tick.C:
				s := atomic.LoadInt64(&sent)
				e := atomic.LoadInt64(&errs)
				f := atomic.LoadInt64(&findings)
				inf := atomic.LoadInt64(&inFlight)
				_ = deps.Persister.UpdateRunProgress(runID, int(s), int(e), int(f))
				elapsed := time.Since(startTime).Seconds()
				curRate := float64(s) / max1(elapsed)
				publish(&WsFuzzEvent{
					Type: EventProgress,
					Progress: &WsFuzzProgress{
						Sent:              int(s),
						Errors:            int(e),
						Findings:          int(f),
						InFlight:          int(inf),
						PlannedIterations: plannedCount,
						CurrentRate:       curRate,
						ElapsedSeconds:    int(elapsed),
					},
				})
			}
		}
	}()

	// 6. Iterate assignments through the pool.
	assignments := strategy.Iterate(runCtx, flat, resolved)
	jitterMs := cfg.ExecutionOptions.JitterMs
	var limiter *rate.Limiter
	if rps := cfg.ExecutionOptions.RPS; rps > 0 {
		limiter = rate.NewLimiter(rate.Limit(rps), rps)
	}
assignLoop:
	for assignment := range assignments {
		if limiter != nil {
			if err := limiter.Wait(runCtx); err != nil {
				break assignLoop
			}
		}
		if jitterMs > 0 {
			select {
			case <-runCtx.Done():
				break assignLoop
			case <-time.After(time.Duration(rand.Intn(jitterMs+1)) * time.Millisecond):
			}
		}
		// Pause gate: block scheduling new iterations while paused.
		if gate != nil {
			if _, err := gate.Wait(runCtx); err != nil {
				break assignLoop // ctx cancelled while waiting
			}
		}
		if runCtx.Err() != nil {
			break assignLoop
		}
		a := assignment // capture loop var for goroutine
		atomic.AddInt64(&inFlight, 1)
		wp.Go(func() {
			defer atomic.AddInt64(&inFlight, -1)
			defer func() {
				if rec := recover(); rec != nil {
					log.Error().Interface("panic", rec).Uint("run_id", runID).Int("index", a.Index).Msg("wsfuzz: iteration panicked, isolating")
					atomic.AddInt64(&sent, 1)
					atomic.AddInt64(&errs, 1)
				}
			}()
			res, _ := RunIteration(runCtx, cfg, a.Index, refs, a.Payloads, deps.RunScopeVars, baseline, IterationDeps{
				Dial:        deps.Dial,
				HTTPRespRef: deps.HTTPRespRef,
			})
			res.RunID = runID
			if perr := deps.Persister.SaveIteration(res); perr != nil {
				log.Warn().Err(perr).Uint("run_id", runID).Int("index", a.Index).Msg("wsfuzz: persist iteration")
			}
			atomic.AddInt64(&sent, 1)
			if res.Status.CountsTowardErrorRate() {
				atomic.AddInt64(&errs, 1)
			}
			if res.Status == StatusCheckFailed {
				atomic.AddInt64(&findings, 1)
			}
			publish(&WsFuzzEvent{Type: EventResult, Result: &res})

			// stop_on_error_rate: abort the run when the cumulative error
			// rate exceeds the threshold AND we have enough samples to make
			// the rate meaningful (≥10 sent so a single early error doesn't
			// torpedo the run).
			if er := cfg.ExecutionOptions.StopOnErrorRate; er > 0 {
				s := atomic.LoadInt64(&sent)
				e := atomic.LoadInt64(&errs)
				if s >= 10 && float64(e)/float64(s) >= er {
					cancel()
				}
			}
		})
	}

	wp.Wait()
	close(stopProgress)
	flushWG.Wait()

	// Final progress flush — short runs may complete before the ticker fires,
	// leaving sent_count=0 in the DB.
	finalSent := atomic.LoadInt64(&sent)
	finalErrs := atomic.LoadInt64(&errs)
	finalFindings := atomic.LoadInt64(&findings)
	_ = deps.Persister.UpdateRunProgress(runID, int(finalSent), int(finalErrs), int(finalFindings))

	// 7. Terminal state.
	finalStatus := "succeeded"
	failureReason := ""
	if runCtx.Err() != nil {
		finalStatus = "cancelled"
	}
	finished := time.Now()
	_ = deps.Persister.UpdateRunFinishedAt(runID, finished)
	setStatus("running", finalStatus, failureReason)
	publish(&WsFuzzEvent{
		Type: EventDone,
		Done: &WsFuzzDone{
			Status:          finalStatus,
			Sent:            int(atomic.LoadInt64(&sent)),
			Errors:          int(atomic.LoadInt64(&errs)),
			Findings:        int(atomic.LoadInt64(&findings)),
			DurationSeconds: int(time.Since(startTime).Seconds()),
			FailureReason:   failureReason,
			FinishedAt:      finished,
		},
	})
	if deps.Broadcaster != nil {
		deps.Broadcaster.Close()
	}
	return nil
}

func max1(x float64) float64 {
	if x < 1 {
		return 1
	}
	return x
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

type engineError string

func (e engineError) Error() string { return string(e) }

func wrapErr(s string) error { return engineError("wsfuzz: " + s) }
