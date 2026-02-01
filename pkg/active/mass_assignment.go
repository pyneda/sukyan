package active

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
)

// MassAssignmentScan probes JSON endpoints for unsafe binding of privileged fields.
func MassAssignmentScan(history *db.History, opts ActiveModuleOptions) {
	auditLog := log.With().Str("audit", "mass-assignment").Str("url", history.URL).Uint("workspace", opts.WorkspaceID).Logger()

	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		auditLog.Debug().Msg("Context cancelled, skipping mass assignment scan")
		return
	default:
	}

	if history == nil {
		return
	}

	method := strings.ToUpper(history.Method)
	if method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch {
		auditLog.Debug().Str("method", method).Msg("Skipping mass assignment: method not applicable")
		return
	}

	// Only run on JSON to avoid noisy probes.
	if !strings.Contains(strings.ToLower(history.RequestContentType), "application/json") {
		auditLog.Debug().Str("content_type", history.RequestContentType).Msg("Skipping mass assignment: non-JSON content")
		return
	}

	// Parse original body to merge fields; if it is not JSON object, skip unless fuzz.
	originalBody, err := history.RequestBody()
	if err != nil {
		auditLog.Debug().Err(err).Msg("Skipping mass assignment: unable to read baseline body")
		return
	}

	if opts.ScanMode == options.ScanModeFast && len(originalBody) == 0 {
		auditLog.Debug().Msg("Skipping mass assignment: empty body and fast mode")
		return
	}

	var baseObj map[string]any
	if len(originalBody) > 0 {
		if err := json.Unmarshal(originalBody, &baseObj); err != nil {
			auditLog.Debug().Err(err).Msg("Skipping mass assignment: baseline body not JSON object")
			return
		}
	}
	if baseObj == nil {
		baseObj = map[string]any{}
	}

	// Add privileged fields only when absent to reduce noise.
	candidateFields := map[string]any{
		"admin":    true,
		"isAdmin":  true,
		"role":     "admin",
		"verified": true,
		"active":   true,
	}
	for k, v := range candidateFields {
		if _, exists := baseObj[k]; !exists {
			baseObj[k] = v
		}
	}

	payloadBytes, err := json.Marshal(baseObj)
	if err != nil {
		auditLog.Error().Err(err).Msg("Failed to marshal mass assignment payload")
		return
	}

	req, err := http_utils.BuildRequestFromHistoryItem(history)
	if err != nil {
		auditLog.Error().Err(err).Msg("Error rebuilding baseline request")
		return
	}
	req = req.WithContext(ctx)
	req.Method = method
	req.Body = io.NopCloser(bytes.NewReader(payloadBytes))
	req.ContentLength = int64(len(payloadBytes))
	req.Header.Set("Content-Type", "application/json")

	client := opts.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

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
		auditLog.Debug().Err(result.Err).Msg("Mass assignment probe failed")
		return
	}

	if result.History.StatusCode < 200 || result.History.StatusCode >= 300 {
		return
	}

	respBody, _ := result.History.ResponseBody()
	var respObj map[string]any
	if err := json.Unmarshal(respBody, &respObj); err != nil {
		auditLog.Debug().Err(err).Msg("Skipping mass assignment check: response not a JSON object")
		return
	}

	baselineBody, err := history.ResponseBody()
	if err != nil || len(baselineBody) == 0 {
		auditLog.Debug().Msg("Skipping mass assignment check: unable to read baseline response")
		return
	}
	var baselineObj map[string]any
	if err := json.Unmarshal(baselineBody, &baselineObj); err != nil {
		auditLog.Debug().Err(err).Msg("Skipping mass assignment check: baseline response not JSON")
		return
	}

	found := []string{}
	for fieldName, injectedValue := range candidateFields {
		respValue, exists := respObj[fieldName]
		if !exists {
			continue
		}
		if baselineObj != nil {
			if _, existedBefore := baselineObj[fieldName]; existedBefore {
				continue
			}
		}
		if fmt.Sprintf("%v", respValue) == fmt.Sprintf("%v", injectedValue) {
			found = append(found, fieldName)
		}
	}

	if len(found) == 0 {
		return
	}

	details := fmt.Sprintf(`Potential mass assignment behavior detected.

Baseline response status: %d
Probe response status: %d
Fields observed in response (not in baseline): %s

Request: %s %s
Payload sent:
%s
`, history.StatusCode, result.History.StatusCode, strings.Join(found, ", "), method, history.URL, string(payloadBytes))

	db.CreateIssueFromHistoryAndTemplate(
		result.History,
		db.ApiMassAssignmentCode,
		details,
		70,
		"",
		&opts.WorkspaceID,
		&opts.TaskID,
		&opts.TaskJobID,
		&opts.ScanID,
		&opts.ScanJobID,
	)

	auditLog.Warn().Int("status", result.History.StatusCode).Strs("fields", found).Msg("Potential mass assignment detected")
}
