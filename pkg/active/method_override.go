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

	for _, targetMethod := range overrideMethods {
		for _, headerName := range overrideHeaderNames {
			if runMethodOverrideProbe(ctx, history, opts, client, headerName, targetMethod, false, auditLog) {
				return
			}
		}

		if baseMethod == http.MethodGet {
			runMethodOverrideProbe(ctx, history, opts, client, "_method", targetMethod, true, auditLog)
		}
	}
}

func runMethodOverrideProbe(ctx context.Context, baseline *db.History, opts ActiveModuleOptions, client *http.Client, key, value string, useQuery bool, auditLog zerolog.Logger) bool {
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
	if result.History.StatusCode == 400 || result.History.StatusCode == 405 {
		return false
	}

	probeSuccess := result.History.StatusCode >= 200 && result.History.StatusCode < 400
	baselineBlocked := baseline.StatusCode == 405 || baseline.StatusCode == 403 || baseline.StatusCode == 401

	if !baselineBlocked || !probeSuccess {
		return false
	}

	headersStr := http_utils.HeadersToString(req.Header)
	details := fmt.Sprintf(`HTTP method override accepted.

Baseline:
- Request: %s
- Status: %d

Override attempt:
- Mechanism: %s
- Override to: %s
- Status: %d
- URL: %s
- Headers sent:
%s
`, baseline.Method+" "+baseline.URL, baseline.StatusCode, overrideMechanismDescription(useQuery, key), value, result.History.StatusCode, result.History.URL, headersStr)

	confidence := 80
	if result.History.StatusCode >= 200 && result.History.StatusCode < 300 {
		confidence = 90
	}

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
