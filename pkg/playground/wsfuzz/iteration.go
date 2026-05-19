package wsfuzz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
)

// SessionHandle is the subset of *wsreplay.Session used by the iteration
// runner. Re-exported so the engine + tests can plug in either a real session
// or a fake without dialing.
type SessionHandle interface {
	Send(opcode int, content string) error
	NextFrame(timeout time.Duration) (wsreplay.Frame, error)
	Close()
	ConnectionID() uint
}

// IterationDeps is everything runIteration needs from the outside. Decoupled
// from the engine so iteration tests can swap dial/persistence at the seam.
type IterationDeps struct {
	Dial        func(ctx context.Context, cfg wsreplay.SessionConfig) (SessionHandle, error)
	HTTPRespRef *HTTPResponseRef // optional, from PreSetup
}

// WrapSession adapts a *wsreplay.Session to SessionHandle. The engine uses
// this to thread a real dial result into the iteration runner.
func WrapSession(s *wsreplay.Session) SessionHandle { return &wsreplaySessionAdapter{s: s} }

type wsreplaySessionAdapter struct{ s *wsreplay.Session }

func (a *wsreplaySessionAdapter) Send(opcode int, content string) error {
	return a.s.Send(opcode, content)
}
func (a *wsreplaySessionAdapter) NextFrame(timeout time.Duration) (wsreplay.Frame, error) {
	return a.s.NextFrame(timeout)
}
func (a *wsreplaySessionAdapter) Close()             { a.s.Close() }
func (a *wsreplaySessionAdapter) ConnectionID() uint { return a.s.ConnectionID() }

// RunIteration executes one iteration of the script and returns its result.
// All errors that terminate the iteration are mapped to an IterationStatus
// on the returned result; the function only returns a non-nil error for
// programming bugs (e.g., nil deps).
//
// Substitution order: payload first (byte offsets are valid against the
// original step.Content), then variables (${name} expansion). This order is
// required by the spec — see docs/superpowers/specs/2026-05-20-websocket-fuzzer-design.md §3.
func RunIteration(
	ctx context.Context,
	cfg WsFuzzerConfig,
	iterationIndex int,
	positionRefs []WsPositionRef,
	payloadAssignment []string,
	runScopeVars map[string]string,
	baseline *WsBaselineFingerprint,
	deps IterationDeps,
) (WsIterationResult, error) {
	if deps.Dial == nil {
		return WsIterationResult{}, errors.New("wsfuzz: IterationDeps.Dial is required")
	}
	start := time.Now()

	// vars is the per-iteration variable scope. Seeded from run-scope vars,
	// then mutated by per-step extractions.
	vars := make(map[string]string, len(runScopeVars))
	for k, v := range runScopeVars {
		vars[k] = v
	}

	// 1. Build the concrete script for this iteration.
	concrete := make([]concreteStep, len(cfg.Script))
	for i, s := range cfg.Script {
		content := s.Content
		if s.Role == RoleFuzz {
			pos, pay := PositionsAndPayloadsForStep(i, positionRefs, payloadAssignment)
			content = fuzz.ReplacePayloads(content, pos, pay)
		}
		expanded, _ := SubstituteVars(content, vars)
		concrete[i] = concreteStep{spec: s, content: expanded}
	}

	// 2. Iteration-wide context with a wall-clock budget.
	timeoutBudget := time.Duration(cfg.ExecutionOptions.RequestTimeoutSeconds) * time.Second
	if timeoutBudget == 0 {
		timeoutBudget = 60 * time.Second
	}
	iterCtx, cancel := context.WithTimeout(ctx, timeoutBudget)
	defer cancel()

	// 3. Dial a fresh socket. Source is tagged "ws_fuzz" so the captures UI
	// can exclude these connections.
	dialCfg := wsreplay.SessionConfig{
		TargetURL:      cfg.TargetURL,
		Headers:        cfg.RequestHeaders,
		Instance:       wsreplay.RunInstance(0), // engine isn't tracking a Run ID here
		Source:         "ws_fuzz",
		TLSConfig:      BuildTLSConfig(cfg.TLSConfig),
		ConnectTimeout: time.Duration(cfg.ConnectionTimeout) * time.Millisecond,
	}
	sess, err := deps.Dial(iterCtx, dialCfg)
	if err != nil {
		return finalize(start, iterationIndex, payloadAssignment, StatusConnectionError, "dial: "+err.Error(), nil, 0, nil, vars), nil
	}
	defer closeWithTimeout(sess, 5*time.Second)

	// 4. Walk steps.
	var frames []wsreplay.Frame
	var status IterationStatus = StatusCompleted
	var failureReason string
	var failedStep *int
	var peerCloseCode *int
	var checkResults []checkResultEntry

stepLoop:
	for i, cs := range concrete {
		// Delay before sending.
		if cs.spec.DelayMs > 0 {
			select {
			case <-iterCtx.Done():
				status = StatusIterationTimeout
				failureReason = "iteration context cancelled during delay"
				break stepLoop
			case <-time.After(time.Duration(cs.spec.DelayMs) * time.Millisecond):
			}
		}

		// Send (setup + fuzz steps; check steps are assertion-only).
		opcode := cs.spec.Opcode
		if opcode == 0 {
			opcode = 1
		}
		if cs.spec.Role == RoleSetup || cs.spec.Role == RoleFuzz {
			if sendErr := sess.Send(opcode, cs.content); sendErr != nil {
				idx := i
				failedStep = &idx
				if iterCtx.Err() != nil {
					status = StatusIterationTimeout
					failureReason = "iteration timeout during send"
				} else {
					status = StatusConnectionError
					failureReason = "send: " + sendErr.Error()
				}
				frames = append(frames, wsreplay.Frame{Opcode: opcode, Content: cs.content, Direction: "sent", Ts: time.Now()})
				break stepLoop
			}
			frames = append(frames, wsreplay.Frame{Opcode: opcode, Content: cs.content, Direction: "sent", Ts: time.Now()})
		}

		// Wait for a matching response.
		if cs.spec.WaitFor != nil {
			waitTO := time.Duration(cs.spec.WaitFor.TimeoutMs) * time.Millisecond
			matched := false
			for !matched {
				remaining := remainingBudget(iterCtx, waitTO)
				if remaining <= 0 {
					if cs.spec.OnTimeout == PolicyAbort {
						idx := i
						failedStep = &idx
						status = StatusStepFailedTimeout
						failureReason = "wait_for budget exhausted"
						break stepLoop
					}
					break // continue to next step
				}
				f, ferr := sess.NextFrame(remaining)
				if ferr != nil {
					if isPeerClose(ferr) {
						code := extractCloseCode(ferr)
						peerCloseCode = &code
						idx := i
						failedStep = &idx
						status = StatusPeerClosed
						failureReason = "peer closed: " + ferr.Error()
						break stepLoop
					}
					if iterCtx.Err() != nil {
						status = StatusIterationTimeout
						failureReason = "iteration timeout"
						break stepLoop
					}
					if isTimeout(ferr) {
						if cs.spec.OnTimeout == PolicyAbort {
							idx := i
							failedStep = &idx
							status = StatusStepFailedTimeout
							failureReason = "wait_for timeout"
							break stepLoop
						}
						break // OnTimeout=continue → next step
					}
					// network error
					idx := i
					failedStep = &idx
					status = StatusConnectionError
					failureReason = "next_frame: " + ferr.Error()
					break stepLoop
				}
				frames = append(frames, f)
				if wsreplay.Match(*cs.spec.WaitFor, f.Content) {
					matched = true
					break
				}
				if cs.spec.OnNoMatch == PolicyAbort {
					idx := i
					failedStep = &idx
					status = StatusStepFailedNoMatch
					failureReason = "wait_for no_match"
					break stepLoop
				}
				// continue looping for the next frame
			}
		}

		// Extract variables.
		for _, ext := range cs.spec.Extract {
			val, ok := ApplyExtraction(ext, frames, deps.HTTPRespRef)
			if !ok && ext.FallbackPolicy == FallbackAbort {
				idx := i
				failedStep = &idx
				status = StatusStepFailedExtraction
				failureReason = fmt.Sprintf("extraction %q failed", ext.Name)
				break stepLoop
			}
			vars[ext.Name] = val
		}

		// Check step assertion (short-circuits on failure to skip remaining steps).
		if cs.spec.Role == RoleCheck && cs.spec.CheckAssert != nil {
			passed, details := evaluateCheck(*cs.spec.CheckAssert, frames, vars)
			checkResults = append(checkResults, details)
			if !passed {
				idx := i
				failedStep = &idx
				status = StatusCheckFailed
				failureReason = "check assertion failed"
				break stepLoop
			}
		}
	}

	// Finalize.
	connID := uint(0)
	if sess != nil {
		connID = sess.ConnectionID()
	}
	handshakeCode := 0 // exposing handshake status from wsreplay.Session is future work
	res := finalize(start, iterationIndex, payloadAssignment, status, failureReason, failedStep, handshakeCode, &connID, vars)
	res.PeerCloseCode = peerCloseCode
	if len(checkResults) > 0 {
		b, _ := json.Marshal(checkResults)
		res.CheckResults = b
	}
	if baseline != nil {
		fp := ComputeFingerprint(frames, handshakeCode)
		res.BaselineMatch = CompareFingerprint(fp, *baseline)
	}
	return res, nil
}

type concreteStep struct {
	spec    WsFuzzStep
	content string
}

// checkResultEntry is the per-iteration record produced by evaluateCheck.
// Filled in by Task 20 (matcher evaluator); placeholder here so the package
// compiles with a generic shape.
type checkResultEntry struct {
	Logic  AssertionLogic     `json:"logic"`
	Passed bool               `json:"passed"`
	Rules  []checkRuleOutcome `json:"rules"`
}

type checkRuleOutcome struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
	Actual   string `json:"actual"`
	Passed   bool   `json:"passed"`
}


func remainingBudget(ctx context.Context, stepTO time.Duration) time.Duration {
	if dl, ok := ctx.Deadline(); ok {
		left := time.Until(dl)
		if left < stepTO {
			return left
		}
	}
	return stepTO
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "timeout") || errors.Is(err, context.DeadlineExceeded)
}

func isPeerClose(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "websocket: close") || strings.Contains(msg, "going away")
}

// extractCloseCode best-effort parses a gorilla close-error string for the
// numeric close code. wsreplay currently surfaces only string errors;
// structuring them is future work.
func extractCloseCode(err error) int {
	if err == nil {
		return 0
	}
	msg := err.Error()
	if idx := strings.Index(msg, "websocket: close "); idx >= 0 {
		var code int
		fmt.Sscanf(msg[idx+len("websocket: close "):], "%d", &code)
		return code
	}
	return 0
}

func closeWithTimeout(sess SessionHandle, d time.Duration) {
	if sess == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		sess.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
		// Bounded so the iteration goroutine never wedges on a slow upstream.
	}
}

func finalize(start time.Time, idx int, payloads []string, status IterationStatus, reason string, failedStep *int, handshake int, connID *uint, vars map[string]string) WsIterationResult {
	res := WsIterationResult{
		IterationIndex:        idx,
		Status:                status,
		PayloadValues:         payloads,
		DurationMs:            int(time.Since(start).Milliseconds()),
		HandshakeStatusCode:   handshake,
		WebSocketConnectionID: connID,
		FailureReason:         reason,
		FailedStepIndex:       failedStep,
		Ts:                    time.Now(),
	}
	if len(vars) > 0 {
		res.VariablesSnapshot = vars
	}
	return res
}
