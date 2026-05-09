package wsreplay

import (
	"context"
	"encoding/json"
	"time"
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
func WalkScript(ctx context.Context, sess *Session, script []ScriptEntry, opts SessionOptions, b *Broadcaster) RunResult {
	res := RunResult{Status: "succeeded"}
	publish := func(t string, data map[string]any) {
		raw, _ := json.Marshal(data)
		b.Publish(Event{Type: t, Instance: sess.Instance(), Data: raw, Ts: time.Now()})
	}
	runID := sess.Instance().RunID

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

		if err := sess.Send(step.Opcode, step.Content); err != nil {
			res.Status = "failed"
			res.FailureReason = "send: " + err.Error()
			return res
		}

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
		publish("run_step_completed", map[string]any{"run_id": runID, "step_index": i})
	}
	return res
}
