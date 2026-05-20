package wsfuzz

import (
	"context"
	"io"
	"net/http"
	"strings"
)

// runPreSetup executes the configured PreSetup once per run and returns the
// captured run-scope variables + optional HTTP response ref (for further
// extractions during iterations, though current usage is run-scope only).
//
// Currently supports SetupHTTPRequest. SetupWsScript (ws-script setup) is a
// future extension; the engine treats RoleSetup steps inside the main script
// as per-iteration setup already.
func runPreSetup(ctx context.Context, p *PreSetup) (map[string]string, *HTTPResponseRef, error) {
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
	}

	// Apply extractions against whatever sources we have.
	for _, ext := range p.Extract {
		val, ok := ApplyExtraction(ext, nil, httpResp)
		if !ok && ext.FallbackPolicy == FallbackAbort {
			continue
		}
		vars[ext.Name] = val
	}
	return vars, httpResp, nil
}
