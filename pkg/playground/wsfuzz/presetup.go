package wsfuzz

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
)

// runPreSetup executes the configured PreSetup once per run and returns the
// captured run-scope variables + optional HTTP response ref (for further
// extractions during iterations, though current usage is run-scope only).
//
// Supports SetupHTTPRequest and SetupWsScript. The ws-script branch dials
// the run's target_url, runs each PreSetup step sequentially (Send + optional
// wait_for + extract), then closes the socket. The collected vars become the
// run-scope vars visible to every iteration via SubstituteVars.
func runPreSetup(ctx context.Context, p *PreSetup, runCfg WsFuzzerConfig, dial func(context.Context, wsreplay.SessionConfig) (SessionHandle, error)) (map[string]string, *HTTPResponseRef, error) {
	if p == nil || p.Kind == SetupNone || p.Kind == "" {
		return nil, nil, nil
	}
	vars := map[string]string{}
	var httpResp *HTTPResponseRef

	switch p.Kind {
	case SetupHTTPRequest:
		if p.HTTPRequest == nil {
			return vars, nil, nil
		}
		req, err := http.NewRequestWithContext(ctx, p.HTTPRequest.Method, p.HTTPRequest.URL, strings.NewReader(p.HTTPRequest.Body))
		if err != nil {
			return vars, nil, err
		}
		for k, v := range p.HTTPRequest.Headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return vars, nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		hdrs := map[string]string{}
		for k, v := range resp.Header {
			if len(v) > 0 {
				hdrs[k] = v[0]
			}
		}
		httpResp = &HTTPResponseRef{
			StatusCode: resp.StatusCode,
			Headers:    hdrs,
			Body:       string(body),
		}

	case SetupWsScript:
		if dial == nil {
			return vars, nil, errors.New("ws_script presetup: dial function not provided")
		}
		if len(p.Steps) == 0 {
			return vars, nil, nil
		}
		dialCfg := wsreplay.SessionConfig{
			TargetURL:      runCfg.TargetURL,
			Headers:        runCfg.RequestHeaders,
			Instance:       wsreplay.RunInstance(0),
			Source:         "ws_fuzz",
			TLSConfig:      BuildTLSConfig(runCfg.TLSConfig),
			Subprotocols:   runCfg.Subprotocols,
			ConnectTimeout: time.Duration(runCfg.ConnectionTimeout) * time.Millisecond,
		}
		sess, err := dial(ctx, dialCfg)
		if err != nil {
			return vars, nil, fmt.Errorf("ws_script presetup dial: %w", err)
		}
		defer closeWithTimeout(sess, 5*time.Second)

		var frames []wsreplay.Frame
		for i, step := range p.Steps {
			// Expand ${vars} captured so far in case later steps depend on
			// earlier ones (matches the per-iteration semantics).
			content, _ := SubstituteVars(step.Content, vars)
			opcode := step.Opcode
			if opcode == 0 {
				opcode = 1
			}
			if step.Role == RoleSetup || step.Role == RoleFuzz || step.Role == "" {
				if err := sess.Send(opcode, content); err != nil {
					return vars, nil, fmt.Errorf("ws_script presetup step %d send: %w", i, err)
				}
				frames = append(frames, wsreplay.Frame{Opcode: opcode, Content: content, Direction: "sent", Ts: time.Now()})
			}
			// wait_for
			if step.WaitFor != nil {
				waitTO := time.Duration(step.WaitFor.TimeoutMs) * time.Millisecond
				if waitTO <= 0 {
					waitTO = 5 * time.Second
				}
				matched := false
				deadline := time.Now().Add(waitTO)
				for time.Now().Before(deadline) {
					left := time.Until(deadline)
					f, ferr := sess.NextFrame(left)
					if ferr != nil {
						if isPeerClose(ferr) || isTimeout(ferr) {
							break
						}
						return vars, nil, fmt.Errorf("ws_script presetup step %d wait_for: %w", i, ferr)
					}
					frames = append(frames, f)
					if wsreplay.Match(*step.WaitFor, f.Content) {
						matched = true
						break
					}
				}
				if !matched && step.OnTimeout == PolicyAbort {
					return vars, nil, fmt.Errorf("ws_script presetup step %d wait_for: timeout/no-match (abort policy)", i)
				}
			}
		}

		// Run extractions against the collected frames.
		for _, ext := range p.Extract {
			val, ok := ApplyExtraction(ext, frames, nil)
			if !ok && ext.FallbackPolicy == FallbackAbort {
				return vars, nil, fmt.Errorf("ws_script presetup extraction %q failed (abort policy)", ext.Name)
			}
			vars[ext.Name] = val
		}
		return vars, nil, nil
	}

	// HTTP / other branches: apply extractions against whatever sources we have.
	for _, ext := range p.Extract {
		val, ok := ApplyExtraction(ext, nil, httpResp)
		if !ok && ext.FallbackPolicy == FallbackAbort {
			continue
		}
		vars[ext.Name] = val
	}
	return vars, httpResp, nil
}
