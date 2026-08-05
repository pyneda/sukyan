package active

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// MethodOverrideScan detects servers that honor method-override headers or query params on GET baselines.
func MethodOverrideScan(history *db.History, opts ActiveModuleOptions) {
	auditLog := log.With().Str("audit", "method-override").Str("url", history.URL).Uint("workspace", opts.WorkspaceID).Logger()

	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		auditLog.Debug().Msg("Context cancelled, skipping method override scan")
		return
	default:
	}

	if history == nil {
		return
	}

	baseMethod := strings.ToUpper(history.Method)
	overrideMethods := methodOverrideTargets(baseMethod)
	if len(overrideMethods) == 0 {
		auditLog.Debug().Str("method", baseMethod).Msg("Skipping method override scan: no override targets for this method")
		return
	}

	if opts.ScanMode != options.ScanModeFuzz && (history.StatusCode == 400 || history.StatusCode == 405) {
		auditLog.Debug().Int("status", history.StatusCode).Msg("Skipping method override scan: baseline already rejects methods")
		return
	}

	client := opts.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	overrideHeaderNames := []string{
		"X-HTTP-Method-Override",
		"X-Method-Override",
		"X-HTTP-Method",
	}

	control := &baselineControl{}

	// A honoured override on a permitted baseline mutates the resource, so a
	// control taken after the probes can no longer reproduce it. Resolve it up
	// front; sync.Once makes every probe reuse this pre-probe snapshot.
	if probesPermittedBaseline(history.StatusCode, opts.ScanMode) {
		if _, err := control.resolve(ctx, client, history, opts); err != nil {
			auditLog.Warn().Err(err).Msg("Could not capture a pre-probe control for the permitted baseline")
		}
	}

	for _, targetMethod := range overrideMethods {
		for _, headerName := range overrideHeaderNames {
			if runMethodOverrideProbe(ctx, history, opts, client, headerName, targetMethod, false, auditLog, control) {
				return
			}
		}

		if baseMethod == http.MethodGet {
			runMethodOverrideProbe(ctx, history, opts, client, "_method", targetMethod, true, auditLog, control)
		}
	}
}

func runMethodOverrideProbe(ctx context.Context, baseline *db.History, opts ActiveModuleOptions, client *http.Client, key, value string, useQuery bool, auditLog zerolog.Logger, control *baselineControl) bool {
	select {
	case <-ctx.Done():
		return false
	default:
	}

	req, err := http_utils.BuildRequestFromHistoryItem(baseline)
	if err != nil {
		auditLog.Error().Err(err).Msg("Error rebuilding baseline request")
		return false
	}

	if useQuery {
		urlStr := req.URL.String()
		separator := "?"
		if strings.Contains(urlStr, "?") {
			separator = "&"
		}
		// Avoid double-appending if already present.
		if strings.Contains(strings.ToLower(urlStr), "_method=") {
			auditLog.Debug().Msg("Skipping query override probe: _method already present")
			return false
		}
		req.URL, _ = url.Parse(urlStr + separator + key + "=" + value)
	} else {
		req.Header.Set(key, value)
	}

	req = req.WithContext(ctx)

	result := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		Client:        client,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:              db.SourceScanner,
			WorkspaceID:         opts.WorkspaceID,
			TaskID:              opts.TaskID,
			ScanID:              opts.ScanID,
			ScanJobID:           opts.ScanJobID,
			CreateNewBodyStream: false,
		},
	})

	if result.Err != nil || result.History == nil {
		auditLog.Debug().Err(result.Err).Msg("Override probe failed")
		return false
	}

	if result.History.StatusCode == baseline.StatusCode {
		return false
	}
	if result.History.StatusCode < 200 || result.History.StatusCode >= 400 {
		return false
	}

	baselineBlocked := isMethodOverrideBaselineBlocked(baseline.StatusCode)
	if !baselineBlocked && !probesPermittedBaseline(baseline.StatusCode, opts.ScanMode) {
		return false
	}

	// The stored baseline can go stale between crawl and scan (e.g. the
	// resource stops being blocked), so re-confirm it with an unmodified
	// replay before treating this probe as a genuine override.
	controlHistory, err := control.resolve(ctx, client, baseline, opts)
	if err != nil {
		auditLog.Warn().Err(err).Msg("Could not re-validate baseline with a control request, suppressing method override finding")
		return false
	}

	confidence := 70
	if result.History.StatusCode < 300 {
		confidence = 80
	}
	var evidence string

	if baselineBlocked {
		if !isMethodOverrideBaselineBlocked(controlHistory.StatusCode) {
			if controlIsOpen(controlHistory.StatusCode) {
				auditLog.Debug().Int("control_status", controlHistory.StatusCode).Msg("Control request no longer blocked, baseline is stale, skipping")
			} else {
				auditLog.Warn().Int("control_status", controlHistory.StatusCode).Msg("Control request could not confirm the blocked baseline, suppressing method override finding")
			}
			return false
		}
		confidence += 10
		evidence = "The control replay is still denied, so the override bypassed the access control on this resource."
	} else {
		if controlHistory.StatusCode != baseline.StatusCode {
			auditLog.Debug().Int("control_status", controlHistory.StatusCode).Int("baseline_status", baseline.StatusCode).Msg("Control request did not reproduce the baseline status, endpoint is unstable, skipping")
			return false
		}
		evidence = fmt.Sprintf("The control replay reproduced the baseline, so the %s caused the changed response.", overrideMechanismDescription(useQuery, key))
	}

	headersStr := http_utils.HeadersToString(req.Header)
	details := fmt.Sprintf(`HTTP method override accepted.

Baseline:
- Request: %s
- Status: %d
- Control re-validation status: %d

Override attempt:
- Mechanism: %s
- Override to: %s
- Status: %d
- URL: %s
- Headers sent:
%s
Evidence: %s
`, baseline.Method+" "+baseline.URL, baseline.StatusCode, controlHistory.StatusCode, overrideMechanismDescription(useQuery, key), value, result.History.StatusCode, result.History.URL, headersStr, evidence)

	db.CreateIssueFromHistoryAndTemplate(
		result.History,
		db.HttpMethodOverrideCode,
		details,
		confidence,
		"",
		&opts.WorkspaceID,
		&opts.TaskID,
		&opts.TaskJobID,
		&opts.ScanID,
		&opts.ScanJobID,
	)

	auditLog.Warn().Int("baseline_status", baseline.StatusCode).Int("override_status", result.History.StatusCode).Msg("Potential method override detected")
	return true
}

func isMethodOverrideBaselineBlocked(statusCode int) bool {
	return statusCode == http.StatusMethodNotAllowed || isForbiddenStatus(statusCode)
}

// probesPermittedBaseline reports whether an already-successful baseline is
// worth probing. A permitted endpoint that changes its answer once the override
// is added is honouring it, but proving that costs an extra control request, so
// it is only spent from smart mode upwards.
func probesPermittedBaseline(statusCode int, mode options.ScanMode) bool {
	return statusCode >= 200 && statusCode < 300 && mode.IsHigherOrEqual(options.ScanModeSmart)
}

func methodOverrideTargets(baseMethod string) []string {
	switch baseMethod {
	case http.MethodGet:
		return []string{"DELETE"}
	case http.MethodPost:
		return []string{"DELETE", "PUT"}
	case http.MethodPut:
		return []string{"DELETE"}
	case http.MethodPatch:
		return []string{"DELETE"}
	default:
		return nil
	}
}

func overrideMechanismDescription(useQuery bool, key string) string {
	if useQuery {
		return fmt.Sprintf("query parameter %s", key)
	}
	return fmt.Sprintf("header %s", key)
}
