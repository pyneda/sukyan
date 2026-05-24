package wsreplay

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pyneda/sukyan/pkg/playground/stream"
)

// RunResult captures the outcome of one WalkScript invocation.
type RunResult struct {
	Status           string // "succeeded" | "failed" | "cancelled"
	CurrentStepIndex int
	FailureReason    string
}

// WalkScript executes the script against a connected session, emitting per-step
// events on the broadcaster and honoring the script entries' on_timeout / on_no_match
// policies. Returns when the script completes, fails, or ctx is cancelled.
//
// Contracts:
//   - Cancellation: ctx cancellation is observed at delay boundaries and between
//     NextFrame calls. To interrupt an in-flight wait_for during a NextFrame call,
//     callers must also close the session. The Manager's CancelRun (Task 15) does
//     both.
//   - Exclusive frame ownership: WalkScript reads received frames via
//     sess.NextFrame, which is destructive on the session's frame channel. The
//     run-instance socket model gives each run its own session, so this is fine
//     in practice; do not invoke WalkScript on a session that has another active
//     consumer (e.g. the interactive socket).
//   - Terminal events: WalkScript emits per-step events (run_step_started,
//     wait_*, run_step_completed) only. The terminal events (run_started,
//     run_finished, run_failed, run_cancelled) are the caller's responsibility,
//     paired with the DB-row status transitions.
//   - On a NextFrame error during wait_for, the walker classifies it as a
//     timeout for backward compatibility with existing tests. A closed upstream
//     would surface as wait_timeout rather than a distinct event; this is a
//     known minor mis-categorization (TODO: distinguish via an explicit
//     ErrSessionClosed sentinel from session.NextFrame).
//   - wait_timeout is emitted unconditionally when the deadline elapses, even
//     when on_timeout=abort. UIs see "timed out, then aborted" rather than the
//     run silently failing.
func WalkScript(ctx context.Context, sess *Session, script []ScriptEntry, opts SessionOptions, b *stream.Broadcaster) RunResult {
	res := RunResult{Status: "succeeded"}
	publish := func(t string, data map[string]any) {
		raw, _ := json.Marshal(data)
		b.Publish(&Event{Type: t, Instance: sess.Instance(), Data: raw, Ts: time.Now()})
	}
	runID := sess.Instance().RunID

	// vars accumulates the run-scoped variables produced by Extract specs on
	// earlier steps. Later steps can reference them via `${name}` in their
	// Content. WS Replay is single-execution, so there's no per-iteration
	// scope to worry about; vars live for the whole run.
	vars := map[string]string{}

	for i, step := range script {
		res.CurrentStepIndex = i
		publish("run_step_started", map[string]any{"run_id": runID, "step_index": i})

		delay := time.Duration(step.DelayMs+opts.InterStepDelayMs) * time.Millisecond
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				res.Status = "cancelled"
				return res
			}
		}

		// Expand ${var} references in the step's content. A typo'd or
		// out-of-order variable name fails the run immediately rather than
		// shipping the literal `${name}` to the peer (which would silently
		// confuse downstream protocols).
		expandedContent, undefined, _ := SubstituteVarsStrict(step.Content, vars)
		if len(undefined) > 0 {
			res.Status = "failed"
			res.FailureReason = formatUndefinedVarReason(undefined)
			publish("run_step_failed", map[string]any{
				"run_id": runID, "step_index": i, "reason": res.FailureReason,
			})
			return res
		}

		if err := sess.Send(step.Opcode, expandedContent); err != nil {
			res.Status = "failed"
			res.FailureReason = "send: " + err.Error()
			return res
		}

		var matchedFrame *Frame
		if step.WaitFor != nil {
			publish("wait_started", map[string]any{
				"run_id": runID, "step_index": i,
				"match_type": step.WaitFor.MatchType, "pattern": step.WaitFor.Pattern, "timeout_ms": step.WaitFor.TimeoutMs,
			})
			deadline := time.Now().Add(time.Duration(step.WaitFor.TimeoutMs) * time.Millisecond)
		waitLoop:
			for {
				remaining := time.Until(deadline)
				if remaining <= 0 {
					publish("wait_timeout", map[string]any{"run_id": runID, "step_index": i})
					if step.OnTimeout == PolicyAbort {
						res.Status = "failed"
						res.FailureReason = "wait_timeout"
						return res
					}
					break waitLoop
				}
				select {
				case <-ctx.Done():
					res.Status = "cancelled"
					return res
				default:
				}
				frame, err := sess.NextFrame(remaining)
				if err != nil {
					publish("wait_timeout", map[string]any{"run_id": runID, "step_index": i})
					if step.OnTimeout == PolicyAbort {
						res.Status = "failed"
						res.FailureReason = "wait_timeout"
						return res
					}
					break waitLoop
				}
				if Match(*step.WaitFor, frame.Content) {
					matched := frame
					matchedFrame = &matched
					publish("wait_matched", map[string]any{"run_id": runID, "step_index": i, "message_id": frame.MessageID})
					break waitLoop
				}
				publish("wait_no_match", map[string]any{"run_id": runID, "step_index": i, "message_id": frame.MessageID})
				if step.OnNoMatch == PolicyAbort {
					res.Status = "failed"
					res.FailureReason = "wait_no_match"
					return res
				}
				// continue policy: keep looping until deadline
			}
		}

		// Apply Extract specs against the matched frame. We require a
		// wait_for match to have a deterministic source frame — extracting
		// from "whatever comes in" would be racy and lead to flaky scripts.
		// Steps without a wait_for that nevertheless declare Extract specs
		// fail the run with a clear reason rather than skipping silently.
		if len(step.Extract) > 0 {
			if matchedFrame == nil {
				res.Status = "failed"
				res.FailureReason = "extract requires wait_for: step has Extract specs but no matched frame"
				publish("run_step_failed", map[string]any{
					"run_id": runID, "step_index": i, "reason": res.FailureReason,
				})
				return res
			}
			for _, ext := range step.Extract {
				value, ok := ext.Apply(*matchedFrame)
				if !ok && ext.OnFailure != PolicyContinue {
					res.Status = "failed"
					res.FailureReason = "extract " + ext.Name + " failed: no value captured"
					publish("run_step_failed", map[string]any{
						"run_id": runID, "step_index": i, "reason": res.FailureReason,
					})
					return res
				}
				vars[ext.Name] = value
				publish("extracted_variable", map[string]any{
					"run_id": runID, "step_index": i, "name": ext.Name,
				})
			}
		}

		publish("run_step_completed", map[string]any{"run_id": runID, "step_index": i})
	}
	return res
}

// formatUndefinedVarReason builds a human-readable failure_reason for the
// run row. Singular/plural variant + the names inline so the UI can show
// the offending variable name(s) without parsing.
func formatUndefinedVarReason(names []string) string {
	if len(names) == 0 {
		return ""
	}
	plural := ""
	if len(names) > 1 {
		plural = "s"
	}
	out := "undefined variable" + plural + " in step content: "
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += "${" + n + "}"
	}
	return out
}
